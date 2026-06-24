package handlers

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"videodl/internal/models"
)

// Scan handles POST /api/scan. It accepts either a JSON body
// {"urls": "..."} or an HTMX form field "urls", splits on newlines, and
// scans each URL concurrently via yt-dlp --dump-json.
func (a *App) Scan(w http.ResponseWriter, r *http.Request) {
	var rawURLs string

	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var req models.ScanRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		rawURLs = req.URLs
	} else {
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid form body")
			return
		}
		rawURLs = r.FormValue("urls")
	}

	lines := strings.Split(rawURLs, "\n")
	var urls []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		urls = append(urls, l)
	}
	if len(urls) == 0 {
		writeError(w, http.StatusBadRequest, "no URLs provided")
		return
	}

	settings, _ := a.Repo.GetSettings()
	globalProxy := settings.ProxyHTTPS
	if globalProxy == "" {
		globalProxy = settings.ProxyHTTP
	}

	results := make([]models.ScanResult, len(urls))
	var wg sync.WaitGroup
	for i, u := range urls {
		if !validURL(u) {
			results[i] = models.ScanResult{URL: u, Error: "invalid URL: must be http(s)"}
			continue
		}
		wg.Add(1)
		go func(idx int, u string) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			results[idx] = a.Runner.Scan(ctx, u, globalProxy)
		}(i, u)
	}
	wg.Wait()

	if isHTMXRequest(r) {
		a.Tmpl.ExecuteTemplate(w, "scan_results.html", map[string]any{"Results": results})
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
