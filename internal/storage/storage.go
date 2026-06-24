package storage

import "context"

// Storage is an abstraction over underlying storage backends
type Storage interface {
	// StoreVideo stores video bytes; key should be returned for retrieval
	StoreVideo(ctx context.Context, videoID string, data []byte) (key string, err error)

	// StoreThumbnail stores thumbnail bytes; returns key
	StoreThumbnail(ctx context.Context, videoID string, data []byte) (key string, err error)

	// StoreMetadata stores metadata JSON optionally; returns key
	StoreMetadata(ctx context.Context, videoID string, data []byte) (key string, err error)

	// GetURL returns an opaque URL for a stored key (could be filesystem path or S3 URL)
	GetURL(ctx context.Context, key string) (string, error)

	// Delete removes an object
	Delete(ctx context.Context, key string) error
}
