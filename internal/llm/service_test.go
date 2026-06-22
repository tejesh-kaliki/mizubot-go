package llm

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeCompleter struct {
	responses []string
	requests  []CompletionRequest
	chat      []ChatResponse
	chats     []ChatRequest
}

func (f *fakeCompleter) Complete(_ context.Context, request CompletionRequest) (string, error) {
	f.requests = append(f.requests, request)
	if len(f.responses) == 0 {
		return "", nil
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return resp, nil
}

func (f *fakeCompleter) Chat(_ context.Context, request ChatRequest) (ChatResponse, error) {
	f.chats = append(f.chats, request)
	if len(f.chat) == 0 {
		return ChatResponse{}, nil
	}
	resp := f.chat[0]
	f.chat = f.chat[1:]
	return resp, nil
}

func TestServiceGenerateResponseWithTool(t *testing.T) {
	completer := &fakeCompleter{chat: []ChatResponse{
		{ToolCalls: []ChatToolCall{{Name: "test_tool", Arguments: json.RawMessage(`{}`)}}},
		{Content: "final answer"},
	}}
	var gotToolCtx ToolContext
	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, toolCtx ToolContext, _ json.RawMessage) (ToolResult, error) {
			gotToolCtx = toolCtx
			return ToolResult{Content: "tool result"}, nil
		},
	}
	service := NewService(completer, tool)

	got, err := service.GenerateResponse(context.Background(), Message{
		UserID:    "u",
		Username:  "name",
		ChannelID: "c",
		GuildID:   "g",
		Content:   "use a tool",
	})
	if err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if got != "final answer" {
		t.Fatalf("response = %q, want final answer", got)
	}
	if len(completer.chats) != 2 {
		t.Fatalf("chats = %d, want 2", len(completer.chats))
	}
	if len(completer.chats[0].Tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(completer.chats[0].Tools))
	}
	if gotToolCtx.UserID != "u" || gotToolCtx.ChannelID != "c" || gotToolCtx.GuildID != "g" {
		t.Fatalf("tool context mismatch: %#v", gotToolCtx)
	}
}

func TestServiceGenerateResponseAllowsMultipleToolRounds(t *testing.T) {
	completer := &fakeCompleter{chat: []ChatResponse{
		{ToolCalls: []ChatToolCall{{Name: "test_tool", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []ChatToolCall{{Name: "test_tool", Arguments: json.RawMessage(`{}`)}}},
		{Content: "final answer"},
	}}
	calls := 0
	service := NewService(completer, Tool{
		Name:        "test_tool",
		Description: "A test tool.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ ToolContext, _ json.RawMessage) (ToolResult, error) {
			calls++
			return ToolResult{Content: "tool result"}, nil
		},
	})

	got, err := service.GenerateResponse(context.Background(), Message{Content: "use tools"})
	if err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if got != "final answer" {
		t.Fatalf("response = %q, want final answer", got)
	}
	if calls != 2 {
		t.Fatalf("tool calls = %d, want 2", calls)
	}
	if len(completer.chats) != 3 {
		t.Fatalf("chats = %d, want 3", len(completer.chats))
	}
}

func TestServiceGenerateResponseStopsToolLoopAtLimit(t *testing.T) {
	completer := &fakeCompleter{chat: []ChatResponse{
		{ToolCalls: []ChatToolCall{{Name: "test_tool", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []ChatToolCall{{Name: "test_tool", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []ChatToolCall{{Name: "test_tool", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []ChatToolCall{{Name: "test_tool", Arguments: json.RawMessage(`{}`)}}},
		{Content: "limited final answer"},
	}}
	service := NewService(completer, Tool{
		Name:        "test_tool",
		Description: "A test tool.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ ToolContext, _ json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "tool result"}, nil
		},
	})

	got, err := service.GenerateResponse(context.Background(), Message{Content: "loop tools"})
	if err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if got != "limited final answer" {
		t.Fatalf("response = %q, want limited final answer", got)
	}
	if len(completer.chats) != maxToolIterations+1 {
		t.Fatalf("chats = %d, want %d", len(completer.chats), maxToolIterations+1)
	}
	if len(completer.chats[len(completer.chats)-1].Tools) != 0 {
		t.Fatalf("final chat should not include tools")
	}
}

func TestServiceGenerateResponseWithoutToolCall(t *testing.T) {
	completer := &fakeCompleter{chat: []ChatResponse{
		{Content: "direct answer"},
	}}
	service := NewService(completer, Tool{
		Name:        "test_tool",
		Description: "A test tool.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ ToolContext, _ json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "tool result"}, nil
		},
	})

	got, err := service.GenerateResponse(context.Background(), Message{Content: "hello"})
	if err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if got != "direct answer" {
		t.Fatalf("response = %q, want direct answer", got)
	}
	if len(completer.chats) != 1 {
		t.Fatalf("chats = %d, want 1", len(completer.chats))
	}
}
