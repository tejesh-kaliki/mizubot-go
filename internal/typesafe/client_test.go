package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestClientEvaluate(t *testing.T) {
	var gotReq EvaluateRequest
	var gotAuth string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		var body bytes.Buffer
		_ = json.NewEncoder(&body).Encode(map[string]any{
			"model": "jev-latest",
			"answers": map[string]any{
				"reminders_tool": map[string]any{
					"type": "noul",
					"noul": 0.87,
				},
			},
			"usage": map[string]any{
				"input_tokens":  42,
				"output_tokens": 6,
			},
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(&body),
			Header:     make(http.Header),
		}, nil
	})

	client := NewClient(Config{
		APIKey:     "test-key",
		Model:      "jev-latest",
		Timeout:    time.Second,
		HTTPClient: &http.Client{Transport: transport},
	})

	resp, err := client.Evaluate(context.Background(), "remind me tomorrow", map[string]Question{
		"reminders_tool": {
			Type:         QuestionNoul,
			Instructions: "Should the reminders tool be used?",
		},
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotReq.Model != "jev-latest" {
		t.Fatalf("request model = %q, want jev-latest", gotReq.Model)
	}
	if gotReq.State != "remind me tomorrow" {
		t.Fatalf("request state = %v, want %q", gotReq.State, "remind me tomorrow")
	}

	answer, ok := resp.Answers["reminders_tool"]
	if !ok {
		t.Fatalf("missing answer for reminders_tool")
	}
	if answer.Noul == nil || *answer.Noul != 0.87 {
		t.Fatalf("noul = %v, want 0.87", answer.Noul)
	}
	if resp.Usage.InputTokens != 42 || resp.Usage.OutputTokens != 6 {
		t.Fatalf("usage = %+v, want input=42 output=6", resp.Usage)
	}
}

func TestClientEvaluateAPIError(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Status:     "429 Too Many Requests",
			Body:       io.NopCloser(bytes.NewBufferString(`{"error":"rate limited"}`)),
			Header:     make(http.Header),
		}, nil
	})

	client := NewClient(Config{
		APIKey:     "test-key",
		Timeout:    time.Second,
		HTTPClient: &http.Client{Transport: transport},
	})

	_, err := client.Evaluate(context.Background(), "hi", map[string]Question{
		"q": {Type: QuestionNoul, Instructions: "test"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", apiErr.StatusCode)
	}
}

func TestClientEvaluateRequiresAPIKey(t *testing.T) {
	client := NewClient(Config{Timeout: time.Second})
	_, err := client.Evaluate(context.Background(), "hi", map[string]Question{
		"q": {Type: QuestionNoul, Instructions: "test"},
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}
