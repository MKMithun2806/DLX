package handlers

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

// ListDownloads handles GET /api/jobs (and doubles as the history feed).
func (a *App) ListDownloads(w http.ResponseWriter, r *http.Request) {
	downloads, err := a.Repo.ListDownloads(200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if isHTMXRequest(r) {
		a.Tmpl.ExecuteTemplate(w, "history_rows.html", map[string]any{"Downloads": downloads})
		return
	}
	writeJSON(w, http.StatusOK, downloads)
}

// DeleteDownload handles DELETE /api/downloads/{id}. For local storage the
// underlying file is also removed; for S3 only the database record is
// removed (object lifecycle on the bucket is left to the user/provider).
func (a *App) DeleteDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, err := a.Repo.GetDownload(id)
	if err == nil && d.StorageType == "local" && d.LocalPath != "" {
		_ = os.Remove(d.LocalPath)
	}
	if err := a.Repo.DeleteDownload(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if isHTMXRequest(r) {
		downloads, _ := a.Repo.ListDownloads(200)
		a.Tmpl.ExecuteTemplate(w, "history_rows.html", map[string]any{"Downloads": downloads})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DownloadFile handles GET /api/downloads/{id}/file - streams a locally
// stored file back to the browser. S3-stored files are instead expected to
// be accessed directly via the provider (a pre-signed URL endpoint could be
// added here in the future).
func (a *App) DownloadFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, err := a.Repo.GetDownload(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "download not found")
		return
	}
	if d.StorageType != "local" || d.LocalPath == "" {
		writeError(w, http.StatusBadRequest, "file is not available for direct download (stored in S3)")
		return
	}
	if _, err := os.Stat(d.LocalPath); err != nil {
		writeError(w, http.StatusNotFound, "file no longer exists on disk")
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(d.LocalPath)+"\"")
	http.ServeFile(w, r, d.LocalPath)
}
