package reminders

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type Service struct {
	store *Store
}

type CreateReminderInput struct {
	UserID    string
	ChannelID string
	GuildID   string
	Message   string
	Schedule  string
	At        string
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) CreateReminder(ctx context.Context, input CreateReminderInput) (*Reminder, error) {
	schedule := Schedule(strings.ToLower(strings.TrimSpace(input.Schedule)))
	if schedule != ScheduleOnce && schedule != ScheduleHourly && schedule != ScheduleDaily {
		return nil, errors.New("Invalid schedule. Use once, hourly, or daily.")
	}

	now := time.Now().UTC()
	nextRun, atTime, err := nextRunForSchedule(now, schedule, input.At)
	if err != nil {
		return nil, err
	}

	reminder := &Reminder{
		UserID:    input.UserID,
		ChannelID: input.ChannelID,
		GuildID:   sql.NullString{String: input.GuildID, Valid: input.GuildID != ""},
		Message:   input.Message,
		Schedule:  schedule,
		AtTime:    atTime,
		NextRun:   nextRun,
	}
	if err := s.store.Create(ctx, reminder); err != nil {
		return nil, err
	}
	return reminder, nil
}

func (s *Service) ListUserReminders(ctx context.Context, userID string) ([]Reminder, error) {
	return s.store.ListByUser(ctx, userID)
}

func (s *Service) DeleteReminder(ctx context.Context, id int64, userID string) (bool, error) {
	return s.store.Delete(ctx, id, userID)
}

func nextRunForSchedule(now time.Time, schedule Schedule, at string) (time.Time, sql.NullString, error) {
	now = now.UTC()
	at = strings.TrimSpace(at)

	switch schedule {
	case ScheduleOnce:
		if at == "" {
			nextRun := now.Add(5 * time.Minute)
			return nextRun, sql.NullString{String: nextRun.Format(time.RFC3339), Valid: true}, nil
		}
		parsed, err := parseFlexibleTimeUTC(at)
		if err != nil || !parsed.After(now) {
			return time.Time{}, sql.NullString{}, errors.New("Invalid time. Use 'YYYY-MM-DD HH:MM' or ISO like '2025-01-31T15:04:05Z' (UTC).")
		}
		return parsed, sql.NullString{String: parsed.Format(time.RFC3339), Valid: true}, nil
	case ScheduleHourly:
		if at == "" {
			return now.Truncate(time.Minute).Add(time.Minute), sql.NullString{}, nil
		}
		parsed, err := parseHourMinuteUTC(now, at)
		if err != nil || !parsed.After(now) {
			return now.Add(time.Hour), sql.NullString{}, nil
		}
		return parsed, sql.NullString{}, nil
	case ScheduleDaily:
		hhmm := at
		if hhmm == "" {
			nm := now.Truncate(time.Minute).Add(time.Minute)
			hhmm = nm.Format("15:04")
		}
		if len(hhmm) != 5 || hhmm[2] != ':' {
			return time.Time{}, sql.NullString{}, errors.New("Use HH:MM (UTC), e.g., 09:00.")
		}
		hour := (int(hhmm[0]-'0') * 10) + int(hhmm[1]-'0')
		min := (int(hhmm[3]-'0') * 10) + int(hhmm[4]-'0')
		nextRun := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)
		if !nextRun.After(now) {
			nextRun = nextRun.Add(24 * time.Hour)
		}
		return nextRun, sql.NullString{String: hhmm, Valid: true}, nil
	default:
		return time.Time{}, sql.NullString{}, errors.New("unknown schedule")
	}
}

func parseFlexibleTimeUTC(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	const layout = "2006-01-02 15:04"
	if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, errors.New("bad time format")
}

func parseHourMinuteUTC(now time.Time, s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) == 3 && s[0] == ':' {
		min := (int(s[1]-'0') * 10) + int(s[2]-'0')
		if min < 0 || min > 59 {
			return time.Time{}, errors.New("bad minutes")
		}
		base := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)
		t := base.Add(time.Duration(min) * time.Minute)
		if !t.After(now) {
			t = base.Add(time.Hour).Add(time.Duration(min) * time.Minute)
		}
		return t, nil
	}
	if len(s) == 5 && s[2] == ':' {
		hour := (int(s[0]-'0') * 10) + int(s[1]-'0')
		min := (int(s[3]-'0') * 10) + int(s[4]-'0')
		if hour < 0 || hour > 23 || min < 0 || min > 59 {
			return time.Time{}, errors.New("bad hhmm")
		}
		t := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)
		if !t.After(now) {
			t = t.Add(24 * time.Hour)
		}
		return t, nil
	}
	return time.Time{}, errors.New("bad format")
}
