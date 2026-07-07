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
	Response        string `json:"response"`
	Error           string `json:"error"`
	PromptEvalCount int64  `json:"prompt_eval_count"`
	EvalCount       int64  `json:"eval_count"`
}

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Tools    []ollamaTool        `json:"tools,omitempty"`
	Stream   bool                `json:"stream"`
}

type ollamaChatMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ollamaChatResponse struct {
	Message         ollamaChatMessage `json:"message"`
	Error           string            `json:"error"`
	PromptEvalCount int64             `json:"prompt_eval_count"`
	EvalCount       int64             `json:"eval_count"`
}

func (c *OllamaClient) Complete(ctx context.Context, request CompletionRequest) (string, error) {
	response, err := c.CompleteWithMetrics(ctx, request)
	return response.Content, err
}

func (c *OllamaClient) CompleteWithMetrics(ctx context.Context, request CompletionRequest) (CompletionResponse, error) {
	if c == nil {
		return CompletionResponse{}, nil
	}
	reqBody := ollamaGenerateRequest{
		Model:  c.model,
		System: request.SystemPrompt,
		Prompt: request.UserPrompt,
		Stream: false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("create ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("call ollama: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("read ollama response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CompletionResponse{}, fmt.Errorf("ollama returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var out ollamaGenerateResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return CompletionResponse{}, fmt.Errorf("decode ollama response: %w", err)
	}
	if out.Error != "" {
		return CompletionResponse{}, fmt.Errorf("ollama error: %s", out.Error)
	}
	return CompletionResponse{
		Content: strings.TrimSpace(out.Response),
		Usage: Usage{
			PromptTokens:     out.PromptEvalCount,
			CompletionTokens: out.EvalCount,
		},
	}, nil
}

func (c *OllamaClient) GenerateResponse(ctx context.Context, message Message) (string, error) {
	return c.Complete(ctx, CompletionRequest{
		SystemPrompt: buildSystemPrompt(message.BotName),
		UserPrompt:   buildUserPrompt(message),
	})
}

func (c *OllamaClient) Chat(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	if c == nil {
		return ChatResponse{}, nil
	}
	reqBody := ollamaChatRequest{
		Model:    c.model,
		Messages: ollamaMessages(request.Messages),
		Tools:    ollamaTools(request.Tools),
		Stream:   false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal ollama chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create ollama chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("call ollama chat: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read ollama chat response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ChatResponse{}, fmt.Errorf("ollama chat returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var out ollamaChatResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return ChatResponse{}, fmt.Errorf("decode ollama chat response: %w", err)
	}
	if out.Error != "" {
		return ChatResponse{}, fmt.Errorf("ollama chat error: %s", out.Error)
	}
	return ChatResponse{
		Content:   strings.TrimSpace(out.Message.Content),
		ToolCalls: chatToolCalls(out.Message.ToolCalls),
		Usage: Usage{
			PromptTokens:     out.PromptEvalCount,
			CompletionTokens: out.EvalCount,
		},
	}, nil
}

func ollamaMessages(messages []ChatMessage) []ollamaChatMessage {
	out := make([]ollamaChatMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, ollamaChatMessage{
			Role:      message.Role,
			Content:   message.Content,
			ToolName:  message.ToolName,
			ToolCalls: ollamaToolCalls(message.ToolCalls),
		})
	}
	return out
}

func ollamaTools(tools []ChatTool) []ollamaTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]ollamaTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, ollamaTool{
			Type:     "function",
			Function: ollamaToolFunction(tool),
		})
	}
	return out
}

func ollamaToolCalls(calls []ChatToolCall) []ollamaToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ollamaToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, ollamaToolCall{Function: ollamaToolCallFunction(call)})
	}
	return out
}

func chatToolCalls(calls []ollamaToolCall) []ChatToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ChatToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, ChatToolCall{
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		})
	}
	return out
}

func buildSystemPrompt(botName string) string {
	botName = strings.TrimSpace(botName)
	if botName == "" {
		botName = "MizuBot"
	}
	return fmt.Sprintf(`You are %s, a simple helper Discord bot that answers users' questions clearly and concisely.
You were created by Mizuna, a software engineer who likes experimenting with technology and likes the Ascendance of a Bookworm series.
Let that origin inform a warm, curious, technically capable personality, but do not force references to Mizuna or the series unless relevant.
Stay helpful, conversational, and direct.
Do not mention that you are using an LLM.`, botName)
}

// buildUserPromptWithHistory embeds prior conversation turns as a text block
// ahead of the current message, for completers that only take a flat
// system/user prompt pair rather than a role-tagged message array.
func buildUserPromptWithHistory(message Message) string {
	history := buildHistoryBlock(message.History, message.BotName)
	if history == "" {
		return buildUserPrompt(message)
	}
	return history + "\n" + buildUserPrompt(message)
}

func buildHistoryBlock(history []HistoryMessage, botName string) string {
	if len(history) == 0 {
		return ""
	}
	botName = strings.TrimSpace(botName)
	if botName == "" {
		botName = "MizuBot"
	}
	var b strings.Builder
	b.WriteString("Conversation history (oldest first):\n")
	for _, h := range history {
		speaker := historySpeakerLabel(h.Author)
		if h.IsBot {
			speaker = botName
		}
		fmt.Fprintf(&b, "%s: %s\n", speaker, h.Content)
	}
	return b.String()
}

func buildUserPrompt(message Message) string {
	username := strings.TrimSpace(message.Username)
	if username == "" {
		username = "the Discord user"
	}
	now := message.Now
	if now.IsZero() {
		now = time.Now()
	}
	timezone := strings.TrimSpace(message.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
		timezone = "UTC"
	}
	localNow := now.In(loc)
	return fmt.Sprintf(`User: %s
Current date: %s
Current time: %s
User timezone: %s
Message: %s

Response:`, username, localNow.Format("2006-01-02"), localNow.Format(time.RFC3339), timezone, message.Content)
}
