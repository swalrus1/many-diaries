package instagram

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/swalrus1/many-diaries/internal/index"
	"github.com/swalrus1/many-diaries/internal/storage"
)

const Name = "instagram"

//go:embed instagram.html
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
		tmpl: template.Must(template.New("instagram").Parse(pageHTML)),
	}
}

func (m *Medium) Name() string { return Name }

func (m *Medium) ConfigPanel(_ *http.Request, message string) (template.HTML, error) {
	var body strings.Builder
	if err := m.tmpl.Execute(&body, struct{ Message string }{message}); err != nil {
		return "", err
	}
	return template.HTML(body.String()), nil
}

func (m *Medium) HandleAction(r *http.Request) (string, error) {
	if r.FormValue("action") != "sync" {
		return "", errors.New("unknown action")
	}
	sessionID := strings.TrimSpace(r.FormValue("sessionid"))
	if sessionID == "" {
		return "", errors.New("session key is required")
	}
	synced, skipped, err := m.Sync(r.Context(), sessionID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Synced %d records (%d already persisted, skipped).", synced, skipped), nil
}

// Sync fetches posts, reels, and stories (live + archive) and persists new ones.
func (m *Medium) Sync(ctx context.Context, sessionID string) (synced, skipped int, err error) {
	c := NewClient(sessionID)
	var items []Item
	for _, fetch := range []func(context.Context) ([]Item, error){
		c.FetchFeed, c.FetchLiveStories, c.FetchArchivedStories,
	} {
		got, err := fetch(ctx)
		if err != nil {
			return synced, skipped, err
		}
		items = append(items, got...)
	}
	for _, it := range items {
		recID := Name + "/" + recordKey(it)
		exists, err := m.idx.HasRecord(ctx, recID)
		if err != nil {
			return synced, skipped, err
		}
		if exists {
			skipped++
			continue
		}
		if err := m.persist(ctx, c, it, recID); err != nil {
			return synced, skipped, err
		}
		synced++
	}
	return synced, skipped, m.idx.Flush(ctx)
}

func recordKey(it Item) string {
	if it.ID != "" {
		return it.ID
	}
	return strconv.FormatInt(it.PK, 10)
}

func (m *Medium) persist(ctx context.Context, c *Client, it Item, recID string) error {
	for i, u := range it.MediaURLs() {
		body, err := c.Download(ctx, u)
		if err != nil {
			log.Printf("instagram: download %s: %v", recID, err)
			continue
		}
		putErr := m.st.Put(ctx, fmt.Sprintf("%s/%d%s", recID, i, mediaExt(u)), body)
		body.Close()
		if putErr != nil {
			return putErr
		}
	}
	if it.Caption != nil && it.Caption.Text != "" {
		if err := m.st.Put(ctx, recID+"/caption.txt", strings.NewReader(it.Caption.Text)); err != nil {
			return err
		}
	}
	attrs := map[string]string{
		"instagram_id": recordKey(it),
		"storage_key":  recID,
	}
	if it.Code != "" {
		kind := "p"
		if it.ProductType == "clips" {
			kind = "reel"
		}
		attrs["permalink"] = "https://www.instagram.com/" + kind + "/" + it.Code + "/"
	}
	if it.ProductType != "" {
		attrs["product_type"] = it.ProductType
	}
	return m.idx.UpsertRecord(ctx, index.Record{
		ID:        recID,
		Medium:    Name,
		CreatedAt: time.Unix(it.TakenAt, 0),
		Attrs:     attrs,
	})
}

func mediaExt(rawURL string) string {
	ext := path.Ext(strings.SplitN(rawURL, "?", 2)[0])
	if ext == "" {
		return ".bin"
	}
	return ext
}
