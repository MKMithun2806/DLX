package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"videodl/internal/models"
	"videodl/internal/storage"
)

type watchPayload struct {
	Download      models.Download `json:"download"`
	VideoURL      string          `json:"video_url"`
	ThumbnailURL  string          `json:"thumbnail_url"`
	MetadataURL   string          `json:"metadata_url"`
	MetadataJSON  string          `json:"metadata_json,omitempty"`
	Metadata      map[string]any  `json:"metadata,omitempty"`
	MetadataError string          `json:"metadata_error,omitempty"`
}

// Watch renders the dedicated content viewer for a completed download.
func (a *App) Watch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	download, err := a.Repo.GetDownload(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "download not found")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.Tmpl.ExecuteTemplate(w, "watch.html", map[string]any{
		"Download": download,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// WatchData returns the structured download + metadata payload used by the
// watch page to hydrate the player and metadata sidebar.
func (a *App) WatchData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	download, err := a.Repo.GetDownload(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "download not found")
		return
	}

	payload, err := a.buildWatchPayload(r.Context(), download)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// WatchAsset serves a stored asset for the viewer. Local files are streamed
// directly. S3-backed media is redirected to a short-lived presigned URL so
// the browser can handle range requests natively.
func (a *App) WatchAsset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	kind := chi.URLParam(r, "kind")

	download, err := a.Repo.GetDownload(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "download not found")
		return
	}

	backend, storeCfg, err := a.storageBackend(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	switch strings.ToLower(kind) {
	case "video":
		if storeCfg.Mode == "s3" {
			key := firstNonEmpty(download.VideoS3Key, download.S3Key)
			if key == "" {
				writeError(w, http.StatusNotFound, "video not available")
				return
			}
			if signer, ok := backend.(interface {
				PresignGetObject(context.Context, string, time.Duration) (string, error)
			}); ok {
				url, err := signer.PresignGetObject(r.Context(), key, 15*time.Minute)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				http.Redirect(w, r, url, http.StatusTemporaryRedirect)
				return
			}
			data, err := backend.ReadFile(r.Context(), key)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}

		path, err := localAssetPath(download, storeCfg, kind)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		http.ServeFile(w, r, path)
		return

	case "thumbnail":
		if storeCfg.Mode == "s3" {
			key := firstNonEmpty(download.ThumbnailS3Key, download.S3Key)
			if key == "" {
				writeError(w, http.StatusNotFound, "thumbnail not available")
				return
			}
			if signer, ok := backend.(interface {
				PresignGetObject(context.Context, string, time.Duration) (string, error)
			}); ok {
				url, err := signer.PresignGetObject(r.Context(), key, 15*time.Minute)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				http.Redirect(w, r, url, http.StatusTemporaryRedirect)
				return
			}
			data, err := backend.ReadFile(r.Context(), key)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}

		path, err := localAssetPath(download, storeCfg, kind)
		if err != nil {
			thumbURL := download.Thumbnail
			if thumbURL == "" {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			http.Redirect(w, r, thumbURL, http.StatusTemporaryRedirect)
			return
		}
		http.ServeFile(w, r, path)
		return

	case "metadata":
		raw, err := a.resolveMetadata(r.Context(), download)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(raw) == 0 {
			writeError(w, http.StatusNotFound, "metadata not available")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
		return
	default:
		writeError(w, http.StatusBadRequest, "unknown asset kind")
		return
	}
}

func (a *App) buildWatchPayload(ctx context.Context, d models.Download) (watchPayload, error) {
	metadataRaw, metadata, metadataErr := a.readMetadata(ctx, d)
	payload := watchPayload{
		Download:     d,
		VideoURL:     "/api/watch/" + d.ID + "/asset/video",
		ThumbnailURL: "/api/watch/" + d.ID + "/asset/thumbnail",
		MetadataURL:  "/api/watch/" + d.ID + "/asset/metadata",
		MetadataJSON: string(metadataRaw),
		Metadata:     metadata,
	}
	if metadataErr != nil {
		payload.MetadataError = metadataErr.Error()
	}
	if payload.ThumbnailURL == "" {
		payload.ThumbnailURL = d.Thumbnail
	}
	return payload, nil
}

func (a *App) readMetadata(ctx context.Context, d models.Download) ([]byte, map[string]any, error) {
	raw, err := a.resolveMetadata(ctx, d)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) == 0 {
		return nil, nil, nil
	}
	var meta map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&meta); err != nil {
		return raw, nil, err
	}
	return raw, meta, nil
}

func (a *App) resolveMetadata(ctx context.Context, d models.Download) ([]byte, error) {
	if strings.TrimSpace(d.MetadataJSON) != "" {
		return []byte(d.MetadataJSON), nil
	}
	if strings.TrimSpace(d.MetadataS3Key) == "" {
		return nil, nil
	}
	backend, _, err := a.storageBackend(ctx)
	if err != nil {
		return nil, err
	}
	return backend.ReadFile(ctx, d.MetadataS3Key)
}

func (a *App) storageBackend(ctx context.Context) (storage.Backend, models.StorageConfig, error) {
	storeCfg, err := a.Repo.GetStorageConfig()
	if err != nil {
		return nil, models.StorageConfig{}, err
	}

	switch storeCfg.Mode {
	case "s3":
		secret, _ := a.Box.Decrypt(storeCfg.S3SecretKey)
		backend, err := storage.NewS3(ctx, storage.S3Config{
			Endpoint:     storeCfg.S3Endpoint,
			Region:       storeCfg.S3Region,
			Bucket:       storeCfg.S3Bucket,
			AccessKey:    storeCfg.S3AccessKey,
			SecretKey:    secret,
			Prefix:       storeCfg.S3Prefix,
			UsePathStyle: storeCfg.S3UsePathStyle,
		})
		if err != nil {
			return nil, models.StorageConfig{}, fmt.Errorf("s3 backend init failed: %w", err)
		}
		return backend, storeCfg, nil
	default:
		return storage.NewLocal(storeCfg.LocalPath), storeCfg, nil
	}
}

func localAssetPath(d models.Download, storeCfg models.StorageConfig, kind string) (string, error) {
	root := storeCfg.LocalPath
	if strings.TrimSpace(root) == "" {
		root = "/downloads"
	}

	switch kind {
	case "video":
		if strings.TrimSpace(d.LocalPath) != "" {
			return d.LocalPath, nil
		}
		if key := firstNonEmpty(d.VideoS3Key, d.S3Key); key != "" {
			return filepath.Join(root, filepath.FromSlash(key)), nil
		}
	case "thumbnail":
		if key := firstNonEmpty(d.ThumbnailS3Key, guessStoredKey(d.LocalPath, "thumbnail")); key != "" {
			return filepath.Join(root, filepath.FromSlash(key)), nil
		}
	case "metadata":
		if key := firstNonEmpty(d.MetadataS3Key, guessStoredKey(d.LocalPath, "metadata.json")); key != "" {
			return filepath.Join(root, filepath.FromSlash(key)), nil
		}
	}
	return "", fmt.Errorf("%s not available", kind)
}

func guessStoredKey(localPath, wanted string) string {
	if strings.TrimSpace(localPath) == "" {
		return ""
	}
	dir := filepath.Dir(localPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if wanted == "metadata.json" {
			if strings.EqualFold(name, "metadata.json") || strings.HasSuffix(strings.ToLower(name), ".info.json") {
				return filepath.ToSlash(filepath.Join(filepath.Base(dir), name))
			}
			continue
		}
		if strings.HasPrefix(strings.ToLower(name), wanted) {
			return filepath.ToSlash(filepath.Join(filepath.Base(dir), name))
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
