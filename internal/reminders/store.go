package reminders

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"mizubot-go/internal/data"

	"github.com/robfig/cron/v3"
)

type Schedule string

const (
	ScheduleOnce   Schedule = "once"
	ScheduleHourly Schedule = "hourly"
	ScheduleDaily  Schedule = "daily"
	ScheduleCron   Schedule = "cron"
)

type Reminder struct {
	ID        int64
	UserID    string
	ChannelID string
	GuildID   sql.NullString
	Message   string
	Schedule  Schedule
	AtTime    sql.NullString // RFC3339 for once; HH:MM for daily
	CronExpr  string
	Once      bool
	Timezone  string
	NextRun   time.Time
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

func (s *Store) Create(ctx context.Context, r *Reminder) error {
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now
	var guildIDPtr *string
	if r.GuildID.Valid {
		v := r.GuildID.String
		guildIDPtr = &v
	}
	var atPtr *string
	if r.AtTime.Valid {
		v := r.AtTime.String
		atPtr = &v
	}
	created, err := s.q.CreateReminder(ctx, s.db, data.CreateReminderParams{
		UserID:    r.UserID,
		ChannelID: r.ChannelID,
		GuildID:   guildIDPtr,
		Message:   r.Message,
		Schedule:  string(r.Schedule),
		AtTime:    atPtr,
		CronExpr:  r.CronExpr,
		Once:      boolToInt64(r.Once),
		Timezone:  timezoneOrUTC(r.Timezone),
		NextRun:   r.NextRun.UTC().Unix(),
		CreatedAt: r.CreatedAt.Unix(),
		UpdatedAt: r.UpdatedAt.Unix(),
	})
	if err != nil {
		return err
	}
	*r = convertCreateReminderRow(created)
	return nil
}

func (s *Store) ListByUser(ctx context.Context, userID string) ([]Reminder, error) {
	recs, err := s.q.ListByUser(ctx, s.db, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Reminder, 0, len(recs))
	for _, it := range recs {
		out = append(out, convertListByUserRow(it))
	}
	return out, nil
}

func (s *Store) Delete(ctx context.Context, id int64, userID string) (bool, error) {
	n, err := s.q.DeleteOwned(ctx, s.db, id, userID)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Store) GetOwned(ctx context.Context, id int64, userID string) (Reminder, bool, error) {
	row, err := s.q.GetOwned(ctx, s.db, id, userID)
	if err == sql.ErrNoRows {
		return Reminder{}, false, nil
	}
	if err != nil {
		return Reminder{}, false, err
	}
	return convertGetOwnedRow(row), true, nil
}

func (s *Store) Due(ctx context.Context, now time.Time, limit int) ([]Reminder, error) {
	recs, err := s.q.ListDue(ctx, s.db, now.UTC().Unix(), int64(limit))
	if err != nil {
		return nil, err
	}
	out := make([]Reminder, 0, len(recs))
	for _, it := range recs {
		out = append(out, convertListDueRow(it))
	}
	return out, nil
}

func (s *Store) SetNextRun(ctx context.Context, id int64, t time.Time) error {
	return s.q.SetNextRun(ctx, s.db, data.SetNextRunParams{NextRun: t.UTC().Unix(), UpdatedAt: time.Now().UTC().Unix(), ID: id})
}

func (s *Store) DeleteID(ctx context.Context, id int64) error {
	return s.q.DeleteByID(ctx, s.db, id)
}

func NextAfter(r Reminder, from time.Time) (time.Time, bool, error) {
	if r.Once || r.Schedule == ScheduleOnce {
		return time.Time{}, false, nil
	}
	if r.CronExpr != "" {
		loc, err := time.LoadLocation(timezoneOrUTC(r.Timezone))
		if err != nil {
			return time.Time{}, false, err
		}
		schedule, err := cron.ParseStandard(r.CronExpr)
		if err != nil {
			return time.Time{}, false, err
		}
		return schedule.Next(from.In(loc)).UTC(), true, nil
	}

	switch r.Schedule {
	case ScheduleOnce:
		return time.Time{}, false, nil
	case ScheduleHourly:
		return from.Add(time.Hour), true, nil
	case ScheduleDaily:
		if !r.AtTime.Valid {
			return time.Time{}, false, errors.New("daily schedule missing at_time")
		}
		// parse HH:MM
		at := r.AtTime.String
		if len(at) != 5 || at[2] != ':' {
			return time.Time{}, false, errors.New("invalid daily at_time format, want HH:MM")
		}
		hour := (int(at[0]-'0')*10 + int(at[1]-'0'))
		min := (int(at[3]-'0')*10 + int(at[4]-'0'))
		next := time.Date(from.Year(), from.Month(), from.Day(), hour, min, 0, 0, time.UTC)
		if !next.After(from) {
			next = next.Add(24 * time.Hour)
		}
		return next, true, nil
	default:
		return time.Time{}, false, errors.New("unknown schedule")
	}
}

func convertCreateReminderRow(m data.CreateReminderRow) Reminder {
	return Reminder{
		ID:        m.ID,
		UserID:    m.UserID,
		ChannelID: m.ChannelID,
		GuildID:   nullStringFromPtr(m.GuildID),
		Message:   m.Message,
		Schedule:  Schedule(m.Schedule),
		AtTime:    nullStringFromPtr(m.AtTime),
		CronExpr:  m.CronExpr,
		Once:      m.Once != 0,
		Timezone:  timezoneOrUTC(m.Timezone),
		NextRun:   time.Unix(m.NextRun, 0).UTC(),
		CreatedAt: time.Unix(m.CreatedAt, 0).UTC(),
		UpdatedAt: time.Unix(m.UpdatedAt, 0).UTC(),
	}
}

func convertListByUserRow(m data.ListByUserRow) Reminder {
	return Reminder{
		ID:        m.ID,
		UserID:    m.UserID,
		ChannelID: m.ChannelID,
		GuildID:   nullStringFromPtr(m.GuildID),
		Message:   m.Message,
		Schedule:  Schedule(m.Schedule),
		AtTime:    nullStringFromPtr(m.AtTime),
		CronExpr:  m.CronExpr,
		Once:      m.Once != 0,
		Timezone:  timezoneOrUTC(m.Timezone),
		NextRun:   time.Unix(m.NextRun, 0).UTC(),
		CreatedAt: time.Unix(m.CreatedAt, 0).UTC(),
		UpdatedAt: time.Unix(m.UpdatedAt, 0).UTC(),
	}
}

func convertListDueRow(m data.ListDueRow) Reminder {
	return Reminder{
		ID:        m.ID,
		UserID:    m.UserID,
		ChannelID: m.ChannelID,
		GuildID:   nullStringFromPtr(m.GuildID),
		Message:   m.Message,
		Schedule:  Schedule(m.Schedule),
		AtTime:    nullStringFromPtr(m.AtTime),
		CronExpr:  m.CronExpr,
		Once:      m.Once != 0,
		Timezone:  timezoneOrUTC(m.Timezone),
		NextRun:   time.Unix(m.NextRun, 0).UTC(),
		CreatedAt: time.Unix(m.CreatedAt, 0).UTC(),
		UpdatedAt: time.Unix(m.UpdatedAt, 0).UTC(),
	}
}

func convertGetOwnedRow(m data.GetOwnedRow) Reminder {
	return Reminder{
		ID:        m.ID,
		UserID:    m.UserID,
		ChannelID: m.ChannelID,
		GuildID:   nullStringFromPtr(m.GuildID),
		Message:   m.Message,
		Schedule:  Schedule(m.Schedule),
		AtTime:    nullStringFromPtr(m.AtTime),
		CronExpr:  m.CronExpr,
		Once:      m.Once != 0,
		Timezone:  timezoneOrUTC(m.Timezone),
		NextRun:   time.Unix(m.NextRun, 0).UTC(),
		CreatedAt: time.Unix(m.CreatedAt, 0).UTC(),
		UpdatedAt: time.Unix(m.UpdatedAt, 0).UTC(),
	}
}

func nullStringFromPtr(v *string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}

func boolToInt64(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func timezoneOrUTC(timezone string) string {
	if timezone == "" {
		return "UTC"
	}
	return timezone
}
