package storage

import (
	"context"
	"fmt"
	"os"

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
	f, err := os.Open(localSourcePath)
	if err != nil {
		return "", fmt.Errorf("s3 store: open: %w", err)
	}
	defer f.Close()

	objectKey := key
	if s.cfg.Prefix != "" {
		objectKey = fmt.Sprintf("%s/%s", trimSlash(s.cfg.Prefix), key)
	}

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

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
