package instagram

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

const (
	defaultBase = "https://i.instagram.com"
	mobileAppID = "567067343352427"
	mobileUA    = "Instagram 448.0.0.0.20 Android (34/14; 480dpi; 1344x2992; Google/google; Pixel 8 Pro; husky; husky; en_US; 1065560286)"
	// IGProfileTimelineQuery — Instagram's own app GraphQL doc id for the profile grid.
	profileTimelineDocID = "56030350814417327502004290437"
)

type Client struct {
	sessionID string
	hc        *http.Client
	base      string
	deviceID  string
}

func NewClient(sessionID string) *Client {
	return &Client{
		sessionID: sessionID,
		hc: &http.Client{
			// Do not follow redirects: Instagram answers unauthenticated/blocked
			// API calls with a 302 to an HTML page; we want to see and report it.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		base:     defaultBase,
		deviceID: newUUID(),
	}
}

// userID derives the numeric user ID from the sessionid prefix ("<uid>:..." or URL-encoded).
func (c *Client) userID() (string, error) {
	sid, err := url.QueryUnescape(c.sessionID)
	if err != nil {
		return "", fmt.Errorf("instagram: invalid sessionid: %w", err)
	}
	uid, _, ok := strings.Cut(sid, ":")
	if !ok || uid == "" {
		return "", fmt.Errorf("instagram: cannot derive user id from sessionid")
	}
	return uid, nil
}

// authorization builds the "Bearer IGT:2:..." header instagrapi derives from a sessionid.
func (c *Client) authorization() (string, error) {
	uid, err := c.userID()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]any{
		"ds_user_id":                     uid,
		"sessionid":                      c.sessionID,
		"should_use_header_over_cookies": true,
	})
	if err != nil {
		return "", err
	}
	return "Bearer IGT:2:" + base64.StdEncoding.EncodeToString(payload), nil
}

type mediaCandidate struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type media struct {
	ImageVersions2 *struct {
		Candidates []mediaCandidate `json:"candidates"`
	} `json:"image_versions2"`
	VideoVersions []mediaCandidate `json:"video_versions"`
}

type Item struct {
	PK          int64  `json:"pk"`
	ID          string `json:"id"`
	Code        string `json:"code"`
	TakenAt     int64  `json:"taken_at"`
	MediaType   int    `json:"media_type"`
	ProductType string `json:"product_type"`
	Caption     *struct {
		Text string `json:"text"`
	} `json:"caption"`
	media
	CarouselMedia []media `json:"carousel_media"`
}

// MediaURLs returns the best-quality download URL per media piece (carousel-expanded).
func (it Item) MediaURLs() []string {
	if len(it.CarouselMedia) > 0 {
		var urls []string
		for _, m := range it.CarouselMedia {
			if u := bestURL(m); u != "" {
				urls = append(urls, u)
			}
		}
		return urls
	}
	if u := bestURL(it.media); u != "" {
		return []string{u}
	}
	return nil
}

func bestURL(m media) string {
	if len(m.VideoVersions) > 0 {
		return m.VideoVersions[0].URL
	}
	if m.ImageVersions2 != nil && len(m.ImageVersions2.Candidates) > 0 {
		return m.ImageVersions2.Candidates[0].URL
	}
	return ""
}

// do executes an authenticated request and returns the response body.
// Non-200 statuses and HTML pages are turned into actionable errors.
func (c *Client) do(req *http.Request) ([]byte, error) {
	if err := c.setHeaders(req); err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, apiError(body, resp.Status)
	}
	if !bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) {
		return nil, fmt.Errorf("instagram: unexpected non-JSON response (HTTP %s) — session may be invalid or expired", resp.Status)
	}
	return body, nil
}

// apiError extracts Instagram's own message (e.g. rate limiting) from an error body.
func apiError(body []byte, status string) error {
	var e struct {
		Message          string `json:"message"`
		FeedbackRequired bool   `json:"feedback_required"`
	}
	if json.Unmarshal(body, &e) == nil && e.Message != "" {
		if e.FeedbackRequired {
			return fmt.Errorf("instagram: rate limited by Instagram (%s) — wait a while and retry", status)
		}
		return fmt.Errorf("instagram: %s (%s)", e.Message, status)
	}
	return fmt.Errorf("instagram: unexpected response: %s", status)
}

func (c *Client) setHeaders(req *http.Request) error {
	auth, err := c.authorization()
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", "sessionid="+c.sessionID)
	req.Header.Set("Authorization", auth)
	req.Header.Set("X-IG-App-ID", mobileAppID)
	req.Header.Set("User-Agent", mobileUA)
	req.Header.Set("X-IG-Device-ID", c.deviceID)
	req.Header.Set("X-IG-App-Locale", "en_US")
	req.Header.Set("X-IG-Device-Locale", "en_US")
	req.Header.Set("X-IG-Mapped-Locale", "en_US")
	req.Header.Set("X-IG-Timezone-Offset", "0")
	req.Header.Set("X-IG-Connection-Type", "WIFI")
	req.Header.Set("X-IG-Capabilities", "3brTv10=")
	req.Header.Set("X-IG-WWW-Claim", "0")
	req.Header.Set("Accept-Language", "en-US")
	return nil
}

func (c *Client) get(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	body, err := c.do(req)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("instagram: GET %s: %w", rawURL, err)
	}
	return nil
}

// FetchFeed lists the account's posts and reels (stories excluded).
// Primary: v1 REST feed, which paginates the full history. Fallback: the app
// GraphQL profile timeline, which Instagram currently caps at ~33 recent items
// (pagination variables are ignored) but which stays up when the v1 feed is
// rate-limited ("feedback_required"). Records already persisted are skipped on
// later syncs, so a re-sync after the block lifts fills in the rest.
func (c *Client) FetchFeed(ctx context.Context) ([]Item, error) {
	items, err := c.fetchFeedV1(ctx)
	if err == nil {
		return items, nil
	}
	log.Printf("instagram: v1 feed failed (%v), falling back to GraphQL timeline (recent items only)", err)
	return c.fetchFeedGraphQL(ctx)
}

type feedResponse struct {
	Items         []Item `json:"items"`
	MoreAvailable bool   `json:"more_available"`
	NextMaxID     string `json:"next_max_id"`
}

// fetchFeedV1 paginates GET /api/v1/feed/user/{uid}/ with max_id.
func (c *Client) fetchFeedV1(ctx context.Context) ([]Item, error) {
	uid, err := c.userID()
	if err != nil {
		return nil, err
	}
	var items []Item
	maxID := ""
	seen := map[string]bool{}
	for {
		u := fmt.Sprintf("%s/api/v1/feed/user/%s/?count=33&ranked_content=true&rank_token=%s_%s",
			c.base, uid, uid, c.deviceID)
		if maxID != "" {
			u += "&max_id=" + url.QueryEscape(maxID)
		}
		var page feedResponse
		if err := c.get(ctx, u, &page); err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if !page.MoreAvailable || page.NextMaxID == "" || seen[page.NextMaxID] {
			return items, nil
		}
		seen[page.NextMaxID] = true
		maxID = page.NextMaxID
	}
}

// fetchFeedGraphQL lists recent media via the app GraphQL profile timeline.
// The response is newline-delimited JSON: a timeline shell followed by one
// deferred chunk per media item with the full media dict in "data".
// NOTE: Instagram ignores pagination variables on this query and caps the
// result (~33 items), so a single large-count page is fetched.
func (c *Client) fetchFeedGraphQL(ctx context.Context) ([]Item, error) {
	uid, err := c.userID()
	if err != nil {
		return nil, err
	}
	vars := map[string]any{
		"user_id":                  uid,
		"count":                    1000,
		"request_media_chunk":      true,
		"fetch_profile_grid_items": true,
	}
	varsJSON, err := json.Marshal(vars)
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"method":                   {"post"},
		"pretty":                   {"false"},
		"format":                   {"json"},
		"server_timestamps":        {"true"},
		"locale":                   {"en_US"},
		"fb_api_req_friendly_name": {"IGProfileTimelineQuery"},
		"fb_api_caller_class":      {"graphservice"},
		"client_doc_id":            {profileTimelineDocID},
		"variables":                {string(varsJSON)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/graphql/query", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-FB-Friendly-Name", "IGProfileTimelineQuery")
	body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	items, _, err := parseTimeline(body)
	return items, err
}

// parseTimeline decodes the NDJSON profile timeline response and returns the
// media items plus the next cursor ("" when the page is the last one).
// Deferred chunks lack taken_at, so it is filled from the shell's lightweight
// grid items (keyed by media id).
func parseTimeline(body []byte) (items []Item, next string, err error) {
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 1<<20), 32<<20)
	takenAt := map[string]int64{}
	first := true
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if first {
			first = false
			next, takenAt, err = parseTimelineShell(line)
			if err != nil {
				return nil, "", err
			}
			continue
		}
		var chunk struct {
			Data *Item `json:"data"`
		}
		if json.Unmarshal(line, &chunk) == nil && chunk.Data != nil && chunk.Data.ID != "" {
			items = append(items, *chunk.Data)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, "", err
	}
	if first {
		return nil, "", fmt.Errorf("instagram: empty timeline response")
	}
	for i := range items {
		if items[i].TakenAt == 0 {
			items[i].TakenAt = takenAt[items[i].ID]
		}
	}
	return items, next, nil
}

// parseTimelineShell finds the dict containing "profile_grid_items" in the
// first response object and extracts pagination info plus per-media taken_at.
func parseTimelineShell(line []byte) (next string, takenAt map[string]int64, err error) {
	var root any
	if err := json.Unmarshal(line, &root); err != nil {
		return "", nil, fmt.Errorf("instagram: bad timeline response: %w", err)
	}
	tl, ok := findKey(root, "profile_grid_items").(map[string]any)
	if !ok {
		return "", nil, fmt.Errorf("instagram: profile timeline not found in response")
	}
	takenAt = map[string]int64{}
	if gridItems, ok := tl["profile_grid_items"].([]any); ok {
		for _, gi := range gridItems {
			m, ok := gi.(map[string]any)["media"].(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			ts, _ := m["taken_at"].(float64)
			if id != "" {
				takenAt[id] = int64(ts)
			}
		}
	}
	if more, _ := tl["more_available"].(bool); !more {
		return "", takenAt, nil
	}
	if s, _ := tl["next_max_id"].(string); s != "" {
		return s, takenAt, nil
	}
	s, _ := tl["profile_grid_items_cursor"].(string)
	return s, takenAt, nil
}

// findKey returns the parent map that contains key, searching recursively.
func findKey(node any, key string) any {
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	if _, ok := m[key]; ok {
		return m
	}
	for _, v := range m {
		if found := findKey(v, key); found != nil {
			return found
		}
	}
	return nil
}

// FetchLiveStories lists currently live stories (24h window).
func (c *Client) FetchLiveStories(ctx context.Context) ([]Item, error) {
	uid, err := c.userID()
	if err != nil {
		return nil, err
	}
	var resp struct {
		Reel *struct {
			Items []Item `json:"items"`
		} `json:"reel"`
	}
	if err := c.get(ctx, fmt.Sprintf("%s/api/v1/feed/user/%s/story/", c.base, uid), &resp); err != nil {
		return nil, err
	}
	if resp.Reel == nil {
		return nil, nil
	}
	return resp.Reel.Items, nil
}

type dayShellsResponse struct {
	Items []struct {
		ID string `json:"id"`
	} `json:"items"`
	MaxID json.RawMessage `json:"max_id"`
}

// FetchArchivedStories lists all stories in the owner's archive.
func (c *Client) FetchArchivedStories(ctx context.Context) ([]Item, error) {
	var dayIDs []string
	maxID := ""
	for {
		u := c.base + "/api/v1/archive/reel/day_shells_paginated/?timezone_offset=0&include_memories=0"
		if maxID != "" {
			u += "&max_id=" + url.QueryEscape(maxID)
		}
		var page dayShellsResponse
		if err := c.get(ctx, u, &page); err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			dayIDs = append(dayIDs, it.ID)
		}
		next := rawString(page.MaxID)
		if len(page.Items) == 0 || next == "" {
			break
		}
		maxID = next
	}
	var items []Item
	const batchSize = 50
	for i := 0; i < len(dayIDs); i += batchSize {
		batch := dayIDs[i:min(i+batchSize, len(dayIDs))]
		got, err := c.fetchReelsMedia(ctx, batch)
		if err != nil {
			return nil, err
		}
		items = append(items, got...)
	}
	return items, nil
}

func (c *Client) fetchReelsMedia(ctx context.Context, reelIDs []string) ([]Item, error) {
	payload, err := json.Marshal(map[string]any{
		"reel_ids":   reelIDs,
		"reason":     "on_tap",
		"source":     "archive",
		"batch_size": len(reelIDs),
	})
	if err != nil {
		return nil, err
	}
	body := "signed_body=SIGNATURE." + url.QueryEscape(string(payload))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/api/v1/feed/reels_media_stream/", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	respBody, err := c.do(req)
	if err != nil {
		return nil, err
	}
	// The response is a stream: one JSON object per line, each carrying a
	// subset of reels under "reels" and/or "reels_media".
	var items []Item
	sc := bufio.NewScanner(bytes.NewReader(respBody))
	sc.Buffer(make([]byte, 0, 1<<20), 32<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var out struct {
			Reels map[string]struct {
				Items []Item `json:"items"`
			} `json:"reels"`
			ReelsMedia []struct {
				Items []Item `json:"items"`
			} `json:"reels_media"`
		}
		if err := json.Unmarshal(line, &out); err != nil {
			return nil, fmt.Errorf("instagram: reels_media_stream: %w", err)
		}
		for _, reel := range out.Reels {
			items = append(items, reel.Items...)
		}
		for _, reel := range out.ReelsMedia {
			items = append(items, reel.Items...)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// rawString decodes a JSON value that may be a string, number, or null.
func rawString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str
	}
	return s
}

// Download fetches a signed CDN media URL (no auth needed, but short-lived).
func (c *Client) Download(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("instagram: download: %s", resp.Status)
	}
	return resp.Body, nil
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
