package anilist

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

func fixtureServer(t *testing.T, body []byte, hits *atomic.Int32, got *gqlRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q, want application/json", ct)
		}
		if got != nil {
			if err := json.NewDecoder(r.Body).Decode(got); err != nil {
				t.Errorf("decode request: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSearchBuildsRequestAndParsesFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/search_frieren.json")
	if err != nil {
		t.Fatal(err)
	}
	var req gqlRequest
	srv := fixtureServer(t, body, nil, &req)

	c := NewClient(Config{BaseURL: srv.URL, CacheTTL: -1})
	results, err := c.Search(context.Background(), "  Frieren season 2 ", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if !strings.Contains(req.Query, "type: ANIME") {
		t.Errorf("query should be restricted to anime: %s", req.Query)
	}
	if req.Variables["search"] != "Frieren season 2" {
		t.Errorf("search variable = %v, want trimmed query", req.Variables["search"])
	}
	if req.Variables["perPage"] != float64(5) {
		t.Errorf("perPage variable = %v, want 5", req.Variables["perPage"])
	}

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	first := results[0]
	if first.ID != 182255 || first.Format != "TV" || first.SeasonYear != 2026 {
		t.Fatalf("first result = %+v", first)
	}
	if first.Title.Display() != "Frieren: Beyond Journey’s End Season 2" {
		t.Errorf("display title = %q, want the English title", first.Title.Display())
	}
	if first.CoverColor != "#5dc9f1" || first.CoverURL == "" || first.SiteURL == "" {
		t.Errorf("cover/site fields missing: %+v", first)
	}
	if first.NextAiring != nil {
		t.Errorf("NextAiring = %+v, want nil when AniList reports none", first.NextAiring)
	}
	if strings.ContainsAny(first.Description, "<>") {
		t.Errorf("description still has markup: %q", first.Description)
	}
	// Entries with no English title fall back to romaji.
	if results[1].Title.English != "" || results[1].Title.Display() != results[1].Title.Romaji {
		t.Errorf("second result title = %+v", results[1].Title)
	}
}

func TestSearchParsesAiringAndNulls(t *testing.T) {
	body := `{"data":{"Page":{"media":[{
		"id": 7, "title": {"romaji": "Foo", "english": null, "native": null},
		"format": "TV", "status": "RELEASING", "episodes": null, "duration": 24,
		"season": "FALL", "seasonYear": 2026,
		"startDate": {"year": 2026, "month": 10, "day": null}, "endDate": {"year": null, "month": null, "day": null},
		"genres": ["Action"], "averageScore": null,
		"description": "Line one<br><br>Line <i>two</i> &amp; more",
		"siteUrl": "https://anilist.co/anime/7",
		"coverImage": {"large": null, "color": null},
		"studios": {"nodes": [{"name": "Studio A"}]},
		"nextAiringEpisode": {"episode": 5, "airingAt": 1790000000}
	}]}}}`
	srv := fixtureServer(t, []byte(body), nil, nil)
	c := NewClient(Config{BaseURL: srv.URL, CacheTTL: -1})

	results, err := c.Search(context.Background(), "foo", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	m := results[0]
	if m.NextAiring == nil || m.NextAiring.Episode != 5 || m.NextAiring.At.Unix() != 1790000000 {
		t.Fatalf("NextAiring = %+v", m.NextAiring)
	}
	if m.Episodes != 0 || m.AverageScore != 0 || m.CoverURL != "" {
		t.Errorf("null fields should decode to zero values: %+v", m)
	}
	if got := m.StartDate.String(); got != "2026-10" {
		t.Errorf("partial start date = %q, want 2026-10", got)
	}
	if !m.EndDate.IsZero() {
		t.Errorf("end date = %+v, want zero", m.EndDate)
	}
	if m.Description != "Line one\n\nLine two & more" {
		t.Errorf("description = %q", m.Description)
	}
	if len(m.Studios) != 1 || m.Studios[0] != "Studio A" {
		t.Errorf("studios = %v", m.Studios)
	}
}

func TestSearchEmptyResults(t *testing.T) {
	srv := fixtureServer(t, []byte(`{"data":{"Page":{"media":[]}}}`), nil, nil)
	c := NewClient(Config{BaseURL: srv.URL, CacheTTL: -1})
	results, err := c.Search(context.Background(), "zzzz", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results = %v, want none", results)
	}
}

func TestSearchRejectsEmptyQuery(t *testing.T) {
	c := NewClient(Config{BaseURL: "http://127.0.0.1:1", CacheTTL: -1})
	if _, err := c.Search(context.Background(), "   ", 5); err == nil {
		t.Fatal("expected error for blank query")
	}
}

func TestErrorResponses(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"rate limited", http.StatusTooManyRequests, `{"errors":[{"message":"Too Many Requests."}]}`, "429"},
		{"server error", http.StatusInternalServerError, `oops`, "500"},
		{"graphql error", http.StatusOK, `{"errors":[{"message":"Validation failed"}],"data":null}`, "Validation failed"},
		{"malformed json", http.StatusOK, `{not json`, "decode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			c := NewClient(Config{BaseURL: srv.URL, CacheTTL: -1})
			_, err := c.Search(context.Background(), "x", 5)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestByID(t *testing.T) {
	body := `{"data":{"Media":{"id":42,"title":{"romaji":"Answer","english":null,"native":null},
		"startDate":{},"endDate":{},"coverImage":{},"studios":{"nodes":[]}}}}`
	var req gqlRequest
	srv := fixtureServer(t, []byte(body), nil, &req)
	c := NewClient(Config{BaseURL: srv.URL, CacheTTL: -1})

	m, err := c.ByID(context.Background(), 42)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if m.ID != 42 || req.Variables["id"] != float64(42) {
		t.Fatalf("media = %+v, request vars = %v", m, req.Variables)
	}

	missing := fixtureServer(t, []byte(`{"data":{"Media":null}}`), nil, nil)
	c = NewClient(Config{BaseURL: missing.URL, CacheTTL: -1})
	if _, err := c.ByID(context.Background(), 43); err == nil {
		t.Fatal("expected error when AniList has no such id")
	}
	if _, err := c.ByID(context.Background(), 0); err == nil {
		t.Fatal("expected error for invalid id")
	}
}

func TestSearchCachesByNormalizedQuery(t *testing.T) {
	body, _ := os.ReadFile("testdata/search_frieren.json")
	var hits atomic.Int32
	srv := fixtureServer(t, body, &hits, nil)
	c := NewClient(Config{BaseURL: srv.URL})

	if _, err := c.Search(context.Background(), "Frieren  Season 2", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Search(context.Background(), "frieren season 2", 5); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits = %d, want 1 (second search should hit the cache)", got)
	}

	// Search results also seed the by-id cache.
	if _, err := c.ByID(context.Background(), 182255); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits = %d after ByID, want 1", got)
	}
}

func TestCacheDisabled(t *testing.T) {
	body, _ := os.ReadFile("testdata/search_frieren.json")
	var hits atomic.Int32
	srv := fixtureServer(t, body, &hits, nil)
	c := NewClient(Config{BaseURL: srv.URL, CacheTTL: -1})
	for range 2 {
		if _, err := c.Search(context.Background(), "frieren", 5); err != nil {
			t.Fatal(err)
		}
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("server hits = %d, want 2 with caching disabled", got)
	}
}

func TestTTLCacheExpiry(t *testing.T) {
	c := NewTTLCache[int](time.Minute)
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }

	c.Set("a", 1)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = %v, %v; want 1, true", v, ok)
	}
	now = now.Add(59 * time.Second)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("entry should still be live just before the TTL")
	}
	now = now.Add(2 * time.Second)
	if _, ok := c.Get("a"); ok {
		t.Fatal("entry should have expired")
	}
}

func TestNormalizeKey(t *testing.T) {
	if got := NormalizeKey("  Attack   ON  Titan "); got != "attack on titan" {
		t.Fatalf("NormalizeKey = %q", got)
	}
}
