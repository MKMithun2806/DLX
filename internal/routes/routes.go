package routes

import (
	"database/sql"

	"github.com/gorilla/mux"
	"github.com/MKMithun2806/DLX/internal/api"
	"github.com/MKMithun2806/DLX/internal/service"
)

func RegisterRoutes(r *mux.Router, db *sql.DB, vs *service.VideoService) {
	api.RegisterViewerRoutes(r, db, vs)
	api.RegisterSearchRoutes(r, db)
}
