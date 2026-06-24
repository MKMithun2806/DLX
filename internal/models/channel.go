package models

import "time"

type Channel struct {
	ChannelID        string    `json:"channel_id" db:"channel_id"`
	Name             string    `json:"name" db:"name"`
	ThumbnailS3Key   string    `json:"thumbnail_s3_key" db:"thumbnail_s3_key"`
	ChannelURL       string    `json:"channel_url" db:"channel_url"`
	MetadataJSON     string    `json:"metadata_json" db:"metadata_json"`
	Slug             string    `json:"slug" db:"slug"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}
