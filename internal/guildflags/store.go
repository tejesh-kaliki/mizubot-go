// Package guildflags lets guild admins define lightweight content flags:
// a short description of what to flag and guidance on how to respond when
// flagged. Flags are evaluated by the TypeSafe classifier alongside tool
// routing, and matched guidance is injected into the LLM's system prompt
// only for messages that trip the flag, instead of bloating every prompt
// with guild-specific policy text.
package guildflags

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"mizubot-go/internal/data"
)

type Flag struct {
	ID          int64
	GuildID     string
	Name        string
	Description string
	Guidance    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Store struct {
	db *sql.DB
	q  *data.Queries
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, q: data.New()}
}

func (s *Store) Create(ctx context.Context, guildID, name, description, guidance string) (Flag, error) {
	guildID = strings.TrimSpace(guildID)
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	guidance = strings.TrimSpace(guidance)
	if guildID == "" {
		return Flag{}, errors.New("missing guild id")
	}
	if name == "" {
		return Flag{}, errors.New("missing flag name")
	}
	if description == "" {
		return Flag{}, errors.New("description is required")
	}
	if guidance == "" {
		return Flag{}, errors.New("guidance is required")
	}

	now := time.Now().UTC().Unix()
	row, err := s.q.CreateGuildFlag(ctx, s.db, data.CreateGuildFlagParams{
		GuildID:     guildID,
		Name:        name,
		Description: description,
		Guidance:    guidance,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return Flag{}, err
	}
	return convertFlag(row), nil
}

func (s *Store) ListByGuild(ctx context.Context, guildID string) ([]Flag, error) {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return nil, nil
	}
	rows, err := s.q.ListGuildFlagsByGuild(ctx, s.db, guildID)
	if err != nil {
		return nil, err
	}
	out := make([]Flag, 0, len(rows))
	for _, row := range rows {
		out = append(out, convertFlag(row))
	}
	return out, nil
}

func (s *Store) Update(ctx context.Context, id int64, guildID, description, guidance string) (Flag, error) {
	guildID = strings.TrimSpace(guildID)
	description = strings.TrimSpace(description)
	guidance = strings.TrimSpace(guidance)
	if guildID == "" {
		return Flag{}, errors.New("missing guild id")
	}
	if description == "" {
		return Flag{}, errors.New("description is required")
	}
	if guidance == "" {
		return Flag{}, errors.New("guidance is required")
	}

	row, err := s.q.UpdateGuildFlag(ctx, s.db, data.UpdateGuildFlagParams{
		Description: description,
		Guidance:    guidance,
		UpdatedAt:   time.Now().UTC().Unix(),
		ID:          id,
		GuildID:     guildID,
	})
	if err != nil {
		return Flag{}, err
	}
	return convertFlag(row), nil
}

// Delete removes a guild's flag, returning false when no matching flag
// existed for that guild.
func (s *Store) Delete(ctx context.Context, id int64, guildID string) (bool, error) {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return false, errors.New("missing guild id")
	}
	rows, err := s.q.DeleteGuildFlag(ctx, s.db, id, guildID)
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func convertFlag(row data.GuildFlag) Flag {
	return Flag{
		ID:          row.ID,
		GuildID:     row.GuildID,
		Name:        row.Name,
		Description: row.Description,
		Guidance:    row.Guidance,
		CreatedAt:   time.Unix(row.CreatedAt, 0).UTC(),
		UpdatedAt:   time.Unix(row.UpdatedAt, 0).UTC(),
	}
}
