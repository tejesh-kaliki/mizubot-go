package usersettings

import (
	"context"
	"database/sql"
	"time"

	"mizubot-go/internal/data"
)

type Settings struct {
	UserID    string
	Timezone  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Store struct {
	db *sql.DB
	q  *data.Queries
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, q: data.New()}
}

func (s *Store) Get(ctx context.Context, userID string) (Settings, bool, error) {
	row, err := s.q.GetUserSettings(ctx, s.db, userID)
	if err == sql.ErrNoRows {
		return Settings{}, false, nil
	}
	if err != nil {
		return Settings{}, false, err
	}
	return convertSettings(row), true, nil
}

func (s *Store) SetTimezone(ctx context.Context, userID, timezone string) (Settings, error) {
	now := time.Now().UTC()
	row, err := s.q.UpsertUserTimezone(ctx, s.db, data.UpsertUserTimezoneParams{
		UserID:    userID,
		Timezone:  timezone,
		CreatedAt: now.Unix(),
		UpdatedAt: now.Unix(),
	})
	if err != nil {
		return Settings{}, err
	}
	return convertSettings(row), nil
}

func convertSettings(row data.UserSetting) Settings {
	return Settings{
		UserID:    row.UserID,
		Timezone:  row.Timezone,
		CreatedAt: time.Unix(row.CreatedAt, 0).UTC(),
		UpdatedAt: time.Unix(row.UpdatedAt, 0).UTC(),
	}
}
