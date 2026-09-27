package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/swalrus1/many-diaries/internal/storage"
)

const blobKey = "index.db"

type Record struct {
	ID        string
	Medium    string
	CreatedAt time.Time
}

type MetadataIndex interface {
	UpsertRecord(ctx context.Context, r Record) error
	ListRecords(ctx context.Context, medium string) ([]Record, error)
	SetSetting(ctx context.Context, medium, key, value string) error
	GetSetting(ctx context.Context, medium, key string) (string, error)
	Flush(ctx context.Context) error
	Close(ctx context.Context) error
}

type SQLite struct {
	db  *sql.DB
	st  storage.Storage
	dir string
}

func OpenSQLite(ctx context.Context, st storage.Storage) (*SQLite, error) {
	dir, err := os.MkdirTemp("", "many-diaries-index-")
	if err != nil {
		return nil, fmt.Errorf("index: %w", err)
	}
	local := filepath.Join(dir, blobKey)
	rc, err := st.Get(ctx, blobKey)
	switch {
	case err == nil:
		f, err := os.Create(local)
		if err != nil {
			rc.Close()
			return nil, fmt.Errorf("index: %w", err)
		}
		_, copyErr := io.Copy(f, rc)
		f.Close()
		rc.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("index: %w", copyErr)
		}
	case errors.Is(err, storage.ErrNotExist):
	default:
		return nil, fmt.Errorf("index: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+local+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("index: %w", err)
	}
	s := &SQLite{db: db, st: st, dir: dir}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLite) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS records (
			id TEXT PRIMARY KEY,
			medium TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS settings (
			medium TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			PRIMARY KEY (medium, key)
		);
	`)
	return err
}

func (s *SQLite) UpsertRecord(_ context.Context, r Record) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO records (id, medium, created_at) VALUES (?, ?, ?)`,
		r.ID, r.Medium, r.CreatedAt.Unix(),
	)
	return err
}

func (s *SQLite) ListRecords(_ context.Context, medium string) ([]Record, error) {
	q := `SELECT id, medium, created_at FROM records`
	var args []any
	if medium != "" {
		q += ` WHERE medium = ?`
		args = append(args, medium)
	}
	q += ` ORDER BY created_at`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		var ts int64
		if err := rows.Scan(&r.ID, &r.Medium, &ts); err != nil {
			return nil, err
		}
		r.CreatedAt = time.Unix(ts, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLite) SetSetting(_ context.Context, medium, key, value string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO settings (medium, key, value) VALUES (?, ?, ?)`,
		medium, key, value,
	)
	return err
}

func (s *SQLite) GetSetting(_ context.Context, medium, key string) (string, error) {
	var value string
	err := s.db.QueryRow(
		`SELECT value FROM settings WHERE medium = ? AND key = ?`, medium, key,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *SQLite) Flush(ctx context.Context) error {
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("index: %w", err)
	}
	f, err := os.Open(filepath.Join(s.dir, blobKey))
	if err != nil {
		return fmt.Errorf("index: %w", err)
	}
	defer f.Close()
	tmp := blobKey + ".uploading"
	if err := s.st.Put(ctx, tmp, f); err != nil {
		return fmt.Errorf("index: %w", err)
	}
	if err := s.st.Copy(ctx, tmp, blobKey); err != nil {
		return fmt.Errorf("index: %w", err)
	}
	return s.st.Delete(ctx, tmp)
}

func (s *SQLite) Close(ctx context.Context) error {
	flushErr := s.Flush(ctx)
	dbErr := s.db.Close()
	os.RemoveAll(s.dir)
	return errors.Join(flushErr, dbErr)
}
