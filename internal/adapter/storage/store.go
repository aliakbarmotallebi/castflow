package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"castflow/internal/config"
	"castflow/internal/domain"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// NewFromConfig creates the configured object storage backend.
func NewFromConfig(cfg config.StorageConfig) (domain.ObjectStorage, error) {
	switch cfg.Driver {
	case "s3":
		return NewS3Store(cfg)
	case "local", "":
		return NewLocalStore(cfg.LocalDir, cfg.PublicURL)
	default:
		return nil, fmt.Errorf("unsupported storage driver: %s", cfg.Driver)
	}
}

// LocalStore stores files on disk (dev / single-node deploy).
type LocalStore struct {
	baseDir   string
	publicURL string
}

func NewLocalStore(baseDir, publicURL string) (*LocalStore, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	return &LocalStore{baseDir: baseDir, publicURL: strings.TrimRight(publicURL, "/")}, nil
}

func (s *LocalStore) EnsureBucket(_ context.Context) error { return nil }

func (s *LocalStore) Upload(_ context.Context, key string, body io.Reader, _ int64, _ string) error {
	path := s.fullPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, body)
	return err
}

func (s *LocalStore) Download(_ context.Context, key string) (io.ReadCloser, error) {
	return os.Open(s.fullPath(key))
}

func (s *LocalStore) DeletePrefix(_ context.Context, prefix string) error {
	return os.RemoveAll(s.fullPath(prefix))
}

func (s *LocalStore) PublicURL(key string) string {
	return s.publicURL + "/" + strings.TrimPrefix(key, "/")
}

func (s *LocalStore) fullPath(key string) string {
	return filepath.Join(s.baseDir, filepath.FromSlash(key))
}

// S3Store stores files on S3-compatible storage (RustFS, AWS S3, …).
type S3Store struct {
	client    *minio.Client
	bucket    string
	publicURL string
}

func NewS3Store(cfg config.StorageConfig) (*S3Store, error) {
	endpoint := strings.TrimPrefix(strings.TrimPrefix(cfg.Endpoint, "https://"), "http://")
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 client: %w", err)
	}
	return &S3Store{client: client, bucket: cfg.Bucket, publicURL: strings.TrimRight(cfg.PublicURL, "/")}, nil
}

func (s *S3Store) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
	}
	return nil
}

func (s *S3Store) Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, body, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *S3Store) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (s *S3Store) DeletePrefix(ctx context.Context, prefix string) error {
	ch := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true})
	for obj := range ch {
		if obj.Err != nil {
			return obj.Err
		}
		if err := s.client.RemoveObject(ctx, s.bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func (s *S3Store) PublicURL(key string) string {
	return s.publicURL + "/" + strings.TrimPrefix(key, "/")
}
