package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"videodl/internal/models"
	"videodl/internal/storage"
)

// RecoverFromStorage rebuilds SQLite records by scanning package folders
// in the active storage backend. It is intended for disaster recovery
// when the SQLite database has been lost.
func (m *Manager) RecoverFromStorage(ctx context.Context) (models.RecoveryReport, error) {
	storeCfg, err := m.repo.GetStorageConfig()
	if err != nil {
		return models.RecoveryReport{}, err
	}
	backend, storageType, err := m.backendForConfig(ctx, storeCfg)
	if err != nil {
		return models.RecoveryReport{}, err
	}

	roots, err := backend.ListPackageRoots(ctx)
	if err != nil {
		return models.RecoveryReport{}, err
	}

	report := models.RecoveryReport{
		Items: make([]models.RecoveryReportItem, 0, len(roots)),
	}
	for _, root := range roots {
		item, download, err := recoverPackage(ctx, backend, storageType, storeCfg, root)
		if err != nil {
			item.Warning = err.Error()
			report.Warnings = append(report.Warnings, err.Error())
			report.Items = append(report.Items, item)
			continue
		}
		if err := m.repo.UpsertDownload(download); err != nil {
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

func recoverPackage(ctx context.Context, backend storage.Backend, storageType string, storeCfg models.StorageConfig, root string) (models.RecoveryReportItem, models.Download, error) {
	item := models.RecoveryReportItem{DownloadID: path.Base(root), RootKey: root}

	files, err := backend.ListPackageFiles(ctx, root)
	if err != nil {
		return item, models.Download{}, err
	}

	var videoFile, thumbnailFile bool
	for _, name := range files {
		switch {
		case name == "metadata.json":
			// handled below
		case strings.HasPrefix(name, "video."):
			videoFile = true
		case strings.HasPrefix(name, "thumbnail."):
			thumbnailFile = true
		}
	}

	metadataKey := path.Join(root, "metadata.json")
	metadataBytes, err := backend.ReadFile(ctx, metadataKey)
	if err != nil {
		return item, models.Download{}, fmt.Errorf("read metadata.json: %w", err)
	}

	meta, err := decodeJSONMap(metadataBytes)
	if err != nil {
		return item, models.Download{}, fmt.Errorf("parse metadata.json: %w", err)
	}

	videoKey := findStoredFile(root, files, "video.")
	thumbKey := findStoredFile(root, files, "thumbnail.")
	missing := make([]string, 0, 2)
	if !videoFile || videoKey == "" {
		missing = append(missing, "video")
	}
	if !thumbnailFile || thumbKey == "" {
		missing = append(missing, "thumbnail")
	}
	item.Missing = missing
	if len(missing) > 0 {
		item.Warning = strings.Join(missing, ", ") + " missing in package"
	}

	now := time.Now().UTC()
	download := models.Download{
		ID:             path.Base(root),
		SourceURL:      stringField(meta, "webpage_url", "original_url", "url"),
		Title:          stringField(meta, "title"),
		Thumbnail:      stringField(meta, "thumbnail"),
		Uploader:       stringField(meta, "uploader", "channel", "channel_url"),
		Duration:       int(numberField(meta, "duration")),
		FormatID:       stringField(meta, "format_id"),
		Resolution:     stringField(meta, "resolution"),
		Filesize:       int64(numberField(meta, "filesize")),
		StorageType:    storageType,
		S3Key:          videoKey,
		VideoS3Key:     videoKey,
		ThumbnailS3Key: thumbKey,
		MetadataS3Key:  metadataKey,
		MetadataJSON:   string(metadataBytes),
		Status:         "complete",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if videoKey == "" {
		download.Status = "failed"
		download.Error = "missing video in package"
	}
	if download.Resolution == "" {
		if width, height := int(numberField(meta, "width")), int(numberField(meta, "height")); width > 0 && height > 0 {
			download.Resolution = fmt.Sprintf("%dx%d", width, height)
		}
	}
	if download.SourceURL == "" {
		download.SourceURL = stringField(meta, "id")
	}
	if storageType == "local" && videoKey != "" {
		download.LocalPath = filepath.Join(storeCfg.LocalPath, filepath.FromSlash(videoKey))
	}
	return item, download, nil
}

func decodeJSONMap(raw []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func stringField(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func numberField(m map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch n := v.(type) {
			case json.Number:
				if f, err := n.Float64(); err == nil {
					return f
				}
			case float64:
				return n
			case int64:
				return float64(n)
			case int:
				return float64(n)
			}
		}
	}
	return 0
}

func findStoredFile(root string, files []string, prefix string) string {
	for _, name := range files {
		if strings.HasPrefix(name, prefix) {
			return path.Join(root, name)
		}
	}
	return ""
}
