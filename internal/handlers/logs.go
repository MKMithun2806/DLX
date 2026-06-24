package handlers

import (
	"net/http"
)

// Logs handles GET /api/logs?category=&q=
func (a *App) Logs(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	q := r.URL.Query().Get("q")

	entries, err := a.Repo.SearchLogs(category, q, 500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if isHTMXRequest(r) {
		a.Tmpl.ExecuteTemplate(w, "log_rows.html", map[string]any{"Logs": entries})
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// JobsStream handles GET /events - the SSE feed of live job progress.
func (a *App) JobsStream(w http.ResponseWriter, r *http.Request) {
	a.Manager.Events.ServeHTTP(w, r)
}
