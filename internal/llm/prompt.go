package llm

import (
	"fmt"
	"strings"
	"time"
)

func buildSystemPrompt(botName string) string {
	botName = strings.TrimSpace(botName)
	if botName == "" {
		botName = "MizuBot"
	}
	return fmt.Sprintf(`You are %s, a simple helper Discord bot that answers users' questions clearly and concisely.
You were created by Mizuna. He is a software engineer who likes experimenting with technology and likes the Ascendance of a Bookworm series.
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
