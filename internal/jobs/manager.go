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
	outPath, err := m.runner.Download(ctx, d.SourceURL, d.FormatID, effectiveProxy, outputTemplate, func(pct float64, line string) {
		m.repo.AddLog("ytdlp", downloadID, line)
		if pct >= 0 && time.Since(lastUpdate) > 300*time.Millisecond {
			m.setState(downloadID, "downloading", pct, line)
			lastUpdate = time.Now()
		}
	})

	if err != nil && settings.DirectFallback && effectiveProxy != "" {
		m.repo.AddLog("proxy", downloadID, fmt.Sprintf("proxy attempt failed (%v); falling back to direct connection", err))
		outPath, err = m.runner.Download(ctx, d.SourceURL, d.FormatID, "", outputTemplate, func(pct float64, line string) {
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

	if outPath == "" {
		// best effort: pick the only file in workDir if yt-dlp didn't print one
		outPath = firstFileIn(workDir)
	}
	if outPath == "" {
		m.fail(downloadID, "yt-dlp completed but no output file was found")
		return
	}

	m.setState(downloadID, "uploading", 0, "storing file")
	storeCfg, err := m.repo.GetStorageConfig()
	if err != nil {
		m.fail(downloadID, fmt.Sprintf("could not load storage config: %v", err))
		return
	}

	key := sanitizeFilename(fmt.Sprintf("%s_%s", downloadID[:8], filepath.Base(outPath)))
	var backend storage.Backend
	var storageType string

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
			m.fail(downloadID, fmt.Sprintf("s3 backend init failed: %v", err))
			return
		}
		backend = s3b
		storageType = "s3"
	default:
		backend = storage.NewLocal(storeCfg.LocalPath)
		storageType = "local"
	}

	info, _ := os.Stat(outPath)
	var size int64
	if info != nil {
		size = info.Size()
	}

	ref, err := backend.Store(ctx, outPath, key)
	if err != nil {
		m.fail(downloadID, fmt.Sprintf("storage upload failed: %v", err))
		m.repo.AddLog("upload", downloadID, fmt.Sprintf("upload failed: %v", err))
		return
	}
	m.repo.AddLog("upload", downloadID, fmt.Sprintf("stored as %s (%s)", ref, storageType))

	if storageType == "s3" {
		m.repo.UpdateDownloadStorageResult(downloadID, "s3", "", ref, size)
	} else {
		m.repo.UpdateDownloadStorageResult(downloadID, "local", ref, "", size)
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
