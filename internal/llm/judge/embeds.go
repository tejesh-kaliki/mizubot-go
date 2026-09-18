package judge

import (
	"context"
	"fmt"
	"strconv"

	"mizubot-go/internal/llm"
	"mizubot-go/internal/typesafe"
	"mizubot-go/internal/typesafestats"
)

// embedKeepThreshold is the minimum "yes" probability to keep an embed.
const embedKeepThreshold = 0.5

// EmbedFilter asks Jev, once per embed and after the reply is written,
// whether the embed is still worth attaching. It implements llm.EmbedFilter.
type EmbedFilter struct {
	eval   Evaluator
	logger Logger
}

func NewEmbedFilter(eval Evaluator, logger Logger) *EmbedFilter {
	return &EmbedFilter{eval: eval, logger: logger}
}

type embedFilterState struct {
	UserMessage string            `json:"user_message"`
	BotReply    string            `json:"bot_reply"`
	Embeds      []embedFilterItem `json:"embeds"`
}

type embedFilterItem struct {
	Intent      string `json:"intent"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

func embedQuestionKey(i int) string { return "embed::" + strconv.Itoa(i) }

// Filter returns the embeds Jev judges worth attaching. On any error it
// returns the error so the caller can keep everything.
func (f *EmbedFilter) Filter(ctx context.Context, message llm.Message, reply string, embeds []llm.Embed) ([]llm.Embed, error) {
	if f == nil || f.eval == nil {
		return nil, fmt.Errorf("embed filter: not configured")
	}
	if len(embeds) == 0 {
		return nil, nil
	}

	state := embedFilterState{UserMessage: message.Content, BotReply: reply}
	questions := make(map[string]typesafe.Question, len(embeds))
	for i, e := range embeds {
		state.Embeds = append(state.Embeds, embedFilterItem{Intent: e.Intent, Title: e.Title, Description: e.Description})
		questions[embedQuestionKey(i)] = typesafe.Question{
			Type: typesafe.QuestionNoul,
			Instructions: fmt.Sprintf(
				"Embed %d (%q, %s) is about to be attached under the bot's reply. Should it be attached?",
				i, e.Title, embedIntent(e)),
			Criteria: map[string]string{
				"true":  "The embed is about the subject the user asked about, so the card is a helpful supplement to the reply (extra details, cover art, a link).",
				"false": "The embed is not what the user asked about, or the reply says the item couldn't be found or identified, or attaching it would just be noise.",
			},
		}
	}

	var kept []llm.Embed
	_, err := evaluate(ctx, f.eval, f.logger, callMeta{
		GuildID:   message.GuildID,
		ChannelID: message.ChannelID,
		UserID:    message.UserID,
		MessageID: message.MessageID,
		Kind:      typesafestats.KindEmbedFilter,
	}, state, questions, func(resp typesafe.EvaluateResponse) []string {
		var titles []string
		for i, e := range embeds {
			answer, ok := resp.Answers[embedQuestionKey(i)]
			// A missing or malformed answer keeps the embed: failures
			// should show more, never less.
			if ok && answer.Noul != nil && *answer.Noul < embedKeepThreshold {
				continue
			}
			kept = append(kept, e)
			titles = append(titles, e.Title)
		}
		return titles
	})
	if err != nil {
		return nil, err
	}
	return kept, nil
}

func embedIntent(e llm.Embed) string {
	if e.Intent != "" {
		return e.Intent
	}
	return "embed"
}
