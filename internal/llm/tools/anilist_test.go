package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"mizubot-go/internal/anilist"
	"mizubot-go/internal/llm"
)

type fakeAnimeSource struct {
	searchResults []anilist.Media
	searchErr     error
	byID          map[int]anilist.Media
	searchCalls   int
	byIDCalls     int
	lastQuery     string
}

func (f *fakeAnimeSource) Search(_ context.Context, query string, limit int) ([]anilist.Media, error) {
	f.searchCalls++
	f.lastQuery = query
	if limit != anilistSearchLimit {
		return nil, fmt.Errorf("limit = %d, want %d", limit, anilistSearchLimit)
	}
	return f.searchResults, f.searchErr
}

func (f *fakeAnimeSource) ByID(_ context.Context, id int) (anilist.Media, error) {
	f.byIDCalls++
	if m, ok := f.byID[id]; ok {
		return m, nil
	}
	for _, m := range f.searchResults {
		if m.ID == id {
			return m, nil
		}
	}
	return anilist.Media{}, fmt.Errorf("no anime with id %d", id)
}

type fakePicker struct {
	pick   anilist.Pick
	err    error
	calls  int
	gotCtx llm.ToolContext
}

func (f *fakePicker) Pick(_ context.Context, toolCtx llm.ToolContext, _ string, _ []anilist.Media) (anilist.Pick, error) {
	f.calls++
	f.gotCtx = toolCtx
	return f.pick, f.err
}

func testMedia(id int, romaji, english string) anilist.Media {
	return anilist.Media{
		ID:           id,
		Title:        anilist.Title{Romaji: romaji, English: english},
		Format:       "TV",
		Status:       "FINISHED",
		Episodes:     12,
		Season:       "SPRING",
		SeasonYear:   2013,
		Genres:       []string{"Action", "Drama"},
		AverageScore: 84,
		Description:  "A description.",
		SiteURL:      fmt.Sprintf("https://anilist.co/anime/%d", id),
		CoverURL:     "https://img/cover.jpg",
		CoverColor:   "#ff8800",
		Studios:      []string{"Wit Studio"},
	}
}

func toolByName(t *testing.T, tools []llm.Tool, name string) llm.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return llm.Tool{}
}

func runTool(t *testing.T, tool llm.Tool, args string) (llm.ToolResult, error) {
	t.Helper()
	return tool.Execute(context.Background(), llm.ToolContext{UserID: "u", MessageID: "m1", Message: "user text"}, json.RawMessage(args))
}

func TestNewAniListToolsNilSource(t *testing.T) {
	if got := NewAniListTools(nil, nil); got != nil {
		t.Fatalf("tools = %v, want nil without a source", got)
	}
}

func TestSearchImpliesDetails(t *testing.T) {
	tools := NewAniListTools(&fakeAnimeSource{}, nil)
	search := toolByName(t, tools, "anilist_search")
	if len(search.ImpliesTools) != 1 || search.ImpliesTools[0] != "anilist_details" {
		t.Fatalf("ImpliesTools = %v, want [anilist_details]", search.ImpliesTools)
	}
	toolByName(t, tools, "anilist_details")
	if search.ClassifierHint == "" {
		t.Error("search tool should carry a ClassifierHint")
	}
}

func TestSearchSingleCandidateSkipsPicker(t *testing.T) {
	src := &fakeAnimeSource{searchResults: []anilist.Media{testMedia(1, "Shingeki no Kyojin", "Attack on Titan")}}
	picker := &fakePicker{}
	search := toolByName(t, NewAniListTools(src, picker), "anilist_search")

	res, err := runTool(t, search, `{"query":"aot"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if picker.calls != 0 {
		t.Fatalf("picker calls = %d, want 0 for a single candidate", picker.calls)
	}
	if !strings.Contains(res.Content, "Title: Attack on Titan (romaji: Shingeki no Kyojin)") {
		t.Errorf("content missing title line: %s", res.Content)
	}
	if len(res.Embeds) != 1 || res.Embeds[0].Title != "Attack on Titan" {
		t.Fatalf("embeds = %+v", res.Embeds)
	}
}

func TestSearchDecisivePickReturnsChosenEntry(t *testing.T) {
	src := &fakeAnimeSource{searchResults: []anilist.Media{
		testMedia(1, "Part One", ""),
		testMedia(2, "Part Two", ""),
	}}
	picker := &fakePicker{pick: anilist.Pick{ID: 2, Top: 0.9, RunnerUp: 0.1}}
	search := toolByName(t, NewAniListTools(src, picker), "anilist_search")

	res, err := runTool(t, search, `{"query":"part two"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Content, "AniList ID: 2") || len(res.Embeds) != 1 {
		t.Fatalf("expected entry 2 with an embed, got %q, %d embeds", res.Content, len(res.Embeds))
	}
	if picker.gotCtx.Message != "user text" || picker.gotCtx.MessageID != "m1" {
		t.Errorf("picker should receive the user message context, got %+v", picker.gotCtx)
	}
}

func TestSearchUnsureOrFailedPickReturnsCandidates(t *testing.T) {
	src := func() *fakeAnimeSource {
		return &fakeAnimeSource{searchResults: []anilist.Media{
			testMedia(1, "Part One", ""),
			testMedia(2, "Part Two", ""),
		}}
	}
	tests := []struct {
		name   string
		picker MediaPicker
	}{
		{"low confidence", &fakePicker{pick: anilist.Pick{ID: 1, Top: 0.4, RunnerUp: 0.3}}},
		{"too close", &fakePicker{pick: anilist.Pick{ID: 1, Top: 0.55, RunnerUp: 0.45}}},
		{"picker error", &fakePicker{err: errors.New("jev down")}},
		{"pick outside candidates", &fakePicker{pick: anilist.Pick{ID: 99, Top: 0.9}}},
		{"no picker", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			search := toolByName(t, NewAniListTools(src(), tt.picker), "anilist_search")
			res, err := runTool(t, search, `{"query":"part"}`)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if len(res.Embeds) != 0 {
				t.Errorf("candidate list should carry no embed, got %d", len(res.Embeds))
			}
			for _, want := range []string{"Several anime match", "1. Part One (AniList ID 1)", "2. Part Two (AniList ID 2)"} {
				if !strings.Contains(res.Content, want) {
					t.Errorf("content missing %q:\n%s", want, res.Content)
				}
			}
		})
	}
}

func TestSearchByIDSkipsSearchAndPicker(t *testing.T) {
	src := &fakeAnimeSource{byID: map[int]anilist.Media{7: testMedia(7, "Seven", "")}}
	picker := &fakePicker{}
	search := toolByName(t, NewAniListTools(src, picker), "anilist_search")

	res, err := runTool(t, search, `{"id":7}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if src.searchCalls != 0 || picker.calls != 0 {
		t.Fatalf("search calls = %d, picker calls = %d; want none", src.searchCalls, picker.calls)
	}
	if !strings.Contains(res.Content, "AniList ID: 7") {
		t.Fatalf("content = %s", res.Content)
	}
}

func TestSearchErrors(t *testing.T) {
	search := toolByName(t, NewAniListTools(&fakeAnimeSource{}, nil), "anilist_search")
	if _, err := runTool(t, search, `{}`); err == nil {
		t.Error("expected error when neither query nor id is given")
	}
	if _, err := runTool(t, search, `{"query":`); err == nil {
		t.Error("expected error for malformed arguments")
	}
	if _, err := runTool(t, search, `{"query":"nothing"}`); err == nil || !strings.Contains(err.Error(), "no anime found") {
		t.Errorf("err = %v, want a no-results error", err)
	}

	failing := toolByName(t, NewAniListTools(&fakeAnimeSource{searchErr: errors.New("anilist down")}, nil), "anilist_search")
	if _, err := runTool(t, failing, `{"query":"x"}`); err == nil || !strings.Contains(err.Error(), "anilist down") {
		t.Errorf("err = %v, want the AniList error passed through", err)
	}
}

func TestDetailsUsesResolvedCacheFromSearch(t *testing.T) {
	src := &fakeAnimeSource{searchResults: []anilist.Media{
		testMedia(1, "Part One", ""),
		testMedia(2, "Part Two", ""),
	}}
	picker := &fakePicker{pick: anilist.Pick{ID: 2, Top: 0.9, RunnerUp: 0.05}}
	tools := NewAniListTools(src, picker)

	if _, err := runTool(t, toolByName(t, tools, "anilist_search"), `{"query":"Part  Two"}`); err != nil {
		t.Fatal(err)
	}
	res, err := runTool(t, toolByName(t, tools, "anilist_details"), `{"query":"part two"}`)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if picker.calls != 1 || src.searchCalls != 1 {
		t.Fatalf("picker calls = %d, search calls = %d; want the follow-up to reuse the resolution (1, 1)", picker.calls, src.searchCalls)
	}
	if !strings.Contains(res.Content, "AniList ID: 2") {
		t.Fatalf("details content = %s", res.Content)
	}
	if len(res.Embeds) != 0 {
		t.Errorf("details should not attach an embed, got %d", len(res.Embeds))
	}
}

func TestDetailsResolvesOnCacheMiss(t *testing.T) {
	src := &fakeAnimeSource{searchResults: []anilist.Media{testMedia(5, "Solo", "")}}
	details := toolByName(t, NewAniListTools(src, nil), "anilist_details")
	res, err := runTool(t, details, `{"query":"solo"}`)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if src.searchCalls != 1 || !strings.Contains(res.Content, "Studios: Wit Studio") {
		t.Fatalf("search calls = %d, content = %s", src.searchCalls, res.Content)
	}
}

func TestSearchContentCapsDescriptionForLLM(t *testing.T) {
	m := testMedia(1, "Long", "")
	m.Description = strings.Repeat("plot ", 400)
	content := searchContent(m, time.Now())

	desc := content[strings.Index(content, "Description: ")+len("Description: "):]
	desc = desc[:strings.Index(desc, "\n")]
	if n := len([]rune(desc)); n > anilistToolDescriptionChars {
		t.Fatalf("description is %d runes, cap is %d", n, anilistToolDescriptionChars)
	}
	if !strings.Contains(content, "call anilist_details") {
		t.Error("a shortened description should point the LLM at anilist_details")
	}

	short := testMedia(2, "Short", "")
	if strings.Contains(searchContent(short, time.Now()), "call anilist_details") {
		t.Error("an untruncated description should not mention anilist_details")
	}
}

func TestSearchContentOmitsEmbedOnlyFields(t *testing.T) {
	content := searchContent(testMedia(1, "T", ""), time.Now())
	for _, banned := range []string{"anilist.co", "cover", "Wit Studio", "#ff8800"} {
		if strings.Contains(content, banned) {
			t.Errorf("LLM content should not include %q:\n%s", banned, content)
		}
	}
}

func TestDetailsContentCapsDescription(t *testing.T) {
	m := testMedia(1, "Long", "")
	m.Description = strings.Repeat("plot ", 1000)
	content := detailsContent(m)
	if n := len([]rune(content)); n > anilistDetailsDescriptionChars+200 {
		t.Fatalf("details content is %d runes, should be near the %d cap", n, anilistDetailsDescriptionChars)
	}
}

func TestSearchContentNextAiring(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	m := testMedia(1, "Airing", "")
	m.Status = "RELEASING"
	m.NextAiring = &anilist.Airing{Episode: 6, At: now.Add(3*24*time.Hour + 5*time.Hour)}

	content := searchContent(m, now)
	if !strings.Contains(content, "Next episode: episode 6 airs 2026-09-21T17:00:00Z (in 3 days)") {
		t.Fatalf("content = %s", content)
	}
}

func TestMediaEmbed(t *testing.T) {
	m := testMedia(9, "Shingeki", "Attack on Titan")
	m.Description = strings.Repeat("plot ", 200)
	m.NextAiring = &anilist.Airing{Episode: 3, At: time.Unix(1790000000, 0)}

	e := mediaEmbed(m)
	if e.Title != "Attack on Titan" || e.URL != m.SiteURL || e.ThumbnailURL != m.CoverURL {
		t.Errorf("embed header = %+v", e)
	}
	if e.Color != 0xff8800 {
		t.Errorf("color = %#x, want the cover color", e.Color)
	}
	if n := len([]rune(e.Description)); n > anilistEmbedDescriptionChars {
		t.Errorf("embed description is %d runes, cap is %d", n, anilistEmbedDescriptionChars)
	}
	if e.Intent != "anime info card" || !strings.Contains(e.Footer, "9") {
		t.Errorf("intent/footer = %q / %q", e.Intent, e.Footer)
	}

	fields := map[string]string{}
	for _, f := range e.Fields {
		fields[f.Name] = f.Value
	}
	for name, want := range map[string]string{
		"Format":       "TV",
		"Status":       "Finished",
		"Episodes":     "12",
		"Season":       "Spring 2013",
		"Score":        "84/100",
		"Studio":       "Wit Studio",
		"Genres":       "Action, Drama",
		"Next episode": "Episode 3: <t:1790000000:F> (<t:1790000000:R>)",
	} {
		if fields[name] != want {
			t.Errorf("field %q = %q, want %q", name, fields[name], want)
		}
	}
}

func TestMediaEmbedSkipsMissingData(t *testing.T) {
	m := anilist.Media{ID: 3, Title: anilist.Title{Romaji: "Bare"}}
	e := mediaEmbed(m)
	if e.Color != anilistEmbedColor {
		t.Errorf("color = %#x, want the default", e.Color)
	}
	for _, f := range e.Fields {
		if strings.TrimSpace(f.Value) == "" {
			t.Errorf("empty field emitted: %+v", f)
		}
	}
}

func TestHumanEnum(t *testing.T) {
	tests := map[string]string{
		"NOT_YET_RELEASED": "Not yet released",
		"FINISHED":         "Finished",
		"TV":               "TV",
		"OVA":              "OVA",
		"MOVIE":            "Movie",
		"":                 "",
	}
	for in, want := range tests {
		if got := humanEnum(in); got != want {
			t.Errorf("humanEnum(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		offset time.Duration
		want   string
	}{
		{20 * time.Second, "now"},
		{time.Minute, "in 1 minute"},
		{45 * time.Minute, "in 45 minutes"},
		{5 * time.Hour, "in 5 hours"},
		{47 * time.Hour, "in 47 hours"},
		{72 * time.Hour, "in 3 days"},
		{-2 * time.Hour, "2 hours ago"},
	}
	for _, tt := range tests {
		if got := relativeTime(now, now.Add(tt.offset)); got != tt.want {
			t.Errorf("relativeTime(%v) = %q, want %q", tt.offset, got, tt.want)
		}
	}
}
