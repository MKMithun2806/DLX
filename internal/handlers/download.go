package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"videodl/internal/models"
)

// Download handles POST /api/download. It re-scans the URL (cheap, and
// ensures we have fresh metadata/format info even if the user downloads
// without ever pressing Scan), then enqueues a background job.
func (a *App) Download(w http.ResponseWriter, r *http.Request) {
	var req models.DownloadRequest

	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid form body")
			return
		}
		req.URL = r.FormValue("url")
		req.FormatID = r.FormValue("format_id")
		req.ProxyMode = r.FormValue("proxy_mode")
		req.CustomProxy = r.FormValue("custom_proxy")
	}

	req.URL = strings.TrimSpace(req.URL)
	if !validURL(req.URL) {
		writeError(w, http.StatusBadRequest, "invalid URL: must be http(s)")
		return
	}
	if req.ProxyMode == "" {
		req.ProxyMode = "global"
	}
	if req.ProxyMode == "custom" && strings.TrimSpace(req.CustomProxy) == "" {
		writeError(w, http.StatusBadRequest, "custom proxy mode selected but no proxy URL was provided")
		return
	}

	settings, _ := a.Repo.GetSettings()
	globalProxy := settings.ProxyHTTPS
	if globalProxy == "" {
		globalProxy = settings.ProxyHTTP
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	scan := a.Runner.Scan(ctx, req.URL, globalProxy)
	// Even if the metadata scan fails we still attempt the download - some
	// extractors fail --dump-json for sites that nonetheless download fine.

	id, err := a.Manager.Enqueue(req, scan)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enqueue download: "+err.Error())
		return
	}

	if isHTMXRequest(r) {
		downloads, _ := a.Repo.ListDownloads(200)
		a.Tmpl.ExecuteTemplate(w, "history_rows.html", map[string]any{"Downloads": downloads})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id, "status": "queued"})
}

// RetryDownload handles POST /api/downloads/{id}/retry.
func (a *App) RetryDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if err := a.Manager.Retry(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
}
