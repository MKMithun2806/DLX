package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"path/filepath"

	"videodl/internal/config"
	"videodl/internal/crypto"
	"videodl/internal/db"
	"videodl/internal/jobs"
	"videodl/internal/proxy"
	"videodl/internal/ytdlp"
)

// App bundles every dependency handlers need: the repository, background
// job manager, yt-dlp runner, proxy pool, crypto box, app config, and
// parsed HTML templates.
type App struct {
	Repo    *db.Repo
	Manager *jobs.Manager
	Runner  *ytdlp.Runner
	Pool    *proxy.Pool
	Box     *crypto.Box
	Cfg     *config.Config
	Tmpl    *template.Template
}

func NewApp(repo *db.Repo, mgr *jobs.Manager, runner *ytdlp.Runner, pool *proxy.Pool, box *crypto.Box, cfg *config.Config, templatesDir string) (*App, error) {
	funcs := template.FuncMap{
		"dict": func(values ...any) map[string]any {
			m := make(map[string]any)
			for i := 0; i+1 < len(values); i += 2 {
				key, _ := values[i].(string)
				m[key] = values[i+1]
			}
			return m
		},
	}
	tmpl, err := template.New("root").Funcs(funcs).ParseGlob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		return nil, err
	}
	tmpl, err = tmpl.ParseGlob(filepath.Join(templatesDir, "partials", "*.html"))
	if err != nil {
		return nil, err
	}
	return &App{Repo: repo, Manager: mgr, Runner: runner, Pool: pool, Box: box, Cfg: cfg, Tmpl: tmpl}, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// validURL performs basic sanity validation on user-supplied source URLs to
// guard against obviously malformed or non-http(s) input before handing
// them to yt-dlp/exec.
func validURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
