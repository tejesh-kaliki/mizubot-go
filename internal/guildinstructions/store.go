package guildinstructions

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"mizubot-go/internal/data"
)

type Instruction struct {
	GuildID      string
	Instructions string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Store struct {
	db *sql.DB
	q  *data.Queries
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, q: data.New()}
}

func (s *Store) Get(ctx context.Context, guildID string) (Instruction, bool, error) {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return Instruction{}, false, nil
	}

	row, err := s.q.GetGuildInstructions(ctx, s.db, guildID)
	if err == sql.ErrNoRows {
		return Instruction{}, false, nil
	}
	if err != nil {
		return Instruction{}, false, err
	}
	return convertInstruction(row), true, nil
}

func (s *Store) Upsert(ctx context.Context, guildID, instructions string) (Instruction, error) {
	guildID = strings.TrimSpace(guildID)
	instructions = strings.TrimSpace(instructions)
	if guildID == "" {
		return Instruction{}, errors.New("missing guild id")
	}
	if instructions == "" {
		return Instruction{}, errors.New("instructions are required")
	}

	now := time.Now().UTC().Unix()
	row, err := s.q.UpsertGuildInstructions(ctx, s.db, data.UpsertGuildInstructionsParams{
		GuildID:      guildID,
		Instructions: instructions,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return Instruction{}, err
	}
	return convertInstruction(row), nil
}

func (s *Store) GetGuildInstruction(ctx context.Context, guildID string) (string, bool, error) {
	instruction, ok, err := s.Get(ctx, guildID)
	if err != nil || !ok {
		return "", ok, err
	}
	return instruction.Instructions, true, nil
}

func Seed(ctx context.Context, store *Store, instructions map[string]string) error {
	for guildID, instruction := range instructions {
		if strings.TrimSpace(guildID) == "" || strings.TrimSpace(instruction) == "" {
			continue
		}
		if _, err := store.Upsert(ctx, guildID, instruction); err != nil {
			return err
		}
	}
	return nil
}

func convertInstruction(row data.GuildInstruction) Instruction {
	return Instruction{
		GuildID:      row.GuildID,
		Instructions: row.Instructions,
		CreatedAt:    time.Unix(row.CreatedAt, 0).UTC(),
		UpdatedAt:    time.Unix(row.UpdatedAt, 0).UTC(),
	}
}
