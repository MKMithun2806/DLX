package handlers

import (
	"net/http"
	"strconv"
)

// RecoverStorage handles POST /api/recovery and rebuilds SQLite from the
// active storage backend.
func (a *App) RecoverStorage(w http.ResponseWriter, r *http.Request) {
	report, err := a.Manager.RecoverFromStorage(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if isHTMXRequest(r) {
		w.Write([]byte(`<div class="text-emerald-400 text-sm">Recovered ` + strconv.Itoa(report.Recovered) + ` package(s).</div>`))
		return
	}
	writeJSON(w, http.StatusOK, report)
}
