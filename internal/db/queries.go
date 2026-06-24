package db

import (
	"database/sql"
	"fmt"
	"time"

	"videodl/internal/models"
)

// Repo bundles all persistence operations against the SQLite database.
type Repo struct {
	DB *sql.DB
}

func NewRepo(conn *sql.DB) *Repo { return &Repo{DB: conn} }

// ---------- settings ----------

func (r *Repo) GetSettings() (models.Settings, error) {
	rows, err := r.DB.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return models.Settings{}, err
	}
	defer rows.Close()

	s := models.Settings{RotationMode: "random"}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return s, err
		}
		switch k {
		case "proxy_http":
			s.ProxyHTTP = v
		case "proxy_https":
			s.ProxyHTTPS = v
		case "proxy_socks5":
			s.ProxySOCKS5 = v
		case "rotation_mode":
			s.RotationMode = v
		case "direct_fallback":
			s.DirectFallback = v == "1"
		}
	}
	return s, nil
}

func (r *Repo) SaveSettings(s models.Settings) error {
	kv := map[string]string{
		"proxy_http":       s.ProxyHTTP,
		"proxy_https":      s.ProxyHTTPS,
		"proxy_socks5":     s.ProxySOCKS5,
		"rotation_mode":    s.RotationMode,
		"direct_fallback":  boolToStr(s.DirectFallback),
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return err
	}
	for k, v := range kv {
		if _, err := tx.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func boolToStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// ---------- proxies ----------

func (r *Repo) ListProxies() ([]models.Proxy, error) {
	rows, err := r.DB.Query(`SELECT id, name, proxy_url, enabled, created_at FROM proxies ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Proxy
	for rows.Next() {
		var p models.Proxy
		var enabled int
		var created string
		if err := rows.Scan(&p.ID, &p.Name, &p.ProxyURL, &enabled, &created); err != nil {
			return nil, err
		}
		p.Enabled = enabled == 1
		p.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, p)
	}
	return out, nil
}

func (r *Repo) CreateProxy(p models.Proxy) error {
	_, err := r.DB.Exec(`INSERT INTO proxies (id, name, proxy_url, enabled) VALUES (?, ?, ?, ?)`,
		p.ID, p.Name, p.ProxyURL, boolToInt(p.Enabled))
	return err
}

func (r *Repo) UpdateProxy(p models.Proxy) error {
	_, err := r.DB.Exec(`UPDATE proxies SET name = ?, proxy_url = ?, enabled = ? WHERE id = ?`,
		p.Name, p.ProxyURL, boolToInt(p.Enabled), p.ID)
	return err
}

func (r *Repo) DeleteProxy(id string) error {
	_, err := r.DB.Exec(`DELETE FROM proxies WHERE id = ?`, id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------- storage config ----------

func (r *Repo) GetStorageConfig() (models.StorageConfig, error) {
	var sc models.StorageConfig
	var pathStyle int
	err := r.DB.QueryRow(`SELECT local_path, s3_endpoint, s3_region, s3_bucket, s3_access_key,
		s3_secret_key, s3_prefix, s3_use_path_style, mode FROM storage_config WHERE id = 1`).
		Scan(&sc.LocalPath, &sc.S3Endpoint, &sc.S3Region, &sc.S3Bucket, &sc.S3AccessKey,
			&sc.S3SecretKey, &sc.S3Prefix, &pathStyle, &sc.Mode)
	if err != nil {
		return sc, err
	}
	sc.S3UsePathStyle = pathStyle == 1
	return sc, nil
}

func (r *Repo) SaveStorageConfig(sc models.StorageConfig) error {
	_, err := r.DB.Exec(`UPDATE storage_config SET local_path = ?, s3_endpoint = ?, s3_region = ?,
		s3_bucket = ?, s3_access_key = ?, s3_secret_key = ?, s3_prefix = ?, s3_use_path_style = ?, mode = ?
		WHERE id = 1`,
		sc.LocalPath, sc.S3Endpoint, sc.S3Region, sc.S3Bucket, sc.S3AccessKey,
		sc.S3SecretKey, sc.S3Prefix, boolToInt(sc.S3UsePathStyle), sc.Mode)
	return err
}

// ---------- downloads ----------

func (r *Repo) CreateDownload(d models.Download) error {
	_, err := r.DB.Exec(`INSERT INTO downloads (id, source_url, title, thumbnail, uploader, duration,
		format_id, resolution, filesize, storage_type, status, proxy_mode, custom_proxy)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.SourceURL, d.Title, d.Thumbnail, d.Uploader, d.Duration,
		d.FormatID, d.Resolution, d.Filesize, d.StorageType, d.Status, d.ProxyMode, d.CustomProxy)
	return err
}

func (r *Repo) UpdateDownloadStatus(id, status, errMsg string) error {
	_, err := r.DB.Exec(`UPDATE downloads SET status = ?, error = ?, updated_at = datetime('now') WHERE id = ?`,
		status, errMsg, id)
	return err
}

func (r *Repo) UpdateDownloadStorageResult(id, storageType, localPath, s3Key string, filesize int64) error {
	_, err := r.DB.Exec(`UPDATE downloads SET storage_type = ?, local_path = ?, s3_key = ?, filesize = ?,
		updated_at = datetime('now') WHERE id = ?`, storageType, localPath, s3Key, filesize, id)
	return err
}

func (r *Repo) GetDownload(id string) (models.Download, error) {
	var d models.Download
	var created, updated string
	err := r.DB.QueryRow(`SELECT id, source_url, title, thumbnail, uploader, duration, format_id,
		resolution, filesize, storage_type, local_path, s3_key, status, error, proxy_mode, custom_proxy,
		created_at, updated_at FROM downloads WHERE id = ?`, id).Scan(
		&d.ID, &d.SourceURL, &d.Title, &d.Thumbnail, &d.Uploader, &d.Duration, &d.FormatID,
		&d.Resolution, &d.Filesize, &d.StorageType, &d.LocalPath, &d.S3Key, &d.Status, &d.Error,
		&d.ProxyMode, &d.CustomProxy, &created, &updated)
	if err != nil {
		return d, err
	}
	d.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	d.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updated)
	return d, nil
}

func (r *Repo) ListDownloads(limit int) ([]models.Download, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.DB.Query(`SELECT id, source_url, title, thumbnail, uploader, duration, format_id,
		resolution, filesize, storage_type, local_path, s3_key, status, error, proxy_mode, custom_proxy,
		created_at, updated_at FROM downloads ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Download
	for rows.Next() {
		var d models.Download
		var created, updated string
		if err := rows.Scan(&d.ID, &d.SourceURL, &d.Title, &d.Thumbnail, &d.Uploader, &d.Duration, &d.FormatID,
			&d.Resolution, &d.Filesize, &d.StorageType, &d.LocalPath, &d.S3Key, &d.Status, &d.Error,
			&d.ProxyMode, &d.CustomProxy, &created, &updated); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		d.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updated)
		out = append(out, d)
	}
	return out, nil
}

func (r *Repo) DeleteDownload(id string) error {
	_, err := r.DB.Exec(`DELETE FROM downloads WHERE id = ?`, id)
	if err != nil {
		return err
	}
	_, err = r.DB.Exec(`DELETE FROM jobs WHERE download_id = ?`, id)
	return err
}

// ---------- jobs ----------

func (r *Repo) CreateJob(j models.Job) error {
	_, err := r.DB.Exec(`INSERT INTO jobs (id, download_id, state, progress, message) VALUES (?, ?, ?, ?, ?)`,
		j.ID, j.DownloadID, j.State, j.Progress, j.Message)
	return err
}

func (r *Repo) UpdateJob(id, state string, progress float64, message string) error {
	_, err := r.DB.Exec(`UPDATE jobs SET state = ?, progress = ?, message = ?, updated_at = datetime('now')
		WHERE id = ?`, state, progress, message, id)
	return err
}

// UpdateLatestJobForDownload updates the most recently created job row
// associated with downloadID - this is what the worker calls as it does
// not track individual job IDs once enqueued.
func (r *Repo) UpdateLatestJobForDownload(downloadID, state string, progress float64, message string) error {
	_, err := r.DB.Exec(`UPDATE jobs SET state = ?, progress = ?, message = ?, updated_at = datetime('now')
		WHERE id = (SELECT id FROM jobs WHERE download_id = ? ORDER BY created_at DESC LIMIT 1)`,
		state, progress, message, downloadID)
	return err
}

func (r *Repo) ListJobs(limit int) ([]models.Job, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.DB.Query(`SELECT id, download_id, state, progress, message, created_at, updated_at
		FROM jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Job
	for rows.Next() {
		var j models.Job
		var created, updated string
		if err := rows.Scan(&j.ID, &j.DownloadID, &j.State, &j.Progress, &j.Message, &created, &updated); err != nil {
			return nil, err
		}
		j.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		j.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updated)
		out = append(out, j)
	}
	return out, nil
}

// ---------- logs ----------

func (r *Repo) AddLog(category, downloadID, message string) error {
	_, err := r.DB.Exec(`INSERT INTO logs (category, download_id, message) VALUES (?, ?, ?)`,
		category, downloadID, message)
	return err
}

func (r *Repo) SearchLogs(category, query string, limit int) ([]models.LogEntry, error) {
	if limit <= 0 {
		limit = 500
	}
	q := `SELECT id, category, download_id, message, created_at FROM logs WHERE 1=1`
	args := []any{}
	if category != "" && category != "all" {
		q += ` AND category = ?`
		args = append(args, category)
	}
	if query != "" {
		q += ` AND message LIKE ?`
		args = append(args, "%"+query+"%")
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.LogEntry
	for rows.Next() {
		var l models.LogEntry
		var created string
		if err := rows.Scan(&l.ID, &l.Category, &l.DownloadID, &l.Message, &created); err != nil {
			return nil, err
		}
		l.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, l)
	}
	return out, nil
}

var ErrNotFound = fmt.Errorf("not found")
