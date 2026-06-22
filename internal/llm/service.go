package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

const maxToolIterations = 4

type Message struct {
	UserID    string
	Username  string
	ChannelID string
	GuildID   string
	Content   string
}

type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
}

type Completer interface {
	Complete(ctx context.Context, request CompletionRequest) (string, error)
}

type ChatCompleter interface {
	Chat(ctx context.Context, request ChatRequest) (ChatResponse, error)
}

type ChatRequest struct {
	Messages []ChatMessage
	Tools    []ChatTool
}

type ChatMessage struct {
	Role      string
	Content   string
	ToolName  string
	ToolCalls []ChatToolCall
}

type ChatTool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type ChatToolCall struct {
	Name      string
	Arguments json.RawMessage
}

type ChatResponse struct {
	Content   string
	ToolCalls []ChatToolCall
}

type ToolContext struct {
	UserID    string
	Username  string
	ChannelID string
	GuildID   string
}

type ToolResult struct {
	Content string
}

type ToolHandler func(ctx context.Context, toolCtx ToolContext, args json.RawMessage) (ToolResult, error)

type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	Execute     ToolHandler
}

type Service struct {
	completer Completer
	tools     map[string]Tool
}

func NewService(completer Completer, tools ...Tool) *Service {
	toolMap := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		if tool.Name == "" || tool.Execute == nil {
			continue
		}
		toolMap[tool.Name] = tool
	}
	return &Service{completer: completer, tools: toolMap}
}

func (s *Service) GenerateResponse(ctx context.Context, message Message) (string, error) {
	if s == nil || s.completer == nil {
		return "", nil
	}
	message.Content = strings.TrimSpace(message.Content)
	if message.Content == "" {
		return "", nil
	}
	if len(s.tools) == 0 {
		return s.completer.Complete(ctx, CompletionRequest{
			SystemPrompt: buildSystemPrompt(),
			UserPrompt:   buildUserPrompt(message),
		})
	}
	return s.generateWithTools(ctx, message)
}

type toolDecision struct {
	FinalResponse string     `json:"final_response"`
	ToolCalls     []toolCall `json:"tool_calls"`
}

type toolCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type toolExecutionResult struct {
	Name   string `json:"name"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (s *Service) generateWithTools(ctx context.Context, message Message) (string, error) {
	if chatCompleter, ok := s.completer.(ChatCompleter); ok {
		return s.generateWithNativeTools(ctx, chatCompleter, message)
	}

	decisionText, err := s.completer.Complete(ctx, CompletionRequest{
		SystemPrompt: buildToolSystemPrompt(s.tools),
		UserPrompt:   buildToolDecisionPrompt(message),
	})
	if err != nil {
		return "", err
	}

	decision, ok := parseToolDecision(decisionText)
	if !ok {
		return strings.TrimSpace(decisionText), nil
	}
	if len(decision.ToolCalls) == 0 {
		return strings.TrimSpace(decision.FinalResponse), nil
	}

	toolCtx := ToolContext{
		UserID:    message.UserID,
		Username:  message.Username,
		ChannelID: message.ChannelID,
		GuildID:   message.GuildID,
	}
	results := make([]toolExecutionResult, 0, len(decision.ToolCalls))
	for _, call := range decision.ToolCalls {
		tool, ok := s.tools[call.Name]
		if !ok {
			results = append(results, toolExecutionResult{Name: call.Name, Error: "unknown tool"})
			continue
		}
		log.Printf("llm tool call: name=%s user_id=%s channel_id=%s", call.Name, message.UserID, message.ChannelID)
		result, err := tool.Execute(ctx, toolCtx, call.Args)
		if err != nil {
			results = append(results, toolExecutionResult{Name: call.Name, Error: err.Error()})
			continue
		}
		results = append(results, toolExecutionResult{Name: call.Name, Result: result.Content})
	}

	resultJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal tool results: %w", err)
	}
	return s.completer.Complete(ctx, CompletionRequest{
		SystemPrompt: buildSystemPrompt(),
		UserPrompt:   buildToolResultPrompt(message, string(resultJSON)),
	})
}

func (s *Service) generateWithNativeTools(ctx context.Context, chatCompleter ChatCompleter, message Message) (string, error) {
	tools := chatTools(s.tools)
	messages := []ChatMessage{
		{Role: "system", Content: buildSystemPrompt() + "\n\n" + buildToolResponseStylePrompt()},
		{Role: "user", Content: buildUserPrompt(message)},
	}

	toolCtx := ToolContext{
		UserID:    message.UserID,
		Username:  message.Username,
		ChannelID: message.ChannelID,
		GuildID:   message.GuildID,
	}
	for range maxToolIterations {
		response, err := chatCompleter.Chat(ctx, ChatRequest{Messages: messages, Tools: tools})
		if err != nil {
			return "", err
		}
		if len(response.ToolCalls) == 0 {
			return strings.TrimSpace(response.Content), nil
		}

		messages = append(messages, ChatMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})
		for _, call := range response.ToolCalls {
			result := executeToolCall(ctx, s.tools, toolCtx, call)
			messages = append(messages, ChatMessage{
				Role:     "tool",
				ToolName: call.Name,
				Content:  result,
			})
		}
	}

	messages = append(messages, ChatMessage{
		Role:    "system",
		Content: "Tool call limit reached. Write the final response using the tool results already available. Do not call more tools.",
	})
	final, err := chatCompleter.Chat(ctx, ChatRequest{Messages: messages})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(final.Content), nil
}

func executeToolCall(ctx context.Context, tools map[string]Tool, toolCtx ToolContext, call ChatToolCall) string {
	tool, ok := tools[call.Name]
	if !ok {
		return "Error: unknown tool"
	}
	log.Printf("llm tool call: name=%s user_id=%s channel_id=%s", call.Name, toolCtx.UserID, toolCtx.ChannelID)
	result, err := tool.Execute(ctx, toolCtx, call.Arguments)
	if err != nil {
		return "Error: " + err.Error()
	}
	return result.Content
}

func chatTools(tools map[string]Tool) []ChatTool {
	out := make([]ChatTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, ChatTool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		})
	}
	return out
}

func parseToolDecision(raw string) (toolDecision, bool) {
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return toolDecision{}, false
	}
	var decision toolDecision
	if err := json.Unmarshal([]byte(raw[start:end+1]), &decision); err != nil {
		return toolDecision{}, false
	}
	return decision, true
}

func buildToolSystemPrompt(tools map[string]Tool) string {
	var b strings.Builder
	b.WriteString(buildSystemPrompt())
	b.WriteString("\n\nYou may call tools when they are needed to answer the user or perform a supported action.")
	b.WriteString("\nCurrent UTC time: ")
	b.WriteString(time.Now().UTC().Format(time.RFC3339))
	b.WriteString("\n\nAvailable tools:")
	for _, tool := range tools {
		fmt.Fprintf(&b, "\n- %s: %s\n  Parameters JSON schema: %s", tool.Name, tool.Description, string(tool.Parameters))
	}
	b.WriteString(`

Return only JSON in this exact shape:
{"final_response":"message to user if no tool is needed","tool_calls":[{"name":"tool_name","args":{}}]}

Use tool_calls when you need current reminder data or need to create/delete a reminder.
If no tool is needed, return tool_calls as an empty array and put your answer in final_response.
Do not invent tool names or parameters.`)
	return b.String()
}

func buildToolDecisionPrompt(message Message) string {
	return buildUserPrompt(message)
}

func buildToolResultPrompt(message Message, results string) string {
	return fmt.Sprintf(`User request:
%s

Tool results:
%s

%s

Write the final response to the user. Be concise and mention any tool error plainly.`, buildUserPrompt(message), results, buildToolResponseStylePrompt())
}

func buildToolResponseStylePrompt() string {
	return `When using reminder tool results:
- Do not say "cron", "cron job", "tool", or expose implementation details unless the user specifically asks.
- For listed reminders, include the message, next run using the Discord timestamp from the tool result, channel, and timezone.
- Include reminder IDs only when they help the user act on the reminder, such as when listing multiple reminders, disambiguating similar reminders, or after creating/deleting one.
- For deleted reminders, confirm the deletion and include the ID if that is all the tool result provides. If more detail is available, mention what was removed.
- For created reminders, include the reminder ID, message, next run, channel, and timezone.
- When creating reminders, infer a concise reminder message from the user's intent instead of copying the whole command. For example, "remind me to take meds tomorrow" should create message "take meds". Preserve exact text only when the user quotes it or explicitly asks for that exact wording.
- For reminder_create, use once=true with run_at for one-time reminders. Use once=false with cron_expr for repeated reminders. Do not pass slash-command style schedule/at fields.
- Prefer clear Discord-friendly formatting with short bullets for multiple reminders.`
}
