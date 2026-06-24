package handlers

import (
	"net/http"
)

// Dashboard renders the single-page web UI (index.html), which then drives
// itself via HTMX/Alpine + the JSON/SSE API.
func (a *App) Dashboard(w http.ResponseWriter, r *http.Request) {
	settings, _ := a.Repo.GetSettings()
	proxies, _ := a.Repo.ListProxies()
	storageCfg, _ := a.Repo.GetStorageConfig()
	storageCfg.S3SecretKey = "" // never render secrets into the page
	downloads, _ := a.Repo.ListDownloads(100)

	data := map[string]any{
		"Settings":  settings,
		"Proxies":   proxies,
		"Storage":   storageCfg,
		"Downloads": downloads,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.Tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
