package jobs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"videodl/internal/models"
	"videodl/internal/storage"
)

// MigrateLegacyDownloads upgrades rows created by the old single-file
// layout into the new package layout. It is safe to run multiple times.
func (m *Manager) MigrateLegacyDownloads(ctx context.Context) (models.RecoveryReport, error) {
	storeCfg, err := m.repo.GetStorageConfig()
	if err != nil {
		return models.RecoveryReport{}, err
	}
	targetBackend, targetType, err := m.backendForConfig(ctx, storeCfg)
	if err != nil {
		return models.RecoveryReport{}, err
	}

	settings, _ := m.repo.GetSettings()
	globalProxy := settings.ProxyHTTPS
	if globalProxy == "" {
		globalProxy = settings.ProxyHTTP
	}
	if globalProxy == "" {
		globalProxy = settings.ProxySOCKS5
	}

	downloads, err := m.repo.ListDownloads(100000)
	if err != nil {
		return models.RecoveryReport{}, err
	}

	report := models.RecoveryReport{}
	for _, d := range downloads {
		if d.Status != "complete" {
			continue
		}
		if d.VideoS3Key != "" && d.ThumbnailS3Key != "" && d.MetadataJSON != "" {
			continue
		}

		item, migrated, err := m.migrateLegacyDownload(ctx, d, storeCfg, targetBackend, targetType, globalProxy)
		if err != nil {
			item.Warning = err.Error()
			report.Warnings = append(report.Warnings, err.Error())
			report.Items = append(report.Items, item)
			continue
		}
		if err := m.repo.UpsertDownload(migrated); err != nil {
			item.Warning = err.Error()
			report.Warnings = append(report.Warnings, err.Error())
			report.Items = append(report.Items, item)
			continue
		}
		report.Recovered++
		report.Items = append(report.Items, item)
	}
	return report, nil
}

func (m *Manager) migrateLegacyDownload(ctx context.Context, d models.Download, storeCfg models.StorageConfig, targetBackend storage.Backend, targetType, globalProxy string) (models.RecoveryReportItem, models.Download, error) {
	item := models.RecoveryReportItem{DownloadID: d.ID, RootKey: d.ID}

	stageDir, err := os.MkdirTemp("", "dlx-migrate-*")
	if err != nil {
		return item, models.Download{}, err
	}
	defer os.RemoveAll(stageDir)

	videoSource, videoExt, err := m.prepareLegacyVideo(ctx, d, stageDir)
	if err != nil {
		return item, models.Download{}, err
	}
	thumbnailSource, thumbnailExt, thumbMissing, err := m.prepareLegacyThumbnail(ctx, d, stageDir)
	if err != nil {
		return item, models.Download{}, err
	}
	metadataSource, metadataJSON, err := m.prepareLegacyMetadata(ctx, d, stageDir, globalProxy)
	if err != nil {
		return item, models.Download{}, err
	}

	item.Missing = append(item.Missing, thumbMissing...)
	if len(item.Missing) > 0 {
		item.Warning = strings.Join(item.Missing, ", ") + " missing"
	}

	files := storage.Package{
		ID: d.ID,
		Files: []storage.PackageFile{
			{Name: "video" + videoExt, SourcePath: videoSource},
			{Name: "thumbnail" + thumbnailExt, SourcePath: thumbnailSource},
			{Name: "metadata.json", SourcePath: metadataSource},
		},
	}

	ref, err := targetBackend.StorePackage(ctx, files)
	if err != nil {
		return item, models.Download{}, err
	}

	localPath := ""
	if targetType == "local" && ref.VideoKey != "" {
		localPath = filepath.Join(storeCfg.LocalPath, filepath.FromSlash(ref.VideoKey))
	}

	download := d
	download.StorageType = targetType
	download.LocalPath = localPath
	download.S3Key = ref.VideoKey
	download.VideoS3Key = ref.VideoKey
	download.ThumbnailS3Key = ref.ThumbnailKey
	download.MetadataS3Key = ref.MetadataKey
	download.MetadataJSON = string(metadataJSON)
	download.UpdatedAt = time.Now().UTC()
	if download.CreatedAt.IsZero() {
		download.CreatedAt = download.UpdatedAt
	}
	if download.ThumbnailS3Key == "" {
		download.Status = "failed"
		download.Error = "missing thumbnail in migrated package"
	}
	if download.VideoS3Key == "" {
		download.Status = "failed"
		download.Error = "missing video in migrated package"
	}
	return item, download, nil
}

func (m *Manager) prepareLegacyVideo(ctx context.Context, d models.Download, stageDir string) (string, string, error) {
	ext := filepath.Ext(d.LocalPath)
	if ext == "" {
		ext = filepath.Ext(d.S3Key)
	}
	if ext == "" {
		ext = filepath.Ext(d.VideoS3Key)
	}
	if ext == "" {
		ext = ".mp4"
	}

	if d.LocalPath != "" {
		if _, err := os.Stat(d.LocalPath); err == nil {
			dest := filepath.Join(stageDir, "video"+ext)
			in, err := os.Open(d.LocalPath)
			if err != nil {
				return "", "", err
			}
			defer in.Close()
			out, err := os.Create(dest)
			if err != nil {
				return "", "", err
			}
			if _, err := io.Copy(out, in); err != nil {
				out.Close()
				return "", "", err
			}
			if err := out.Close(); err != nil {
				return "", "", err
			}
			return dest, ext, nil
		}
	}

	if d.S3Key == "" && d.VideoS3Key == "" {
		return "", "", fmt.Errorf("video source not found for %s", d.ID)
	}

	sourceKey := d.VideoS3Key
	if sourceKey == "" {
		sourceKey = d.S3Key
	}
	sourceBackend, err := m.legacySourceBackend(ctx, d.StorageType)
	if err != nil {
		return "", "", err
	}
	data, err := sourceBackend.ReadFile(ctx, sourceKey)
	if err != nil {
		return "", "", err
	}
	dest := filepath.Join(stageDir, "video"+ext)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", "", err
	}
	return dest, ext, nil
}

func (m *Manager) prepareLegacyThumbnail(ctx context.Context, d models.Download, stageDir string) (string, string, []string, error) {
	thumbURL := strings.TrimSpace(d.Thumbnail)
	if thumbURL == "" {
		return "", ".jpg", []string{"thumbnail"}, nil
	}
	u, err := url.Parse(thumbURL)
	if err != nil {
		return "", ".jpg", []string{"thumbnail"}, nil
	}

	ext := filepath.Ext(u.Path)
	if ext == "" {
		ext = ".jpg"
	}
	dest := filepath.Join(stageDir, "thumbnail"+ext)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, thumbURL, nil)
	if err != nil {
		return "", ".jpg", []string{"thumbnail"}, nil
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", []string{"thumbnail"}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", []string{"thumbnail"}, nil
	}

	out, err := os.Create(dest)
	if err != nil {
		return "", "", nil, err
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", "", nil, err
	}
	return dest, ext, nil, nil
}

func (m *Manager) prepareLegacyMetadata(ctx context.Context, d models.Download, stageDir string, proxy string) (string, []byte, error) {
	if strings.TrimSpace(d.MetadataJSON) != "" {
		dest := filepath.Join(stageDir, "metadata.json")
		if err := os.WriteFile(dest, []byte(d.MetadataJSON), 0o644); err != nil {
			return "", nil, err
		}
		return dest, []byte(d.MetadataJSON), nil
	}

	raw, err := m.runner.FetchMetadata(ctx, d.SourceURL, proxy)
	if err != nil {
		return "", nil, err
	}
	dest := filepath.Join(stageDir, "metadata.json")
	if err := os.WriteFile(dest, raw, 0o644); err != nil {
		return "", nil, err
	}
	return dest, raw, nil
}

func (m *Manager) legacySourceBackend(ctx context.Context, storageType string) (storage.Backend, error) {
	storeCfg, err := m.repo.GetStorageConfig()
	if err != nil {
		return nil, err
	}
	switch storageType {
	case "s3":
		secret, _ := m.box.Decrypt(storeCfg.S3SecretKey)
		return storage.NewS3(ctx, storage.S3Config{
			Endpoint:     storeCfg.S3Endpoint,
			Region:       storeCfg.S3Region,
			Bucket:       storeCfg.S3Bucket,
			AccessKey:    storeCfg.S3AccessKey,
			SecretKey:    secret,
			Prefix:       storeCfg.S3Prefix,
			UsePathStyle: storeCfg.S3UsePathStyle,
		})
	default:
		return storage.NewLocal(storeCfg.LocalPath), nil
	}
}
