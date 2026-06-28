package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

// LocalBackend stores downloaded files on the local filesystem under Root.
type LocalBackend struct {
	Root string
}

func NewLocal(root string) *LocalBackend {
	return &LocalBackend{Root: root}
}

func (l *LocalBackend) Name() string { return "local" }

// Store moves the source file into Root/key, creating any necessary
// subdirectories. It falls back to a copy if the rename fails (e.g. across
// filesystem boundaries, such as a tmp dir on a different mount).
func (l *LocalBackend) Store(ctx context.Context, localSourcePath, key string) (string, error) {
	dest := filepath.Join(l.Root, key)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("local store: mkdir: %w", err)
	}

	if err := os.Rename(localSourcePath, dest); err == nil {
		return dest, nil
	}

	// Cross-device fallback: copy then remove source.
	in, err := os.Open(localSourcePath)
	if err != nil {
		return "", fmt.Errorf("local store: open source: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("local store: create dest: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return "", fmt.Errorf("local store: copy: %w", err)
	}
	_ = os.Remove(localSourcePath)
	return dest, nil
}

// StorePackage moves or copies the files in pkg into a package directory
// rooted at l.Root/pkg.ID. The backend keeps the package layout identical
// to the object store layout, only changing the root location.
func (l *LocalBackend) StorePackage(ctx context.Context, pkg Package) (PackageResult, error) {
	packageRoot := filepath.Join(l.Root, pkg.ID)
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		return PackageResult{}, fmt.Errorf("local package store: mkdir: %w", err)
	}

	res := PackageResult{PackageRoot: filepath.ToSlash(pkg.ID)}
	for _, file := range pkg.Files {
		if file.SourcePath == "" {
			return PackageResult{}, fmt.Errorf("local package store: missing source for %s", file.Name)
		}
		dest := filepath.Join(packageRoot, file.Name)
		if err := moveFile(file.SourcePath, dest); err != nil {
			return PackageResult{}, err
		}
		rel := filepath.ToSlash(filepath.Join(pkg.ID, file.Name))
		switch file.Name {
		case "video.mp4", "video.webm", "video.mkv", "video.mov", "video.avi":
			res.VideoKey = rel
		case "metadata.json":
			res.MetadataKey = rel
		default:
			if res.ThumbnailKey == "" {
				res.ThumbnailKey = rel
			}
		}
	}
	return res, nil
}

func moveFile(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("local package store: open source: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("local package store: create dest: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("local package store: copy: %w", err)
	}
	_ = os.Remove(src)
	return nil
}

// ReadFile returns the contents of the file at the given storage-relative
// key.
func (l *LocalBackend) ReadFile(ctx context.Context, key string) ([]byte, error) {
	path := filepath.Join(l.Root, filepath.FromSlash(key))
	return os.ReadFile(path)
}

// ListPackageRoots scans the root directory for package folders containing
// metadata.json.
func (l *LocalBackend) ListPackageRoots(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(l.Root)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rel := entry.Name()
		if _, err := os.Stat(filepath.Join(l.Root, rel, "metadata.json")); err == nil {
			roots = append(roots, filepath.ToSlash(rel))
		}
	}
	sort.Strings(roots)
	return roots, nil
}

// Usage reports disk usage statistics for the backend's root path.
func (l *LocalBackend) Usage() (UsageStats, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(l.Root, &stat); err != nil {
		return UsageStats{}, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free
	return UsageStats{TotalBytes: total, UsedBytes: used, FreeBytes: free}, nil
}
