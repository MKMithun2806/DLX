package service

import (
	"context"
	"testing"
	"time"

	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/MKMithun2806/DLX/internal/repo"
	"github.com/MKMithun2806/DLX/internal/storage"
)

func TestVideoServiceIngest(t *testing.T) {
	// setup in-memory sqlite
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil { t.Fatalf("open db: %v", err) }
	defer db.Close()

	// create minimal videos table
	_, err = db.Exec(`CREATE TABLE videos (video_id TEXT PRIMARY KEY, title TEXT, description TEXT, channel_id TEXT, channel_name TEXT, duration INTEGER, thumbnail_s3_key TEXT, video_s3_key TEXT, upload_date TEXT, webpage_url TEXT, extractor TEXT, video_type TEXT, filesize INTEGER, metadata_json TEXT, slug TEXT, created_at TEXT, updated_at TEXT)`)
	if err != nil { t.Fatalf("create table: %v", err) }

	r := repo.NewVideoRepository(db)
	ls := storage.NewLocalStorage("./tmp_test_storage")
	vs := NewVideoService(r, ls, true)

	payload := map[string]interface{}{
		"id": "yt_test_1",
		"title": "Test Video",
		"description": "desc",
		"uploader": "Example",
		"uploader_id": "UCEXAMPLE",
		"duration": 120.0,
		"thumbnail": "https://httpbin.org/image/jpeg",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	v, err := vs.IngestFromYtDlpJSON(ctx, payload)
	if err != nil { t.Fatalf("ingest: %v", err) }
	if v.VideoID != "yt_test_1" { t.Fatalf("unexpected id: %s", v.VideoID) }
}
