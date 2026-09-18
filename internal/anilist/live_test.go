package anilist

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveSearch hits the real AniList API to catch schema drift the fake
// server can't. Skipped unless ANILIST_LIVE=1.
func TestLiveSearch(t *testing.T) {
	if os.Getenv("ANILIST_LIVE") != "1" {
		t.Skip("set ANILIST_LIVE=1 to run against the real AniList API")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := NewClient(Config{CacheTTL: -1})
	results, err := c.Search(ctx, "Frieren", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search returned no results")
	}
	first := results[0]
	if first.ID == 0 || first.Title.Display() == "" || first.SiteURL == "" {
		t.Fatalf("first result missing core fields: %+v", first)
	}

	byID, err := c.ByID(ctx, first.ID)
	if err != nil {
		t.Fatalf("ByID(%d): %v", first.ID, err)
	}
	if byID.ID != first.ID {
		t.Fatalf("ByID returned id %d, want %d", byID.ID, first.ID)
	}
}
