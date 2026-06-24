-- 0001_init.sql
-- Initial schema for Video Downloader WebUI

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

-- Global key/value settings used:
--   proxy_http, proxy_https, proxy_socks5
--   storage_mode  (local | s3)
--   rotation_mode (random | round_robin | sticky)
--   direct_fallback (0|1)

CREATE TABLE IF NOT EXISTS proxies (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    proxy_url  TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS storage_config (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    local_path      TEXT NOT NULL DEFAULT '/downloads',
    s3_endpoint     TEXT NOT NULL DEFAULT '',
    s3_region       TEXT NOT NULL DEFAULT '',
    s3_bucket       TEXT NOT NULL DEFAULT '',
    s3_access_key   TEXT NOT NULL DEFAULT '',
    s3_secret_key   TEXT NOT NULL DEFAULT '', -- stored encrypted
    s3_prefix       TEXT NOT NULL DEFAULT '',
    s3_use_path_style INTEGER NOT NULL DEFAULT 1,
    mode            TEXT NOT NULL DEFAULT 'local' -- local | s3
);

INSERT INTO storage_config (id, local_path, mode)
SELECT 1, '/downloads', 'local'
WHERE NOT EXISTS (SELECT 1 FROM storage_config WHERE id = 1);

CREATE TABLE IF NOT EXISTS downloads (
    id           TEXT PRIMARY KEY,
    source_url   TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    thumbnail    TEXT NOT NULL DEFAULT '',
    uploader     TEXT NOT NULL DEFAULT '',
    duration     INTEGER NOT NULL DEFAULT 0,
    format_id    TEXT NOT NULL DEFAULT '',
    resolution   TEXT NOT NULL DEFAULT '',
    filesize     INTEGER NOT NULL DEFAULT 0,
    storage_type TEXT NOT NULL DEFAULT '', -- local | s3
    local_path   TEXT NOT NULL DEFAULT '',
    s3_key       TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'queued', -- queued|downloading|uploading|complete|failed
    error        TEXT NOT NULL DEFAULT '',
    proxy_mode   TEXT NOT NULL DEFAULT 'global', -- global|direct|custom
    custom_proxy TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS jobs (
    id          TEXT PRIMARY KEY,
    download_id TEXT NOT NULL,
    state       TEXT NOT NULL DEFAULT 'queued', -- queued|downloading|uploading|complete|failed
    progress    REAL NOT NULL DEFAULT 0,
    message     TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (download_id) REFERENCES downloads(id)
);

CREATE TABLE IF NOT EXISTS logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    category    TEXT NOT NULL DEFAULT 'general', -- download|ytdlp|upload|proxy|general
    download_id TEXT NOT NULL DEFAULT '',
    message     TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_downloads_status ON downloads(status);
CREATE INDEX IF NOT EXISTS idx_jobs_download_id ON jobs(download_id);
CREATE INDEX IF NOT EXISTS idx_logs_category ON logs(category);
CREATE INDEX IF NOT EXISTS idx_logs_download_id ON logs(download_id);
