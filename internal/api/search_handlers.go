package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/MKMithun2806/DLX/internal/repo"
)

func RegisterSearchRoutes(r *mux.Router, db *sql.DB) {
	r.HandleFunc("/api/videos/search", func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query().Get("q")
		if q == "" { q = "*" }
		vr := repo.NewVideoRepository(db)
		videos, _ := vr.Search(context.Background(), q, 50)
		_ = json.NewEncoder(w).Encode(videos)
	}).Methods("GET")

	r.HandleFunc("/api/channels/search", func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query().Get("q")
		if q == "" { q = "*" }
		// For brevity we search channels_fts directly
		rows, err := db.QueryContext(context.Background(), "SELECT channel_id, name, thumbnail_s3_key, channel_url, metadata_json FROM channels WHERE rowid IN (SELECT rowid FROM channels_fts WHERE channels_fts MATCH ?) LIMIT 50", q)
		if err != nil { w.WriteHeader(http.StatusInternalServerError); return }
		defer rows.Close()
		out := []map[string]interface{}{}
		for rows.Next() {
			var id, name, thumb, url, meta string
			_ = rows.Scan(&id, &name, &thumb, &url, &meta)
			out = append(out, map[string]interface{}{"channel_id": id, "name": name, "thumbnail_s3_key": thumb, "channel_url": url, "metadata_json": meta})
		}
		_ = json.NewEncoder(w).Encode(out)
	}).Methods("GET")

	r.HandleFunc("/api/playlists/search", func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query().Get("q")
		if q == "" { q = "*" }
		rows, err := db.QueryContext(context.Background(), "SELECT playlist_id, title, description, thumbnail_s3_key, metadata_json FROM playlists WHERE rowid IN (SELECT rowid FROM playlists_fts WHERE playlists_fts MATCH ?) LIMIT 50", q)
		if err != nil { w.WriteHeader(http.StatusInternalServerError); return }
		defer rows.Close()
		out := []map[string]interface{}{}
		for rows.Next() {
			var id, title, desc, thumb, meta string
			_ = rows.Scan(&id, &title, &desc, &thumb, &meta)
			out = append(out, map[string]interface{}{"playlist_id": id, "title": title, "description": desc, "thumbnail_s3_key": thumb, "metadata_json": meta})
		}
		_ = json.NewEncoder(w).Encode(out)
	}).Methods("GET")
}
