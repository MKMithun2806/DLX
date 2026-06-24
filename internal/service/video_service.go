package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/MKMithun2806/DLX/internal/models"
	"github.com/MKMithun2806/DLX/internal/repo"
	"github.com/MKMithun2806/DLX/internal/storage"
)

// VideoService implements business logic for videos
type VideoService struct {
	repo *repo.VideoRepository
	store storage.Storage
	metadataEnabled bool
}

func NewVideoService(r *repo.VideoRepository, s storage.Storage, metadataEnabled bool) *VideoService {
	return &VideoService{repo: r, store: s, metadataEnabled: metadataEnabled}
}

// IngestFromYtDlpJSON accepts the full yt-dlp JSON payload and creates/updates video, channel, playlist records.
func (s *VideoService) IngestFromYtDlpJSON(ctx context.Context, payload map[string]interface{}) (*models.Video, error) {
	// basic required fields
	webpage, _ := payload["webpage_url"].(string)
	id := ""
	if v, ok := payload["id"].(string); ok && v != "" {
		id = v
	} else {
		id = uuid.New().String()
	}
	videoID := id

	title, _ := payload["title"].(string)
	description, _ := payload["description"].(string)
	uploader, _ := payload["uploader"].(string)
	uploaderID, _ := payload["uploader_id"].(string)
	duration := int64(0)
	if d, ok := payload["duration"].(float64); ok { duration = int64(d) }
	filesize := int64(0)
	if f, ok := payload["filesize"].(float64); ok { filesize = int64(f) }
	extractor, _ := payload["extractor"].(string)

	metadataB, _ := json.Marshal(payload)

	// classification
	videoType := classifyVideo(payload)

	v := &models.Video{
		VideoID: videoID,
		Title: title,
		Description: description,
		ChannelID: uploaderID,
		ChannelName: uploader,
		Duration: duration,
		Filesize: filesize,
		WebpageURL: webpage,
		Extractor: extractor,
		VideoType: videoType,
		MetadataJSON: string(metadataB),
	}

	// store metadata optionally
	if s.metadataEnabled {
		_, _ = s.store.StoreMetadata(ctx, videoID, metadataB)
	}

	// thumbnails: attempt to download thumbnail URL(s)
	if thumbRaw, ok := payload["thumbnails"]; ok {
		// thumbnails is often an array of maps; pick highest resolution by preference
		if arr, ok := thumbRaw.([]interface{}); ok && len(arr) > 0 {
			// find thumbnail with largest width/height or last item
			var chosen map[string]interface{}
			for _, it := range arr {
				if m, ok := it.(map[string]interface{}); ok {
					chosen = m
				}
			}
			if chosen != nil {
				if url, ok := chosen["url"].(string); ok && strings.HasPrefix(url, "http") {
					if data, err := downloadBytes(ctx, url); err == nil {
						if key, err := s.store.StoreThumbnail(ctx, videoID, data); err == nil {
							v.ThumbnailS3Key = key
						}
					}
				}
			}
		}
	} else if thumb, ok := payload["thumbnail"].(string); ok && thumb != "" {
		if data, err := downloadBytes(ctx, thumb); err == nil {
			if key, err := s.store.StoreThumbnail(ctx, videoID, data); err == nil {
				v.ThumbnailS3Key = key
			}
		}
	}

	// persist video
	if err := s.repo.Create(ctx, v); err != nil {
		return nil, err
	}

	return v, nil
}

func downloadBytes(ctx context.Context, url string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return nil, errors.New("thumbnail download failed") }
	b, err := io.ReadAll(resp.Body)
	if err != nil { return nil, err }
	return b, nil
}

// classifyVideo applies simple rules to payload to determine video_type
func classifyVideo(payload map[string]interface{}) string {
	// priority: is_live, is_live or live_status -> livestream
	if live, ok := payload["is_live"].(bool); ok && live { return "livestream" }
	if lt, ok := payload["live_status"].(string); ok && lt != "" { return "livestream" }

	// duration short threshold (<= 60s)
	if d, ok := payload["duration"].(float64); ok {
		if int64(d) <= 60 { return "short" }
		return "video"
	}

	return "unknown"
}
