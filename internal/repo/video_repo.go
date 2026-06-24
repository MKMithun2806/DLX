package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/MKMithun2806/DLX/internal/models"
)

// VideoRepository provides DB access for videos
type VideoRepository struct {
	db *sql.DB
}

func NewVideoRepository(db *sql.DB) *VideoRepository {
	return &VideoRepository{db: db}
}

func (r *VideoRepository) Create(ctx context.Context, v *models.Video) error {
	now := time.Now().UTC()
	v.CreatedAt = now
	v.UpdatedAt = now
	q := `INSERT INTO videos (video_id, title, description, channel_id, channel_name, duration, thumbnail_s3_key, video_s3_key, upload_date, webpage_url, extractor, video_type, filesize, metadata_json, slug, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)` 
	_, err := r.db.ExecContext(ctx, q,
		v.VideoID, v.Title, v.Description, v.ChannelID, v.ChannelName, v.Duration, v.ThumbnailS3Key, v.VideoS3Key, v.UploadDate, v.WebpageURL, v.Extractor, v.VideoType, v.Filesize, v.MetadataJSON, v.Slug, v.CreatedAt.Format(time.RFC3339), v.UpdatedAt.Format(time.RFC3339))
	return err
}

func (r *VideoRepository) GetByID(ctx context.Context, id string) (*models.Video, error) {
	q := `SELECT video_id, title, description, channel_id, channel_name, duration, thumbnail_s3_key, video_s3_key, upload_date, webpage_url, extractor, video_type, filesize, metadata_json, slug, created_at, updated_at FROM videos WHERE video_id = ? LIMIT 1` 
	row := r.db.QueryRowContext(ctx, q, id)
	var v models.Video
	var ca, ua string
	if err := row.Scan(&v.VideoID, &v.Title, &v.Description, &v.ChannelID, &v.ChannelName, &v.Duration, &v.ThumbnailS3Key, &v.VideoS3Key, &v.UploadDate, &v.WebpageURL, &v.Extractor, &v.VideoType, &v.Filesize, &v.MetadataJSON, &v.Slug, &ca, &ua); err != nil {
		if errors.Is(err, sql.ErrNoRows) { return nil, nil }
		return nil, err
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339, ca)
	v.UpdatedAt, _ = time.Parse(time.RFC3339, ua)
	return &v, nil
}

func (r *VideoRepository) UpdateMetadata(ctx context.Context, id string, metadata map[string]interface{}) error {
	b, err := json.Marshal(metadata)
	if err != nil { return err }
	q := `UPDATE videos SET metadata_json = ?, updated_at = ? WHERE video_id = ?` 
	_, err = r.db.ExecContext(ctx, q, string(b), time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (r *VideoRepository) Search(ctx context.Context, query string, limit int) ([]*models.Video, error) {
	q := `SELECT video_id, title, description, channel_id, channel_name, duration, thumbnail_s3_key, video_s3_key, upload_date, webpage_url, extractor, video_type, filesize, metadata_json, slug, created_at, updated_at FROM videos WHERE rowid IN (SELECT rowid FROM videos_fts WHERE videos_fts MATCH ? ) LIMIT ?` 
	rows, err := r.db.QueryContext(ctx, q, query, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []*models.Video{}
	for rows.Next() {
		var v models.Video
		var ca, ua string
		if err := rows.Scan(&v.VideoID, &v.Title, &v.Description, &v.ChannelID, &v.ChannelName, &v.Duration, &v.ThumbnailS3Key, &v.VideoS3Key, &v.UploadDate, &v.WebpageURL, &v.Extractor, &v.VideoType, &v.Filesize, &v.MetadataJSON, &v.Slug, &ca, &ua); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339, ca)
		v.UpdatedAt, _ = time.Parse(time.RFC3339, ua)
		out = append(out, &v)
	}
	return out, nil
}
