package storage

import (
	"context"
	"errors"
	"io"
)

var ErrNotExist = errors.New("storage: object does not exist")

type Storage interface {
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Put(ctx context.Context, key string, r io.Reader) error
	List(ctx context.Context, prefix string) ([]string, error)
	Delete(ctx context.Context, key string) error
	Copy(ctx context.Context, src, dst string) error
}
