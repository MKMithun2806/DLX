package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"videodl/internal/models"
)

// ListProxies handles GET /api/proxies.
func (a *App) ListProxies(w http.ResponseWriter, r *http.Request) {
	proxies, err := a.Repo.ListProxies()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if isHTMXRequest(r) {
		a.Tmpl.ExecuteTemplate(w, "proxy_rows.html", map[string]any{"Proxies": proxies})
		return
	}
	writeJSON(w, http.StatusOK, proxies)
}

// CreateProxy handles POST /api/proxies.
func (a *App) CreateProxy(w http.ResponseWriter, r *http.Request) {
	var p models.Proxy
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := decodeJSON(r, &p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid form body")
			return
		}
		p.Name = r.FormValue("name")
		p.ProxyURL = r.FormValue("proxy_url")
		p.Enabled = r.FormValue("enabled") == "on" || r.FormValue("enabled") == "1" || r.FormValue("enabled") == ""
	}
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.ProxyURL) == "" {
		writeError(w, http.StatusBadRequest, "name and proxy_url are required")
		return
	}
	p.ID = uuid.NewString()

	if err := a.Repo.CreateProxy(p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.refreshPool()

	if isHTMXRequest(r) {
		proxies, _ := a.Repo.ListProxies()
		a.Tmpl.ExecuteTemplate(w, "proxy_rows.html", map[string]any{"Proxies": proxies})
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// UpdateProxy handles PUT /api/proxies/{id} (also used for quick
// enable/disable toggles from the UI).
func (a *App) UpdateProxy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var p models.Proxy
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := decodeJSON(r, &p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid form body")
			return
		}
		p.Name = r.FormValue("name")
		p.ProxyURL = r.FormValue("proxy_url")
		p.Enabled = r.FormValue("enabled") == "on" || r.FormValue("enabled") == "1"
	}
	p.ID = id
	if err := a.Repo.UpdateProxy(p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.refreshPool()

	if isHTMXRequest(r) {
		proxies, _ := a.Repo.ListProxies()
		a.Tmpl.ExecuteTemplate(w, "proxy_rows.html", map[string]any{"Proxies": proxies})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// DeleteProxy handles DELETE /api/proxies/{id}.
func (a *App) DeleteProxy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.Repo.DeleteProxy(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.refreshPool()

	if isHTMXRequest(r) {
		proxies, _ := a.Repo.ListProxies()
		a.Tmpl.ExecuteTemplate(w, "proxy_rows.html", map[string]any{"Proxies": proxies})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) refreshPool() {
	proxies, _ := a.Repo.ListProxies()
	a.Pool.SetProxies(proxies)
}
