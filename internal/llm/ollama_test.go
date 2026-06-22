package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestOllamaClientGenerateResponse(t *testing.T) {
	var got ollamaGenerateRequest
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/generate" {
			t.Fatalf("path = %s, want /api/generate", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		var body bytes.Buffer
		_ = json.NewEncoder(&body).Encode(ollamaGenerateResponse{Response: " hi there ", PromptEvalCount: 7, EvalCount: 3})
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(&body),
			Header:     make(http.Header),
		}, nil
	})

	client := NewOllamaClient(OllamaConfig{
		BaseURL: "http://ollama.test",
		Model:   "test-model",
		Timeout: time.Second,
		HTTPClient: &http.Client{
			Transport: transport,
		},
	})
	resp, err := client.GenerateResponse(context.Background(), Message{Username: "Tej", Content: "hello"})
	if err != nil {
		t.Fatalf("GenerateResponse: %v", err)
	}
	if resp != "hi there" {
		t.Fatalf("response = %q, want %q", resp, "hi there")
	}
	metricsResp, err := client.CompleteWithMetrics(context.Background(), CompletionRequest{SystemPrompt: "system", UserPrompt: "hello"})
	if err != nil {
		t.Fatalf("CompleteWithMetrics: %v", err)
	}
	if metricsResp.Usage.PromptTokens != 7 || metricsResp.Usage.CompletionTokens != 3 {
		t.Fatalf("usage = %+v, want prompt=7 completion=3", metricsResp.Usage)
	}
	if got.Model != "test-model" {
		t.Fatalf("model = %q, want test-model", got.Model)
	}
	if got.System == "" {
		t.Fatalf("system prompt is empty")
	}
	if got.Prompt == "" {
		t.Fatalf("user prompt is empty")
	}
	if got.Stream {
		t.Fatalf("stream = true, want false")
	}
}

func TestOllamaClientChatWithTools(t *testing.T) {
	var got ollamaChatRequest
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path = %s, want /api/chat", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		var body bytes.Buffer
		_ = json.NewEncoder(&body).Encode(ollamaChatResponse{
			Message: ollamaChatMessage{
				Role: "assistant",
				ToolCalls: []ollamaToolCall{
					{Function: ollamaToolCallFunction{Name: "reminder_list_active", Arguments: json.RawMessage(`{}`)}},
				},
			},
			PromptEvalCount: 11,
			EvalCount:       2,
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(&body),
			Header:     make(http.Header),
		}, nil
	})

	client := NewOllamaClient(OllamaConfig{
		BaseURL: "http://ollama.test",
		Model:   "test-model",
		HTTPClient: &http.Client{
			Transport: transport,
		},
	})
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "list reminders"}},
		Tools: []ChatTool{{
			Name:        "reminder_list_active",
			Description: "List reminders.",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got.Model != "test-model" {
		t.Fatalf("model = %q, want test-model", got.Model)
	}
	if len(got.Tools) != 1 || got.Tools[0].Type != "function" || got.Tools[0].Function.Name != "reminder_list_active" {
		t.Fatalf("tool request mismatch: %#v", got.Tools)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "reminder_list_active" {
		t.Fatalf("tool calls mismatch: %#v", resp.ToolCalls)
	}
	if resp.Usage.PromptTokens != 11 || resp.Usage.CompletionTokens != 2 {
		t.Fatalf("usage = %+v, want prompt=11 completion=2", resp.Usage)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
