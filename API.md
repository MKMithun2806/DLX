# API Docs

Base URL: `http://<host>:8080`

All mutating requests (`POST` / `PUT` / `DELETE`) require a CSRF token. The
server sets a `vd_csrf` cookie on first contact; echo its value back via the
`X-CSRF-Token` header (the bundled frontend does this automatically). API
clients should fetch `GET /` once to obtain the cookie, then read it from
their cookie jar for subsequent requests.

Both JSON (`Content-Type: application/json`) and HTML form
(`application/x-www-form-urlencoded`) bodies are accepted on every
write endpoint; form submissions additionally receive an HTML partial
response when the request includes the `HX-Request: true` header (used by
the bundled HTMX frontend) instead of JSON.

---

## Scan

### `POST /api/scan`

Scan one or more URLs with `yt-dlp --dump-json`.

**Body (JSON):**
```json
{ "urls": "https://youtube.com/watch?v=abc\nhttps://youtube.com/watch?v=xyz" }
```

**Response `200`:**
```json
[
  {
    "url": "https://youtube.com/watch?v=abc",
    "title": "Example Video",
    "thumbnail": "https://.../thumb.jpg",
    "duration": 212,
    "uploader": "Example Channel",
    "resolution": "1920x1080",
    "filesize": 104857600,
    "formats": [
      { "format_id": "137", "resolution": "1920x1080", "ext": "mp4", "filesize": 104857600, "note": "1080p" }
    ]
  }
]
```
Per-URL failures are returned inline as `{"url": "...", "error": "..."}` rather than failing the whole batch.

---

## Download

### `POST /api/download`

Enqueue a background download job.

**Body (JSON):**
```json
{
  "url": "https://youtube.com/watch?v=abc",
  "format_id": "137",
  "proxy_mode": "global",
  "custom_proxy": ""
}
```
- `proxy_mode`: `global` (default) | `direct` | `custom`
- `custom_proxy`: required when `proxy_mode` is `custom`

**Response `202`:**
```json
{ "id": "f47ac10b-...", "status": "queued" }
```

Completed downloads are stored as packages with `video_s3_key`, `thumbnail_s3_key`, `metadata_s3_key`, and `metadata_json` in SQLite. Those fields are populated once the background job finishes.

### `POST /api/downloads/{id}/retry`

Re-queues a failed (or completed) download for another attempt.

**Response `200`:** `{ "status": "queued" }`

### `GET /api/downloads/{id}/file`

Streams a locally stored file back to the caller as an attachment. Returns
`400` if the download was stored in S3 instead of locally.

### `DELETE /api/downloads/{id}`

Deletes the download record (and the underlying file, if stored locally).

### `POST /api/recovery`

Scans the active storage backend for package folders, reads `metadata.json`, and rebuilds SQLite rows.

**Response `200`:**
```json
{
  "recovered": 12,
  "warnings": [],
  "items": [
    {
      "download_id": "07db9381",
      "root_key": "videos/07db9381"
    }
  ]
}
```

---

## Jobs / History

### `GET /api/jobs`
### `GET /api/downloads`

Both return the same data: the most recent 200 downloads with their current
status. (`/api/jobs` is provided as a semantic alias used by the dashboard's
"Active & Recent Jobs" panel.)

**Response `200`:**
```json
[
  {
    "id": "f47ac10b-...",
    "source_url": "https://youtube.com/watch?v=abc",
    "title": "Example Video",
    "status": "complete",
    "storage_type": "local",
    "local_path": "/downloads/f47ac10b/video.mp4",
    "video_s3_key": "f47ac10b/video.mp4",
    "thumbnail_s3_key": "f47ac10b/thumbnail.jpg",
    "metadata_s3_key": "f47ac10b/metadata.json",
    "metadata_json": "{\"title\":\"Example Video\"}",
    "filesize": 104857600,
    "created_at": "2026-06-24T10:15:00Z",
    "updated_at": "2026-06-24T10:17:42Z"
  }
]
```

### `GET /events`

Server-Sent Events stream. Emits a `connected` event on open, then a
`job_update` event (JSON payload `{download_id, state, progress, message}`)
every time a job's state changes.

## Watcher

### `GET /watch/{id}`

Renders the dedicated content viewer for a completed download.

### `GET /api/watch/{id}`

Returns the watch payload used by the viewer:

```json
{
  "download": { "...": "..." },
  "video_url": "/api/watch/f47ac10b.../asset/video",
  "thumbnail_url": "/api/watch/f47ac10b.../asset/thumbnail",
  "metadata_url": "/api/watch/f47ac10b.../asset/metadata",
  "metadata": { "...": "..." }
}
```

### `GET /api/watch/{id}/asset/{kind}`

Returns one of `video`, `thumbnail`, or `metadata`.

- Local storage serves the file directly.
- S3 storage redirects video/thumbnail requests to a short-lived presigned URL.
- Metadata is returned as JSON from the stored `metadata.json` payload.

---

## Settings (global proxy)

### `GET /api/settings`

```json
{
  "proxy_http": "",
  "proxy_https": "http://proxy.example.com:8080",
  "proxy_socks5": "",
  "rotation_mode": "random",
  "direct_fallback": true
}
```

### `PUT /api/settings`

Body: same shape as the `GET` response. `rotation_mode` must be one of
`random`, `round_robin`, `sticky`.

---

## Proxy pool

### `GET /api/proxies`
```json
[
  { "id": "...", "name": "Residential US", "proxy_url": "http://user:pass@1.2.3.4:8080", "enabled": true, "created_at": "..." }
]
```

### `POST /api/proxies`
Body: `{ "name": "...", "proxy_url": "...", "enabled": true }`

### `PUT /api/proxies/{id}`
Body: same shape as `POST`. Used by the UI for enable/disable toggles as well as edits.

### `DELETE /api/proxies/{id}`
Removes a proxy from the pool. Returns `204`.

---

## Storage

### `GET /api/storage`
```json
{
  "storage": {
    "local_path": "/downloads",
    "s3_endpoint": "",
    "s3_region": "",
    "s3_bucket": "",
    "s3_access_key": "",
    "s3_prefix": "",
    "s3_use_path_style": true,
    "mode": "local"
  },
  "usage": { "total": 500107862016, "used": 120000000000, "free": 380107862016 }
}
```
`s3_secret_key` is never included in responses.

### `PUT /api/storage`
Body: same shape as the `storage` object above, plus `s3_secret_key` (write-only). `mode` must be `local` or `s3`. Leave `s3_secret_key` blank to keep the previously stored (encrypted) value.

---

## Logs

### `GET /api/logs?category=download|ytdlp|upload|proxy|all&q=search+text`

```json
[
  { "id": 102, "category": "ytdlp", "download_id": "f47ac10b-...", "message": "[download]  42.0% of 100.00MiB", "created_at": "..." }
]
```

---

## Error format

Non-2xx responses return:
```json
{ "error": "human readable message" }
```
