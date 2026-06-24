package handlers

import (
	"net/http"

	"videodl/internal/models"
)

// GetSettings handles GET /api/settings.
func (a *App) GetSettings(w http.ResponseWriter, r *http.Request) {
	s, err := a.Repo.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// PutSettings handles PUT /api/settings.
func (a *App) PutSettings(w http.ResponseWriter, r *http.Request) {
	var s models.Settings
	if err := decodeJSON(r, &s); err != nil {
		// also support HTML form submission from the settings page
		if err2 := r.ParseForm(); err2 == nil {
			s.ProxyHTTP = r.FormValue("proxy_http")
			s.ProxyHTTPS = r.FormValue("proxy_https")
			s.ProxySOCKS5 = r.FormValue("proxy_socks5")
			s.RotationMode = r.FormValue("rotation_mode")
			s.DirectFallback = r.FormValue("direct_fallback") == "on" || r.FormValue("direct_fallback") == "1"
		} else {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if s.RotationMode == "" {
		s.RotationMode = "random"
	}
	if err := a.Repo.SaveSettings(s); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.Pool.SetMode(s.RotationMode)

	if isHTMXRequest(r) {
		w.Write([]byte(`<div class="text-emerald-400 text-sm">Settings saved.</div>`))
		return
	}
	writeJSON(w, http.StatusOK, s)
}
