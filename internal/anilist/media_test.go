package anilist

import (
	"strings"
	"testing"
)

func TestCleanDescription(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"line breaks", "One<br>Two<br><br><br><br>Three", "One\nTwo\n\nThree"},
		{"tags stripped", "It is <i>very</i> <b>good</b>", "It is very good"},
		{"entities", "Tom &amp; Jerry &quot;live&quot;", `Tom & Jerry "live"`},
		{"self closing br", "A<br/>B<BR />C", "A\nB\nC"},
		{"whitespace", "  a   b  ", "a b"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanDescription(tt.in); got != tt.want {
				t.Fatalf("CleanDescription(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("short", 10); got != "short" {
		t.Fatalf("no-op truncate = %q", got)
	}
	long := strings.Repeat("word ", 50)
	got := Truncate(long, 40)
	if n := len([]rune(got)); n > 40 {
		t.Fatalf("len = %d, want <= 40", n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated text should end with an ellipsis: %q", got)
	}
	if strings.Contains(strings.TrimSuffix(got, "…"), "wor…") {
		t.Fatalf("should cut at a word boundary: %q", got)
	}

	// Multi-byte text is cut by rune, not byte.
	jp := strings.Repeat("進撃の巨人", 20)
	if n := len([]rune(Truncate(jp, 12))); n > 12 {
		t.Fatalf("rune length = %d, want <= 12", n)
	}
}

func TestTitleDisplayPrefersEnglish(t *testing.T) {
	tests := []struct {
		title Title
		want  string
	}{
		{Title{Romaji: "Shingeki no Kyojin", English: "Attack on Titan"}, "Attack on Titan"},
		{Title{Romaji: "Shingeki no Kyojin"}, "Shingeki no Kyojin"},
		{Title{Native: "進撃の巨人"}, "進撃の巨人"},
	}
	for _, tt := range tests {
		if got := tt.title.Display(); got != tt.want {
			t.Errorf("Display(%+v) = %q, want %q", tt.title, got, tt.want)
		}
	}
}

func TestDateString(t *testing.T) {
	tests := []struct {
		d    Date
		want string
	}{
		{Date{}, ""},
		{Date{Year: 2013}, "2013"},
		{Date{Year: 2013, Month: 4}, "2013-04"},
		{Date{Year: 2013, Month: 4, Day: 7}, "2013-04-07"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("%+v.String() = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestPickDecisive(t *testing.T) {
	tests := []struct {
		name string
		pick Pick
		want bool
	}{
		{"clear win", Pick{ID: 1, Top: 0.9, RunnerUp: 0.05}, true},
		{"low probability", Pick{ID: 1, Top: 0.4, RunnerUp: 0.1}, false},
		{"too close", Pick{ID: 1, Top: 0.55, RunnerUp: 0.45}, false},
		{"exactly at margin", Pick{ID: 1, Top: 0.65, RunnerUp: 0.5}, true},
		{"no id", Pick{Top: 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pick.Decisive(); got != tt.want {
				t.Fatalf("Decisive() = %v, want %v", got, tt.want)
			}
		})
	}
}
