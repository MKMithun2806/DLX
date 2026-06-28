# Video Downloader WebUI

A self-hosted web UI for scanning and downloading videos with [yt-dlp](https://github.com/yt-dlp/yt-dlp), storing the results either on local disk or in any S3-compatible object store (AWS S3, MinIO, Wasabi, Backblaze B2). Built in Go, runs entirely in Docker.

## Features

- Paste single URLs, multiple URLs, or playlist URLs and scan them with `yt-dlp --dump-json` to preview title, thumbnail, duration, uploader, resolutions, and estimated file size.
- Queue downloads as background jobs with live progress via Server-Sent Events (no polling).
- Store completed downloads as self-contained packages with `video`, `thumbnail`, and `metadata.json` under one folder locally or in S3-compatible storage.
- Global proxy configuration (HTTP / HTTPS / SOCKS5) applied to all downloads by default, plus a per-download proxy override (use global, direct, or a one-off custom proxy).
- A full proxy pool with random / round-robin / sticky-session rotation and automatic fallback to a direct connection if every proxy attempt fails.
- Download history with status, file size, retry, delete, and package key visibility for downstream viewers.
- Searchable logs across download, yt-dlp, upload, and proxy events.
- CSRF protection, per-IP rate limiting, and AES-256-GCM encryption of S3/proxy secrets at rest in SQLite.

## Tech stack

- **Backend:** Go 1.22+, [chi](https://github.com/go-chi/chi) router, SQLite (pure-Go `modernc.org/sqlite` driver, no CGO), AWS SDK v2 for S3.
- **Frontend:** plain HTML + [HTMX](https://htmx.org) + [Alpine.js](https://alpinejs.dev) + Tailwind CSS (all loaded from CDN, no Node build step).
- **Video engine:** [yt-dlp](https://github.com/yt-dlp/yt-dlp) + ffmpeg, installed in the runtime container image.

## Quick start (Docker Compose)

```bash
git clone <this-repo> video-downloader-webui
cd video-downloader-webui

# Set a real secret used to encrypt S3/proxy credentials at rest:
export APP_SECRET="$(openssl rand -hex 32)"

docker compose up -d --build
```

Then open **http://localhost:8080**.

Downloaded files land in `./data/downloads` on the host (bind-mounted to `/downloads` in the container); the SQLite config database lives in `./data/config/app.db`.

## Quick start (plain Docker)

```bash
docker build -t video-downloader-webui .
docker run -d \
  --name video-downloader-webui \
  -p 8080:8080 \
  -e APP_SECRET="$(openssl rand -hex 32)" \
  -v $(pwd)/data/config:/config \
  -v $(pwd)/data/downloads:/downloads \
  video-downloader-webui
```

## Running locally without Docker

Requires Go 1.22+, `yt-dlp` and `ffmpeg` on your `PATH`.

```bash
go mod download
go run ./cmd/server
```

The server defaults to `:8080`, a SQLite DB at `/config/app.db`, and downloads in `/downloads` - override with the environment variables below if those paths aren't writable on your machine (e.g. `DB_PATH=./app.db DOWNLOADS_PATH=./downloads go run ./cmd/server`).

## Environment variables

| Variable          | Default              | Description                                              |
|--------------------|-----------------------|------------------------------------------------------------|
| `APP_PORT`         | `8080`               | HTTP listen port                                          |
| `DB_PATH`          | `/config/app.db`     | SQLite database file path                                 |
| `DOWNLOADS_PATH`   | `/downloads`         | Default local storage root (also set via Storage settings)|
| `APP_SECRET`       | *(insecure default)* | Key used to encrypt S3/proxy secrets at rest - **set this**|
| `YTDLP_PATH`       | `yt-dlp`             | Path to the yt-dlp binary                                  |
| `TEMPLATES_DIR`    | `web/templates`      | HTML template directory (set automatically in the image)   |
| `STATIC_DIR`       | `web/static`         | Static asset directory (set automatically in the image)    |

## Using the app

1. **Dashboard tab** - paste one or more URLs (one per line) and click **Scan**. Each result is shown as a card with title, thumbnail, uploader, duration, and a format dropdown. Pick a per-download proxy mode (global / direct / custom) and click **Download**.
2. **Proxy Settings tab** - set the global HTTP/HTTPS/SOCKS5 proxy applied by default, choose a rotation mode for the proxy pool, enable direct-connection fallback, and manage the pool of named proxies (add / enable / disable / remove).
3. **Storage tab** - drag the slider between **Local Only** and **S3 Only**, then fill in the local download path or your S3-compatible credentials (endpoint, region, bucket, access/secret key, prefix, path-style toggle). The secret key field is write-only - leave it blank on subsequent saves to keep the existing encrypted value. Use the recovery action there to rebuild SQLite from storage if the database is lost.
4. **History tab** - see every download with its status, storage backend, size, and package keys; download local files directly, retry failed jobs, or delete records (local package folders are removed from disk too).
5. **Logs tab** - filter by category (download / yt-dlp / upload / proxy) and free-text search across everything that's been logged.
6. **Watch page** - click `watch` from a completed download to open the dedicated player view with thumbnail, metadata, and storage-backed media playback.

## Project layout

```
cmd/server/            main.go - wires config, DB, services, router
internal/config/       environment-based configuration
internal/db/           SQLite connection, migrations, repository (queries)
internal/models/       shared structs (Settings, Proxy, Download, Job, ...)
internal/crypto/       AES-256-GCM encryption for secrets at rest
internal/ytdlp/        yt-dlp process wrapper (scan + download w/ progress)
internal/storage/      Backend interface + local disk + S3 implementations
internal/proxy/        Proxy pool with rotation modes + fallback resolution
internal/jobs/         Background queue/worker pool + SSE broadcaster
internal/handlers/     HTTP handlers (scan, download, settings, proxies, storage, history, logs)
internal/middleware/   CSRF protection + per-IP rate limiting
migrations/            SQL migration files (source of truth, mirrored into internal/db/migrations_files for go:embed)
web/templates/         HTML templates (index.html + HTMX partials)
web/static/            app.js (SSE client) + app.css
```

See [API.md](./API.md) for the full REST API reference.

## Security notes

- All mutating requests (`POST`/`PUT`/`DELETE`) require a CSRF token, issued as a cookie and echoed back via the `X-CSRF-Token` header (handled automatically by the bundled `app.js`/HTMX config).
- S3 secret keys and any proxy credentials embedded in proxy URLs are encrypted with AES-256-GCM before being written to SQLite, using a key derived from `APP_SECRET`. **Always set a strong, unique `APP_SECRET` in production.**
- Requests are rate-limited per client IP (5 req/s sustained, burst of 20) to reduce abuse of the scan/download endpoints.
- Input URLs are validated to be well-formed `http`/`https` URLs before being handed to `yt-dlp`.
