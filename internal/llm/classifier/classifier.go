// Package classifier implements llm.ToolClassifier using the TypeSafe
// evaluation API: one "noul" (yes/no) question per candidate tool, plus one
// per guild content flag (see guildflags), all asked in a single request,
// replacing the substring keyword match. Every request and response is
// logged (including token usage) via the supplied Logger, so the jev
// model's routing and flagging behavior can be inspected after the fact.
package classifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"mizubot-go/internal/guildflags"
	"mizubot-go/internal/llm"
	"mizubot-go/internal/typesafe"
	"mizubot-go/internal/typesafestats"
)

// Logger records a classification request/response. typesafestats.Store
// satisfies this.
type Logger interface {
	Create(ctx context.Context, params typesafestats.CreateClassificationLogParams) (typesafestats.ClassificationLog, error)
}

// FlagProvider loads a guild's content flags. guildflags.Store satisfies
// this.
type FlagProvider interface {
	ListByGuild(ctx context.Context, guildID string) ([]guildflags.Flag, error)
}

// relevanceThreshold is the minimum "yes" probability for a tool to be
// considered relevant to the message.
const relevanceThreshold = 0.5

// maxHistoryMessages caps how much prior conversation is sent as context,
// to keep classification requests small and cheap.
const maxHistoryMessages = 6

// classificationState is the structured `state` sent to TypeSafe: the
// current message plus enough recent history to resolve references like
// "yes, do that" to what they're replying to.
type classificationState struct {
	Message string               `json:"message"`
	History []classificationTurn `json:"history,omitempty"`
}

type classificationTurn struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	IsBot   bool   `json:"is_bot"`
}

func buildState(message llm.Message) classificationState {
	history := message.History
	if len(history) > maxHistoryMessages {
		history = history[len(history)-maxHistoryMessages:]
	}
	turns := make([]classificationTurn, 0, len(history))
	for _, h := range history {
		turns = append(turns, classificationTurn{
			Name:    h.Author,
			Content: h.Content,
			IsBot:   h.IsBot,
		})
	}
	return classificationState{
		Message: message.Content,
		History: turns,
	}
}

type TypeSafeClassifier struct {
	client *typesafe.Client
	logger Logger
	flags  FlagProvider
}

func New(client *typesafe.Client, logger Logger, flags FlagProvider) *TypeSafeClassifier {
	return &TypeSafeClassifier{client: client, logger: logger, flags: flags}
}

// flagQuestionKey namespaces a guild flag's TypeSafe question id so it can
// never collide with a tool name.
func flagQuestionKey(flagName string) string {
	return "flag::" + flagName
}

// Classify asks one noul question per tool ("should this tool be used to
// help answer this message?") plus one noul question per guild content
// flag ("does this message trigger this flag?"), in a single TypeSafe
// request. It returns the tools judged relevant (keyed by name, with the
// model's confidence) and the flags judged to apply (with their guidance).
func (c *TypeSafeClassifier) Classify(ctx context.Context, message llm.Message, tools map[string]llm.Tool) (llm.ClassificationResult, error) {
	if c == nil || c.client == nil {
		return llm.ClassificationResult{}, fmt.Errorf("typesafe classifier: not configured")
	}

	var guildFlags []guildflags.Flag
	if c.flags != nil && message.GuildID != "" {
		loaded, err := c.flags.ListByGuild(ctx, message.GuildID)
		if err != nil {
			log.Printf("typesafe classifier: failed to load guild flags for guild_id=%s: %v", message.GuildID, err)
		} else {
			guildFlags = loaded
		}
	}

	if len(tools) == 0 && len(guildFlags) == 0 {
		return llm.ClassificationResult{}, nil
	}

	questions := make(map[string]typesafe.Question, len(tools)+len(guildFlags))
	for name, tool := range tools {
		hint := tool.ClassifierHint
		if hint == "" {
			hint = tool.Description
		}
		questions[name] = typesafe.Question{
			Type:         typesafe.QuestionNoul,
			Instructions: fmt.Sprintf("Should the %q tool be used to help respond to this message?", name),
			Criteria: map[string]string{
				"true":  "The message needs this tool. " + hint,
				"false": "This tool is not relevant to the message.",
			},
		}
	}
	flagsByKey := make(map[string]guildflags.Flag, len(guildFlags))
	for _, flag := range guildFlags {
		key := flagQuestionKey(flag.Name)
		flagsByKey[key] = flag
		questions[key] = typesafe.Question{
			Type:         typesafe.QuestionNoul,
			Instructions: fmt.Sprintf("Does this message trigger the %q content flag?", flag.Name),
			Criteria: map[string]string{
				"true":  flag.Description,
				"false": "This flag does not apply to the message.",
			},
		}
	}

	state := buildState(message)
	stateJSON, _ := json.Marshal(state)

	start := time.Now()
	resp, evalErr := c.client.Evaluate(ctx, state, questions)
	latency := time.Since(start)

	status := typesafestats.StatusSuccess
	errMsg := ""
	selectedTools := make(map[string]float64)
	var matchedFlags []llm.MatchedFlag
	var matchedFlagNames []string
	var responseJSON []byte
	if evalErr != nil {
		status = typesafestats.StatusError
		errMsg = evalErr.Error()
	} else {
		for name := range tools {
			if answer, ok := resp.Answers[name]; ok && answer.Noul != nil && *answer.Noul >= relevanceThreshold {
				selectedTools[name] = *answer.Noul
			}
		}
		addImpliedTools(selectedTools, tools, resp.Answers)

		for key, flag := range flagsByKey {
			answer, ok := resp.Answers[key]
			if !ok || answer.Noul == nil || *answer.Noul < relevanceThreshold {
				continue
			}
			matchedFlags = append(matchedFlags, llm.MatchedFlag{
				Name:       flag.Name,
				Guidance:   flag.Guidance,
				Confidence: *answer.Noul,
			})
			matchedFlagNames = append(matchedFlagNames, flag.Name)
		}

		responseJSON, _ = json.Marshal(resp.Answers)
	}

	matchedFlagsJSON, _ := json.Marshal(matchedFlagNames)
	c.logResult(ctx, message, stateJSON, questions, responseJSON, selectedTools, string(matchedFlagsJSON), resp.Usage, latency, status, errMsg)

	if evalErr != nil {
		return llm.ClassificationResult{}, evalErr
	}
	return llm.ClassificationResult{Tools: selectedTools, Flags: matchedFlags}, nil
}

// addImpliedTools pulls in any tool named by a selected tool's ImpliesTools,
// using its own answered confidence if TypeSafe scored it, or 0 (below
// threshold, but present) if it wasn't scored highly enough to be selected
// on its own.
func addImpliedTools(selected map[string]float64, tools map[string]llm.Tool, answers map[string]typesafe.Answer) {
	for name := range selected {
		tool, ok := tools[name]
		if !ok {
			continue
		}
		for _, implied := range tool.ImpliesTools {
			if _, ok := tools[implied]; !ok {
				continue
			}
			if _, already := selected[implied]; already {
				continue
			}
			confidence := 0.0
			if answer, ok := answers[implied]; ok && answer.Noul != nil {
				confidence = *answer.Noul
			}
			selected[implied] = confidence
		}
	}
}

func (c *TypeSafeClassifier) logResult(
	ctx context.Context,
	message llm.Message,
	stateJSON []byte,
	questions map[string]typesafe.Question,
	responseJSON []byte,
	selected map[string]float64,
	matchedFlagsJSON string,
	usage typesafe.Usage,
	latency time.Duration,
	status, errMsg string,
) {
	if c.logger == nil {
		return
	}
	questionsJSON, _ := json.Marshal(questions)
	selectedNames := make([]string, 0, len(selected))
	for name := range selected {
		selectedNames = append(selectedNames, name)
	}
	selectedJSON, _ := json.Marshal(selectedNames)

	_, err := c.logger.Create(ctx, typesafestats.CreateClassificationLogParams{
		GuildID:          message.GuildID,
		ChannelID:        message.ChannelID,
		UserID:           message.UserID,
		MessageID:        message.MessageID,
		Model:            c.client.Model(),
		RequestState:     string(stateJSON),
		RequestQuestions: string(questionsJSON),
		ResponseAnswers:  string(responseJSON),
		SelectedTools:    string(selectedJSON),
		MatchedFlags:     matchedFlagsJSON,
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		Latency:          latency,
		Status:           status,
		Error:            errMsg,
	})
	if err != nil {
		log.Printf("typesafe classifier: failed to log classification: %v", err)
	}
}
