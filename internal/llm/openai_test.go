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

func TestOpenAIClientGenerateResponse(t *testing.T) {
	var got map[string]any
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		var body bytes.Buffer
		_ = json.NewEncoder(&body).Encode(map[string]any{
			"id":      "chatcmpl-1",
			"object":  "chat.completion",
			"created": 1,
			"model":   "test-model",
			"choices": []map[string]any{
				{
					"index":         0,
					"finish_reason": "stop",
					"message": map[string]any{
						"role":    "assistant",
						"content": " hi there ",
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     7,
				"completion_tokens": 3,
				"total_tokens":      10,
			},
		})
		header := make(http.Header)
		header.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(&body),
			Header:     header,
		}, nil
	})

	client := NewOpenAIClient(OpenAIConfig{
		BaseURL: "http://bifrost.test/v1",
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
	if got["model"] != "test-model" {
		t.Fatalf("model = %v, want test-model", got["model"])
	}
	messages, _ := got["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v, want system + user", messages)
	}
}

func TestOpenAIClientChatWithTools(t *testing.T) {
	var got map[string]any
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		var body bytes.Buffer
		_ = json.NewEncoder(&body).Encode(map[string]any{
			"id":      "chatcmpl-2",
			"object":  "chat.completion",
			"created": 1,
			"model":   "test-model",
			"choices": []map[string]any{
				{
					"index":         0,
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"role":    "assistant",
						"content": nil,
						"tool_calls": []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "reminder_list_active",
									"arguments": "{}",
								},
							},
						},
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     11,
				"completion_tokens": 2,
				"total_tokens":      13,
			},
		})
		header := make(http.Header)
		header.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(&body),
			Header:     header,
		}, nil
	})

	client := NewOpenAIClient(OpenAIConfig{
		BaseURL: "http://bifrost.test/v1",
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
	if got["model"] != "test-model" {
		t.Fatalf("model = %v, want test-model", got["model"])
	}
	tools, _ := got["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tool request mismatch: %#v", got["tools"])
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "reminder_list_active" || resp.ToolCalls[0].ID != "call_1" {
		t.Fatalf("tool calls mismatch: %#v", resp.ToolCalls)
	}
	if resp.Usage.PromptTokens != 11 || resp.Usage.CompletionTokens != 2 {
		t.Fatalf("usage = %+v, want prompt=11 completion=2", resp.Usage)
	}
}

func TestOpenAIMessagesRoundTripsToolCallID(t *testing.T) {
	messages := openaiMessages([]ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []ChatToolCall{{ID: "call_1", Name: "foo", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", ToolName: "foo", ToolCallID: "call_1", Content: "result"},
	})
	if len(messages) != 4 {
		t.Fatalf("messages = %d, want 4", len(messages))
	}
	toolMsg := messages[3]
	if toolMsg.OfTool == nil || toolMsg.OfTool.ToolCallID != "call_1" {
		t.Fatalf("tool message = %#v, want tool_call_id=call_1", toolMsg)
	}
	assistantMsg := messages[2]
	if assistantMsg.OfAssistant == nil || len(assistantMsg.OfAssistant.ToolCalls) != 1 || assistantMsg.OfAssistant.ToolCalls[0].ID != "call_1" {
		t.Fatalf("assistant message = %#v, want tool call id=call_1", assistantMsg)
	}
}

func TestNewOpenAIClientAppendsV1(t *testing.T) {
	cases := map[string]string{
		"http://bifrost.test":           "http://bifrost.test/v1",
		"http://bifrost.test/":          "http://bifrost.test/v1",
		"http://bifrost.test/v1":        "http://bifrost.test/v1",
		"http://bifrost.test/v1/":       "http://bifrost.test/v1",
		"http://bifrost.test/openai/v1": "http://bifrost.test/openai/v1",
	}
	for in, want := range cases {
		var gotURL string
		transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotURL = "http://" + r.URL.Host + r.URL.Path
			gotURL = gotURL[:len(gotURL)-len("/chat/completions")]
			var body bytes.Buffer
			_ = json.NewEncoder(&body).Encode(map[string]any{
				"id": "x", "object": "chat.completion", "created": 1, "model": "m",
				"choices": []map[string]any{{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "ok"}}},
				"usage":   map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
			})
			header := make(http.Header)
			header.Set("Content-Type", "application/json")
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(&body), Header: header}, nil
		})
		client := NewOpenAIClient(OpenAIConfig{BaseURL: in, Model: "m", HTTPClient: &http.Client{Transport: transport}})
		if _, err := client.Complete(context.Background(), CompletionRequest{UserPrompt: "hi"}); err != nil {
			t.Fatalf("Complete(%q): %v", in, err)
		}
		if gotURL != want {
			t.Fatalf("BaseURL(%q) -> %q, want %q", in, gotURL, want)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
