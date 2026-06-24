package config

import (
	"os"
)

// Config holds runtime configuration sourced from environment variables.
type Config struct {
	AppPort       string // HTTP listen port
	DBPath        string // path to SQLite database file
	DownloadsRoot string // root directory for local downloads (mirrors storage.local_path default)
	AppSecret     string // 32-byte key (hex/base64/plain, hashed internally) used for encrypting secrets at rest
	YtDlpPath     string // path to the yt-dlp binary
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Load reads configuration from the environment, applying sane defaults
// so the application can run standalone in Docker with minimal setup.
func Load() *Config {
	return &Config{
		AppPort:       getEnv("APP_PORT", "8080"),
		DBPath:        getEnv("DB_PATH", "/config/app.db"),
		DownloadsRoot: getEnv("DOWNLOADS_PATH", "/downloads"),
		AppSecret:     getEnv("APP_SECRET", "change-me-please-32-bytes-secret!"),
		YtDlpPath:     getEnv("YTDLP_PATH", "yt-dlp"),
	}
}
