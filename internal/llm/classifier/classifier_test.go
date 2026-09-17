package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"mizubot-go/internal/llm"
	"mizubot-go/internal/typesafe"
	"mizubot-go/internal/typesafestats"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type fakeLogger struct {
	params []typesafestats.CreateClassificationLogParams
}

func (f *fakeLogger) Create(_ context.Context, params typesafestats.CreateClassificationLogParams) (typesafestats.ClassificationLog, error) {
	f.params = append(f.params, params)
	return typesafestats.ClassificationLog{}, nil
}

func TestTypeSafeClassifierRelevantTools(t *testing.T) {
	var gotReq typesafe.EvaluateRequest
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		var body bytes.Buffer
		_ = json.NewEncoder(&body).Encode(map[string]any{
			"model": "jev-latest",
			"answers": map[string]any{
				"reminder_tool": map[string]any{"type": "noul", "noul": 0.9},
				"weather_tool":  map[string]any{"type": "noul", "noul": 0.1},
			},
			"usage": map[string]any{"input_tokens": 100, "output_tokens": 10},
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(&body),
			Header:     make(http.Header),
		}, nil
	})

	client := typesafe.NewClient(typesafe.Config{
		APIKey:     "test-key",
		Timeout:    time.Second,
		HTTPClient: &http.Client{Transport: transport},
	})
	logger := &fakeLogger{}
	c := New(client, logger)

	tools := map[string]llm.Tool{
		"reminder_tool": {Name: "reminder_tool", Description: "Manages reminders."},
		"weather_tool":  {Name: "weather_tool", Description: "Gets weather."},
	}

	selected, err := c.RelevantTools(context.Background(), llm.Message{
		Content:   "remind me to drink water",
		GuildID:   "guild-1",
		ChannelID: "chan-1",
		UserID:    "user-1",
	}, tools)
	if err != nil {
		t.Fatalf("RelevantTools() error = %v", err)
	}
	if confidence, ok := selected["reminder_tool"]; !ok || confidence != 0.9 {
		t.Fatalf("selected = %+v, want reminder_tool with confidence 0.9", selected)
	}
	if _, ok := selected["weather_tool"]; ok {
		t.Fatalf("selected = %+v, want weather_tool excluded", selected)
	}
	if len(gotReq.Questions) != 2 {
		t.Fatalf("questions sent = %d, want 2", len(gotReq.Questions))
	}

	if len(logger.params) != 1 {
		t.Fatalf("log calls = %d, want 1", len(logger.params))
	}
	logged := logger.params[0]
	if logged.Status != typesafestats.StatusSuccess {
		t.Fatalf("logged status = %q, want success", logged.Status)
	}
	if logged.InputTokens != 100 || logged.OutputTokens != 10 {
		t.Fatalf("logged usage = %+v, want input=100 output=10", logged)
	}
	if !strings.Contains(logged.RequestState, `"message":"remind me to drink water"`) {
		t.Fatalf("logged request state = %q", logged.RequestState)
	}
}

func TestTypeSafeClassifierSendsCappedHistory(t *testing.T) {
	var gotReq typesafe.EvaluateRequest
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		var body bytes.Buffer
		_ = json.NewEncoder(&body).Encode(map[string]any{
			"model":   "jev-latest",
			"answers": map[string]any{"tool_a": map[string]any{"type": "noul", "noul": 0.6}},
			"usage":   map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(&body),
			Header:     make(http.Header),
		}, nil
	})
	client := typesafe.NewClient(typesafe.Config{
		APIKey:     "test-key",
		Timeout:    time.Second,
		HTTPClient: &http.Client{Transport: transport},
	})
	c := New(client, nil)

	history := make([]llm.HistoryMessage, 0, 10)
	for i := range 10 {
		history = append(history, llm.HistoryMessage{Author: "user", Content: fmt.Sprintf("turn %d", i)})
	}

	_, err := c.RelevantTools(context.Background(), llm.Message{
		Content: "latest message",
		History: history,
	}, map[string]llm.Tool{"tool_a": {Name: "tool_a", Description: "desc"}})
	if err != nil {
		t.Fatalf("RelevantTools() error = %v", err)
	}

	state, ok := gotReq.State.(map[string]any)
	if !ok {
		t.Fatalf("state type = %T, want map", gotReq.State)
	}
	if state["message"] != "latest message" {
		t.Fatalf("state message = %v, want %q", state["message"], "latest message")
	}
	sentHistory, ok := state["history"].([]any)
	if !ok {
		t.Fatalf("state history type = %T, want []any", state["history"])
	}
	if len(sentHistory) != maxHistoryMessages {
		t.Fatalf("history length = %d, want %d", len(sentHistory), maxHistoryMessages)
	}
	firstTurn, ok := sentHistory[0].(map[string]any)
	if !ok || firstTurn["content"] != "turn 4" {
		t.Fatalf("first sent turn = %v, want content=turn 4 (oldest of the last %d)", sentHistory[0], maxHistoryMessages)
	}
}

func TestTypeSafeClassifierPrefersClassifierHintOverDescription(t *testing.T) {
	var gotReq typesafe.EvaluateRequest
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		var body bytes.Buffer
		_ = json.NewEncoder(&body).Encode(map[string]any{
			"model":   "jev-latest",
			"answers": map[string]any{"tz_tool": map[string]any{"type": "noul", "noul": 0.5}},
			"usage":   map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(&body),
			Header:     make(http.Header),
		}, nil
	})
	client := typesafe.NewClient(typesafe.Config{
		APIKey:     "test-key",
		Timeout:    time.Second,
		HTTPClient: &http.Client{Transport: transport},
	})
	c := New(client, nil)

	_, err := c.RelevantTools(context.Background(), llm.Message{Content: "change me to jst"}, map[string]llm.Tool{
		"tz_tool": {
			Name:           "tz_tool",
			Description:    "Set the user's timezone. Use IANA names like Asia/Kolkata.",
			ClassifierHint: "Covers abbreviations like JST, IST, PST too, not just IANA names.",
		},
	})
	if err != nil {
		t.Fatalf("RelevantTools() error = %v", err)
	}

	question := gotReq.Questions["tz_tool"]
	criteria, ok := question.Criteria.(map[string]any)
	if !ok {
		t.Fatalf("criteria type = %T, want map", question.Criteria)
	}
	trueCriteria, _ := criteria["true"].(string)
	if !strings.Contains(trueCriteria, "JST, IST, PST") {
		t.Fatalf("criteria[true] = %q, want it to use ClassifierHint", trueCriteria)
	}
	if strings.Contains(trueCriteria, "IANA names like Asia/Kolkata") {
		t.Fatalf("criteria[true] = %q, should not fall back to Description when ClassifierHint is set", trueCriteria)
	}
}

func TestTypeSafeClassifierAddsImpliedToolsBelowThreshold(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body bytes.Buffer
		_ = json.NewEncoder(&body).Encode(map[string]any{
			"model": "jev-latest",
			"answers": map[string]any{
				"reminder_delete":      map[string]any{"type": "noul", "noul": 0.97},
				"reminder_list_active": map[string]any{"type": "noul", "noul": 0.44},
			},
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(&body),
			Header:     make(http.Header),
		}, nil
	})
	client := typesafe.NewClient(typesafe.Config{
		APIKey:     "test-key",
		Timeout:    time.Second,
		HTTPClient: &http.Client{Transport: transport},
	})
	c := New(client, nil)

	selected, err := c.RelevantTools(context.Background(), llm.Message{Content: "clear all my reminders"}, map[string]llm.Tool{
		"reminder_delete":      {Name: "reminder_delete", Description: "desc", ImpliesTools: []string{"reminder_list_active"}},
		"reminder_list_active": {Name: "reminder_list_active", Description: "desc"},
	})
	if err != nil {
		t.Fatalf("RelevantTools() error = %v", err)
	}
	if confidence, ok := selected["reminder_delete"]; !ok || confidence != 0.97 {
		t.Fatalf("selected = %+v, want reminder_delete with confidence 0.97", selected)
	}
	if confidence, ok := selected["reminder_list_active"]; !ok || confidence != 0.44 {
		t.Fatalf("selected = %+v, want reminder_list_active pulled in below threshold with confidence 0.44", selected)
	}
}

func TestTypeSafeClassifierLogsErrors(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Status:     "429",
			Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
			Header:     make(http.Header),
		}, nil
	})
	client := typesafe.NewClient(typesafe.Config{
		APIKey:     "test-key",
		Timeout:    time.Second,
		HTTPClient: &http.Client{Transport: transport},
	})
	logger := &fakeLogger{}
	c := New(client, logger)

	_, err := c.RelevantTools(context.Background(), llm.Message{Content: "hi"}, map[string]llm.Tool{
		"tool_a": {Name: "tool_a", Description: "desc"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(logger.params) != 1 {
		t.Fatalf("log calls = %d, want 1", len(logger.params))
	}
	if logger.params[0].Status != typesafestats.StatusError {
		t.Fatalf("logged status = %q, want error", logger.params[0].Status)
	}
}
