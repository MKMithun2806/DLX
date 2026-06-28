package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"videodl/internal/config"
	"videodl/internal/crypto"
	appdb "videodl/internal/db"
	"videodl/internal/handlers"
	"videodl/internal/jobs"
	appmw "videodl/internal/middleware"
	"videodl/internal/proxy"
	"videodl/internal/ytdlp"
)

func main() {
	cfg := config.Load()

	conn, err := appdb.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer conn.Close()
	repo := appdb.NewRepo(conn)

	box, err := crypto.New(cfg.AppSecret)
	if err != nil {
		log.Fatalf("failed to init crypto box: %v", err)
	}

	if err := os.MkdirAll(cfg.DownloadsRoot, 0o755); err != nil {
		log.Fatalf("failed to create downloads root %s: %v", cfg.DownloadsRoot, err)
	}

	settings, _ := repo.GetSettings()
	pool := proxy.NewPool(settings.RotationMode)
	if existing, err := repo.ListProxies(); err == nil {
		pool.SetProxies(existing)
	}

	runner := ytdlp.New(cfg.YtDlpPath)
	tmpRoot := "/tmp/videodl-jobs"
	mgr := jobs.NewManager(repo, runner, pool, box, tmpRoot, 2)

	if report, err := mgr.MigrateLegacyDownloads(context.Background()); err != nil {
		log.Printf("legacy package migration failed: %v", err)
	} else if report.Recovered > 0 || len(report.Warnings) > 0 {
		log.Printf("legacy package migration completed: migrated=%d warnings=%d", report.Recovered, len(report.Warnings))
	}

	templatesDir := envOr("TEMPLATES_DIR", "web/templates")
	app, err := handlers.NewApp(repo, mgr, runner, pool, box, cfg, templatesDir)
	if err != nil {
		log.Fatalf("failed to load templates: %v", err)
	}

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(chimw.Timeout(120 * time.Second))
	r.Use(appmw.CSRF)

	limiter := appmw.NewRateLimiter(5, 20) // 5 req/s sustained, burst of 20 per client IP
	r.Use(limiter.Middleware)

	staticDir := envOr("STATIC_DIR", "web/static")
	fs := http.FileServer(http.Dir(staticDir))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))

	r.Get("/", app.Dashboard)
	r.Get("/events", app.JobsStream)

	r.Route("/api", func(r chi.Router) {
		r.Post("/scan", app.Scan)
		r.Post("/download", app.Download)

		r.Get("/jobs", app.ListDownloads)

		r.Get("/settings", app.GetSettings)
		r.Put("/settings", app.PutSettings)
		r.Post("/settings", app.PutSettings) // HTML forms can't PUT natively

		r.Get("/storage", app.GetStorage)
		r.Put("/storage", app.PutStorage)
		r.Post("/storage", app.PutStorage)

		r.Get("/proxies", app.ListProxies)
		r.Post("/proxies", app.CreateProxy)
		r.Put("/proxies/{id}", app.UpdateProxy)
		r.Post("/proxies/{id}", app.UpdateProxy) // form-friendly alias
		r.Delete("/proxies/{id}", app.DeleteProxy)

		r.Get("/downloads", app.ListDownloads)
		r.Delete("/downloads/{id}", app.DeleteDownload)
		r.Post("/downloads/{id}/delete", app.DeleteDownload) // form-friendly alias
		r.Get("/downloads/{id}/file", app.DownloadFile)
		r.Post("/downloads/{id}/retry", app.RetryDownload)
		r.Post("/recovery", app.RecoverStorage)

		r.Get("/logs", app.Logs)
	})

	addr := ":" + cfg.AppPort
	log.Printf("video downloader webui listening on %s (db=%s downloads=%s)", addr, cfg.DBPath, cfg.DownloadsRoot)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
