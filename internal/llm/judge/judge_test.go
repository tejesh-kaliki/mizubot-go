package judge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"mizubot-go/internal/anilist"
	"mizubot-go/internal/llm"
	"mizubot-go/internal/typesafe"
	"mizubot-go/internal/typesafestats"
)

type fakeEvaluator struct {
	resp     typesafe.EvaluateResponse
	err      error
	gotState any
	gotQs    map[string]typesafe.Question
	calls    int
}

func (f *fakeEvaluator) Evaluate(_ context.Context, state any, qs map[string]typesafe.Question) (typesafe.EvaluateResponse, error) {
	f.calls++
	f.gotState = state
	f.gotQs = qs
	return f.resp, f.err
}

func (f *fakeEvaluator) Model() string { return "jev-test" }

type fakeLogger struct {
	params []typesafestats.CreateClassificationLogParams
}

func (f *fakeLogger) Create(_ context.Context, p typesafestats.CreateClassificationLogParams) (typesafestats.ClassificationLog, error) {
	f.params = append(f.params, p)
	return typesafestats.ClassificationLog{}, nil
}

func fptr(v float64) *float64 { return &v }

func media(id int, romaji, english, format string, year int) anilist.Media {
	return anilist.Media{ID: id, Title: anilist.Title{Romaji: romaji, English: english}, Format: format, SeasonYear: year, Status: "FINISHED"}
}

func TestPickFromAnswer(t *testing.T) {
	candidates := []anilist.Media{media(1, "A", "", "TV", 2020), media(2, "B", "", "TV", 2021), media(3, "C", "", "TV", 2022)}
	tests := []struct {
		name   string
		answer typesafe.Answer
		want   anilist.Pick
	}{
		{
			"probabilities",
			typesafe.Answer{Probabilities: map[string]float64{"1": 0.1, "2": 0.7, "3": 0.2}},
			anilist.Pick{ID: 2, Top: 0.7, RunnerUp: 0.2},
		},
		{
			"probabilities ignore unknown options",
			typesafe.Answer{Probabilities: map[string]float64{"99": 0.9, "1": 0.6, "2": 0.3}},
			anilist.Pick{ID: 1, Top: 0.6, RunnerUp: 0.3},
		},
		{
			"single probability",
			typesafe.Answer{Probabilities: map[string]float64{"3": 0.8}},
			anilist.Pick{ID: 3, Top: 0.8},
		},
		{
			"choice with confidence",
			typesafe.Answer{Choice: "2", Confidence: fptr(0.75)},
			anilist.Pick{ID: 2, Top: 0.75},
		},
		{
			"choice without confidence",
			typesafe.Answer{Choice: "1"},
			anilist.Pick{ID: 1, Top: 1},
		},
		{"choice not a candidate", typesafe.Answer{Choice: "77"}, anilist.Pick{}},
		{"empty answer", typesafe.Answer{}, anilist.Pick{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickFromAnswer(tt.answer, candidates); got != tt.want {
				t.Fatalf("pick = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMediaPickerSendsChoiceQuestionAndLogs(t *testing.T) {
	eval := &fakeEvaluator{resp: typesafe.EvaluateResponse{
		Answers: map[string]typesafe.Answer{
			pickQuestionKey: {Type: "choice", Choice: "2", Probabilities: map[string]float64{"1": 0.15, "2": 0.85}},
		},
		Usage: typesafe.Usage{InputTokens: 120, OutputTokens: 4},
	}}
	logger := &fakeLogger{}
	picker := NewMediaPicker(eval, logger)

	candidates := []anilist.Media{
		media(1, "Sousou no Frieren", "Frieren: Beyond Journey's End", "TV", 2023),
		media(2, "Sousou no Frieren 2nd Season", "Frieren Season 2", "TV", 2026),
	}
	pick, err := picker.Pick(context.Background(), llm.ToolContext{
		GuildID: "g", ChannelID: "c", UserID: "u", MessageID: "m", Message: "when does frieren season 2 air",
	}, "Frieren season 2", candidates)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if pick.ID != 2 || pick.Top != 0.85 || pick.RunnerUp != 0.15 || !pick.Decisive() {
		t.Fatalf("pick = %+v", pick)
	}

	q, ok := eval.gotQs[pickQuestionKey]
	if !ok || len(eval.gotQs) != 1 {
		t.Fatalf("questions = %v, want exactly %q", eval.gotQs, pickQuestionKey)
	}
	if q.Type != typesafe.QuestionChoice {
		t.Errorf("question type = %q, want choice", q.Type)
	}
	criteria, ok := q.Criteria.(map[string]string)
	if !ok || len(criteria) != 2 || !strings.Contains(criteria["2"], "Frieren Season 2") {
		t.Errorf("criteria = %#v", q.Criteria)
	}

	state, ok := eval.gotState.(pickState)
	if !ok || state.UserMessage != "when does frieren season 2 air" || state.SearchQuery != "Frieren season 2" || len(state.Candidates) != 2 {
		t.Errorf("state = %#v", eval.gotState)
	}

	if len(logger.params) != 1 {
		t.Fatalf("log rows = %d, want 1", len(logger.params))
	}
	row := logger.params[0]
	if row.Status != typesafestats.StatusSuccess || row.InputTokens != 120 || row.MessageID != "m" || row.Model != "jev-test" {
		t.Errorf("log row = %+v", row)
	}
	if row.Kind != typesafestats.KindAniListPick {
		t.Errorf("kind = %q, want %q", row.Kind, typesafestats.KindAniListPick)
	}
	var selected []string
	if err := json.Unmarshal([]byte(row.SelectedTools), &selected); err != nil || len(selected) != 1 || selected[0] != "2" {
		t.Errorf("selected_tools = %q (%v), want [\"2\"]", row.SelectedTools, err)
	}
}

func TestMediaPickerErrors(t *testing.T) {
	candidates := []anilist.Media{media(1, "A", "", "TV", 2020), media(2, "B", "", "TV", 2021)}

	t.Run("evaluation error is logged and returned", func(t *testing.T) {
		logger := &fakeLogger{}
		picker := NewMediaPicker(&fakeEvaluator{err: errors.New("boom")}, logger)
		if _, err := picker.Pick(context.Background(), llm.ToolContext{}, "q", candidates); err == nil {
			t.Fatal("expected error")
		}
		if len(logger.params) != 1 || logger.params[0].Status != typesafestats.StatusError || logger.params[0].Error != "boom" {
			t.Fatalf("log rows = %+v", logger.params)
		}
	})
	t.Run("unusable answer", func(t *testing.T) {
		picker := NewMediaPicker(&fakeEvaluator{resp: typesafe.EvaluateResponse{Answers: map[string]typesafe.Answer{}}}, nil)
		if _, err := picker.Pick(context.Background(), llm.ToolContext{}, "q", candidates); err == nil {
			t.Fatal("expected error when the answer is missing")
		}
	})
	t.Run("no candidates", func(t *testing.T) {
		eval := &fakeEvaluator{}
		if _, err := NewMediaPicker(eval, nil).Pick(context.Background(), llm.ToolContext{}, "q", nil); err == nil || eval.calls != 0 {
			t.Fatalf("err = %v, calls = %d; want an error and no Jev call", err, eval.calls)
		}
	})
	t.Run("nil picker", func(t *testing.T) {
		var picker *MediaPicker
		if _, err := picker.Pick(context.Background(), llm.ToolContext{}, "q", candidates); err == nil {
			t.Fatal("expected error from a nil picker")
		}
	})
}

func TestEmbedFilterKeepsAndDrops(t *testing.T) {
	eval := &fakeEvaluator{resp: typesafe.EvaluateResponse{
		Answers: map[string]typesafe.Answer{
			"embed::0": {Type: "noul", Noul: fptr(0.92)},
			"embed::1": {Type: "noul", Noul: fptr(0.08)},
			// embed::2 has no answer and must be kept.
		},
	}}
	logger := &fakeLogger{}
	filter := NewEmbedFilter(eval, logger)

	embeds := []llm.Embed{
		{Title: "Keep", Intent: "anime info card"},
		{Title: "Drop", Intent: "anime info card"},
		{Title: "Unanswered"},
	}
	kept, err := filter.Filter(context.Background(), llm.Message{Content: "tell me about it", MessageID: "m"}, "Here you go.", embeds)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(kept) != 2 || kept[0].Title != "Keep" || kept[1].Title != "Unanswered" {
		t.Fatalf("kept = %+v", kept)
	}

	if len(eval.gotQs) != 3 {
		t.Fatalf("questions = %d, want one per embed", len(eval.gotQs))
	}
	for key, q := range eval.gotQs {
		if q.Type != typesafe.QuestionNoul {
			t.Errorf("%s type = %q, want noul", key, q.Type)
		}
	}
	state, ok := eval.gotState.(embedFilterState)
	if !ok || state.BotReply != "Here you go." || state.UserMessage != "tell me about it" || len(state.Embeds) != 3 {
		t.Errorf("state = %#v", eval.gotState)
	}

	if len(logger.params) != 1 {
		t.Fatalf("log rows = %d, want 1", len(logger.params))
	}
	var selected []string
	_ = json.Unmarshal([]byte(logger.params[0].SelectedTools), &selected)
	if len(selected) != 2 || selected[0] != "Keep" || selected[1] != "Unanswered" {
		t.Errorf("selected_tools = %v, want the kept titles", selected)
	}
	if logger.params[0].Kind != typesafestats.KindEmbedFilter {
		t.Errorf("kind = %q, want %q", logger.params[0].Kind, typesafestats.KindEmbedFilter)
	}
}

func TestEmbedFilterThresholdBoundary(t *testing.T) {
	eval := &fakeEvaluator{resp: typesafe.EvaluateResponse{
		Answers: map[string]typesafe.Answer{"embed::0": {Noul: fptr(embedKeepThreshold)}},
	}}
	kept, err := NewEmbedFilter(eval, nil).Filter(context.Background(), llm.Message{}, "r", []llm.Embed{{Title: "Edge"}})
	if err != nil || len(kept) != 1 {
		t.Fatalf("kept = %+v, err = %v; an answer exactly at the threshold should be kept", kept, err)
	}
}

func TestEmbedFilterErrors(t *testing.T) {
	logger := &fakeLogger{}
	filter := NewEmbedFilter(&fakeEvaluator{err: errors.New("timeout")}, logger)
	if _, err := filter.Filter(context.Background(), llm.Message{}, "r", []llm.Embed{{Title: "x"}}); err == nil {
		t.Fatal("expected the evaluation error to be returned so the caller can keep everything")
	}
	if len(logger.params) != 1 || logger.params[0].Status != typesafestats.StatusError {
		t.Fatalf("log rows = %+v", logger.params)
	}

	eval := &fakeEvaluator{}
	kept, err := NewEmbedFilter(eval, nil).Filter(context.Background(), llm.Message{}, "r", nil)
	if err != nil || len(kept) != 0 || eval.calls != 0 {
		t.Fatalf("no embeds: kept=%v err=%v calls=%d; want no Jev call", kept, err, eval.calls)
	}

	var nilFilter *EmbedFilter
	if _, err := nilFilter.Filter(context.Background(), llm.Message{}, "r", []llm.Embed{{Title: "x"}}); err == nil {
		t.Fatal("expected error from a nil filter")
	}
}
