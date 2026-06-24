package models

import "time"

// Video represents a canonical video entity stored for the viewer/backend
type Video struct {
	VideoID         string    `json:"video_id" db:"video_id"`
	Title           string    `json:"title" db:"title"`
	Description     string    `json:"description" db:"description"`

	ChannelID       string    `json:"channel_id" db:"channel_id"`
	ChannelName     string    `json:"channel_name" db:"channel_name"`

	Duration        int64     `json:"duration" db:"duration"`

	ThumbnailS3Key  string    `json:"thumbnail_s3_key" db:"thumbnail_s3_key"`
	VideoS3Key      string    `json:"video_s3_key" db:"video_s3_key"`

	UploadDate      string    `json:"upload_date" db:"upload_date"`

	WebpageURL      string    `json:"webpage_url" db:"webpage_url"`
	Extractor       string    `json:"extractor" db:"extractor"`
	VideoType       string    `json:"video_type" db:"video_type"`
	Filesize        int64     `json:"filesize" db:"filesize"`
	MetadataJSON    string    `json:"metadata_json" db:"metadata_json"`
	Slug            string    `json:"slug" db:"slug"`

	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}
