package pagemonitor

import (
	"context"
	"database/sql"
	"time"

	"mizubot-go/internal/data"
)

type Monitor struct {
	ID            int64
	UserID        string
	ChannelID     string
	GuildID       *string
	URL           string
	Label         string
	Selector      string
	LastStatus    string
	ContentHash   string
	LastContent   string
	CheckInterval time.Duration
	NextCheck     time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Store struct {
	db *sql.DB
	q  *data.Queries
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, q: data.New()}
}

func (s *Store) Create(ctx context.Context, m *Monitor) error {
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now
	rec, err := s.q.CreatePageMonitor(ctx, s.db, data.CreatePageMonitorParams{
		UserID:        m.UserID,
		ChannelID:     m.ChannelID,
		GuildID:       m.GuildID,
		Url:           m.URL,
		Label:         m.Label,
		Selector:      m.Selector,
		LastStatus:    m.LastStatus,
		ContentHash:   m.ContentHash,
		LastContent:   m.LastContent,
		CheckInterval: int64(m.CheckInterval.Seconds()),
		NextCheck:     m.NextCheck.UTC().Unix(),
		CreatedAt:     now.Unix(),
		UpdatedAt:     now.Unix(),
	})
	if err != nil {
		return err
	}
	m.ID = rec.ID
	return nil
}

func (s *Store) ListByUser(ctx context.Context, userID string) ([]Monitor, error) {
	recs, err := s.q.ListPageMonitorsByUser(ctx, s.db, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Monitor, 0, len(recs))
	for _, r := range recs {
		out = append(out, monitorFromListRow(r))
	}
	return out, nil
}

func (s *Store) Due(ctx context.Context, now time.Time, limit int) ([]Monitor, error) {
	recs, err := s.q.ListDuePageMonitors(ctx, s.db, now.UTC().Unix(), int64(limit))
	if err != nil {
		return nil, err
	}
	out := make([]Monitor, 0, len(recs))
	for _, r := range recs {
		out = append(out, monitorFromDueRow(r))
	}
	return out, nil
}

func (s *Store) UpdateContent(ctx context.Context, id int64, status, hash, content string, nextCheck time.Time) error {
	return s.q.UpdatePageMonitorContent(ctx, s.db, data.UpdatePageMonitorContentParams{
		LastStatus:  status,
		ContentHash: hash,
		LastContent: content,
		NextCheck:   nextCheck.UTC().Unix(),
		UpdatedAt:   time.Now().UTC().Unix(),
		ID:          id,
	})
}

func (s *Store) Delete(ctx context.Context, id int64, userID string) (bool, error) {
	n, err := s.q.DeletePageMonitor(ctx, s.db, id, userID)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func monitorFromListRow(r data.ListPageMonitorsByUserRow) Monitor {
	return Monitor{
		ID:            r.ID,
		UserID:        r.UserID,
		ChannelID:     r.ChannelID,
		GuildID:       r.GuildID,
		URL:           r.Url,
		Label:         r.Label,
		Selector:      r.Selector,
		LastStatus:    r.LastStatus,
		ContentHash:   r.ContentHash,
		LastContent:   r.LastContent,
		CheckInterval: time.Duration(r.CheckInterval) * time.Second,
		NextCheck:     time.Unix(r.NextCheck, 0).UTC(),
		CreatedAt:     time.Unix(r.CreatedAt, 0).UTC(),
		UpdatedAt:     time.Unix(r.UpdatedAt, 0).UTC(),
	}
}

func monitorFromDueRow(r data.ListDuePageMonitorsRow) Monitor {
	return Monitor{
		ID:            r.ID,
		UserID:        r.UserID,
		ChannelID:     r.ChannelID,
		GuildID:       r.GuildID,
		URL:           r.Url,
		Label:         r.Label,
		Selector:      r.Selector,
		LastStatus:    r.LastStatus,
		ContentHash:   r.ContentHash,
		LastContent:   r.LastContent,
		CheckInterval: time.Duration(r.CheckInterval) * time.Second,
		NextCheck:     time.Unix(r.NextCheck, 0).UTC(),
		CreatedAt:     time.Unix(r.CreatedAt, 0).UTC(),
		UpdatedAt:     time.Unix(r.UpdatedAt, 0).UTC(),
	}
}
