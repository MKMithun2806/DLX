package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config holds the connection parameters for an S3-compatible endpoint.
// Endpoint may be left empty to use real AWS S3, or set to a MinIO/Wasabi/
// B2 endpoint URL for self-hosted/alternative providers.
type S3Config struct {
	Endpoint     string
	Region       string
	Bucket       string
	AccessKey    string
	SecretKey    string
	Prefix       string
	UsePathStyle bool
}

// S3Backend uploads files to any S3-compatible object store.
type S3Backend struct {
	cfg      S3Config
	client   *s3.Client
	uploader *manager.Uploader
}

func NewS3(ctx context.Context, cfg S3Config) (*S3Backend, error) {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("s3 config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	})

	return &S3Backend{
		cfg:      cfg,
		client:   client,
		uploader: manager.NewUploader(client),
	}, nil
}

func (s *S3Backend) Name() string { return "s3" }

// Store uploads localSourcePath to the configured bucket under
// <prefix>/<key>, removing the local temp file once the upload succeeds.
func (s *S3Backend) Store(ctx context.Context, localSourcePath, key string) (string, error) {
	objectKey := key
	if s.cfg.Prefix != "" {
		objectKey = fmt.Sprintf("%s/%s", trimSlash(s.cfg.Prefix), key)
	}
	return s.uploadObject(ctx, localSourcePath, objectKey)
}

func (s *S3Backend) uploadObject(ctx context.Context, localSourcePath, objectKey string) (string, error) {
	f, err := os.Open(localSourcePath)
	if err != nil {
		return "", fmt.Errorf("s3 store: open: %w", err)
	}
	defer f.Close()

	_, err = s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(objectKey),
		Body:   f,
	})
	if err != nil {
		return "", fmt.Errorf("s3 store: upload: %w", err)
	}
	_ = os.Remove(localSourcePath)
	return objectKey, nil
}

// StorePackage uploads the package files under a single backend-relative
// folder. The returned keys mirror the object layout, e.g.
// videos/<id>/video.mp4.
func (s *S3Backend) StorePackage(ctx context.Context, pkg Package) (PackageResult, error) {
	root := s.packageRoot(pkg.ID)
	res := PackageResult{PackageRoot: root}
	for _, file := range pkg.Files {
		if file.SourcePath == "" {
			continue
		}
		key := path.Join(root, file.Name)
		stored, err := s.uploadObject(ctx, file.SourcePath, key)
		if err != nil {
			return PackageResult{}, err
		}
		switch file.Name {
		case "video.mp4", "video.webm", "video.mkv", "video.mov", "video.avi":
			res.VideoKey = stored
		case "metadata.json":
			res.MetadataKey = stored
		default:
			if res.ThumbnailKey == "" {
				res.ThumbnailKey = stored
			}
		}
	}
	return res, nil
}

// ReadFile downloads a backend object into memory. It is used during
// recovery to reconstruct the SQLite database from storage.
func (s *S3Backend) ReadFile(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

// ListPackageRoots enumerates package roots by looking for metadata.json
// objects and trimming the filename.
func (s *S3Backend) ListPackageRoots(ctx context.Context) ([]string, error) {
	prefix := s.packagePrefix()
	if prefix != "" {
		prefix += "/"
	}
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.cfg.Bucket),
		Prefix: aws.String(prefix),
	})
	roots := make([]string, 0)
	seen := map[string]struct{}{}
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if !strings.HasSuffix(key, "/metadata.json") && key != "metadata.json" {
				continue
			}
			root := strings.TrimSuffix(key, "metadata.json")
			root = strings.TrimSuffix(root, "/")
			if root == "" {
				continue
			}
			root = strings.TrimSuffix(root, "/")
			if _, ok := seen[root]; ok {
				continue
			}
			seen[root] = struct{}{}
			roots = append(roots, root)
		}
	}
	return roots, nil
}

// ListPackageFiles returns the filenames directly beneath a package root.
func (s *S3Backend) ListPackageFiles(ctx context.Context, root string) ([]string, error) {
	prefix := strings.TrimSuffix(root, "/") + "/"
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.cfg.Bucket),
		Prefix: aws.String(prefix),
	})
	files := make([]string, 0)
	seen := map[string]struct{}{}
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			rest := strings.TrimPrefix(key, prefix)
			if rest == "" || strings.Contains(rest, "/") {
				continue
			}
			if _, ok := seen[rest]; ok {
				continue
			}
			seen[rest] = struct{}{}
			files = append(files, rest)
		}
	}
	sort.Strings(files)
	return files, nil
}

func (s *S3Backend) packagePrefix() string {
	if s.cfg.Prefix != "" {
		return trimSlash(s.cfg.Prefix)
	}
	return "videos"
}

func (s *S3Backend) packageRoot(id string) string {
	return path.Join(s.packagePrefix(), id)
}

func (s *S3Backend) withPrefix(key string) string {
	objectKey := key
	if s.cfg.Prefix != "" {
		objectKey = path.Join(trimSlash(s.cfg.Prefix), key)
	}
	return objectKey
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
