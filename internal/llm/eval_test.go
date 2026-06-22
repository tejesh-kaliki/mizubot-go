package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestLLMEvalReminderCreateUsesNormalizedCronShape(t *testing.T) {
	eval := newToolEval(t, ChatToolCall{
		Name: "reminder_create",
		Arguments: json.RawMessage(`{
			"message": "take meds",
			"once": false,
			"cron_expr": "0 9 * * *",
			"timezone": "Asia/Kolkata"
		}`),
	})

	got, err := eval.service.GenerateResponse(context.Background(), Message{
		UserID:    "u",
		ChannelID: "c",
		Content:   "remind me to take meds every day at 9",
	})
	if err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if got != "created" {
		t.Fatalf("response = %q, want created", got)
	}

	call := eval.calls[0]
	if call.Name != "reminder_create" {
		t.Fatalf("tool = %q, want reminder_create", call.Name)
	}
	var args struct {
		Message  string `json:"message"`
		Once     bool   `json:"once"`
		CronExpr string `json:"cron_expr"`
		Timezone string `json:"timezone"`
		Schedule string `json:"schedule"`
		At       string `json:"at"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil {
		t.Fatalf("decode args: %v", err)
	}
	if args.Message != "take meds" {
		t.Fatalf("message = %q, want inferred reminder text", args.Message)
	}
	if args.Once {
		t.Fatalf("once = true, want false")
	}
	if args.CronExpr != "0 9 * * *" {
		t.Fatalf("cron_expr = %q, want 0 9 * * *", args.CronExpr)
	}
	if args.Schedule != "" || args.At != "" {
		t.Fatalf("unexpected slash-style fields: schedule=%q at=%q", args.Schedule, args.At)
	}
}

func TestLLMEvalReminderCreateOnceUsesRunAt(t *testing.T) {
	eval := newToolEval(t, ChatToolCall{
		Name: "reminder_create",
		Arguments: json.RawMessage(`{
			"message": "submit report",
			"once": true,
			"run_at": "2026-06-22 09:00"
		}`),
	})

	if _, err := eval.service.GenerateResponse(context.Background(), Message{Content: "remind me to submit report tomorrow at 9"}); err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	var args struct {
		Message string `json:"message"`
		Once    bool   `json:"once"`
		RunAt   string `json:"run_at"`
	}
	if err := json.Unmarshal(eval.calls[0].Args, &args); err != nil {
		t.Fatalf("decode args: %v", err)
	}
	if args.Message != "submit report" || !args.Once || args.RunAt == "" {
		t.Fatalf("bad once reminder args: %+v", args)
	}
}

func TestLLMEvalReminderListFinalResponseIsUserFriendly(t *testing.T) {
	completer := &fakeCompleter{chat: []ChatResponse{
		{ToolCalls: []ChatToolCall{{Name: "reminder_list_active", Arguments: json.RawMessage(`{}`)}}},
		{Content: "- take meds: <t:1780000000:F> (<t:1780000000:R>) in #general"},
	}}
	service := NewService(completer, Tool{
		Name:        "reminder_list_active",
		Description: "Load reminders.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ ToolContext, _ json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "Reminder ID 3\nMessage: take meds\nNext run: <t:1780000000:F> (<t:1780000000:R>)\nChannel: #general\nTimezone: Asia/Kolkata\nRepeat: daily"}, nil
		},
	})

	got, err := service.GenerateResponse(context.Background(), Message{Content: "what reminders do I have"})
	if err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if strings.Contains(strings.ToLower(got), "cron") || strings.Contains(strings.ToLower(got), "tool") {
		t.Fatalf("response exposed implementation detail: %q", got)
	}
	if !strings.Contains(got, "<t:1780000000:F>") {
		t.Fatalf("response missing discord timestamp: %q", got)
	}
}

func TestLLMEvalDeleteReminderReturnsDetailsToFinalResponse(t *testing.T) {
	completer := &fakeCompleter{chat: []ChatResponse{
		{ToolCalls: []ChatToolCall{{Name: "reminder_delete", Arguments: json.RawMessage(`{"id":3}`)}}},
		{Content: "Deleted your reminder to take meds."},
	}}
	service := NewService(completer, Tool{
		Name:        "reminder_delete",
		Description: "Delete reminder.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ ToolContext, _ json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "Deleted reminder ID 3.\nMessage: take meds\nNext run: <t:1780000000:F> (<t:1780000000:R>)\nChannel: #general\nTimezone: Asia/Kolkata\nRepeat: daily"}, nil
		},
	})

	got, err := service.GenerateResponse(context.Background(), Message{Content: "delete reminder 3"})
	if err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if !strings.Contains(strings.ToLower(got), "take meds") {
		t.Fatalf("response should mention deleted reminder detail: %q", got)
	}
}

func TestLLMEvalTimezoneSetTool(t *testing.T) {
	eval := newToolEval(t, ChatToolCall{
		Name:      "user_timezone_set",
		Arguments: json.RawMessage(`{"timezone":"Asia/Kolkata"}`),
	})

	if _, err := eval.service.GenerateResponse(context.Background(), Message{Content: "set my timezone to India"}); err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if eval.calls[0].Name != "user_timezone_set" {
		t.Fatalf("tool = %q, want user_timezone_set", eval.calls[0].Name)
	}
	if !strings.Contains(string(eval.calls[0].Args), "Asia/Kolkata") {
		t.Fatalf("timezone args = %s, want Asia/Kolkata", eval.calls[0].Args)
	}
}

type toolEval struct {
	service *Service
	calls   []toolEvalCall
}

type toolEvalCall struct {
	Name string
	Args json.RawMessage
}

func newToolEval(t *testing.T, call ChatToolCall) *toolEval {
	t.Helper()
	eval := &toolEval{}
	completer := &fakeCompleter{chat: []ChatResponse{
		{ToolCalls: []ChatToolCall{call}},
		{Content: "created"},
	}}
	eval.service = NewService(completer, Tool{
		Name:        call.Name,
		Description: "eval tool",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ ToolContext, args json.RawMessage) (ToolResult, error) {
			eval.calls = append(eval.calls, toolEvalCall{Name: call.Name, Args: args})
			return ToolResult{Content: "ok"}, nil
		},
	})
	return eval
}
