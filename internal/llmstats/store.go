package llmstats

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"mizubot-go/internal/data"
)

const (
	StatusSuccess = "success"
	StatusError   = "error"
)

type MessageLog struct {
	ID               int64
	GuildID          string
	ChannelID        string
	UserID           string
	MessageID        string
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	LLMTurns         int64
	ToolCalls        int64
	Latency          time.Duration
	Status           string
	Error            string
	CreatedAt        time.Time
}

type CreateMessageLogParams struct {
	GuildID          string
	ChannelID        string
	UserID           string
	MessageID        string
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	LLMTurns         int64
	ToolCalls        int64
	Latency          time.Duration
	Status           string
	Error            string
}

type Store struct {
	db *sql.DB
	q  *data.Queries
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, q: data.New()}
}

func (s *Store) Create(ctx context.Context, params CreateMessageLogParams) (MessageLog, error) {
	status := strings.TrimSpace(params.Status)
	if status == "" {
		status = StatusSuccess
	}

	row, err := s.q.CreateLLMMessageLog(ctx, s.db, data.CreateLLMMessageLogParams{
		GuildID:          nullableString(params.GuildID),
		ChannelID:        strings.TrimSpace(params.ChannelID),
		UserID:           strings.TrimSpace(params.UserID),
		MessageID:        strings.TrimSpace(params.MessageID),
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TotalTokens:      params.TotalTokens,
		LlmTurns:         params.LLMTurns,
		ToolCalls:        params.ToolCalls,
		LatencyMs:        params.Latency.Milliseconds(),
		Status:           status,
		Error:            strings.TrimSpace(params.Error),
		CreatedAt:        time.Now().UTC().Unix(),
	})
	if err != nil {
		return MessageLog{}, err
	}
	return convertMessageLog(row), nil
}

func (s *Store) ListByGuild(ctx context.Context, guildID string, limit int64) ([]MessageLog, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q.ListLLMMessageLogsByGuild(ctx, s.db, nullableString(guildID), limit)
	if err != nil {
		return nil, err
	}
	out := make([]MessageLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, convertMessageLog(row))
	}
	return out, nil
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func convertMessageLog(row data.LlmMessageLog) MessageLog {
	guildID := ""
	if row.GuildID != nil {
		guildID = *row.GuildID
	}
	return MessageLog{
		ID:               row.ID,
		GuildID:          guildID,
		ChannelID:        row.ChannelID,
		UserID:           row.UserID,
		MessageID:        row.MessageID,
		PromptTokens:     row.PromptTokens,
		CompletionTokens: row.CompletionTokens,
		TotalTokens:      row.TotalTokens,
		LLMTurns:         row.LlmTurns,
		ToolCalls:        row.ToolCalls,
		Latency:          time.Duration(row.LatencyMs) * time.Millisecond,
		Status:           row.Status,
		Error:            row.Error,
		CreatedAt:        time.Unix(row.CreatedAt, 0).UTC(),
	}
}
