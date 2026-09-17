// Package typesafe is a client for the TypeSafe evaluation API
// (https://docs.typesafe.ai/api), which turns a piece of state and a set of
// typed questions into structured judgments from a System One model (Jev).
package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the TypeSafe evaluation endpoint.
	DefaultBaseURL = "https://api.typesafe.ai/v1/systemone"
	// DefaultModel is TypeSafe's flagship System One model.
	DefaultModel = "jev-latest"
)

type QuestionType string

const (
	QuestionNoul   QuestionType = "noul"
	QuestionChoice QuestionType = "choice"
	QuestionScore  QuestionType = "score"
)

// Question is one typed judgment to ask of the state. Criteria's shape
// depends on Type: a map[string]string for noul ("true"/"false" meanings),
// a map[string]string for choice (option -> rubric), or a []string for
// score (ordered level descriptions).
type Question struct {
	Type         QuestionType `json:"type"`
	Instructions string       `json:"instructions"`
	Criteria     any          `json:"criteria,omitempty"`
}

// Answer is one judgment result, keyed by the same id as its Question.
type Answer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type EvaluateRequest struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

type EvaluateResponse struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// APIError is returned for non-2xx responses from the evaluation endpoint.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("typesafe: request failed with status %d: %s", e.StatusCode, e.Body)
}

type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

func NewClient(cfg Config) *Client {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultModel
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		model:      model,
	}
}

// Model returns the model this client evaluates against, for logging.
func (c *Client) Model() string {
	if c == nil {
		return ""
	}
	return c.model
}

// Evaluate asks the TypeSafe API to judge state against questions, using
// the client's configured model.
func (c *Client) Evaluate(ctx context.Context, state any, questions map[string]Question) (EvaluateResponse, error) {
	if c == nil {
		return EvaluateResponse{}, fmt.Errorf("typesafe: nil client")
	}
	if c.apiKey == "" {
		return EvaluateResponse{}, fmt.Errorf("typesafe: missing API key")
	}
	if len(questions) == 0 {
		return EvaluateResponse{}, fmt.Errorf("typesafe: no questions to evaluate")
	}

	reqBody := EvaluateRequest{
		State:     state,
		Model:     c.model,
		Questions: questions,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return EvaluateResponse{}, fmt.Errorf("typesafe: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(payload))
	if err != nil {
		return EvaluateResponse{}, fmt.Errorf("typesafe: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return EvaluateResponse{}, fmt.Errorf("typesafe: call evaluate: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return EvaluateResponse{}, fmt.Errorf("typesafe: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return EvaluateResponse{}, &APIError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	var evalResp EvaluateResponse
	if err := json.Unmarshal(body, &evalResp); err != nil {
		return EvaluateResponse{}, fmt.Errorf("typesafe: parse response: %w", err)
	}
	return evalResp, nil
}
