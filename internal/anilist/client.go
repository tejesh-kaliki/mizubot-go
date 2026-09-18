package anilist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is AniList's public GraphQL endpoint.
	DefaultBaseURL = "https://graphql.anilist.co"
	// DefaultCacheTTL is how long search results and lookups are reused.
	DefaultCacheTTL = 10 * time.Minute
)

const mediaFields = `
	id
	title { romaji english native }
	format
	status
	episodes
	duration
	season
	seasonYear
	startDate { year month day }
	endDate { year month day }
	genres
	averageScore
	description(asHtml: false)
	siteUrl
	coverImage { large color }
	studios(isMain: true) { nodes { name } }
	nextAiringEpisode { episode airingAt }
`

const searchQuery = `query ($search: String, $perPage: Int) {
  Page(page: 1, perPage: $perPage) {
    media(search: $search, type: ANIME, sort: SEARCH_MATCH) {` + mediaFields + `}
  }
}`

const byIDQuery = `query ($id: Int) {
  Media(id: $id, type: ANIME) {` + mediaFields + `}
}`

// APIError is returned for non-2xx responses and GraphQL-level errors.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("anilist: request failed with status %d: %s", e.StatusCode, e.Body)
}

type Config struct {
	BaseURL    string
	Timeout    time.Duration
	CacheTTL   time.Duration // 0 uses DefaultCacheTTL; negative disables caching
	HTTPClient *http.Client
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	searches   *TTLCache[[]Media]
	byID       *TTLCache[Media]
}

func NewClient(cfg Config) *Client {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	c := &Client{httpClient: httpClient, baseURL: baseURL}
	ttl := cfg.CacheTTL
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	if ttl > 0 {
		c.searches = NewTTLCache[[]Media](ttl)
		c.byID = NewTTLCache[Media](ttl)
	}
	return c
}

// Search returns up to limit anime matching query, best match first.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Media, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("anilist: empty search query")
	}
	if limit <= 0 {
		limit = 5
	}
	key := NormalizeKey(query) + "|" + strconv.Itoa(limit)
	if c.searches != nil {
		if hit, ok := c.searches.Get(key); ok {
			return hit, nil
		}
	}

	var resp struct {
		Data struct {
			Page struct {
				Media []rawMedia `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}
	if err := c.do(ctx, searchQuery, map[string]any{"search": query, "perPage": limit}, &resp); err != nil {
		return nil, err
	}
	out := make([]Media, 0, len(resp.Data.Page.Media))
	for _, raw := range resp.Data.Page.Media {
		out = append(out, raw.convert())
	}
	if c.searches != nil {
		c.searches.Set(key, out)
		for _, m := range out {
			c.byID.Set(strconv.Itoa(m.ID), m)
		}
	}
	return out, nil
}

// ByID fetches one anime by AniList ID.
func (c *Client) ByID(ctx context.Context, id int) (Media, error) {
	if id <= 0 {
		return Media{}, fmt.Errorf("anilist: invalid id %d", id)
	}
	key := strconv.Itoa(id)
	if c.byID != nil {
		if hit, ok := c.byID.Get(key); ok {
			return hit, nil
		}
	}
	var resp struct {
		Data struct {
			Media *rawMedia `json:"Media"`
		} `json:"data"`
	}
	if err := c.do(ctx, byIDQuery, map[string]any{"id": id}, &resp); err != nil {
		return Media{}, err
	}
	if resp.Data.Media == nil {
		return Media{}, fmt.Errorf("anilist: no anime with id %d", id)
	}
	m := resp.Data.Media.convert()
	if c.byID != nil {
		c.byID.Set(key, m)
	}
	return m, nil
}

func (c *Client) do(ctx context.Context, query string, variables map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("anilist: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("anilist: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &APIError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(truncateBody(string(raw)))}
	}

	var envelope struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.Errors) > 0 {
		return fmt.Errorf("anilist: %s", envelope.Errors[0].Message)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("anilist: decode response: %w", err)
	}
	return nil
}

func truncateBody(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}

type rawDate struct {
	Year  *int `json:"year"`
	Month *int `json:"month"`
	Day   *int `json:"day"`
}

func (d rawDate) convert() Date {
	return Date{Year: deref(d.Year), Month: deref(d.Month), Day: deref(d.Day)}
}

type rawMedia struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  *string `json:"romaji"`
		English *string `json:"english"`
		Native  *string `json:"native"`
	} `json:"title"`
	Format       *string  `json:"format"`
	Status       *string  `json:"status"`
	Episodes     *int     `json:"episodes"`
	Duration     *int     `json:"duration"`
	Season       *string  `json:"season"`
	SeasonYear   *int     `json:"seasonYear"`
	StartDate    rawDate  `json:"startDate"`
	EndDate      rawDate  `json:"endDate"`
	Genres       []string `json:"genres"`
	AverageScore *int     `json:"averageScore"`
	Description  *string  `json:"description"`
	SiteURL      *string  `json:"siteUrl"`
	CoverImage   struct {
		Large *string `json:"large"`
		Color *string `json:"color"`
	} `json:"coverImage"`
	Studios struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"studios"`
	NextAiringEpisode *struct {
		Episode  int   `json:"episode"`
		AiringAt int64 `json:"airingAt"`
	} `json:"nextAiringEpisode"`
}

func (r rawMedia) convert() Media {
	m := Media{
		ID: r.ID,
		Title: Title{
			Romaji:  derefStr(r.Title.Romaji),
			English: derefStr(r.Title.English),
			Native:  derefStr(r.Title.Native),
		},
		Format:       derefStr(r.Format),
		Status:       derefStr(r.Status),
		Episodes:     deref(r.Episodes),
		Duration:     deref(r.Duration),
		Season:       derefStr(r.Season),
		SeasonYear:   deref(r.SeasonYear),
		StartDate:    r.StartDate.convert(),
		EndDate:      r.EndDate.convert(),
		Genres:       r.Genres,
		AverageScore: deref(r.AverageScore),
		Description:  CleanDescription(derefStr(r.Description)),
		SiteURL:      derefStr(r.SiteURL),
		CoverURL:     derefStr(r.CoverImage.Large),
		CoverColor:   derefStr(r.CoverImage.Color),
	}
	for _, node := range r.Studios.Nodes {
		if node.Name != "" {
			m.Studios = append(m.Studios, node.Name)
		}
	}
	if r.NextAiringEpisode != nil && r.NextAiringEpisode.AiringAt > 0 {
		m.NextAiring = &Airing{
			Episode: r.NextAiringEpisode.Episode,
			At:      time.Unix(r.NextAiringEpisode.AiringAt, 0).UTC(),
		}
	}
	return m
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
