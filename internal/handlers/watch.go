package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
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
				key = packageAssetKey(r.Context(), backend, storeCfg, download, "video")
			}
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
			key := firstNonEmpty(download.VideoS3Key, download.S3Key)
			if key == "" {
				key = packageAssetKey(r.Context(), backend, storeCfg, download, "video")
			}
			if key == "" {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			path = localStoredPath(storeCfg.LocalPath, key)
			if _, statErr := os.Stat(path); statErr != nil {
				writeError(w, http.StatusNotFound, "video not available")
				return
			}
		}
		http.ServeFile(w, r, path)
		return

	case "thumbnail":
		if storeCfg.Mode == "s3" {
			key := firstNonEmpty(download.ThumbnailS3Key, download.S3Key)
			if key == "" {
				key = packageAssetKey(r.Context(), backend, storeCfg, download, "thumbnail")
			}
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
			key := firstNonEmpty(download.ThumbnailS3Key, download.S3Key)
			if key == "" {
				key = packageAssetKey(r.Context(), backend, storeCfg, download, "thumbnail")
			}
			if key == "" {
				thumbURL := download.Thumbnail
				if thumbURL == "" {
					writeError(w, http.StatusNotFound, err.Error())
					return
				}
				http.Redirect(w, r, thumbURL, http.StatusTemporaryRedirect)
				return
			}
			path = localStoredPath(storeCfg.LocalPath, key)
			if _, statErr := os.Stat(path); statErr != nil {
				thumbURL := download.Thumbnail
				if thumbURL == "" {
					writeError(w, http.StatusNotFound, "thumbnail not available")
					return
				}
				http.Redirect(w, r, thumbURL, http.StatusTemporaryRedirect)
				return
			}
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

	backend, storeCfg, err := a.storageBackend(ctx)
	if err != nil {
		return nil, err
	}

	for _, candidate := range metadataCandidates(d) {
		if candidate == "" {
			continue
		}
		raw, err := backend.ReadFile(ctx, candidate)
		if err == nil && len(raw) > 0 {
			return raw, nil
		}
	}

	if key := packageAssetKey(ctx, backend, storeCfg, d, "metadata"); key != "" {
		raw, err := backend.ReadFile(ctx, key)
		if err == nil && len(raw) > 0 {
			return raw, nil
		}
	}

	return nil, nil
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

func metadataCandidates(d models.Download) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	add := func(key string) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}

	add(d.MetadataS3Key)
	add(deriveMetadataKey(d.VideoS3Key))
	add(deriveMetadataKey(d.S3Key))
	add(guessStoredKey(d.LocalPath, "metadata.json"))

	return out
}

func packageAssetKey(ctx context.Context, backend storage.Backend, storeCfg models.StorageConfig, d models.Download, kind string) string {
	for _, root := range packageRootCandidates(storeCfg, d) {
		files, err := backend.ListPackageFiles(ctx, root)
		if err != nil {
			continue
		}
		name := packageFilenameForKind(files, kind)
		if name == "" {
			continue
		}
		return path.Join(root, name)
	}
	return ""
}

func packageRootCandidates(storeCfg models.StorageConfig, d models.Download) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 2)
	add := func(root string) {
		root = strings.Trim(root, "/")
		if root == "" {
			return
		}
		if _, ok := seen[root]; ok {
			return
		}
		seen[root] = struct{}{}
		out = append(out, root)
	}

	if storeCfg.Mode == "s3" {
		prefix := strings.Trim(storeCfg.S3Prefix, "/")
		if prefix == "" {
			add(path.Join("videos", d.ID))
		} else {
			add(path.Join(prefix, d.ID))
		}
	} else {
		add(d.ID)
	}

	if d.LocalPath != "" {
		add(filepath.Base(filepath.Dir(d.LocalPath)))
	}

	return out
}

func packageFilenameForKind(files []string, kind string) string {
	for _, name := range files {
		switch kind {
		case "video":
			if strings.HasPrefix(strings.ToLower(name), "video.") {
				return name
			}
		case "thumbnail":
			if strings.HasPrefix(strings.ToLower(name), "thumbnail.") {
				return name
			}
		case "metadata":
			if strings.EqualFold(name, "metadata.json") || strings.HasSuffix(strings.ToLower(name), ".info.json") {
				return name
			}
		}
	}
	return ""
}

func deriveMetadataKey(sourceKey string) string {
	sourceKey = strings.TrimSpace(sourceKey)
	if sourceKey == "" {
		return ""
	}

	normalized := path.Clean(filepath.ToSlash(sourceKey))
	if normalized == "." || normalized == "/" {
		return "metadata.json"
	}
	return path.Join(path.Dir(normalized), "metadata.json")
}

func localStoredPath(root, key string) string {
	if strings.TrimSpace(root) == "" {
		root = "/downloads"
	}
	return filepath.Join(root, filepath.FromSlash(key))
}
