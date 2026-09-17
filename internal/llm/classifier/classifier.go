// Package classifier implements llm.ToolClassifier using the TypeSafe
// evaluation API: one "noul" (yes/no) question per candidate tool, asked in
// a single request, replacing the substring keyword match. Every request
// and response is logged (including token usage) via the supplied Logger,
// so the jev model's routing behavior can be inspected after the fact.
package classifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"mizubot-go/internal/llm"
	"mizubot-go/internal/typesafe"
	"mizubot-go/internal/typesafestats"
)

// Logger records a classification request/response. typesafestats.Store
// satisfies this.
type Logger interface {
	Create(ctx context.Context, params typesafestats.CreateClassificationLogParams) (typesafestats.ClassificationLog, error)
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
}

func New(client *typesafe.Client, logger Logger) *TypeSafeClassifier {
	return &TypeSafeClassifier{client: client, logger: logger}
}

// RelevantTools asks one noul question per tool ("should this tool be used
// to help answer this message?") in a single TypeSafe request and returns
// the tools it judged relevant, keyed by name, with the model's confidence
// (the "yes" probability) that the tool applies.
func (c *TypeSafeClassifier) RelevantTools(ctx context.Context, message llm.Message, tools map[string]llm.Tool) (map[string]float64, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("typesafe classifier: not configured")
	}
	if len(tools) == 0 {
		return nil, nil
	}

	questions := make(map[string]typesafe.Question, len(tools))
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

	state := buildState(message)
	stateJSON, _ := json.Marshal(state)

	start := time.Now()
	resp, evalErr := c.client.Evaluate(ctx, state, questions)
	latency := time.Since(start)

	status := typesafestats.StatusSuccess
	errMsg := ""
	selected := make(map[string]float64)
	var responseJSON []byte
	if evalErr != nil {
		status = typesafestats.StatusError
		errMsg = evalErr.Error()
	} else {
		for name := range tools {
			if answer, ok := resp.Answers[name]; ok && answer.Noul != nil && *answer.Noul >= relevanceThreshold {
				selected[name] = *answer.Noul
			}
		}
		addImpliedTools(selected, tools, resp.Answers)
		responseJSON, _ = json.Marshal(resp.Answers)
	}

	c.logResult(ctx, message, stateJSON, questions, responseJSON, selected, resp.Usage, latency, status, errMsg)

	if evalErr != nil {
		return nil, evalErr
	}
	return selected, nil
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
