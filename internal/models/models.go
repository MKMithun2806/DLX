package models

import "time"

// Settings represents the global key/value settings table, exposed as a
// strongly typed struct for the API and templates.
type Settings struct {
	ProxyHTTP      string `json:"proxy_http"`
	ProxyHTTPS     string `json:"proxy_https"`
	ProxySOCKS5    string `json:"proxy_socks5"`
	RotationMode   string `json:"rotation_mode"` // random | round_robin | sticky
	DirectFallback bool   `json:"direct_fallback"`
}

// Proxy is a single entry in the proxy pool.
type Proxy struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ProxyURL  string    `json:"proxy_url"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// StorageConfig captures both local and S3 storage settings. SecretKey is
// never serialized back to clients in plaintext (see handlers).
type StorageConfig struct {
	LocalPath      string `json:"local_path"`
	S3Endpoint     string `json:"s3_endpoint"`
	S3Region       string `json:"s3_region"`
	S3Bucket       string `json:"s3_bucket"`
	S3AccessKey    string `json:"s3_access_key"`
	S3SecretKey    string `json:"s3_secret_key,omitempty"`
	S3Prefix       string `json:"s3_prefix"`
	S3UsePathStyle bool   `json:"s3_use_path_style"`
	Mode           string `json:"mode"` // local | s3
}

// ScanResult describes a single video discovered by yt-dlp --dump-json.
type ScanResult struct {
	URL        string   `json:"url"`
	Title      string   `json:"title"`
	Thumbnail  string   `json:"thumbnail"`
	Duration   int      `json:"duration"` // seconds
	Uploader   string   `json:"uploader"`
	Formats    []Format `json:"formats"`
	Resolution string   `json:"resolution"` // best available, convenience field
	Filesize   int64    `json:"filesize"`   // best-effort estimate, bytes
	Error      string   `json:"error,omitempty"`
}

// Format is a single downloadable stream/format option for a video.
type Format struct {
	FormatID   string `json:"format_id"`
	Resolution string `json:"resolution"`
	Ext        string `json:"ext"`
	Filesize   int64  `json:"filesize"`
	Note       string `json:"note"`
}

// Download is a persisted row describing a requested/completed download.
type Download struct {
	ID             string    `json:"id"`
	SourceURL      string    `json:"source_url"`
	Title          string    `json:"title"`
	Thumbnail      string    `json:"thumbnail"`
	Uploader       string    `json:"uploader"`
	Duration       int       `json:"duration"`
	FormatID       string    `json:"format_id"`
	Resolution     string    `json:"resolution"`
	Filesize       int64     `json:"filesize"`
	StorageType    string    `json:"storage_type"`
	LocalPath      string    `json:"local_path"`
	S3Key          string    `json:"s3_key"`
	VideoS3Key     string    `json:"video_s3_key"`
	ThumbnailS3Key string    `json:"thumbnail_s3_key"`
	MetadataS3Key  string    `json:"metadata_s3_key"`
	MetadataJSON   string    `json:"metadata_json"`
	Status         string    `json:"status"`
	Error          string    `json:"error"`
	ProxyMode      string    `json:"proxy_mode"`
	CustomProxy    string    `json:"custom_proxy"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Job tracks the live progress of a download/upload pipeline run.
type Job struct {
	ID         string    `json:"id"`
	DownloadID string    `json:"download_id"`
	State      string    `json:"state"` // queued|downloading|uploading|complete|failed
	Progress   float64   `json:"progress"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// LogEntry is a single line written to the searchable logs view.
type LogEntry struct {
	ID         int64     `json:"id"`
	Category   string    `json:"category"`
	DownloadID string    `json:"download_id"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"created_at"`
}

// DownloadRequest is the payload sent from the UI/API to start a download.
type DownloadRequest struct {
	URL         string `json:"url"`
	FormatID    string `json:"format_id"`
	ProxyMode   string `json:"proxy_mode"` // global|direct|custom
	CustomProxy string `json:"custom_proxy"`
}

// ScanRequest is the payload sent to scan one or more URLs.
type ScanRequest struct {
	URLs string `json:"urls"` // newline separated
}
