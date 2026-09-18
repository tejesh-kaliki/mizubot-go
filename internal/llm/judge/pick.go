package judge

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"mizubot-go/internal/anilist"
	"mizubot-go/internal/llm"
	"mizubot-go/internal/typesafe"
	"mizubot-go/internal/typesafestats"
)

const pickQuestionKey = "anilist_pick"

// MediaPicker asks Jev which of several AniList candidates the user meant.
type MediaPicker struct {
	eval   Evaluator
	logger Logger
}

func NewMediaPicker(eval Evaluator, logger Logger) *MediaPicker {
	return &MediaPicker{eval: eval, logger: logger}
}

type pickState struct {
	UserMessage string          `json:"user_message"`
	SearchQuery string          `json:"search_query"`
	Candidates  []pickCandidate `json:"candidates"`
}

type pickCandidate struct {
	ID      int    `json:"id"`
	Romaji  string `json:"romaji,omitempty"`
	English string `json:"english,omitempty"`
	Format  string `json:"format,omitempty"`
	Year    int    `json:"year,omitempty"`
	Status  string `json:"status,omitempty"`
}

func candidateYear(m anilist.Media) int {
	if m.SeasonYear > 0 {
		return m.SeasonYear
	}
	return m.StartDate.Year
}

// Pick returns the candidate Jev judges best, with its probability and the
// runner-up's. Callers decide whether that's decisive enough.
func (p *MediaPicker) Pick(ctx context.Context, toolCtx llm.ToolContext, query string, candidates []anilist.Media) (anilist.Pick, error) {
	if p == nil || p.eval == nil {
		return anilist.Pick{}, fmt.Errorf("anilist picker: not configured")
	}
	if len(candidates) == 0 {
		return anilist.Pick{}, fmt.Errorf("anilist picker: no candidates")
	}

	state := pickState{UserMessage: toolCtx.Message, SearchQuery: query}
	criteria := make(map[string]string, len(candidates))
	for _, m := range candidates {
		state.Candidates = append(state.Candidates, pickCandidate{
			ID:      m.ID,
			Romaji:  m.Title.Romaji,
			English: m.Title.English,
			Format:  m.Format,
			Year:    candidateYear(m),
			Status:  m.Status,
		})
		criteria[strconv.Itoa(m.ID)] = candidateRubric(m)
	}
	questions := map[string]typesafe.Question{
		pickQuestionKey: {
			Type: typesafe.QuestionChoice,
			Instructions: "Which anime did the user mean? Match the title, season/sequel number, " +
				"and year they referred to. Prefer the main series over specials, recaps, or " +
				"spin-offs unless they asked for one.",
			Criteria: criteria,
		},
	}

	var pick anilist.Pick
	_, err := evaluate(ctx, p.eval, p.logger, callMeta{
		GuildID:   toolCtx.GuildID,
		ChannelID: toolCtx.ChannelID,
		UserID:    toolCtx.UserID,
		MessageID: toolCtx.MessageID,
		Kind:      typesafestats.KindAniListPick,
	}, state, questions, func(resp typesafe.EvaluateResponse) []string {
		pick = pickFromAnswer(resp.Answers[pickQuestionKey], candidates)
		return []string{strconv.Itoa(pick.ID)}
	})
	if err != nil {
		return anilist.Pick{}, err
	}
	if pick.ID == 0 {
		return anilist.Pick{}, fmt.Errorf("anilist picker: no usable answer")
	}
	return pick, nil
}

func candidateRubric(m anilist.Media) string {
	title := m.Title.Romaji
	if m.Title.English != "" && m.Title.English != title {
		title += " / " + m.Title.English
	}
	parts := []string{title}
	if m.Format != "" {
		parts = append(parts, m.Format)
	}
	if y := candidateYear(m); y > 0 {
		parts = append(parts, strconv.Itoa(y))
	}
	return "The user means: " + strings.Join(parts, ", ")
}

// pickFromAnswer reads a choice answer into a Pick. It prefers the
// per-option probabilities; when they're absent it falls back to the chosen
// option and reported confidence.
func pickFromAnswer(answer typesafe.Answer, candidates []anilist.Media) anilist.Pick {
	valid := make(map[string]bool, len(candidates))
	for _, m := range candidates {
		valid[strconv.Itoa(m.ID)] = true
	}

	type option struct {
		id   string
		prob float64
	}
	var options []option
	for id, prob := range answer.Probabilities {
		if valid[id] {
			options = append(options, option{id, prob})
		}
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].prob != options[j].prob {
			return options[i].prob > options[j].prob
		}
		return options[i].id < options[j].id
	})

	if len(options) > 0 {
		id, _ := strconv.Atoi(options[0].id)
		pick := anilist.Pick{ID: id, Top: options[0].prob}
		if len(options) > 1 {
			pick.RunnerUp = options[1].prob
		}
		return pick
	}
	if valid[answer.Choice] {
		id, _ := strconv.Atoi(answer.Choice)
		top := 1.0
		if answer.Confidence != nil {
			top = *answer.Confidence
		}
		return anilist.Pick{ID: id, Top: top}
	}
	return anilist.Pick{}
}
