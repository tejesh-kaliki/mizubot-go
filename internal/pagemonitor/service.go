package pagemonitor

import (
	"context"
	"errors"
	"net/url"
	"time"
)

const defaultCheckInterval = 5 * time.Minute

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

type AddMonitorInput struct {
	UserID    string
	ChannelID string
	GuildID   string
	URL       string
	Label     string
	Selector  string
}

func (s *Service) AddMonitor(ctx context.Context, input AddMonitorInput) (*Monitor, error) {
	if input.URL == "" {
		return nil, errors.New("URL is required")
	}
	parsed, err := url.ParseRequestURI(input.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("Invalid URL. Must start with http:// or https://")
	}
	label := input.Label
	if label == "" {
		label = parsed.Host
	}

	m := &Monitor{
		UserID:        input.UserID,
		ChannelID:     input.ChannelID,
		URL:           input.URL,
		Label:         label,
		Selector:      input.Selector,
		LastStatus:    "unknown",
		CheckInterval: defaultCheckInterval,
		NextCheck:     time.Now().UTC(),
	}
	if input.GuildID != "" {
		m.GuildID = &input.GuildID
	}
	if err := s.store.Create(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) ListMonitors(ctx context.Context, userID string) ([]Monitor, error) {
	return s.store.ListByUser(ctx, userID)
}

func (s *Service) RemoveMonitor(ctx context.Context, id int64, userID string) (bool, error) {
	return s.store.Delete(ctx, id, userID)
}

func (s *Service) DueMonitors(ctx context.Context, now time.Time) ([]Monitor, error) {
	return s.store.Due(ctx, now, 50)
}

func (s *Service) UpdateContent(ctx context.Context, m Monitor, status, hash, content string) error {
	next := time.Now().UTC().Add(m.CheckInterval)
	return s.store.UpdateContent(ctx, m.ID, status, hash, content, next)
}
