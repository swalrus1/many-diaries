package obsidian

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/swalrus1/many-diaries/internal/index"
	"github.com/swalrus1/many-diaries/internal/storage"
)

const Name = "obsidian"

//go:embed obsidian.html
var pageHTML string

type Medium struct {
	st   storage.Storage
	idx  index.MetadataIndex
	tmpl *template.Template
}

func New(st storage.Storage, idx index.MetadataIndex) *Medium {
	return &Medium{
		st:   st,
		idx:  idx,
		tmpl: template.Must(template.New("obsidian").Parse(pageHTML)),
	}
}

func (m *Medium) Name() string { return Name }

func (m *Medium) ConfigPanel(r *http.Request, message string) (template.HTML, error) {
	vaultPath, err := m.idx.GetSetting(r.Context(), Name, "vault_path")
	if err != nil {
		return "", err
	}
	var body strings.Builder
	if err := m.tmpl.Execute(&body, struct{ VaultPath, Message string }{vaultPath, message}); err != nil {
		return "", err
	}
	return template.HTML(body.String()), nil
}

func (m *Medium) HandleAction(r *http.Request) (string, error) {
	switch r.FormValue("action") {
	case "save":
		vaultPath := strings.TrimSpace(r.FormValue("vault_path"))
		if err := m.idx.SetSetting(r.Context(), Name, "vault_path", vaultPath); err != nil {
			return "", err
		}
		return "Vault path saved.", nil
	case "sync":
		n, err := m.Sync(r.Context())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Synced %d records.", n), nil
	default:
		return "", errors.New("unknown action")
	}
}

func (m *Medium) Sync(ctx context.Context) (int, error) {
	root, err := m.idx.GetSetting(ctx, Name, "vault_path")
	if err != nil {
		return 0, err
	}
	if root == "" {
		return 0, errors.New("obsidian: vault path not configured")
	}
	n := 0
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() && p != root {
				return filepath.SkipDir
			}
			if !d.IsDir() {
				return nil
			}
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		key := Name + "/" + filepath.ToSlash(rel)
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		putErr := m.st.Put(ctx, key, f)
		f.Close()
		if putErr != nil {
			return putErr
		}
		if err := m.idx.UpsertRecord(ctx, index.Record{ID: key, Medium: Name, CreatedAt: info.ModTime()}); err != nil {
			return err
		}
		n++
		return nil
	})
	if err != nil {
		return n, err
	}
	return n, m.idx.Flush(ctx)
}
