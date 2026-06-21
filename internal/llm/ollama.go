package llm

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
	DefaultOllamaBaseURL = "http://localhost:11434"
	DefaultOllamaModel   = "llama3.2"
)

type OllamaConfig struct {
	BaseURL    string
	Model      string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type OllamaClient struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaClient(cfg OllamaConfig) *OllamaClient {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultOllamaBaseURL
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultOllamaModel
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &OllamaClient{
		baseURL: baseURL,
		model:   model,
		client:  ollamaHTTPClient(cfg.HTTPClient, timeout),
	}
}

func ollamaHTTPClient(client *http.Client, timeout time.Duration) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: timeout}
}

type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	System string `json:"system"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
	Error    string `json:"error"`
}

func (c *OllamaClient) GenerateResponse(ctx context.Context, message Message) (string, error) {
	if c == nil {
		return "", nil
	}
	reqBody := ollamaGenerateRequest{
		Model:  c.model,
		System: buildSystemPrompt(),
		Prompt: buildUserPrompt(message),
		Stream: false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call ollama: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read ollama response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var out ollamaGenerateResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	if out.Error != "" {
		return "", fmt.Errorf("ollama error: %s", out.Error)
	}
	return strings.TrimSpace(out.Response), nil
}

func buildSystemPrompt() string {
	return `You are MizuBot, a simple helper Discord bot that answers users' questions clearly and concisely.
You were created by Mizuna, a software engineer who likes experimenting with technology and likes the Ascendance of a Bookworm series.
Let that origin inform a warm, curious, technically capable personality, but do not force references to Mizuna or the series unless relevant.
Stay helpful, conversational, and direct.
Do not mention that you are using an LLM.`
}

func buildUserPrompt(message Message) string {
	username := strings.TrimSpace(message.Username)
	if username == "" {
		username = "the Discord user"
	}
	return fmt.Sprintf(`User: %s
Message: %s

Response:`, username, message.Content)
}
