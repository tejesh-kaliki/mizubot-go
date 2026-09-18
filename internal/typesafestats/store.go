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

// Kinds of judgement logged in this table.
const (
	KindClassifier  = "classifier"   // tool routing and guild flag classification
	KindAniListPick = "anilist_pick" // choosing among AniList search candidates
	KindEmbedFilter = "embed_filter" // deciding whether a tool's embed is attached
)

type ClassificationLog struct {
	ID               int64
	GuildID          string
	ChannelID        string
	UserID           string
	MessageID        string
	Model            string
	RequestState     string
	RequestQuestions string
	ResponseAnswers  string
	SelectedTools    string
	MatchedFlags     string
	Kind             string
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
	MessageID        string
	Model            string
	RequestState     string
	RequestQuestions string
	ResponseAnswers  string
	SelectedTools    string
	MatchedFlags     string
	// Kind says which judgement this row logs; empty means KindClassifier.
	Kind         string
	InputTokens  int64
	OutputTokens int64
	Latency      time.Duration
	Status       string
	Error        string
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

	kind := strings.TrimSpace(params.Kind)
	if kind == "" {
		kind = KindClassifier
	}

	row, err := s.q.CreateTypeSafeClassificationLog(ctx, s.db, data.CreateTypeSafeClassificationLogParams{
		GuildID:          nullableString(params.GuildID),
		ChannelID:        strings.TrimSpace(params.ChannelID),
		UserID:           strings.TrimSpace(params.UserID),
		MessageID:        strings.TrimSpace(params.MessageID),
		Model:            strings.TrimSpace(params.Model),
		RequestState:     params.RequestState,
		RequestQuestions: params.RequestQuestions,
		ResponseAnswers:  params.ResponseAnswers,
		SelectedTools:    params.SelectedTools,
		MatchedFlags:     params.MatchedFlags,
		Kind:             kind,
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
		MessageID:        row.MessageID,
		Model:            row.Model,
		RequestState:     row.RequestState,
		RequestQuestions: row.RequestQuestions,
		ResponseAnswers:  row.ResponseAnswers,
		SelectedTools:    row.SelectedTools,
		MatchedFlags:     row.MatchedFlags,
		Kind:             row.Kind,
		InputTokens:      row.InputTokens,
		OutputTokens:     row.OutputTokens,
		Latency:          time.Duration(row.LatencyMs) * time.Millisecond,
		Status:           row.Status,
		Error:            row.Error,
		CreatedAt:        time.Unix(row.CreatedAt, 0).UTC(),
	}
}
