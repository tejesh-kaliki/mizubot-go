// Package typesafestats records every TypeSafe classification request and
// response, so the jev model's routing behavior (and token usage) can be
// inspected after the fact.
package typesafestats

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

type ClassificationLog struct {
	ID               int64
	GuildID          string
	ChannelID        string
	UserID           string
	Model            string
	RequestState     string
	RequestQuestions string
	ResponseAnswers  string
	SelectedTools    string
	InputTokens      int64
	OutputTokens     int64
	Latency          time.Duration
	Status           string
	Error            string
	CreatedAt        time.Time
}

type CreateClassificationLogParams struct {
	GuildID          string
	ChannelID        string
	UserID           string
	Model            string
	RequestState     string
	RequestQuestions string
	ResponseAnswers  string
	SelectedTools    string
	InputTokens      int64
	OutputTokens     int64
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

func (s *Store) Create(ctx context.Context, params CreateClassificationLogParams) (ClassificationLog, error) {
	status := strings.TrimSpace(params.Status)
	if status == "" {
		status = StatusSuccess
	}

	row, err := s.q.CreateTypeSafeClassificationLog(ctx, s.db, data.CreateTypeSafeClassificationLogParams{
		GuildID:          nullableString(params.GuildID),
		ChannelID:        strings.TrimSpace(params.ChannelID),
		UserID:           strings.TrimSpace(params.UserID),
		Model:            strings.TrimSpace(params.Model),
		RequestState:     params.RequestState,
		RequestQuestions: params.RequestQuestions,
		ResponseAnswers:  params.ResponseAnswers,
		SelectedTools:    params.SelectedTools,
		InputTokens:      params.InputTokens,
		OutputTokens:     params.OutputTokens,
		LatencyMs:        params.Latency.Milliseconds(),
		Status:           status,
		Error:            strings.TrimSpace(params.Error),
		CreatedAt:        time.Now().UTC().Unix(),
	})
	if err != nil {
		return ClassificationLog{}, err
	}
	return convertClassificationLog(row), nil
}

func (s *Store) ListByGuild(ctx context.Context, guildID string, limit int64) ([]ClassificationLog, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q.ListTypeSafeClassificationLogsByGuild(ctx, s.db, nullableString(guildID), limit)
	if err != nil {
		return nil, err
	}
	out := make([]ClassificationLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, convertClassificationLog(row))
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

func convertClassificationLog(row data.TypesafeClassificationLog) ClassificationLog {
	guildID := ""
	if row.GuildID != nil {
		guildID = *row.GuildID
	}
	return ClassificationLog{
		ID:               row.ID,
		GuildID:          guildID,
		ChannelID:        row.ChannelID,
		UserID:           row.UserID,
		Model:            row.Model,
		RequestState:     row.RequestState,
		RequestQuestions: row.RequestQuestions,
		ResponseAnswers:  row.ResponseAnswers,
		SelectedTools:    row.SelectedTools,
		InputTokens:      row.InputTokens,
		OutputTokens:     row.OutputTokens,
		Latency:          time.Duration(row.LatencyMs) * time.Millisecond,
		Status:           row.Status,
		Error:            row.Error,
		CreatedAt:        time.Unix(row.CreatedAt, 0).UTC(),
	}
}
