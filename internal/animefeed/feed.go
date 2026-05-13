package animefeed

import (
	"html"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/feeds"
	"github.com/mmcdole/gofeed"
)

const defaultFeedURL = "https://nyaa.si/?page=rss&c=1_2&f=0"

var (
	parser     = gofeed.NewParser()
	tagPattern = regexp.MustCompile(`<[^>]+>`)
)

type FeedItem struct {
	GUID        string
	Title       string
	Link        string
	Description string
	PublishedAt *time.Time
}

func fetchFeedItems(feedURL string) ([]FeedItem, error) {
	if feedURL == "" {
		feedURL = defaultFeedURL
	}

	feed, err := parser.ParseURL(feedURL)
	if err != nil {
		log.Printf("anime feed load error: %v", err)
		return nil, err
	}

	items := make([]FeedItem, 0, len(feed.Items))
	for _, item := range feed.Items {
		if item == nil || strings.TrimSpace(item.GUID) == "" {
			continue
		}

		var published *time.Time
		if item.PublishedParsed != nil {
			t := item.PublishedParsed.UTC()
			published = &t
		}

		link := strings.TrimSpace(item.Link)
		items = append(items, FeedItem{
			GUID:        strings.TrimSpace(item.GUID),
			Title:       item.Title,
			Link:        link,
			Description: cleanDescription(item.Description),
			PublishedAt: published,
		})
	}

	return items, nil
}

func matchesAllKeywords(title string, keywords []string) bool {
	title = strings.ToLower(title)
	for _, keyword := range keywords {
		if !strings.Contains(title, strings.ToLower(strings.TrimSpace(keyword))) {
			return false
		}
	}
	return true
}

func buildUserFeedXML(feedName string, items []FeedItem) (string, error) {
	feed := &feeds.Feed{
		Title:   feedName,
		Updated: time.Now().UTC(),
	}

	for _, item := range items {
		created := time.Now().UTC()
		if item.PublishedAt != nil {
			created = item.PublishedAt.UTC()
		}

		feed.Add(&feeds.Item{
			Title:       item.Title,
			Link:        &feeds.Link{Href: item.Link},
			Description: item.Description,
			Created:     created,
			Id:          item.GUID,
		})
	}

	return feed.ToRss()
}

func cleanDescription(description string) string {
	description = tagPattern.ReplaceAllString(description, " ")
	description = html.UnescapeString(description)
	description = strings.Join(strings.Fields(description), " ")
	if len(description) > 280 {
		description = description[:277] + "..."
	}
	return description
}
