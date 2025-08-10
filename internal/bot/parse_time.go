package bot

import (
	"errors"
	"strings"
	"time"
)

// parseFlexibleTimeUTC parses either RFC3339 or "YYYY-MM-DD HH:MM" in UTC.
func parseFlexibleTimeUTC(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	// try "YYYY-MM-DD HH:MM"
	const layout = "2006-01-02 15:04"
	if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, errors.New("bad time format")
}

// parseHourMinuteUTC parses either ":MM" (next top of hour plus minutes) or "HH:MM" on the same day in UTC.
func parseHourMinuteUTC(now time.Time, s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) == 3 && s[0] == ':' { // ":MM"
		min := (int(s[1]-'0')*10 + int(s[2]-'0'))
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
	if len(s) == 5 && s[2] == ':' { // "HH:MM"
		hour := (int(s[0]-'0')*10 + int(s[1]-'0'))
		min := (int(s[3]-'0')*10 + int(s[4]-'0'))
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
