package models

import "time"

type Playlist struct {
	PlaylistID       string    `json:"playlist_id" db:"playlist_id"`
	Title            string    `json:"title" db:"title"`
	Description      string    `json:"description" db:"description"`
	ThumbnailS3Key   string    `json:"thumbnail_s3_key" db:"thumbnail_s3_key"`
	MetadataJSON     string    `json:"metadata_json" db:"metadata_json"`
	Slug             string    `json:"slug" db:"slug"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}
