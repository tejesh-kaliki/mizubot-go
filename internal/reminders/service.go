package reminders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
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
	CronExpr  string
	Timezone  string
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) CreateReminder(ctx context.Context, input CreateReminderInput) (*Reminder, error) {
	schedule := Schedule(strings.ToLower(strings.TrimSpace(input.Schedule)))
	if schedule != ScheduleOnce && schedule != ScheduleHourly && schedule != ScheduleDaily && schedule != ScheduleCron {
		return nil, errors.New("Invalid schedule. Use once, hourly, daily, or cron.")
	}

	normalized, err := normalizeSchedule(time.Now().UTC(), normalizeInput{
		Schedule: schedule,
		At:       input.At,
		CronExpr: input.CronExpr,
		Timezone: input.Timezone,
	})
	if err != nil {
		return nil, err
	}

	reminder := &Reminder{
		UserID:    input.UserID,
		ChannelID: input.ChannelID,
		GuildID:   sql.NullString{String: input.GuildID, Valid: input.GuildID != ""},
		Message:   input.Message,
		Schedule:  schedule,
		AtTime:    normalized.AtTime,
		CronExpr:  normalized.CronExpr,
		Once:      normalized.Once,
		Timezone:  normalized.Timezone,
		NextRun:   normalized.NextRun,
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
	_, ok, err := s.DeleteReminderWithDetails(ctx, id, userID)
	return ok, err
}

func (s *Service) DeleteReminderWithDetails(ctx context.Context, id int64, userID string) (Reminder, bool, error) {
	reminder, ok, err := s.store.GetOwned(ctx, id, userID)
	if err != nil || !ok {
		return Reminder{}, ok, err
	}
	deleted, err := s.store.Delete(ctx, id, userID)
	if err != nil {
		return Reminder{}, false, err
	}
	if !deleted {
		return Reminder{}, false, nil
	}
	return reminder, true, nil
}

type normalizeInput struct {
	Schedule Schedule
	At       string
	CronExpr string
	Timezone string
}

type normalizedSchedule struct {
	AtTime   sql.NullString
	CronExpr string
	Once     bool
	Timezone string
	NextRun  time.Time
}

func normalizeSchedule(now time.Time, input normalizeInput) (normalizedSchedule, error) {
	timezone := strings.TrimSpace(input.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return normalizedSchedule{}, errors.New("invalid timezone")
	}

	nextRun, atTime, err := nextRunForSchedule(now, input.Schedule, input.At, input.CronExpr, loc)
	if err != nil {
		return normalizedSchedule{}, err
	}

	cronExpr := ""
	if input.Schedule != ScheduleOnce {
		cronExpr, err = cronForSchedule(now, input.Schedule, input.At, input.CronExpr, loc)
		if err != nil {
			return normalizedSchedule{}, err
		}
	}

	return normalizedSchedule{
		AtTime:   atTime,
		CronExpr: cronExpr,
		Once:     input.Schedule == ScheduleOnce,
		Timezone: loc.String(),
		NextRun:  nextRun,
	}, nil
}

func nextRunForSchedule(now time.Time, schedule Schedule, at, cronExpr string, loc *time.Location) (time.Time, sql.NullString, error) {
	now = now.UTC()
	at = strings.TrimSpace(at)

	switch schedule {
	case ScheduleOnce:
		if at == "" {
			nextRun := now.Add(5 * time.Minute)
			return nextRun, sql.NullString{String: nextRun.Format(time.RFC3339), Valid: true}, nil
		}
		if duration, ok := parseRelativeDuration(at); ok {
			nextRun := now.Add(duration)
			return nextRun, sql.NullString{String: nextRun.Format(time.RFC3339), Valid: true}, nil
		}
		parsed, err := parseFlexibleTime(at, loc)
		if err != nil || !parsed.After(now) {
			return time.Time{}, sql.NullString{}, errors.New("Invalid time. For once, use a duration like '10m', '2h', '3d', or an absolute time like '2025-01-31 15:04'.")
		}
		return parsed, sql.NullString{String: parsed.Format(time.RFC3339), Valid: true}, nil
	case ScheduleHourly:
		expr, err := cronForSchedule(now, schedule, at, "", loc)
		if err != nil {
			return time.Time{}, sql.NullString{}, err
		}
		nextRun, err := nextRunForCron(now, expr, loc)
		return nextRun, sql.NullString{}, err
	case ScheduleDaily:
		hhmm := at
		if hhmm == "" {
			hhmm = now.In(loc).Truncate(time.Minute).Add(time.Minute).Format("15:04")
		}
		expr, err := cronForSchedule(now, schedule, hhmm, "", loc)
		if err != nil {
			return time.Time{}, sql.NullString{}, err
		}
		nextRun, err := nextRunForCron(now, expr, loc)
		if err != nil {
			return time.Time{}, sql.NullString{}, err
		}
		return nextRun, sql.NullString{String: hhmm, Valid: true}, nil
	case ScheduleCron:
		expr := strings.TrimSpace(cronExpr)
		if expr == "" {
			expr = at
		}
		nextRun, err := nextRunForCron(now, expr, loc)
		if err != nil {
			return time.Time{}, sql.NullString{}, err
		}
		return nextRun, sql.NullString{}, nil
	default:
		return time.Time{}, sql.NullString{}, errors.New("unknown schedule")
	}
}

func cronForSchedule(now time.Time, schedule Schedule, at, cronExpr string, loc *time.Location) (string, error) {
	at = strings.TrimSpace(at)
	switch schedule {
	case ScheduleHourly:
		minute := now.In(loc).Truncate(time.Minute).Add(time.Minute).Minute()
		if at != "" {
			parsed, err := parseHourMinute(now, at, loc)
			if err != nil {
				return "", errors.New("Use :MM for hourly reminders, e.g., :15.")
			}
			minute = parsed.In(loc).Minute()
		}
		return fmt.Sprintf("%d * * * *", minute), nil
	case ScheduleDaily:
		if at == "" {
			at = now.In(loc).Truncate(time.Minute).Add(time.Minute).Format("15:04")
		}
		hour, minute, err := parseHHMM(at)
		if err != nil {
			return "", errors.New("Use HH:MM, e.g., 09:00.")
		}
		return fmt.Sprintf("%d %d * * *", minute, hour), nil
	case ScheduleCron:
		expr := strings.TrimSpace(cronExpr)
		if expr == "" {
			expr = at
		}
		if _, err := cron.ParseStandard(expr); err != nil {
			return "", err
		}
		return expr, nil
	default:
		return "", nil
	}
}

func nextRunForCron(now time.Time, expr string, loc *time.Location) (time.Time, error) {
	schedule, err := cron.ParseStandard(expr)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(now.In(loc)).UTC(), nil
}

func parseFlexibleTime(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	const layout = "2006-01-02 15:04"
	if t, err := time.ParseInLocation(layout, s, loc); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, errors.New("bad time format")
}

func parseHourMinute(now time.Time, s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) == 3 && s[0] == ':' {
		min := (int(s[1]-'0') * 10) + int(s[2]-'0')
		if min < 0 || min > 59 {
			return time.Time{}, errors.New("bad minutes")
		}
		localNow := now.In(loc)
		base := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), localNow.Hour(), 0, 0, 0, loc)
		t := base.Add(time.Duration(min) * time.Minute)
		if !t.After(localNow) {
			t = base.Add(time.Hour).Add(time.Duration(min) * time.Minute)
		}
		return t.UTC(), nil
	}
	if len(s) == 5 && s[2] == ':' {
		hour, min, err := parseHHMM(s)
		if err != nil {
			return time.Time{}, errors.New("bad hhmm")
		}
		localNow := now.In(loc)
		t := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, min, 0, 0, loc)
		if !t.After(localNow) {
			t = t.Add(24 * time.Hour)
		}
		return t.UTC(), nil
	}
	return time.Time{}, errors.New("bad format")
}

func parseHHMM(s string) (int, int, error) {
	if len(s) != 5 || s[2] != ':' {
		return 0, 0, errors.New("bad hhmm")
	}
	hour := (int(s[0]-'0') * 10) + int(s[1]-'0')
	min := (int(s[3]-'0') * 10) + int(s[4]-'0')
	if hour < 0 || hour > 23 || min < 0 || min > 59 {
		return 0, 0, errors.New("bad hhmm")
	}
	return hour, min, nil
}

func parseFlexibleTimeUTC(s string) (time.Time, error) {
	return parseFlexibleTime(s, time.UTC)
}

func parseHourMinuteUTC(now time.Time, s string) (time.Time, error) {
	return parseHourMinute(now, s, time.UTC)
}

func legacyNextRunForSchedule(now time.Time, schedule Schedule, at string) (time.Time, sql.NullString, error) {
	return nextRunForSchedule(now, schedule, at, "", time.UTC)
}

func parseRelativeDuration(s string) (time.Duration, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "in ")
	if s == "" {
		return 0, false
	}
	if before, ok := strings.CutSuffix(s, "d"); ok {
		days, err := time.ParseDuration(before + "h")
		if err != nil || days <= 0 {
			return 0, false
		}
		return days * 24, true
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}
