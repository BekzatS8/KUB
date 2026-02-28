package storage

import (
	"context"
	"io"
)

type Storage interface {
	Save(ctx context.Context, reader io.Reader, key string) error
	Open(ctx context.Context, key string) (io.ReadSeekCloser, int64, error)
	Delete(ctx context.Context, key string) error
}
