package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

const (
	// DefaultLLMBaseURL points at a local Bifrost gateway
	// (https://getbifrost.ai), which exposes an OpenAI-compatible
	// /v1/chat/completions endpoint on port 8080 by default.
	DefaultLLMBaseURL = "http://localhost:8080/v1"
	DefaultLLMModel   = "llama3.2"
)

// OpenAIConfig configures a client for any OpenAI-compatible chat
// completions API (e.g. a Bifrost gateway), rather than a specific vendor.
type OpenAIConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// OpenAIClient talks to an OpenAI-compatible /chat/completions endpoint
// using the official OpenAI Go SDK.
type OpenAIClient struct {
	client openai.Client
	model  string
}

func NewOpenAIClient(cfg OpenAIConfig) *OpenAIClient {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultLLMBaseURL
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultLLMModel
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		// The SDK requires a non-empty key even against gateways (like a
		// locally-hosted Bifrost) that don't check it.
		apiKey = "not-needed"
	}

	return &OpenAIClient{
		client: openai.NewClient(
			option.WithBaseURL(baseURL),
			option.WithAPIKey(apiKey),
			option.WithHTTPClient(httpClient),
		),
		model: model,
	}
}

func (c *OpenAIClient) Complete(ctx context.Context, request CompletionRequest) (string, error) {
	response, err := c.CompleteWithMetrics(ctx, request)
	return response.Content, err
}

func (c *OpenAIClient) CompleteWithMetrics(ctx context.Context, request CompletionRequest) (CompletionResponse, error) {
	if c == nil {
		return CompletionResponse{}, nil
	}
	var messages []openai.ChatCompletionMessageParamUnion
	if strings.TrimSpace(request.SystemPrompt) != "" {
		messages = append(messages, openai.SystemMessage(request.SystemPrompt))
	}
	messages = append(messages, openai.UserMessage(request.UserPrompt))

	resp, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(c.model),
		Messages: messages,
	})
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("call llm: %w", err)
	}
	if len(resp.Choices) == 0 {
		return CompletionResponse{}, fmt.Errorf("llm returned no choices")
	}
	return CompletionResponse{
		Content: strings.TrimSpace(resp.Choices[0].Message.Content),
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
		},
	}, nil
}

func (c *OpenAIClient) GenerateResponse(ctx context.Context, message Message) (string, error) {
	return c.Complete(ctx, CompletionRequest{
		SystemPrompt: buildSystemPrompt(message.BotName),
		UserPrompt:   buildUserPrompt(message),
	})
}

func (c *OpenAIClient) Chat(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	if c == nil {
		return ChatResponse{}, nil
	}
	tools, err := openaiTools(request.Tools)
	if err != nil {
		return ChatResponse{}, err
	}

	resp, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(c.model),
		Messages: openaiMessages(request.Messages),
		Tools:    tools,
	})
	if err != nil {
		return ChatResponse{}, fmt.Errorf("call llm chat: %w", err)
	}
	if len(resp.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("llm chat returned no choices")
	}
	message := resp.Choices[0].Message
	return ChatResponse{
		Content:   strings.TrimSpace(message.Content),
		ToolCalls: chatToolCallsFromOpenAI(message.ToolCalls),
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
		},
	}, nil
}

func openaiMessages(messages []ChatMessage) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "system":
			out = append(out, openai.SystemMessage(message.Content))
		case "tool":
			out = append(out, openai.ToolMessage(message.Content, message.ToolCallID))
		case "assistant":
			out = append(out, openaiAssistantMessage(message))
		default: // "user" and anything else falls back to a user turn
			out = append(out, openai.UserMessage(message.Content))
		}
	}
	return out
}

func openaiAssistantMessage(message ChatMessage) openai.ChatCompletionMessageParamUnion {
	assistantParam := openai.ChatCompletionAssistantMessageParam{}
	if message.Content != "" {
		assistantParam.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
			OfString: openai.String(message.Content),
		}
	}
	if len(message.ToolCalls) > 0 {
		assistantParam.ToolCalls = openaiToolCallParams(message.ToolCalls)
	}
	return openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantParam}
}

func openaiToolCallParams(calls []ChatToolCall) []openai.ChatCompletionMessageToolCallParam {
	out := make([]openai.ChatCompletionMessageToolCallParam, 0, len(calls))
	for _, call := range calls {
		out = append(out, openai.ChatCompletionMessageToolCallParam{
			ID: call.ID,
			Function: openai.ChatCompletionMessageToolCallFunctionParam{
				Name:      call.Name,
				Arguments: string(call.Arguments),
			},
		})
	}
	return out
}

func openaiTools(tools []ChatTool) ([]openai.ChatCompletionToolParam, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	out := make([]openai.ChatCompletionToolParam, 0, len(tools))
	for _, tool := range tools {
		params, err := openaiToolParameters(tool.Parameters)
		if err != nil {
			return nil, fmt.Errorf("tool %s parameters: %w", tool.Name, err)
		}
		out = append(out, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        tool.Name,
				Description: openai.String(tool.Description),
				Parameters:  params,
			},
		})
	}
	return out, nil
}

func openaiToolParameters(raw json.RawMessage) (shared.FunctionParameters, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var params shared.FunctionParameters
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, err
	}
	return params, nil
}

func chatToolCallsFromOpenAI(calls []openai.ChatCompletionMessageToolCall) []ChatToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ChatToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, ChatToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: json.RawMessage(call.Function.Arguments),
		})
	}
	return out
}
