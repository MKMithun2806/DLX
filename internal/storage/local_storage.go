package storage

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// LocalStorage stores files under a configured base directory
type LocalStorage struct{
	BasePath string
}

func NewLocalStorage(base string) *LocalStorage {
	return &LocalStorage{BasePath: base}
}

func (l *LocalStorage) ensureDir() error {
	return os.MkdirAll(l.BasePath, 0755)
}

func (l *LocalStorage) StoreVideo(ctx context.Context, videoID string, data []byte) (string, error) {
	if err := l.ensureDir(); err != nil { return "", err }
	key := filepath.Join("videos", videoID)
	full := filepath.Join(l.BasePath, key)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil { return "", err }
	if err := ioutil.WriteFile(full, data, 0644); err != nil { return "", err }
	return key, nil
}

func (l *LocalStorage) StoreThumbnail(ctx context.Context, videoID string, data []byte) (string, error) {
	if err := l.ensureDir(); err != nil { return "", err }
	key := filepath.Join("thumbnails", videoID+".jpg")
	full := filepath.Join(l.BasePath, key)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil { return "", err }
	if err := ioutil.WriteFile(full, data, 0644); err != nil { return "", err }
	return key, nil
}

func (l *LocalStorage) StoreMetadata(ctx context.Context, videoID string, data []byte) (string, error) {
	if err := l.ensureDir(); err != nil { return "", err }
	key := filepath.Join("metadata", videoID+".json")
	full := filepath.Join(l.BasePath, key)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil { return "", err }
	if err := ioutil.WriteFile(full, data, 0644); err != nil { return "", err }
	return key, nil
}

func (l *LocalStorage) GetURL(ctx context.Context, key string) (string, error) {
	return fmt.Sprintf("file://%s", filepath.Join(l.BasePath, key)), nil
}

func (l *LocalStorage) Delete(ctx context.Context, key string) error {
	return os.Remove(filepath.Join(l.BasePath, key))
}
