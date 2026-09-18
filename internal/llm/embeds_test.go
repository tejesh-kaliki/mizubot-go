package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeEmbedFilter struct {
	keepTitles map[string]bool
	err        error
	gotReply   string
	gotMessage Message
	calls      int
}

func (f *fakeEmbedFilter) Filter(_ context.Context, message Message, reply string, embeds []Embed) ([]Embed, error) {
	f.calls++
	f.gotReply = reply
	f.gotMessage = message
	if f.err != nil {
		return nil, f.err
	}
	var kept []Embed
	for _, e := range embeds {
		if f.keepTitles[e.Title] {
			kept = append(kept, e)
		}
	}
	return kept, nil
}

func embedTool(embeds ...Embed) Tool {
	return Tool{
		Name:        "embed_tool",
		Description: "Produces embeds.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ ToolContext, _ json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "tool result", Embeds: embeds}, nil
		},
	}
}

func embedToolCompleter() *fakeCompleter {
	return &fakeCompleter{chat: []ChatResponse{
		{ToolCalls: []ChatToolCall{{Name: "embed_tool", Arguments: json.RawMessage(`{}`)}}},
		{Content: "final answer"},
	}}
}

func TestServiceReturnsToolEmbeds(t *testing.T) {
	service := NewService(embedToolCompleter(), embedTool(Embed{Title: "Card A"}, Embed{Title: "Card B"}))

	got, err := service.GenerateResponseWithMetrics(context.Background(), Message{UserID: "u", Content: "hi"})
	if err != nil {
		t.Fatalf("GenerateResponseWithMetrics: %v", err)
	}
	if got.Content != "final answer" || len(got.Embeds) != 2 || got.Embeds[0].Title != "Card A" {
		t.Fatalf("response = %+v", got)
	}
}

func TestServiceEmbedFilterDropsEmbedsAfterReply(t *testing.T) {
	filter := &fakeEmbedFilter{keepTitles: map[string]bool{"Card A": true}}
	service := NewService(embedToolCompleter(), embedTool(Embed{Title: "Card A"}, Embed{Title: "Card B"}))
	service.SetEmbedFilter(filter)

	got, err := service.GenerateResponseWithMetrics(context.Background(), Message{UserID: "u", Content: "hi", MessageID: "m1"})
	if err != nil {
		t.Fatalf("GenerateResponseWithMetrics: %v", err)
	}
	if len(got.Embeds) != 1 || got.Embeds[0].Title != "Card A" {
		t.Fatalf("embeds = %+v, want only Card A", got.Embeds)
	}
	if filter.gotReply != "final answer" || filter.gotMessage.MessageID != "m1" {
		t.Errorf("filter saw reply %q, message %+v; it should run after the reply is written", filter.gotReply, filter.gotMessage)
	}
	if got.Content != "final answer" {
		t.Errorf("content = %q, filtering must not change the reply text", got.Content)
	}
}

func TestServiceEmbedFilterErrorKeepsEmbeds(t *testing.T) {
	filter := &fakeEmbedFilter{err: errors.New("jev timeout")}
	service := NewService(embedToolCompleter(), embedTool(Embed{Title: "Card A"}))
	service.SetEmbedFilter(filter)

	got, err := service.GenerateResponseWithMetrics(context.Background(), Message{UserID: "u", Content: "hi"})
	if err != nil {
		t.Fatalf("a filter failure must not fail the response: %v", err)
	}
	if len(got.Embeds) != 1 {
		t.Fatalf("embeds = %+v, want the embed kept when the filter errors", got.Embeds)
	}
}

func TestServiceEmbedFilterSkippedWithoutEmbeds(t *testing.T) {
	filter := &fakeEmbedFilter{}
	service := NewService(embedToolCompleter(), embedTool())
	service.SetEmbedFilter(filter)

	if _, err := service.GenerateResponseWithMetrics(context.Background(), Message{UserID: "u", Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	if filter.calls != 0 {
		t.Fatalf("filter calls = %d, want 0 when no embeds were produced", filter.calls)
	}
}

func TestServiceCollectsEmbedsOnNonNativePath(t *testing.T) {
	completer := &fakeCompleter{responses: []string{
		`{"tool_calls":[{"name":"embed_tool","args":{}}]}`,
		"final answer",
	}}
	service := NewService(struct{ Completer }{completer}, embedTool(Embed{Title: "Card A"}))

	got, err := service.GenerateResponseWithMetrics(context.Background(), Message{UserID: "u", Content: "hi"})
	if err != nil {
		t.Fatalf("GenerateResponseWithMetrics: %v", err)
	}
	if got.Content != "final answer" || len(got.Embeds) != 1 {
		t.Fatalf("response = %+v", got)
	}
}

func TestToolContextCarriesMessageDetails(t *testing.T) {
	var gotCtx ToolContext
	tool := Tool{
		Name:       "ctx_tool",
		Parameters: json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, toolCtx ToolContext, _ json.RawMessage) (ToolResult, error) {
			gotCtx = toolCtx
			return ToolResult{Content: "ok"}, nil
		},
	}
	completer := &fakeCompleter{chat: []ChatResponse{
		{ToolCalls: []ChatToolCall{{Name: "ctx_tool", Arguments: json.RawMessage(`{}`)}}},
		{Content: "done"},
	}}
	service := NewService(completer, tool)
	if _, err := service.GenerateResponseWithMetrics(context.Background(), Message{UserID: "u", MessageID: "m9", Content: "  look this up  "}); err != nil {
		t.Fatal(err)
	}
	if gotCtx.MessageID != "m9" || gotCtx.Message != "look this up" {
		t.Fatalf("tool context = %+v", gotCtx)
	}
}
