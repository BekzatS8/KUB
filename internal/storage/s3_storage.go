package storage

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Storage implements Storage using an S3-compatible backend (MinIO, AWS, etc.).
type S3Storage struct {
	client *minio.Client
	bucket string
	// prefix — код компании в общем бакете (см. company_prefix.go). Все ключи
	// этой инсталляции живут под «<prefix>/». Пусто — корень бакета.
	prefix string
}

func NewS3Storage(endpoint, region, bucket, accessKey, secretKey string, useSSL bool, prefix string) (*S3Storage, error) {
	prefix = NormalizeCompanyPrefix(prefix)
	if err := ValidateCompanyPrefix(prefix); err != nil {
		return nil, err
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, err
	}
	return &S3Storage{client: client, bucket: bucket, prefix: prefix}, nil
}

// Prefix — код компании, под которым лежат объекты этой инсталляции.
func (s *S3Storage) Prefix() string { return s.prefix }

func (s *S3Storage) key(k string) string { return prefixedKey(s.prefix, k) }

func (s *S3Storage) Save(ctx context.Context, reader io.Reader, key string) error {
	_, err := s.client.PutObject(ctx, s.bucket, s.key(key), reader, -1, minio.PutObjectOptions{})
	return err
}

func (s *S3Storage) Open(ctx context.Context, key string) (io.ReadSeekCloser, int64, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, s.key(key), minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, err
	}
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, 0, err
	}
	return obj, info.Size, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, s.key(key), minio.RemoveObjectOptions{})
}

// SaveSized загружает объект с известным размером и Content-Type.
func (s *S3Storage) SaveSized(ctx context.Context, reader io.Reader, key string, size int64, contentType string) error {
	opts := minio.PutObjectOptions{ContentType: contentType}
	_, err := s.client.PutObject(ctx, s.bucket, s.key(key), reader, size, opts)
	return err
}
