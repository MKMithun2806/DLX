package storage

import "context"

// Backend is the common interface implemented by both local disk and
// S3-compatible storage targets.
type Backend interface {
	// Store moves/uploads the file at localSourcePath into the backend
	// under the given key (a relative path / object key) and returns a
	// backend-specific reference to the stored object.
	Store(ctx context.Context, localSourcePath, key string) (string, error)
	// Name identifies the backend, e.g. "local" or "s3".
	Name() string
}

// UsageStats reports free/used space, used by the local backend for the
// settings UI.
type UsageStats struct {
	TotalBytes uint64
	UsedBytes  uint64
	FreeBytes  uint64
}
