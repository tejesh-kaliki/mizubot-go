package usersettings

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

const DefaultTimezone = "UTC"

var timezoneAliases = map[string]string{
	"calcutta":           "Asia/Kolkata",
	"central":            "America/Chicago",
	"eastern":            "America/New_York",
	"india":              "Asia/Kolkata",
	"ist":                "Asia/Kolkata",
	"japan":              "Asia/Tokyo",
	"jst":                "Asia/Tokyo",
	"kolkata":            "Asia/Kolkata",
	"london":             "Europe/London",
	"los angeles":        "America/Los_Angeles",
	"mountain":           "America/Denver",
	"new york":           "America/New_York",
	"pacific":            "America/Los_Angeles",
	"san francisco":      "America/Los_Angeles",
	"tokyo":              "Asia/Tokyo",
	"uk":                 "Europe/London",
	"united kingdom":     "Europe/London",
	"us central":         "America/Chicago",
	"us eastern":         "America/New_York",
	"us mountain":        "America/Denver",
	"us pacific":         "America/Los_Angeles",
	"washington dc":      "America/New_York",
	"washington d.c.":    "America/New_York",
	"washington, dc":     "America/New_York",
	"washington, d.c.":   "America/New_York",
	"washington dc usa":  "America/New_York",
	"washington d.c. us": "America/New_York",
}

var commonTimezones = []string{
	"UTC",
	"Asia/Kolkata",
	"Asia/Tokyo",
	"Asia/Singapore",
	"Asia/Dubai",
	"Asia/Shanghai",
	"Australia/Sydney",
	"Europe/London",
	"Europe/Paris",
	"Europe/Berlin",
	"America/New_York",
	"America/Chicago",
	"America/Denver",
	"America/Los_Angeles",
	"America/Toronto",
	"America/Vancouver",
}

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) GetTimezone(ctx context.Context, userID string) (string, bool, error) {
	if strings.TrimSpace(userID) == "" {
		return "", false, errors.New("missing user id")
	}
	settings, ok, err := s.store.Get(ctx, userID)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return DefaultTimezone, false, nil
	}
	return settings.Timezone, true, nil
}

func (s *Service) SetTimezone(ctx context.Context, userID, timezone string) (Settings, error) {
	if strings.TrimSpace(userID) == "" {
		return Settings{}, errors.New("missing user id")
	}
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		return Settings{}, errors.New("timezone is required")
	}
	resolved, err := ResolveTimezone(timezone)
	if err != nil {
		return Settings{}, err
	}
	return s.store.SetTimezone(ctx, userID, resolved)
}

func ResolveTimezone(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("timezone is required")
	}
	if loc, err := time.LoadLocation(input); err == nil {
		return loc.String(), nil
	}

	normalized := normalizeTimezoneInput(input)
	if alias, ok := timezoneAliases[normalized]; ok {
		return alias, nil
	}

	matches := matchingTimezones(normalized)
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", errors.New("timezone is ambiguous; use an IANA timezone like Asia/Kolkata or America/Los_Angeles")
	}
	return "", errors.New("invalid timezone; use an IANA timezone like Asia/Kolkata or America/Los_Angeles")
}

func matchingTimezones(normalized string) []string {
	var matches []string
	for _, timezone := range commonTimezones {
		candidate := normalizeTimezoneInput(timezone)
		if strings.Contains(candidate, normalized) {
			matches = append(matches, timezone)
		}
	}
	sort.Strings(matches)
	return matches
}

func normalizeTimezoneInput(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	input = strings.ReplaceAll(input, "_", " ")
	input = strings.ReplaceAll(input, "/", " ")
	input = strings.ReplaceAll(input, "-", " ")
	input = strings.Join(strings.Fields(input), " ")
	return input
}
