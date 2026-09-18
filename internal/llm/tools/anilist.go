package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"mizubot-go/internal/anilist"
	"mizubot-go/internal/llm"
)

const (
	// anilistSearchLimit is how many candidates a search asks AniList for.
	anilistSearchLimit = 5
	// anilistToolDescriptionChars caps the description the LLM sees from the
	// search tool, to protect small local models' context.
	anilistToolDescriptionChars = 500
	// anilistDetailsDescriptionChars caps the fuller description returned by
	// the details tool.
	anilistDetailsDescriptionChars = 2000
	// anilistEmbedDescriptionChars caps the description shown in the embed.
	anilistEmbedDescriptionChars = 350

	anilistEmbedColor = 0x02A9FF
	// anilistResolvedTTL is how long a query -> AniList ID resolution is
	// remembered, so a follow-up details call skips re-searching.
	anilistResolvedTTL = anilist.DefaultCacheTTL
)

var anilistKeywords = []string{"anime", "anilist", "airing"}

// AnimeSource is the slice of the AniList client the tools use.
type AnimeSource interface {
	Search(ctx context.Context, query string, limit int) ([]anilist.Media, error)
	ByID(ctx context.Context, id int) (anilist.Media, error)
}

// MediaPicker chooses which candidate best matches what the user meant. The
// tools work without one; ambiguous searches then return the candidate list.
type MediaPicker interface {
	Pick(ctx context.Context, toolCtx llm.ToolContext, query string, candidates []anilist.Media) (anilist.Pick, error)
}

type anilistTools struct {
	source   AnimeSource
	picker   MediaPicker
	resolved *anilist.TTLCache[int]
	now      func() time.Time
}

// NewAniListTools returns the anime search and details tools. picker may be
// nil, in which case ambiguous searches are handed back to the LLM.
func NewAniListTools(source AnimeSource, picker MediaPicker) []llm.Tool {
	if source == nil {
		return nil
	}
	t := &anilistTools{
		source:   source,
		picker:   picker,
		resolved: anilist.NewTTLCache[int](anilistResolvedTTL),
		now:      time.Now,
	}
	return []llm.Tool{
		{
			Name:        "anilist_search",
			Description: "Look up an anime on AniList: title, format, status, episode count, next airing episode and time, season, genres, score, and a short description. Attaches an info card to the reply. If several anime match, returns a list of candidates instead so you can ask the user which one they mean.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Anime title to search for, as the user wrote it."},"id":{"type":"integer","description":"AniList ID, if already known from an earlier candidate list in this turn. Skips searching."}},"additionalProperties":false}`),
			Keywords:    anilistKeywords,
			ClassifierHint: "The user is asking about a specific anime series: what it is, when the next episode airs, " +
				"whether it is finished or still airing, how many episodes or seasons it has, its genres or rating, " +
				"or wants it looked up. This includes casual phrasing (\"when does frieren come back\", " +
				"\"is aot done\", \"what's that anime about the...\"), not just explicit lookup requests.",
			ImpliesTools: []string{"anilist_details"},
			Execute:      t.search,
		},
		{
			Name:        "anilist_details",
			Description: "Get the full synopsis and studios for an anime on AniList. Use after anilist_search when the user wants more detail about the plot than the short description gave. Provide query (title) or id.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Anime title, as used in the earlier search."},"id":{"type":"integer","description":"AniList ID, if known."}},"additionalProperties":false}`),
			Keywords:    anilistKeywords,
			ClassifierHint: "The user wants more depth on an anime already being discussed or named: the plot, " +
				"premise, synopsis, who made it, or asks to elaborate or \"tell me more\" about it.",
			Execute: t.details,
		},
	}
}

type anilistArgs struct {
	Query string `json:"query"`
	ID    int    `json:"id"`
}

func parseAniListArgs(raw json.RawMessage) (anilistArgs, error) {
	var args anilistArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return args, fmt.Errorf("invalid anilist arguments: %w", err)
		}
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" && args.ID <= 0 {
		return args, fmt.Errorf("provide a query or an id")
	}
	return args, nil
}

func (t *anilistTools) search(ctx context.Context, toolCtx llm.ToolContext, raw json.RawMessage) (llm.ToolResult, error) {
	args, err := parseAniListArgs(raw)
	if err != nil {
		return llm.ToolResult{}, err
	}
	media, candidates, err := t.resolve(ctx, toolCtx, args)
	if err != nil {
		return llm.ToolResult{}, err
	}
	if media == nil {
		return llm.ToolResult{Content: candidateList(candidates)}, nil
	}
	return llm.ToolResult{
		Content: searchContent(*media, t.now()),
		Embeds:  []llm.Embed{mediaEmbed(*media)},
	}, nil
}

func (t *anilistTools) details(ctx context.Context, toolCtx llm.ToolContext, raw json.RawMessage) (llm.ToolResult, error) {
	args, err := parseAniListArgs(raw)
	if err != nil {
		return llm.ToolResult{}, err
	}
	media, candidates, err := t.resolve(ctx, toolCtx, args)
	if err != nil {
		return llm.ToolResult{}, err
	}
	if media == nil {
		return llm.ToolResult{Content: candidateList(candidates)}, nil
	}
	return llm.ToolResult{Content: detailsContent(*media)}, nil
}

// resolve turns tool arguments into one anime. It returns either a media, or
// (when the query is ambiguous and can't be settled) a candidate list.
func (t *anilistTools) resolve(ctx context.Context, toolCtx llm.ToolContext, args anilistArgs) (*anilist.Media, []anilist.Media, error) {
	if args.ID > 0 {
		m, err := t.source.ByID(ctx, args.ID)
		if err != nil {
			return nil, nil, err
		}
		return &m, nil, nil
	}

	key := anilist.NormalizeKey(args.Query)
	if id, ok := t.resolved.Get(key); ok {
		if m, err := t.source.ByID(ctx, id); err == nil {
			return &m, nil, nil
		}
	}

	candidates, err := t.source.Search(ctx, args.Query, anilistSearchLimit)
	if err != nil {
		return nil, nil, err
	}
	switch len(candidates) {
	case 0:
		return nil, nil, fmt.Errorf("no anime found on AniList for %q", args.Query)
	case 1:
		t.resolved.Set(key, candidates[0].ID)
		return &candidates[0], nil, nil
	}

	if t.picker == nil {
		return nil, candidates, nil
	}
	pick, err := t.picker.Pick(ctx, toolCtx, args.Query, candidates)
	if err != nil {
		log.Printf("anilist pick failed, returning candidates: query=%q error=%v", args.Query, err)
		return nil, candidates, nil
	}
	if !pick.Decisive() {
		log.Printf("anilist pick unsure, returning candidates: query=%q id=%d top=%.2f runner_up=%.2f", args.Query, pick.ID, pick.Top, pick.RunnerUp)
		return nil, candidates, nil
	}
	for i := range candidates {
		if candidates[i].ID == pick.ID {
			t.resolved.Set(key, pick.ID)
			return &candidates[i], nil, nil
		}
	}
	return nil, candidates, nil
}

func candidateList(candidates []anilist.Media) string {
	var b strings.Builder
	b.WriteString("Several anime match. Ask the user which one they mean, then call anilist_search again with a more specific title (or the id).\n")
	for i, m := range candidates {
		fmt.Fprintf(&b, "%d. %s (AniList ID %d)", i+1, m.Title.Display(), m.ID)
		if extra := candidateBlurb(m); extra != "" {
			fmt.Fprintf(&b, " - %s", extra)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func candidateBlurb(m anilist.Media) string {
	var parts []string
	if m.Format != "" {
		parts = append(parts, humanEnum(m.Format))
	}
	if m.SeasonYear > 0 {
		parts = append(parts, strconv.Itoa(m.SeasonYear))
	} else if !m.StartDate.IsZero() {
		parts = append(parts, strconv.Itoa(m.StartDate.Year))
	}
	if m.Status != "" {
		parts = append(parts, humanEnum(m.Status))
	}
	return strings.Join(parts, ", ")
}

// searchContent is the compact text the LLM sees: only fields useful for
// answering questions, with a short description. Cover, link and studios go
// only in the embed.
func searchContent(m anilist.Media, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "AniList ID: %d\nTitle: %s", m.ID, m.Title.Display())
	if m.Title.Romaji != "" && m.Title.Romaji != m.Title.Display() {
		fmt.Fprintf(&b, " (romaji: %s)", m.Title.Romaji)
	}
	b.WriteString("\n")
	if m.Format != "" {
		fmt.Fprintf(&b, "Format: %s\n", humanEnum(m.Format))
	}
	if m.Status != "" {
		fmt.Fprintf(&b, "Status: %s\n", humanEnum(m.Status))
	}
	if m.Episodes > 0 {
		fmt.Fprintf(&b, "Episodes: %d\n", m.Episodes)
	}
	if m.Season != "" && m.SeasonYear > 0 {
		fmt.Fprintf(&b, "Season: %s %d\n", humanEnum(m.Season), m.SeasonYear)
	}
	if !m.StartDate.IsZero() {
		fmt.Fprintf(&b, "Started: %s\n", m.StartDate)
	}
	if !m.EndDate.IsZero() {
		fmt.Fprintf(&b, "Ended: %s\n", m.EndDate)
	}
	if len(m.Genres) > 0 {
		fmt.Fprintf(&b, "Genres: %s\n", strings.Join(m.Genres, ", "))
	}
	if m.AverageScore > 0 {
		fmt.Fprintf(&b, "Average score: %d/100\n", m.AverageScore)
	}
	if m.NextAiring != nil {
		fmt.Fprintf(&b, "Next episode: episode %d airs %s (%s)\n",
			m.NextAiring.Episode, m.NextAiring.At.Format(time.RFC3339), relativeTime(now, m.NextAiring.At))
	}
	if m.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", anilist.Truncate(m.Description, anilistToolDescriptionChars))
	}
	b.WriteString("\nAn info card for this anime is attached to your reply automatically. Answer the user's actual question directly and briefly; do not restate every field.")
	if len([]rune(m.Description)) > anilistToolDescriptionChars {
		b.WriteString(" The description above is shortened; call anilist_details if the user wants the full synopsis.")
	}
	return b.String()
}

func detailsContent(m anilist.Media) string {
	var b strings.Builder
	fmt.Fprintf(&b, "AniList ID: %d\nTitle: %s\n", m.ID, m.Title.Display())
	if len(m.Studios) > 0 {
		fmt.Fprintf(&b, "Studios: %s\n", strings.Join(m.Studios, ", "))
	}
	if m.Description == "" {
		b.WriteString("Description: (none on AniList)")
	} else {
		fmt.Fprintf(&b, "Description: %s", anilist.Truncate(m.Description, anilistDetailsDescriptionChars))
	}
	return b.String()
}

func mediaEmbed(m anilist.Media) llm.Embed {
	embed := llm.Embed{
		Title:        m.Title.Display(),
		URL:          m.SiteURL,
		Description:  anilist.Truncate(m.Description, anilistEmbedDescriptionChars),
		Color:        anilistEmbedColor,
		ThumbnailURL: m.CoverURL,
		Footer:       "AniList ID " + strconv.Itoa(m.ID),
		Intent:       "anime info card",
	}
	if color, ok := parseHexColor(m.CoverColor); ok {
		embed.Color = color
	}

	add := func(name, value string, inline bool) {
		if strings.TrimSpace(value) != "" {
			embed.Fields = append(embed.Fields, llm.EmbedField{Name: name, Value: value, Inline: inline})
		}
	}
	add("Format", humanEnum(m.Format), true)
	add("Status", humanEnum(m.Status), true)
	if m.Episodes > 0 {
		add("Episodes", strconv.Itoa(m.Episodes), true)
	}
	if m.Season != "" && m.SeasonYear > 0 {
		add("Season", fmt.Sprintf("%s %d", humanEnum(m.Season), m.SeasonYear), true)
	}
	if m.AverageScore > 0 {
		add("Score", fmt.Sprintf("%d/100", m.AverageScore), true)
	}
	add("Studio", strings.Join(m.Studios, ", "), true)
	add("Genres", strings.Join(m.Genres, ", "), false)
	if m.NextAiring != nil {
		unix := m.NextAiring.At.Unix()
		add("Next episode", fmt.Sprintf("Episode %d: <t:%d:F> (<t:%d:R>)", m.NextAiring.Episode, unix, unix), false)
	}
	return embed
}

func parseHexColor(s string) (int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, false
	}
	return int(v), true
}

// humanEnum turns AniList's SCREAMING_SNAKE enums into "Not yet released".
func humanEnum(s string) string {
	if s == "" {
		return ""
	}
	switch s {
	case "TV", "OVA", "ONA":
		return s
	}
	words := strings.Split(strings.ToLower(s), "_")
	words[0] = strings.ToUpper(words[0][:1]) + words[0][1:]
	return strings.Join(words, " ")
}

// relativeTime renders t relative to now ("in 3 days", "2 hours ago"), so
// the LLM doesn't have to do date math.
func relativeTime(now, t time.Time) string {
	d := t.Sub(now)
	past := d < 0
	if past {
		d = -d
	}
	suffix := func(s string) string {
		if past {
			return s + " ago"
		}
		return "in " + s
	}
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return suffix(plural(int(d/time.Minute), "minute"))
	case d < 48*time.Hour:
		return suffix(plural(int(d/time.Hour), "hour"))
	default:
		return suffix(plural(int(d/(24*time.Hour)), "day"))
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
