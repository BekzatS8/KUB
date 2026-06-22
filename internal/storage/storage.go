package storage

import (
	"context"
	"fmt"
	"io"
)

type Storage interface {
	Save(ctx context.Context, reader io.Reader, key string) error
	Open(ctx context.Context, key string) (io.ReadSeekCloser, int64, error)
	Delete(ctx context.Context, key string) error
}

// S3Config holds settings for S3-compatible object storage.
type S3Config struct {
	Enabled   bool   `yaml:"enabled"`
	Endpoint  string `yaml:"endpoint"`
	Region    string `yaml:"region"`
	Bucket    string `yaml:"bucket"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	UseSSL    bool   `yaml:"use_ssl"`
}

// New returns S3Storage when S3 is enabled, otherwise falls back to LocalStorage.
func New(localRoot string, s3 S3Config) (Storage, error) {
	if s3.Enabled {
		if s3.Endpoint == "" || s3.Bucket == "" || s3.AccessKey == "" || s3.SecretKey == "" {
			return nil, fmt.Errorf("storage: S3 enabled but endpoint/bucket/access_key/secret_key are not set")
		}
		return NewS3Storage(s3.Endpoint, s3.Region, s3.Bucket, s3.AccessKey, s3.SecretKey, s3.UseSSL)
	}
	return NewLocalStorage(localRoot), nil
}
