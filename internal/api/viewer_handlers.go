package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/MKMithun2806/DLX/internal/repo"
	"github.com/MKMithun2806/DLX/internal/service"
)

// RegisterViewerRoutes registers the viewer API endpoints
func RegisterViewerRoutes(r *mux.Router, db *sql.DB, vs *service.VideoService) {
	r.HandleFunc("/api/viewer/home", func(w http.ResponseWriter, req *http.Request) {
		// simple recent videos
		vr := repo.NewVideoRepository(db)
		videos, _ := vr.Search(context.Background(), "*", 50)
		_ = json.NewEncoder(w).Encode(videos)
	}).Methods("GET")

	r.HandleFunc("/api/viewer/video/{id}", func(w http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)
		id := vars["id"]
		vr := repo.NewVideoRepository(db)
		v, _ := vr.GetByID(context.Background(), id)
		if v == nil { w.WriteHeader(http.StatusNotFound); return }
		_ = json.NewEncoder(w).Encode(v)
	}).Methods("GET")

	r.HandleFunc("/api/viewer/recent", func(w http.ResponseWriter, req *http.Request) {
		vr := repo.NewVideoRepository(db)
		videos, _ := vr.Search(context.Background(), "*", 20)
		_ = json.NewEncoder(w).Encode(videos)
	}).Methods("GET")

	r.HandleFunc("/api/videos/{id}/refresh", func(w http.ResponseWriter, req *http.Request) {
		// enqueue a metadata/thumbnail refresh job - for now sync
		vars := mux.Vars(req)
		id := vars["id"]
		// TODO: enqueue job via existing queue system - here we just return accepted
		_ = id
		w.WriteHeader(http.StatusAccepted)
	}).Methods("POST")

}
