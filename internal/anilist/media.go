// Package anilist is a small read-only client for the public AniList
// GraphQL API (https://anilist.gitbook.io/anilist-apiv2-docs), limited to
// anime lookups. No API key is needed for these queries.
package anilist

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Media is one anime entry, reduced to the fields the bot uses.
type Media struct {
	ID           int
	Title        Title
	Format       string
	Status       string
	Episodes     int // 0 when AniList doesn't know
	Duration     int // minutes per episode, 0 when unknown
	Season       string
	SeasonYear   int
	StartDate    Date
	EndDate      Date
	Genres       []string
	AverageScore int // 0-100, 0 when unrated
	Description  string
	SiteURL      string
	CoverURL     string
	CoverColor   string // "#rrggbb" or empty
	Studios      []string
	NextAiring   *Airing
}

type Title struct {
	Romaji  string
	English string
	Native  string
}

// Display is the title to show first: English when AniList has one,
// otherwise romaji.
func (t Title) Display() string {
	if t.English != "" {
		return t.English
	}
	if t.Romaji != "" {
		return t.Romaji
	}
	return t.Native
}

// Date is a possibly partial calendar date; zero fields mean unknown.
type Date struct {
	Year, Month, Day int
}

func (d Date) IsZero() bool { return d.Year == 0 }

func (d Date) String() string {
	switch {
	case d.Year == 0:
		return ""
	case d.Month == 0:
		return strconv.Itoa(d.Year)
	case d.Day == 0:
		return strconv.Itoa(d.Year) + "-" + pad2(d.Month)
	default:
		return strconv.Itoa(d.Year) + "-" + pad2(d.Month) + "-" + pad2(d.Day)
	}
}

// Airing is the next episode to air.
type Airing struct {
	Episode int
	At      time.Time
}

// A pick is decisive only when the winner clears PickMinProbability and
// leads the runner-up by at least PickMinMargin. Anything less goes back to
// the LLM as a candidate list for the user to settle.
const (
	PickMinProbability = 0.5
	PickMinMargin      = 0.15
)

// Pick is a Jev judgement over a candidate list: the winning media ID with
// its probability, plus the runner-up's probability so callers can tell a
// clear win from a coin flip.
type Pick struct {
	ID       int
	Top      float64
	RunnerUp float64
}

// Decisive reports whether the pick is confident enough to act on without
// asking the user.
func (p Pick) Decisive() bool {
	return p.ID != 0 && p.Top >= PickMinProbability && p.Top-p.RunnerUp >= PickMinMargin
}

var (
	tagPattern        = regexp.MustCompile(`<[^>]*>`)
	lineBreakPattern  = regexp.MustCompile(`(?i)<\s*br\s*/?\s*>`)
	blankLinesPattern = regexp.MustCompile(`\n{3,}`)
	spacesPattern     = regexp.MustCompile(`[ \t]+`)
)

// CleanDescription turns AniList's HTML-ish description into plain text:
// line breaks kept, other tags dropped, entities decoded.
func CleanDescription(raw string) string {
	s := lineBreakPattern.ReplaceAllString(raw, "\n")
	s = tagPattern.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = spacesPattern.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	s = strings.Join(lines, "\n")
	s = blankLinesPattern.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// Truncate cuts s to at most max runes, ending in an ellipsis when it had
// to cut. It prefers to cut at a word boundary.
func Truncate(s string, max int) string {
	runes := []rune(s)
	if max <= 0 || len(runes) <= max {
		return s
	}
	cut := runes[:max-1]
	if idx := lastSpace(cut); idx > max/2 {
		cut = cut[:idx]
	}
	return strings.TrimRight(string(cut), " \n.,;:") + "…"
}

func lastSpace(r []rune) int {
	for i := len(r) - 1; i >= 0; i-- {
		if r[i] == ' ' || r[i] == '\n' {
			return i
		}
	}
	return -1
}

func pad2(n int) string {
	return fmt.Sprintf("%02d", n)
}
