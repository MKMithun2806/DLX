package jobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"videodl/internal/crypto"
	"videodl/internal/db"
	"videodl/internal/models"
	"videodl/internal/proxy"
	"videodl/internal/storage"
	"videodl/internal/ytdlp"
)

// Manager owns the download queue, worker pool, and the glue between
// yt-dlp, the proxy pool, and storage backends.
type Manager struct {
	repo    *db.Repo
	runner  *ytdlp.Runner
	pool    *proxy.Pool
	box     *crypto.Box
	tmpRoot string
	Events  *Broadcaster

	queue chan string // download IDs awaiting processing
}

func NewManager(repo *db.Repo, runner *ytdlp.Runner, pool *proxy.Pool, box *crypto.Box, tmpRoot string, workers int) *Manager {
	if workers <= 0 {
		workers = 2
	}
	m := &Manager{
		repo:    repo,
		runner:  runner,
		pool:    pool,
		box:     box,
		tmpRoot: tmpRoot,
		Events:  NewBroadcaster(),
		queue:   make(chan string, 256),
	}
	os.MkdirAll(tmpRoot, 0o755)
	for i := 0; i < workers; i++ {
		go m.worker()
	}
	return m
}

// Enqueue creates a Download + Job row for a scanned video and schedules it
// for background processing. Returns the new download ID.
func (m *Manager) Enqueue(req models.DownloadRequest, scan models.ScanResult) (string, error) {
	id := uuid.NewString()
	d := models.Download{
		ID:          id,
		SourceURL:   req.URL,
		Title:       scan.Title,
		Thumbnail:   scan.Thumbnail,
		Uploader:    scan.Uploader,
		Duration:    scan.Duration,
		FormatID:    req.FormatID,
		Resolution:  scan.Resolution,
		Filesize:    scan.Filesize,
		Status:      "queued",
		ProxyMode:   req.ProxyMode,
		CustomProxy: req.CustomProxy,
	}
	if err := m.repo.CreateDownload(d); err != nil {
		return "", err
	}

	job := models.Job{ID: uuid.NewString(), DownloadID: id, State: "queued", Progress: 0}
	if err := m.repo.CreateJob(job); err != nil {
		return "", err
	}
	m.Events.Publish(job)
	m.queue <- id
	return id, nil
}

// Retry re-queues a failed/complete download for another attempt.
func (m *Manager) Retry(downloadID string) error {
	if err := m.repo.UpdateDownloadStatus(downloadID, "queued", ""); err != nil {
		return err
	}
	job := models.Job{ID: uuid.NewString(), DownloadID: downloadID, State: "queued", Progress: 0}
	if err := m.repo.CreateJob(job); err != nil {
		return err
	}
	m.Events.Publish(job)
	m.queue <- downloadID
	return nil
}

func (m *Manager) worker() {
	for id := range m.queue {
		m.process(id)
	}
}

func (m *Manager) process(downloadID string) {
	ctx := context.Background()

	d, err := m.repo.GetDownload(downloadID)
	if err != nil {
		return
	}

	settings, _ := m.repo.GetSettings()
	proxies, _ := m.repo.ListProxies()
	m.pool.SetProxies(proxies)
	m.pool.SetMode(settings.RotationMode)

	globalProxy := settings.ProxyHTTPS
	if globalProxy == "" {
		globalProxy = settings.ProxyHTTP
	}
	if globalProxy == "" {
		globalProxy = settings.ProxySOCKS5
	}
	effectiveProxy := m.pool.Resolve(d.ProxyMode, d.CustomProxy, globalProxy)

	m.setState(downloadID, "downloading", 0, "starting download")
	m.repo.AddLog("download", downloadID, fmt.Sprintf("starting download for %s (proxy=%s)", d.SourceURL, redactProxy(effectiveProxy)))

	workDir, err := os.MkdirTemp(m.tmpRoot, "job-*")
	if err != nil {
		m.fail(downloadID, fmt.Sprintf("could not create temp dir: %v", err))
		return
	}
	defer os.RemoveAll(workDir)

	outputTemplate := filepath.Join(workDir, "%(title)s.%(ext)s")

	lastUpdate := time.Now()
	artifacts, err := m.runner.DownloadPackage(ctx, d.SourceURL, d.FormatID, effectiveProxy, outputTemplate, func(pct float64, line string) {
		m.repo.AddLog("ytdlp", downloadID, line)
		if pct >= 0 && time.Since(lastUpdate) > 300*time.Millisecond {
			m.setState(downloadID, "downloading", pct, line)
			lastUpdate = time.Now()
		}
	})

	if err != nil && settings.DirectFallback && effectiveProxy != "" {
		m.repo.AddLog("proxy", downloadID, fmt.Sprintf("proxy attempt failed (%v); falling back to direct connection", err))
		artifacts, err = m.runner.DownloadPackage(ctx, d.SourceURL, d.FormatID, "", outputTemplate, func(pct float64, line string) {
			m.repo.AddLog("ytdlp", downloadID, line)
			if pct >= 0 && time.Since(lastUpdate) > 300*time.Millisecond {
				m.setState(downloadID, "downloading", pct, line)
				lastUpdate = time.Now()
			}
		})
	}

	if err != nil {
		m.fail(downloadID, err.Error())
		m.repo.AddLog("download", downloadID, fmt.Sprintf("download failed: %v", err))
		return
	}

	if artifacts.VideoPath == "" {
		// best effort: pick the only media file in workDir if yt-dlp didn't print one
		artifacts.VideoPath = firstVideoFileIn(workDir)
	}
	if artifacts.VideoPath == "" {
		m.fail(downloadID, "yt-dlp completed but no output file was found")
		return
	}
	if artifacts.MetadataPath == "" {
		raw, ferr := m.runner.FetchMetadata(ctx, d.SourceURL, effectiveProxy)
		if ferr != nil {
			m.fail(downloadID, fmt.Sprintf("yt-dlp completed but metadata.json could not be generated: %v", ferr))
			return
		}
		artifacts.MetadataPath = filepath.Join(workDir, "metadata.json")
		if err := os.WriteFile(artifacts.MetadataPath, raw, 0o644); err != nil {
			m.fail(downloadID, fmt.Sprintf("could not persist metadata.json: %v", err))
			return
		}
	}
	if artifacts.ThumbnailPath == "" {
		if alt := thumbnailFromVideoPath(artifacts.VideoPath); alt != "" {
			artifacts.ThumbnailPath = alt
		}
	}
	if artifacts.ThumbnailPath == "" {
		m.fail(downloadID, "yt-dlp completed but no thumbnail was found")
		return
	}
	metadataJSON, err := os.ReadFile(artifacts.MetadataPath)
	if err != nil {
		m.fail(downloadID, fmt.Sprintf("could not read metadata.json: %v", err))
		return
	}

	m.setState(downloadID, "uploading", 0, "storing file")
	storeCfg, err := m.repo.GetStorageConfig()
	if err != nil {
		m.fail(downloadID, fmt.Sprintf("could not load storage config: %v", err))
		return
	}

	backend, storageType, err := m.backendForConfig(ctx, storeCfg)
	if err != nil {
		m.fail(downloadID, err.Error())
		return
	}

	videoExt := filepath.Ext(artifacts.VideoPath)
	thumbExt := filepath.Ext(artifacts.ThumbnailPath)
	packageFiles := storage.Package{
		ID: downloadID,
		Files: []storage.PackageFile{
			{Name: "video" + videoExt, SourcePath: artifacts.VideoPath},
			{Name: "thumbnail" + thumbExt, SourcePath: artifacts.ThumbnailPath},
			{Name: "metadata.json", SourcePath: artifacts.MetadataPath},
		},
	}

	info, _ := os.Stat(artifacts.VideoPath)
	var size int64
	if info != nil {
		size = info.Size()
	}

	ref, err := backend.StorePackage(ctx, packageFiles)
	if err != nil {
		m.fail(downloadID, fmt.Sprintf("storage upload failed: %v", err))
		m.repo.AddLog("upload", downloadID, fmt.Sprintf("upload failed: %v", err))
		return
	}
	m.repo.AddLog("upload", downloadID, fmt.Sprintf("stored package root=%s video=%s thumbnail=%s metadata=%s (%s)",
		ref.PackageRoot, ref.VideoKey, ref.ThumbnailKey, ref.MetadataKey, storageType))

	videoKey := ref.VideoKey
	thumbKey := ref.ThumbnailKey
	metadataKey := ref.MetadataKey
	localPath := ""
	if storageType == "local" && videoKey != "" {
		localPath = filepath.Join(storeCfg.LocalPath, filepath.FromSlash(videoKey))
	}
	if err := m.repo.UpdateDownloadPackageResult(downloadID, storageType, localPath, videoKey, thumbKey, metadataKey, size, string(metadataJSON)); err != nil {
		m.fail(downloadID, fmt.Sprintf("could not persist download result: %v", err))
		return
	}

	m.setState(downloadID, "complete", 100, "done")
	m.repo.UpdateDownloadStatus(downloadID, "complete", "")
}

func (m *Manager) setState(downloadID, state string, progress float64, message string) {
	m.repo.UpdateDownloadStatus(downloadID, state, "")
	job := models.Job{ID: downloadID, DownloadID: downloadID, State: state, Progress: progress, Message: message}
	m.repo.UpdateLatestJobForDownload(downloadID, state, progress, message)
	m.Events.Publish(map[string]any{
		"download_id": downloadID,
		"state":       state,
		"progress":    progress,
		"message":     message,
	})
	_ = job
}

func (m *Manager) fail(downloadID, reason string) {
	m.repo.UpdateDownloadStatus(downloadID, "failed", reason)
	m.Events.Publish(map[string]any{
		"download_id": downloadID,
		"state":       "failed",
		"progress":    0,
		"message":     reason,
	})
}

func redactProxy(p string) string {
	if p == "" {
		return "direct"
	}
	return p
}

func (m *Manager) backendForConfig(ctx context.Context, storeCfg models.StorageConfig) (storage.Backend, string, error) {
	switch storeCfg.Mode {
	case "s3":
		secret, _ := m.box.Decrypt(storeCfg.S3SecretKey)
		s3b, err := storage.NewS3(ctx, storage.S3Config{
			Endpoint:     storeCfg.S3Endpoint,
			Region:       storeCfg.S3Region,
			Bucket:       storeCfg.S3Bucket,
			AccessKey:    storeCfg.S3AccessKey,
			SecretKey:    secret,
			Prefix:       storeCfg.S3Prefix,
			UsePathStyle: storeCfg.S3UsePathStyle,
		})
		if err != nil {
			return nil, "", fmt.Errorf("s3 backend init failed: %w", err)
		}
		return s3b, "s3", nil
	default:
		return storage.NewLocal(storeCfg.LocalPath), "local", nil
	}
}

func firstVideoFileIn(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	wanted := map[string]struct{}{
		".mp4": {}, ".mkv": {}, ".webm": {}, ".mov": {}, ".avi": {}, ".m4a": {}, ".mp3": {},
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".info.json") || strings.EqualFold(name, "metadata.json") {
			continue
		}
		if _, ok := wanted[strings.ToLower(filepath.Ext(name))]; ok {
			return filepath.Join(dir, name)
		}
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".info.json") {
			continue
		}
		if strings.EqualFold(filepath.Ext(name), ".jpg") || strings.EqualFold(filepath.Ext(name), ".jpeg") ||
			strings.EqualFold(filepath.Ext(name), ".webp") || strings.EqualFold(filepath.Ext(name), ".png") {
			continue
		}
		if strings.EqualFold(filepath.Ext(name), ".json") {
			continue
		}
		if filepath.Ext(name) != "" {
			return filepath.Join(dir, name)
		}
	}
	return ""
}

func thumbnailFromVideoPath(videoPath string) string {
	stem := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	for _, ext := range []string{".jpg", ".jpeg", ".webp", ".png"} {
		candidate := stem + ext
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func firstFileIn(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "..", "_")
	return replacer.Replace(name)
}
