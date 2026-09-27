package instagram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const feedV1Fixture = `{
  "items": [
    {
      "pk": 3001,
      "id": "3001_42",
      "code": "ABC123",
      "taken_at": 1715000000,
      "media_type": 1,
      "product_type": "feed",
      "caption": {"text": "hello world"},
      "image_versions2": {"candidates": [
        {"url": "https://cdn.example/hi.jpg?sig=1", "width": 1080, "height": 1080},
        {"url": "https://cdn.example/lo.jpg?sig=1", "width": 320, "height": 320}
      ]}
    },
    {
      "pk": 3002,
      "id": "3002_42",
      "code": "DEF456",
      "taken_at": 1715100000,
      "media_type": 8,
      "product_type": "feed",
      "caption": {"text": "carousel"},
      "carousel_media": [
        {"image_versions2": {"candidates": [{"url": "https://cdn.example/c1.jpg", "width": 1, "height": 1}]}},
        {"video_versions": [{"url": "https://cdn.example/c2.mp4", "width": 1, "height": 1}]}
      ]
    },
    {
      "pk": 3003,
      "id": "3003_42",
      "code": "REEL1",
      "taken_at": 1715200000,
      "media_type": 2,
      "product_type": "clips",
      "caption": null,
      "video_versions": [{"url": "https://cdn.example/r.mp4", "width": 720, "height": 1280}]
    }
  ],
  "more_available": false,
  "status": "ok"
}`

// NDJSON: first line is the timeline shell, then one deferred chunk per media.
const feedGraphQLFixture = `{"data":{"xdt_api__v1__profile_timeline":{"more_available":false,"profile_grid_items":[{"media":{"id":"3001_42","taken_at":1715000000}},{"media":{"id":"3002_42","taken_at":1715100000}},{"media":{"id":"3003_42","taken_at":1715200000}}]}},"status":"ok"}
{"label":"ProfileTimelineMediaItemDeferredFields","data":{"id":"3001_42","code":"ABC123","media_type":1,"product_type":"feed","caption":{"text":"hello world"},"image_versions2":{"candidates":[{"url":"https://cdn.example/hi.jpg?sig=1","width":1080,"height":1080},{"url":"https://cdn.example/lo.jpg?sig=1","width":320,"height":320}]}}}
{"label":"ProfileTimelineMediaItemDeferredFields","data":{"id":"3002_42","code":"DEF456","media_type":8,"product_type":"feed","caption":{"text":"carousel"},"carousel_media":[{"image_versions2":{"candidates":[{"url":"https://cdn.example/c1.jpg","width":1,"height":1}]}},{"video_versions":[{"url":"https://cdn.example/c2.mp4","width":1,"height":1}]}]}}
{"label":"ProfileTimelineMediaItemDeferredFields","data":{"id":"3003_42","code":"REEL1","media_type":2,"product_type":"clips","caption":null,"video_versions":[{"url":"https://cdn.example/r.mp4","width":720,"height":1280}]}}
`

func TestUserID(t *testing.T) {
	for sid, want := range map[string]string{
		"42%3Aabc%3Axyz": "42",
		"42:abc":         "42",
	} {
		c := NewClient(sid)
		got, err := c.userID()
		if err != nil || got != want {
			t.Errorf("userID(%q) = %q, %v; want %q", sid, got, err, want)
		}
	}
	if _, err := NewClient("no-colon").userID(); err == nil {
		t.Error("userID(no-colon) expected error")
	}
}

func checkItems(t *testing.T, items []Item) {
	t.Helper()
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	photo := items[0]
	if photo.Caption == nil || photo.Caption.Text != "hello world" {
		t.Errorf("caption = %+v", photo.Caption)
	}
	if photo.TakenAt != 1715000000 {
		t.Errorf("taken_at = %d", photo.TakenAt)
	}
	if got := photo.MediaURLs(); len(got) != 1 || got[0] != "https://cdn.example/hi.jpg?sig=1" {
		t.Errorf("photo MediaURLs = %v, want best candidate", got)
	}
	if got := items[1].MediaURLs(); len(got) != 2 {
		t.Errorf("carousel MediaURLs = %v, want 2", got)
	}
	reel := items[2]
	if reel.Caption != nil {
		t.Errorf("reel caption should be nil")
	}
	if got := reel.MediaURLs(); len(got) != 1 || got[0] != "https://cdn.example/r.mp4" {
		t.Errorf("reel MediaURLs = %v", got)
	}
}

func checkAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Cookie") != "sessionid=42:abc" {
		t.Errorf("missing sessionid cookie, got %q", r.Header.Get("Cookie"))
	}
	if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "Bearer IGT:2:") {
		t.Errorf("missing IGT authorization header, got %q", auth)
	}
}

// Primary path: v1 REST feed answers.
func TestFetchFeedV1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		if !strings.HasPrefix(r.URL.Path, "/api/v1/feed/user/42/") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(feedV1Fixture))
	}))
	defer srv.Close()

	c := NewClient("42:abc")
	c.base = srv.URL
	items, err := c.FetchFeed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	checkItems(t, items)
}

// Fallback path: v1 feed is rate-limited, GraphQL timeline answers.
func TestFetchFeedGraphQLFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		if strings.HasPrefix(r.URL.Path, "/api/v1/feed/user/") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"message":"feedback_required","feedback_required":true}`))
			return
		}
		if r.URL.Path != "/graphql/query" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.PostForm.Get("client_doc_id"); got != profileTimelineDocID {
			t.Errorf("client_doc_id = %q", got)
		}
		if !strings.Contains(r.PostForm.Get("variables"), `"user_id":"42"`) {
			t.Errorf("variables missing user_id: %s", r.PostForm.Get("variables"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(feedGraphQLFixture))
	}))
	defer srv.Close()

	c := NewClient("42:abc")
	c.base = srv.URL
	items, err := c.FetchFeed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	checkItems(t, items)
}
