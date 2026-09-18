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
	BotName   string
	ChannelID string
	GuildID   string
	MessageID string
	Content   string
	Timezone  string
	Now       time.Time
	History   []HistoryMessage
}

// HistoryMessage is a prior message in the conversation, provided as
// additional context ahead of the current user message.
type HistoryMessage struct {
	Author  string
	Content string
	IsBot   bool
}

type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
}

type Usage struct {
	PromptTokens     int64
	CompletionTokens int64
}

func (u Usage) TotalTokens() int64 {
	return u.PromptTokens + u.CompletionTokens
}

type CompletionResponse struct {
	Content string
	Usage   Usage
}

type Completer interface {
	Complete(ctx context.Context, request CompletionRequest) (string, error)
}

type MetricsCompleter interface {
	CompleteWithMetrics(ctx context.Context, request CompletionRequest) (CompletionResponse, error)
}

type ChatCompleter interface {
	Chat(ctx context.Context, request ChatRequest) (ChatResponse, error)
}

type ChatRequest struct {
	Messages []ChatMessage
	Tools    []ChatTool
}

type ChatMessage struct {
	Role       string
	Content    string
	ToolName   string
	ToolCallID string
	ToolCalls  []ChatToolCall
}

type ChatTool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type ChatToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type ChatResponse struct {
	Content   string
	ToolCalls []ChatToolCall
	Usage     Usage
}

type ToolContext struct {
	UserID    string
	Username  string
	ChannelID string
	GuildID   string
	// MessageID and Message identify the user message being answered, for
	// tools that need it as context for their own judgments or logging.
	MessageID string
	Message   string
}

// EmbedField is one name/value pair in an Embed.
type EmbedField struct {
	Name   string
	Value  string
	Inline bool
}

// Embed is a rich card a tool wants shown alongside the reply. It is
// deliberately Discord-agnostic; the bot converts it when sending.
type Embed struct {
	Title        string
	URL          string
	Description  string
	Color        int
	ThumbnailURL string
	Fields       []EmbedField
	Footer       string
	// Intent is a short plain-language label of what this embed is (e.g.
	// "anime info card"), used to word the EmbedFilter's question.
	Intent string
}

type ToolResult struct {
	Content string
	// Embeds are attached to the final reply. The EmbedFilter, when set,
	// may drop them after the reply is written.
	Embeds []Embed
}

type ToolHandler func(ctx context.Context, toolCtx ToolContext, args json.RawMessage) (ToolResult, error)

type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	Keywords    []string
	// ClassifierHint, when set, is used instead of Description to decide
	// whether this tool is relevant to a message (e.g. via ToolClassifier).
	// Description stays optimized for the LLM's own tool-calling schema;
	// ClassifierHint can spell out phrasing, abbreviations, or examples a
	// classifier needs but that would just be noise in the tool schema.
	ClassifierHint string
	// ImpliesTools names other tools that should also be made available
	// whenever the classifier selects this tool, even if they didn't clear
	// the relevance threshold on their own (e.g. reminder_delete implies
	// reminder_list_active, since deleting by ID often needs a prior list
	// call). The LLM still decides whether to actually call them.
	ImpliesTools []string
	Execute      ToolHandler
}

type GuildInstructionProvider interface {
	GetGuildInstruction(ctx context.Context, guildID string) (string, bool, error)
}

// MatchedFlag is a guild-defined content flag (see guildflags) that the
// classifier judged relevant to a message. Guidance is injected into the
// system prompt verbatim as a required rule for that response.
type MatchedFlag struct {
	Name       string
	Guidance   string
	Confidence float64
}

// ClassificationResult is what a ToolClassifier returns for one message:
// which tools are relevant (with confidence), and which guild flags fired.
type ClassificationResult struct {
	Tools map[string]float64
	Flags []MatchedFlag
}

// ToolClassifier decides which tools and guild flags are relevant to a
// message, as a smarter alternative to keyword matching. If it returns an
// error, classifyMessage falls back to keyword matching for tools (and no
// flags) for that message. Tools are keyed by tool name, with the
// classifier's confidence (0-1) that the tool applies.
type ToolClassifier interface {
	Classify(ctx context.Context, message Message, tools map[string]Tool) (ClassificationResult, error)
}

// EmbedFilter decides, after the reply is written, which of the embeds
// tools produced are still worth attaching. It returns the embeds to keep.
// If it errors, Service keeps every embed.
type EmbedFilter interface {
	Filter(ctx context.Context, message Message, reply string, embeds []Embed) ([]Embed, error)
}

type Service struct {
	completer                Completer
	tools                    map[string]Tool
	guildInstructionProvider GuildInstructionProvider
	toolClassifier           ToolClassifier
	embedFilter              EmbedFilter
}

type Response struct {
	Content   string
	Embeds    []Embed
	Usage     Usage
	LLMTurns  int64
	ToolCalls int64
}

func NewService(completer Completer, tools ...Tool) *Service {
	return NewServiceWithGuildInstructions(completer, nil, tools...)
}

func NewServiceWithGuildInstructions(completer Completer, guildInstructions map[string]string, tools ...Tool) *Service {
	return NewServiceWithGuildInstructionProvider(completer, staticGuildInstructionProvider(normalizeGuildInstructions(guildInstructions)), tools...)
}

func NewServiceWithGuildInstructionProvider(completer Completer, guildInstructionProvider GuildInstructionProvider, tools ...Tool) *Service {
	toolMap := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		if tool.Name == "" || tool.Execute == nil {
			continue
		}
		toolMap[tool.Name] = tool
	}
	return &Service{
		completer:                completer,
		tools:                    toolMap,
		guildInstructionProvider: guildInstructionProvider,
	}
}

// SetToolClassifier installs a ToolClassifier used to pick relevant tools
// for a message. When nil (the default), or when it errors on a given
// message, tools are selected by keyword match instead.
func (s *Service) SetToolClassifier(classifier ToolClassifier) {
	if s == nil {
		return
	}
	s.toolClassifier = classifier
}

// SetEmbedFilter installs an EmbedFilter run over tool-produced embeds once
// the reply is written. When nil (the default), all embeds are kept.
func (s *Service) SetEmbedFilter(filter EmbedFilter) {
	if s == nil {
		return
	}
	s.embedFilter = filter
}

func (s *Service) GenerateResponse(ctx context.Context, message Message) (string, error) {
	response, err := s.GenerateResponseWithMetrics(ctx, message)
	return response.Content, err
}

func (s *Service) GenerateResponseWithMetrics(ctx context.Context, message Message) (Response, error) {
	if s == nil || s.completer == nil {
		return Response{}, nil
	}
	message.Content = strings.TrimSpace(message.Content)
	if message.Content == "" {
		return Response{}, nil
	}
	tools, hints, flags := s.classifyMessage(ctx, message)
	if len(tools) == 0 {
		systemPrompt, err := s.buildSystemPrompt(ctx, message, flags)
		if err != nil {
			return Response{}, err
		}
		result, err := completeWithMetrics(ctx, s.completer, CompletionRequest{
			SystemPrompt: systemPrompt,
			UserPrompt:   buildUserPromptWithHistory(message),
		})
		if err != nil {
			return Response{}, err
		}
		result.LLMTurns = 1
		return result, nil
	}
	response, err := s.generateWithTools(ctx, message, tools, hints, flags)
	if err != nil || len(response.Embeds) == 0 || s.embedFilter == nil {
		return response, err
	}
	kept, filterErr := s.embedFilter.Filter(ctx, message, response.Content, response.Embeds)
	if filterErr != nil {
		log.Printf("llm embed filter failed, keeping all embeds: message_id=%s error=%v", message.MessageID, filterErr)
		return response, nil
	}
	response.Embeds = kept
	return response, nil
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

func (s *Service) generateWithTools(ctx context.Context, message Message, tools map[string]Tool, hints map[string]float64, flags []MatchedFlag) (Response, error) {
	if chatCompleter, ok := s.completer.(ChatCompleter); ok {
		return s.generateWithNativeTools(ctx, chatCompleter, message, tools, hints, flags)
	}

	systemPrompt, err := s.buildToolSystemPrompt(ctx, message, tools, flags)
	if err != nil {
		return Response{}, err
	}
	decisionResponse, err := completeWithMetrics(ctx, s.completer, CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   buildToolDecisionPrompt(message),
	})
	if err != nil {
		return Response{}, err
	}
	decisionResponse.LLMTurns = 1
	usage := decisionResponse.Usage

	decision, ok := parseToolDecision(decisionResponse.Content)
	if !ok {
		return Response{Content: strings.TrimSpace(decisionResponse.Content), Usage: usage, LLMTurns: decisionResponse.LLMTurns}, nil
	}
	if len(decision.ToolCalls) == 0 {
		return Response{Content: strings.TrimSpace(decision.FinalResponse), Usage: usage, LLMTurns: decisionResponse.LLMTurns}, nil
	}

	toolCtx := ToolContext{
		UserID:    message.UserID,
		Username:  message.Username,
		ChannelID: message.ChannelID,
		GuildID:   message.GuildID,
		MessageID: message.MessageID,
		Message:   message.Content,
	}
	results := make([]toolExecutionResult, 0, len(decision.ToolCalls))
	var embeds []Embed
	toolCalls := int64(len(decision.ToolCalls))
	for _, call := range decision.ToolCalls {
		tool, ok := tools[call.Name]
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
		embeds = append(embeds, result.Embeds...)
	}

	resultJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return Response{}, fmt.Errorf("marshal tool results: %w", err)
	}
	systemPrompt, err = s.buildSystemPrompt(ctx, message, flags)
	if err != nil {
		return Response{}, err
	}
	finalResponse, err := completeWithMetrics(ctx, s.completer, CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   buildToolResultPrompt(message, string(resultJSON)),
	})
	if err != nil {
		return Response{}, err
	}
	finalResponse.Usage = addUsage(usage, finalResponse.Usage)
	finalResponse.Embeds = embeds
	finalResponse.LLMTurns = decisionResponse.LLMTurns + 1
	finalResponse.ToolCalls = toolCalls
	return finalResponse, nil
}

func (s *Service) generateWithNativeTools(ctx context.Context, chatCompleter ChatCompleter, message Message, tools map[string]Tool, hints map[string]float64, flags []MatchedFlag) (Response, error) {
	selectedChatTools := chatTools(tools)
	systemPrompt, err := s.buildSystemPrompt(ctx, message, flags)
	if err != nil {
		return Response{}, err
	}
	systemContent := systemPrompt + "\n\n" + buildToolResponseStylePrompt()
	if hintPrompt := buildToolHintPrompt(hints, tools); hintPrompt != "" {
		systemContent += "\n\n" + hintPrompt
	}
	messages := []ChatMessage{
		{Role: "system", Content: systemContent},
	}
	messages = append(messages, historyChatMessages(message.History)...)
	messages = append(messages, ChatMessage{Role: "user", Content: buildUserPrompt(message)})
	usage := Usage{}
	var llmTurns int64
	var toolCalls int64
	var embeds []Embed

	toolCtx := ToolContext{
		UserID:    message.UserID,
		Username:  message.Username,
		ChannelID: message.ChannelID,
		GuildID:   message.GuildID,
		MessageID: message.MessageID,
		Message:   message.Content,
	}
	for range maxToolIterations {
		response, err := chatCompleter.Chat(ctx, ChatRequest{Messages: messages, Tools: selectedChatTools})
		if err != nil {
			return Response{}, err
		}
		llmTurns++
		usage = addUsage(usage, response.Usage)
		if len(response.ToolCalls) == 0 {
			return Response{Content: strings.TrimSpace(response.Content), Embeds: embeds, Usage: usage, LLMTurns: llmTurns, ToolCalls: toolCalls}, nil
		}
		toolCalls += int64(len(response.ToolCalls))

		messages = append(messages, ChatMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})
		for _, call := range response.ToolCalls {
			content, callEmbeds := executeToolCall(ctx, tools, toolCtx, call)
			embeds = append(embeds, callEmbeds...)
			messages = append(messages, ChatMessage{
				Role:       "tool",
				ToolName:   call.Name,
				ToolCallID: call.ID,
				Content:    content,
			})
		}
	}

	messages = append(messages, ChatMessage{
		Role:    "system",
		Content: "Tool call limit reached. Write the final response using the tool results already available. Do not call more tools.",
	})
	final, err := chatCompleter.Chat(ctx, ChatRequest{Messages: messages})
	if err != nil {
		return Response{}, err
	}
	llmTurns++
	usage = addUsage(usage, final.Usage)
	return Response{Content: strings.TrimSpace(final.Content), Embeds: embeds, Usage: usage, LLMTurns: llmTurns, ToolCalls: toolCalls}, nil
}

func executeToolCall(ctx context.Context, tools map[string]Tool, toolCtx ToolContext, call ChatToolCall) (string, []Embed) {
	tool, ok := tools[call.Name]
	if !ok {
		return "Error: unknown tool", nil
	}
	log.Printf("llm tool call: name=%s user_id=%s channel_id=%s", call.Name, toolCtx.UserID, toolCtx.ChannelID)
	result, err := tool.Execute(ctx, toolCtx, call.Arguments)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	return result.Content, result.Embeds
}

func completeWithMetrics(ctx context.Context, completer Completer, request CompletionRequest) (Response, error) {
	if metricsCompleter, ok := completer.(MetricsCompleter); ok {
		response, err := metricsCompleter.CompleteWithMetrics(ctx, request)
		if err != nil {
			return Response{}, err
		}
		return Response{Content: strings.TrimSpace(response.Content), Usage: response.Usage}, nil
	}
	content, err := completer.Complete(ctx, request)
	if err != nil {
		return Response{}, err
	}
	return Response{Content: strings.TrimSpace(content)}, nil
}

// historyChatMessages converts prior conversation turns into ChatMessages so
// native chat completers see them as distinct roles ahead of the current
// user message. Other users' turns are tagged with their display name since
// chat roles alone can't distinguish speakers in a multi-user channel.
func historyChatMessages(history []HistoryMessage) []ChatMessage {
	if len(history) == 0 {
		return nil
	}
	out := make([]ChatMessage, 0, len(history))
	for _, h := range history {
		if h.IsBot {
			out = append(out, ChatMessage{Role: "assistant", Content: h.Content})
			continue
		}
		out = append(out, ChatMessage{Role: "user", Content: historySpeakerLabel(h.Author) + ": " + h.Content})
	}
	return out
}

func historySpeakerLabel(author string) string {
	author = strings.TrimSpace(author)
	if author == "" {
		return "Discord user"
	}
	return author
}

func addUsage(a, b Usage) Usage {
	return Usage{
		PromptTokens:     a.PromptTokens + b.PromptTokens,
		CompletionTokens: a.CompletionTokens + b.CompletionTokens,
	}
}

func (s *Service) classifyMessage(ctx context.Context, message Message) (map[string]Tool, map[string]float64, []MatchedFlag) {
	if len(s.tools) == 0 {
		return nil, nil, nil
	}
	if s.toolClassifier != nil {
		if result, err := s.toolClassifier.Classify(ctx, message, s.tools); err == nil {
			out := make(map[string]Tool, len(result.Tools))
			for name := range result.Tools {
				if tool, ok := s.tools[name]; ok {
					out[name] = tool
				}
			}
			return out, result.Tools, result.Flags
		} else {
			log.Printf("tool classifier error, falling back to keyword match: %v", err)
		}
	}
	return s.keywordToolsForMessage(message), nil, nil
}

// buildFlagPrompt turns matched guild flags into a required-rules block for
// the system prompt. Unlike tool hints, flag guidance is not a suggestion:
// the LLM must follow it for this response.
func buildFlagPrompt(flags []MatchedFlag) string {
	if len(flags) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The following rules were flagged as applicable to this message and must be followed exactly:\n")
	for _, flag := range flags {
		guidance := strings.TrimSpace(flag.Guidance)
		if guidance == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s\n", guidance)
	}
	return strings.TrimSpace(b.String())
}

// buildToolHintPrompt turns per-tool classifier confidence into a short
// system-prompt note, so the LLM gets the classifier's signal instead of
// just a filtered tool list. hints with no matching tool are skipped.
func buildToolHintPrompt(hints map[string]float64, tools map[string]Tool) string {
	if len(hints) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("A pre-classifier flagged these tools as likely relevant to the user's message (not certain, use your own judgment):\n")
	wrote := false
	for name, confidence := range hints {
		if _, ok := tools[name]; !ok {
			continue
		}
		fmt.Fprintf(&b, "- %s (confidence %.2f)\n", name, confidence)
		wrote = true
	}
	if !wrote {
		return ""
	}
	return strings.TrimSpace(b.String())
}

func (s *Service) keywordToolsForMessage(message Message) map[string]Tool {
	normalized := normalizeToolMatchText(message.Content)
	out := make(map[string]Tool)
	for name, tool := range s.tools {
		if len(tool.Keywords) == 0 || matchesAnyKeyword(normalized, tool.Keywords) {
			out[name] = tool
		}
	}
	return out
}

func matchesAnyKeyword(content string, keywords []string) bool {
	for _, keyword := range keywords {
		keyword = normalizeToolMatchText(keyword)
		if keyword != "" && strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}

func normalizeToolMatchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	return strings.Join(strings.Fields(value), " ")
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

func (s *Service) buildSystemPrompt(ctx context.Context, message Message, flags []MatchedFlag) (string, error) {
	prompt := buildSystemPrompt(message.BotName)
	if s.guildInstructionProvider != nil {
		instruction, ok, err := s.guildInstructionProvider.GetGuildInstruction(ctx, message.GuildID)
		if err != nil {
			return "", fmt.Errorf("load guild instructions: %w", err)
		}
		if ok {
			if instruction = strings.TrimSpace(instruction); instruction != "" {
				prompt += "\n\nServer-specific instructions:\n" + instruction
			}
		}
	}
	if flagPrompt := buildFlagPrompt(flags); flagPrompt != "" {
		prompt += "\n\n" + flagPrompt
	}
	return prompt, nil
}

func (s *Service) buildToolSystemPrompt(ctx context.Context, message Message, tools map[string]Tool, flags []MatchedFlag) (string, error) {
	systemPrompt, err := s.buildSystemPrompt(ctx, message, flags)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(systemPrompt)
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
	return b.String(), nil
}

func normalizeGuildInstructions(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for guildID, instruction := range in {
		guildID = strings.TrimSpace(guildID)
		instruction = strings.TrimSpace(instruction)
		if guildID == "" || instruction == "" {
			continue
		}
		out[guildID] = instruction
	}
	return out
}

type staticGuildInstructionProvider map[string]string

func (p staticGuildInstructionProvider) GetGuildInstruction(_ context.Context, guildID string) (string, bool, error) {
	instruction, ok := p[strings.TrimSpace(guildID)]
	return instruction, ok, nil
}

func buildToolDecisionPrompt(message Message) string {
	return buildUserPromptWithHistory(message)
}

func buildToolResultPrompt(message Message, results string) string {
	return fmt.Sprintf(`User request:
%s

Tool results:
%s

%s

Write the final response to the user. Be concise and mention any tool error plainly.`, buildUserPromptWithHistory(message), results, buildToolResponseStylePrompt())
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
