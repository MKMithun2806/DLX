package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
