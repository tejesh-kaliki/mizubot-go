package animefeed

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"mizubot-go/internal/data"
)

const recentMatchLimit = 50

type Publisher interface {
	PublishUserFeed(ctx context.Context, userID string, body string) (string, error)
}

type Notifier interface {
	SendAnimeNotification(channelID string, embed AnimeNotificationEmbed) error
}

type AnimeNotificationEmbed struct {
	UserID      string
	FollowName  string
	Title       string
	Link        string
	Description string
	PublishedAt *time.Time
}

type Entry struct {
	ID                int64
	UserID            string
	Name              string
	Keywords          []string
	ChannelID         string
	LatestGUID        string
	LatestTitle       string
	LatestLink        string
	LatestPublishedAt *time.Time
	LastNotifiedAt    *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Match struct {
	ID               int64
	UserAnimeEntryID int64
	GUID             string
	Title            string
	Link             string
	PublishedAt      *time.Time
	CreatedAt        time.Time
}

type Settings struct {
	UserID           string
	DefaultChannelID string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Service struct {
	db        *sql.DB
	q         *data.Queries
	publisher Publisher
	feedURL   string
}

type FollowInput struct {
	UserID    string
	Name      string
	Keywords  []string
	ChannelID string
}

type UserFeed struct {
	UserID   string
	Location string
}

func NewService(db *sql.DB, publisher Publisher, feedURL string) *Service {
	return &Service{
		db:        db,
		q:         data.New(),
		publisher: publisher,
		feedURL:   feedURL,
	}
}

func (s *Service) Follow(ctx context.Context, input FollowInput) (Entry, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Entry{}, errors.New("name is required")
	}

	keywords := normalizeKeywords(input.Keywords)
	if len(keywords) == 0 {
		return Entry{}, errors.New("at least one keyword is required")
	}

	payload, err := json.Marshal(keywords)
	if err != nil {
		return Entry{}, err
	}

	now := time.Now().UTC()
	var channelPtr *string
	if input.ChannelID != "" {
		channelPtr = &input.ChannelID
	}

	rec, err := s.q.CreateAnimeEntry(ctx, s.db, data.CreateAnimeEntryParams{
		UserID:    input.UserID,
		Name:      name,
		Keywords:  string(payload),
		ChannelID: channelPtr,
		CreatedAt: now.Unix(),
		UpdatedAt: now.Unix(),
	})
	if err != nil {
		return Entry{}, err
	}

	return convertEntry(rec)
}

func (s *Service) ListFollows(ctx context.Context, userID string) ([]Entry, error) {
	recs, err := s.q.ListAnimeEntriesByUser(ctx, s.db, userID)
	if err != nil {
		return nil, err
	}

	out := make([]Entry, 0, len(recs))
	for _, rec := range recs {
		entry, err := convertEntry(rec)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

func (s *Service) GetSettings(ctx context.Context, userID string) (Settings, error) {
	rec, err := s.q.GetAnimeSettingsByUser(ctx, s.db, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Settings{UserID: userID}, nil
		}
		return Settings{}, err
	}
	return convertSettings(rec), nil
}

func (s *Service) SetDefaultChannel(ctx context.Context, userID, channelID string) (Settings, error) {
	now := time.Now().UTC().Unix()
	var channelPtr *string
	if channelID != "" {
		channelPtr = &channelID
	}

	rec, err := s.q.UpsertAnimeSettings(ctx, s.db, data.UpsertAnimeSettingsParams{
		UserID:           userID,
		DefaultChannelID: channelPtr,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		return Settings{}, err
	}
	return convertSettings(rec), nil
}

func (s *Service) Unfollow(ctx context.Context, userID, name string) (bool, error) {
	n, err := s.q.DeleteAnimeEntryOwned(ctx, s.db, userID, strings.TrimSpace(name))
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Service) SetChannel(ctx context.Context, userID, name, channelID string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, errors.New("name is required")
	}

	var channelPtr *string
	if channelID != "" {
		channelPtr = &channelID
	}

	n, err := s.q.SetAnimeEntryChannel(ctx, s.db, data.SetAnimeEntryChannelParams{
		ChannelID: channelPtr,
		UpdatedAt: time.Now().UTC().Unix(),
		UserID:    userID,
		Name:      name,
	})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Service) Sync(ctx context.Context, notifier Notifier) ([]UserFeed, error) {
	items, err := fetchFeedItems(s.feedURL)
	if err != nil {
		return nil, err
	}

	entryRecs, err := s.q.ListAnimeEntries(ctx, s.db)
	if err != nil {
		return nil, err
	}
	if len(entryRecs) == 0 {
		return nil, nil
	}

	entries := make([]Entry, 0, len(entryRecs))
	settingsByUser := make(map[string]Settings)
	for _, rec := range entryRecs {
		entry, err := convertEntry(rec)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
		if _, ok := settingsByUser[entry.UserID]; !ok {
			settings, err := s.GetSettings(ctx, entry.UserID)
			if err != nil {
				return nil, err
			}
			settingsByUser[entry.UserID] = settings
		}
	}

	updatedUsers := make(map[string]struct{})
	var feeds []UserFeed
	processedGUIDs, err := s.processedGUIDSet(ctx, items)
	if err != nil {
		return nil, err
	}

	for _, item := range items {
		if _, ok := processedGUIDs[item.GUID]; ok {
			continue
		}

		for _, entry := range entries {
			if !matchesAllKeywords(item.Title, entry.Keywords) {
				continue
			}

			exists, err := s.q.HasAnimeMatch(ctx, s.db, entry.ID, item.GUID)
			if err != nil {
				return feeds, err
			}
			if exists {
				continue
			}

			if err := s.createMatch(ctx, entry.ID, item); err != nil {
				return feeds, err
			}

			notifiedAt := time.Now().UTC()
			if err := s.updateEntryLatest(ctx, entry.ID, item, notifiedAt); err != nil {
				return feeds, err
			}

			channelID := entry.ChannelID
			if channelID == "" {
				channelID = settingsByUser[entry.UserID].DefaultChannelID
			}

			if channelID != "" && notifier != nil {
				if err := notifier.SendAnimeNotification(channelID, AnimeNotificationEmbed{
					UserID:      entry.UserID,
					FollowName:  entry.Name,
					Title:       item.Title,
					Link:        item.Link,
					Description: item.Description,
					PublishedAt: item.PublishedAt,
				}); err != nil {
					log.Printf("anime notify error for entry %d: %v", entry.ID, err)
				}
			}

			updatedUsers[entry.UserID] = struct{}{}
		}

		if err := s.markProcessed(ctx, item); err != nil {
			return feeds, err
		}
		processedGUIDs[item.GUID] = struct{}{}
	}

	if s.publisher == nil {
		return nil, nil
	}

	userIDs := make([]string, 0, len(updatedUsers))
	for userID := range updatedUsers {
		userIDs = append(userIDs, userID)
	}
	sort.Strings(userIDs)

	for _, userID := range userIDs {
		location, err := s.publishUserFeed(ctx, userID)
		if err != nil {
			return feeds, err
		}
		feeds = append(feeds, UserFeed{UserID: userID, Location: location})
	}

	return feeds, nil
}

func (s *Service) publishUserFeed(ctx context.Context, userID string) (string, error) {
	recs, err := s.q.ListRecentAnimeMatchesByUser(ctx, s.db, userID, recentMatchLimit)
	if err != nil {
		return "", err
	}

	items := make([]FeedItem, 0, len(recs))
	for _, rec := range recs {
		match := convertMatch(rec)
		items = append(items, FeedItem{
			GUID:        match.GUID,
			Title:       match.Title,
			Link:        match.Link,
			PublishedAt: match.PublishedAt,
		})
	}

	xmlBody, err := buildUserFeedXML(fmt.Sprintf("MizuBot Anime Feed %s", userID), items)
	if err != nil {
		return "", err
	}

	return s.publisher.PublishUserFeed(ctx, userID, xmlBody)
}

func (s *Service) markProcessed(ctx context.Context, item FeedItem) error {
	var publishedAt *int64
	if item.PublishedAt != nil {
		v := item.PublishedAt.UTC().Unix()
		publishedAt = &v
	}

	return s.q.CreateProcessedRssEntry(ctx, s.db, data.CreateProcessedRssEntryParams{
		Guid:        item.GUID,
		Title:       item.Title,
		Link:        item.Link,
		PublishedAt: publishedAt,
		CreatedAt:   time.Now().UTC().Unix(),
	})
}

func (s *Service) processedGUIDSet(ctx context.Context, items []FeedItem) (map[string]struct{}, error) {
	guids := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.GUID == "" {
			continue
		}
		if _, ok := seen[item.GUID]; ok {
			continue
		}
		seen[item.GUID] = struct{}{}
		guids = append(guids, item.GUID)
	}

	if len(guids) == 0 {
		return map[string]struct{}{}, nil
	}

	recs, err := s.q.ListProcessedRssEntriesByGUIDs(ctx, s.db, guids)
	if err != nil {
		return nil, err
	}

	out := make(map[string]struct{}, len(recs))
	for _, rec := range recs {
		out[rec.Guid] = struct{}{}
	}
	return out, nil
}

func (s *Service) createMatch(ctx context.Context, entryID int64, item FeedItem) error {
	var publishedAt *int64
	if item.PublishedAt != nil {
		v := item.PublishedAt.UTC().Unix()
		publishedAt = &v
	}

	return s.q.CreateAnimeMatch(ctx, s.db, data.CreateAnimeMatchParams{
		UserAnimeEntryID: entryID,
		Guid:             item.GUID,
		Title:            item.Title,
		Link:             item.Link,
		PublishedAt:      publishedAt,
		CreatedAt:        time.Now().UTC().Unix(),
	})
}

func (s *Service) updateEntryLatest(ctx context.Context, entryID int64, item FeedItem, notifiedAt time.Time) error {
	var publishedAt *int64
	if item.PublishedAt != nil {
		v := item.PublishedAt.UTC().Unix()
		publishedAt = &v
	}

	guid := item.GUID
	title := item.Title
	link := item.Link
	notified := notifiedAt.UTC().Unix()

	return s.q.UpdateAnimeEntryLatest(ctx, s.db, data.UpdateAnimeEntryLatestParams{
		LatestGuid:        &guid,
		LatestTitle:       &title,
		LatestLink:        &link,
		LatestPublishedAt: publishedAt,
		LastNotifiedAt:    &notified,
		UpdatedAt:         time.Now().UTC().Unix(),
		ID:                entryID,
	})
}

func convertEntry(rec data.UserAnimeEntry) (Entry, error) {
	var keywords []string
	if err := json.Unmarshal([]byte(rec.Keywords), &keywords); err != nil {
		return Entry{}, err
	}

	entry := Entry{
		ID:        rec.ID,
		UserID:    rec.UserID,
		Name:      rec.Name,
		Keywords:  keywords,
		CreatedAt: time.Unix(rec.CreatedAt, 0).UTC(),
		UpdatedAt: time.Unix(rec.UpdatedAt, 0).UTC(),
	}
	if rec.ChannelID != nil {
		entry.ChannelID = *rec.ChannelID
	}
	if rec.LatestGuid != nil {
		entry.LatestGUID = *rec.LatestGuid
	}
	if rec.LatestTitle != nil {
		entry.LatestTitle = *rec.LatestTitle
	}
	if rec.LatestLink != nil {
		entry.LatestLink = *rec.LatestLink
	}
	if rec.LatestPublishedAt != nil {
		t := time.Unix(*rec.LatestPublishedAt, 0).UTC()
		entry.LatestPublishedAt = &t
	}
	if rec.LastNotifiedAt != nil {
		t := time.Unix(*rec.LastNotifiedAt, 0).UTC()
		entry.LastNotifiedAt = &t
	}

	return entry, nil
}

func convertMatch(rec data.UserAnimeMatch) Match {
	match := Match{
		ID:               rec.ID,
		UserAnimeEntryID: rec.UserAnimeEntryID,
		GUID:             rec.Guid,
		Title:            rec.Title,
		Link:             rec.Link,
		CreatedAt:        time.Unix(rec.CreatedAt, 0).UTC(),
	}
	if rec.PublishedAt != nil {
		t := time.Unix(*rec.PublishedAt, 0).UTC()
		match.PublishedAt = &t
	}
	return match
}

func convertSettings(rec data.UserAnimeSetting) Settings {
	settings := Settings{
		UserID:    rec.UserID,
		CreatedAt: time.Unix(rec.CreatedAt, 0).UTC(),
		UpdatedAt: time.Unix(rec.UpdatedAt, 0).UTC(),
	}
	if rec.DefaultChannelID != nil {
		settings.DefaultChannelID = *rec.DefaultChannelID
	}
	return settings
}

func normalizeKeywords(keywords []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		normalized := strings.ToLower(strings.TrimSpace(keyword))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}
