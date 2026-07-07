package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
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
		{ToolCalls: []ChatToolCall{{Name: "test_tool", Arguments: json.RawMessage(`{}`)}}, Usage: Usage{PromptTokens: 10, CompletionTokens: 2}},
		{Content: "final answer", Usage: Usage{PromptTokens: 12, CompletionTokens: 4}},
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

	got, err := service.GenerateResponseWithMetrics(context.Background(), Message{
		UserID:    "u",
		Username:  "name",
		ChannelID: "c",
		GuildID:   "g",
		Content:   "use a tool",
	})
	if err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if got.Content != "final answer" {
		t.Fatalf("response = %q, want final answer", got.Content)
	}
	if got.Usage.PromptTokens != 22 || got.Usage.CompletionTokens != 6 || got.Usage.TotalTokens() != 28 {
		t.Fatalf("usage = %+v, want prompt=22 completion=6 total=28", got.Usage)
	}
	if got.LLMTurns != 2 || got.ToolCalls != 1 {
		t.Fatalf("turn/tool counts = %d/%d, want 2/1", got.LLMTurns, got.ToolCalls)
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

func TestServiceFiltersKeywordToolsWhenMessageDoesNotMatch(t *testing.T) {
	completer := &fakeCompleter{responses: []string{"plain answer"}}
	service := NewService(completer, Tool{
		Name:        "keyword_tool",
		Description: "A keyword tool.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Keywords:    []string{"remind"},
		Execute: func(_ context.Context, _ ToolContext, _ json.RawMessage) (ToolResult, error) {
			t.Fatalf("tool should not execute")
			return ToolResult{}, nil
		},
	})

	got, err := service.GenerateResponse(context.Background(), Message{
		Content:  "what is your favorite anime",
		Timezone: "Asia/Kolkata",
		Now:      time.Date(2026, 6, 22, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if got != "plain answer" {
		t.Fatalf("response = %q, want plain answer", got)
	}
	if len(completer.chats) != 0 {
		t.Fatalf("chat calls = %d, want 0", len(completer.chats))
	}
	if len(completer.requests) != 1 {
		t.Fatalf("complete requests = %d, want 1", len(completer.requests))
	}
	if !strings.Contains(completer.requests[0].UserPrompt, "User timezone: Asia/Kolkata") {
		t.Fatalf("user prompt missing timezone context: %q", completer.requests[0].UserPrompt)
	}
}

func TestServiceIncludesKeywordToolsWhenMessageMatches(t *testing.T) {
	completer := &fakeCompleter{chat: []ChatResponse{{Content: "tool-aware answer"}}}
	service := NewService(completer, Tool{
		Name:        "keyword_tool",
		Description: "A keyword tool.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Keywords:    []string{"remind"},
		Execute: func(_ context.Context, _ ToolContext, _ json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "ok"}, nil
		},
	})

	got, err := service.GenerateResponse(context.Background(), Message{Content: "remind me tomorrow"})
	if err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if got != "tool-aware answer" {
		t.Fatalf("response = %q, want tool-aware answer", got)
	}
	if len(completer.chats) != 1 {
		t.Fatalf("chat calls = %d, want 1", len(completer.chats))
	}
	if len(completer.chats[0].Tools) != 1 || completer.chats[0].Tools[0].Name != "keyword_tool" {
		t.Fatalf("tools = %+v, want keyword_tool", completer.chats[0].Tools)
	}
}

func TestServiceGenerateResponseAddsMatchingGuildInstructions(t *testing.T) {
	completer := &fakeCompleter{responses: []string{"answer"}}
	service := NewServiceWithGuildInstructions(completer, map[string]string{
		"guild-1": "Do not discuss banned topic.",
	})

	if _, err := service.GenerateResponse(context.Background(), Message{
		GuildID: "guild-1",
		Content: "hello",
	}); err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if len(completer.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(completer.requests))
	}
	if !strings.Contains(completer.requests[0].SystemPrompt, "Server-specific instructions:") {
		t.Fatalf("system prompt missing server instruction header: %q", completer.requests[0].SystemPrompt)
	}
	if !strings.Contains(completer.requests[0].SystemPrompt, "Do not discuss banned topic.") {
		t.Fatalf("system prompt missing guild instruction: %q", completer.requests[0].SystemPrompt)
	}
}

func TestServiceUsesServerBotNameInSystemPrompt(t *testing.T) {
	completer := &fakeCompleter{responses: []string{"answer"}}
	service := NewService(completer)

	if _, err := service.GenerateResponse(context.Background(), Message{
		BotName: "Mizu Helper",
		Content: "hello",
	}); err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if !strings.Contains(completer.requests[0].SystemPrompt, "You are Mizu Helper,") {
		t.Fatalf("system prompt missing server bot name: %q", completer.requests[0].SystemPrompt)
	}
}

func TestServiceGenerateResponseWithoutToolsEmbedsHistoryInUserPrompt(t *testing.T) {
	completer := &fakeCompleter{responses: []string{"answer"}}
	service := NewService(completer)

	if _, err := service.GenerateResponse(context.Background(), Message{
		BotName: "Mizu",
		Content: "and now?",
		History: []HistoryMessage{
			{Author: "Alice", Content: "what's the weather"},
			{Author: "", Content: "it's sunny", IsBot: true},
		},
	}); err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if len(completer.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(completer.requests))
	}
	prompt := completer.requests[0].UserPrompt
	if !strings.Contains(prompt, "Conversation history") {
		t.Fatalf("user prompt missing history header: %q", prompt)
	}
	if !strings.Contains(prompt, "Alice: what's the weather") {
		t.Fatalf("user prompt missing other-speaker history line: %q", prompt)
	}
	if !strings.Contains(prompt, "Mizu: it's sunny") {
		t.Fatalf("user prompt missing bot history line tagged with bot name: %q", prompt)
	}
	historyIdx := strings.Index(prompt, "Conversation history")
	messageIdx := strings.Index(prompt, "Message: and now?")
	if historyIdx < 0 || messageIdx < 0 || historyIdx > messageIdx {
		t.Fatalf("history should appear before the current message in the prompt: %q", prompt)
	}
}

func TestServiceGenerateResponseWithoutHistoryOmitsHistorySection(t *testing.T) {
	completer := &fakeCompleter{responses: []string{"answer"}}
	service := NewService(completer)

	if _, err := service.GenerateResponse(context.Background(), Message{Content: "hello"}); err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if strings.Contains(completer.requests[0].UserPrompt, "Conversation history") {
		t.Fatalf("user prompt should not mention history when none was provided: %q", completer.requests[0].UserPrompt)
	}
}

func TestServiceGenerateWithNativeToolsIncludesHistoryAsChatMessages(t *testing.T) {
	completer := &fakeCompleter{chat: []ChatResponse{{Content: "final answer"}}}
	service := NewService(completer, Tool{
		Name:        "keyword_tool",
		Description: "A keyword tool.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Keywords:    []string{"remind"},
		Execute: func(_ context.Context, _ ToolContext, _ json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "ok"}, nil
		},
	})

	got, err := service.GenerateResponse(context.Background(), Message{
		Content: "remind me tomorrow",
		History: []HistoryMessage{
			{Author: "Alice", Content: "hey there"},
			{Content: "hi Alice, how can I help?", IsBot: true},
		},
	})
	if err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if got != "final answer" {
		t.Fatalf("response = %q, want final answer", got)
	}
	if len(completer.chats) != 1 {
		t.Fatalf("chats = %d, want 1", len(completer.chats))
	}
	messages := completer.chats[0].Messages
	if len(messages) != 4 {
		t.Fatalf("messages = %d, want 4 (system, 2 history, user): %#v", len(messages), messages)
	}
	if messages[0].Role != "system" {
		t.Fatalf("messages[0].Role = %q, want system", messages[0].Role)
	}
	if messages[1].Role != "user" || messages[1].Content != "Alice: hey there" {
		t.Fatalf("messages[1] = %#v, want user Alice: hey there", messages[1])
	}
	if messages[2].Role != "assistant" || messages[2].Content != "hi Alice, how can I help?" {
		t.Fatalf("messages[2] = %#v, want assistant reply", messages[2])
	}
	if messages[3].Role != "user" || !strings.Contains(messages[3].Content, "Message: remind me tomorrow") {
		t.Fatalf("messages[3] = %#v, want current user message last", messages[3])
	}
	if strings.Contains(messages[3].Content, "Conversation history") {
		t.Fatalf("current user message should not duplicate history text: %#v", messages[3])
	}
}

func TestServiceGenerateResponseSkipsOtherGuildInstructions(t *testing.T) {
	completer := &fakeCompleter{responses: []string{"answer"}}
	service := NewServiceWithGuildInstructions(completer, map[string]string{
		"guild-1": "Do not discuss banned topic.",
	})

	if _, err := service.GenerateResponse(context.Background(), Message{
		GuildID: "guild-2",
		Content: "hello",
	}); err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if strings.Contains(completer.requests[0].SystemPrompt, "Do not discuss banned topic.") {
		t.Fatalf("system prompt included instruction for another guild: %q", completer.requests[0].SystemPrompt)
	}
}
