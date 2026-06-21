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
		_ = json.NewEncoder(&body).Encode(ollamaGenerateResponse{Response: " hi there "})
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
