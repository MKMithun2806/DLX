package handlers

import (
	"net/http"
	"strings"

	"videodl/internal/models"
	"videodl/internal/storage"
)

// GetStorage handles GET /api/storage. The S3 secret key is never returned.
func (a *App) GetStorage(w http.ResponseWriter, r *http.Request) {
	sc, err := a.Repo.GetStorageConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sc.S3SecretKey = ""

	usage := map[string]uint64{}
	if local := storage.NewLocal(sc.LocalPath); local != nil {
		if u, err := local.Usage(); err == nil {
			usage["total"] = u.TotalBytes
			usage["used"] = u.UsedBytes
			usage["free"] = u.FreeBytes
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"storage": sc, "usage": usage})
}

// PutStorage handles PUT /api/storage. If S3SecretKey is left blank in the
// request, the previously stored (encrypted) secret is preserved rather
// than being overwritten with an empty value.
func (a *App) PutStorage(w http.ResponseWriter, r *http.Request) {
	var sc models.StorageConfig
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := decodeJSON(r, &sc); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid form body")
			return
		}
		sc.LocalPath = r.FormValue("local_path")
		sc.S3Endpoint = r.FormValue("s3_endpoint")
		sc.S3Region = r.FormValue("s3_region")
		sc.S3Bucket = r.FormValue("s3_bucket")
		sc.S3AccessKey = r.FormValue("s3_access_key")
		sc.S3SecretKey = r.FormValue("s3_secret_key")
		sc.S3Prefix = r.FormValue("s3_prefix")
		sc.S3UsePathStyle = r.FormValue("s3_use_path_style") == "on" || r.FormValue("s3_use_path_style") == "1"
		sc.Mode = r.FormValue("mode")
	}

	if sc.Mode != "s3" {
		sc.Mode = "local"
	}
	if strings.TrimSpace(sc.LocalPath) == "" {
		sc.LocalPath = "/downloads"
	}

	existing, _ := a.Repo.GetStorageConfig()
	if sc.S3SecretKey == "" {
		sc.S3SecretKey = existing.S3SecretKey // keep previously encrypted value
	} else {
		enc, err := a.Box.Encrypt(sc.S3SecretKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt secret: "+err.Error())
			return
		}
		sc.S3SecretKey = enc
	}

	if err := a.Repo.SaveStorageConfig(sc); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if isHTMXRequest(r) {
		w.Write([]byte(`<div class="text-emerald-400 text-sm">Storage settings saved.</div>`))
		return
	}
	sc.S3SecretKey = ""
	writeJSON(w, http.StatusOK, sc)
}
