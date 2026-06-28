package storage

import (
	"context"
	"io"
)

// Backend is the common interface implemented by both local disk and
// S3-compatible storage targets.
type Backend interface {
	// Store moves/uploads the file at localSourcePath into the backend
	// under the given key (a relative path / object key) and returns a
	// backend-specific reference to the stored object.
	Store(ctx context.Context, localSourcePath, key string) (string, error)
	// StorePackage uploads or moves a full download package consisting of
	// video, thumbnail, and metadata JSON files.
	StorePackage(ctx context.Context, pkg Package) (PackageResult, error)
	// ReadFile reads a backend object/file identified by key. It is used
	// by recovery logic to rebuild SQLite from storage.
	ReadFile(ctx context.Context, key string) ([]byte, error)
	// ListPackageRoots returns the package directories/roots known to the
	// backend. Each entry is a storage-relative package root such as
	// "videos/<id>" or "<id>".
	ListPackageRoots(ctx context.Context) ([]string, error)
	// ListPackageFiles returns the direct child files within a package root.
	ListPackageFiles(ctx context.Context, root string) ([]string, error)
	// Name identifies the backend, e.g. "local" or "s3".
	Name() string
}

// PackageFile describes a single file belonging to a package on disk or in
// object storage.
type PackageFile struct {
	Name       string
	SourcePath string
}

// Package represents a self-contained video package to be stored.
type Package struct {
	ID    string
	Files []PackageFile
}

// PackageResult reports the backend-specific object keys/paths that were
// assigned to a stored package.
type PackageResult struct {
	PackageRoot  string
	VideoKey     string
	ThumbnailKey string
	MetadataKey  string
}

// UsageStats reports free/used space, used by the local backend for the
// settings UI.
type UsageStats struct {
	TotalBytes uint64
	UsedBytes  uint64
	FreeBytes  uint64
}

// readAll is shared by storage backends.
func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
