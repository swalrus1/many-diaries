package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Disk struct {
	root string
}

func NewDisk(root string) (*Disk, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	return &Disk{root: root}, nil
}

func (d *Disk) path(key string) string {
	return filepath.Join(d.root, filepath.FromSlash(path.Clean("/"+key)))
}

func (d *Disk) Get(_ context.Context, key string) (io.ReadCloser, error) {
	f, err := os.Open(d.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotExist
	}
	return f, err
}

func (d *Disk) Put(_ context.Context, key string, r io.Reader) error {
	dst := d.path(key)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-*")
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return fmt.Errorf("storage: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	return os.Rename(tmp.Name(), dst)
}

func (d *Disk) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	err := filepath.WalkDir(d.root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() || strings.HasPrefix(e.Name(), ".tmp-") {
			return nil
		}
		rel, err := filepath.Rel(d.root, p)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	return keys, err
}

func (d *Disk) Delete(_ context.Context, key string) error {
	err := os.Remove(d.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotExist
	}
	return err
}

func (d *Disk) Copy(ctx context.Context, src, dst string) error {
	rc, err := d.Get(ctx, src)
	if err != nil {
		return err
	}
	defer rc.Close()
	return d.Put(ctx, dst, rc)
}
