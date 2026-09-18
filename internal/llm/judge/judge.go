// Package judge holds small Jev (TypeSafe System One) judgements used by
// tools and the reply path: picking the right anime among search
// candidates, and deciding whether a tool's embed is worth attaching. Each
// call is logged via the supplied Logger (typesafestats.Store satisfies it),
// so the model's behavior can be inspected after the fact.
package judge

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"mizubot-go/internal/typesafe"
	"mizubot-go/internal/typesafestats"
)

// Logger records a TypeSafe request/response. typesafestats.Store satisfies
// this.
type Logger interface {
	Create(ctx context.Context, params typesafestats.CreateClassificationLogParams) (typesafestats.ClassificationLog, error)
}

// Evaluator is the slice of typesafe.Client the judges use.
type Evaluator interface {
	Evaluate(ctx context.Context, state any, questions map[string]typesafe.Question) (typesafe.EvaluateResponse, error)
	Model() string
}

type callMeta struct {
	GuildID   string
	ChannelID string
	UserID    string
	MessageID string
	// Kind is the typesafestats kind this judge logs under.
	Kind string
}

// evaluate runs one TypeSafe request and logs it, returning the response and
// the evaluation error (logging failures never fail the call). outcome, when
// set, summarizes the decision (stored in selected_tools) and runs only on
// success.
func evaluate(
	ctx context.Context,
	eval Evaluator,
	logger Logger,
	meta callMeta,
	state any,
	questions map[string]typesafe.Question,
	outcome func(typesafe.EvaluateResponse) []string,
) (typesafe.EvaluateResponse, error) {
	start := time.Now()
	resp, err := eval.Evaluate(ctx, state, questions)
	latency := time.Since(start)

	status := typesafestats.StatusSuccess
	errMsg := ""
	var responseJSON []byte
	outcomes := []string{}
	if err != nil {
		status = typesafestats.StatusError
		errMsg = err.Error()
	} else {
		responseJSON, _ = json.Marshal(resp.Answers)
		if outcome != nil {
			outcomes = outcome(resp)
		}
	}

	if logger != nil {
		stateJSON, _ := json.Marshal(state)
		questionsJSON, _ := json.Marshal(questions)
		selected, _ := json.Marshal(outcomes)
		_, logErr := logger.Create(ctx, typesafestats.CreateClassificationLogParams{
			GuildID:          meta.GuildID,
			ChannelID:        meta.ChannelID,
			UserID:           meta.UserID,
			MessageID:        meta.MessageID,
			Model:            eval.Model(),
			RequestState:     string(stateJSON),
			RequestQuestions: string(questionsJSON),
			ResponseAnswers:  string(responseJSON),
			SelectedTools:    string(selected),
			MatchedFlags:     "[]",
			Kind:             meta.Kind,
			InputTokens:      resp.Usage.InputTokens,
			OutputTokens:     resp.Usage.OutputTokens,
			Latency:          latency,
			Status:           status,
			Error:            errMsg,
		})
		if logErr != nil {
			log.Printf("judge: failed to log %s call: %v", meta.Kind, logErr)
		}
	}
	return resp, err
}
