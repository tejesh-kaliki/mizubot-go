package bot

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"mizubot-go/internal/llm"

	"github.com/bwmarrin/discordgo"
)

type channelMessagesCall struct {
	channelID string
	limit     int
	beforeID  string
}

type channelMessageCall struct {
	channelID string
	messageID string
}

type stubHistoryFetcher struct {
	messagesByID map[string]*discordgo.Message
	bufferReturn []*discordgo.Message
	bufferErr    error

	channelMessagesCalls []channelMessagesCall
	channelMessageCalls  []channelMessageCall
}

func (f *stubHistoryFetcher) ChannelMessages(channelID string, limit int, beforeID, afterID, aroundID string, _ ...discordgo.RequestOption) ([]*discordgo.Message, error) {
	f.channelMessagesCalls = append(f.channelMessagesCalls, channelMessagesCall{channelID: channelID, limit: limit, beforeID: beforeID})
	return f.bufferReturn, f.bufferErr
}

func (f *stubHistoryFetcher) ChannelMessage(channelID, messageID string, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.channelMessageCalls = append(f.channelMessageCalls, channelMessageCall{channelID: channelID, messageID: messageID})
	if msg, ok := f.messagesByID[messageID]; ok {
		return msg, nil
	}
	return nil, nil
}

func testState(botID string) *discordgo.State {
	state := discordgo.NewState()
	_ = state.GuildAdd(&discordgo.Guild{
		ID: "guild1",
		Members: []*discordgo.Member{
			{User: &discordgo.User{ID: botID, Username: "mizubot"}, Nick: "Mizu"},
			{User: &discordgo.User{ID: "user1", Username: "account1"}, Nick: "Alice"},
			{User: &discordgo.User{ID: "user2", Username: "account2"}, Nick: "Bob"},
		},
	})
	return state
}

func newTestSession(botID string) *discordgo.Session {
	state := testState(botID)
	state.User = &discordgo.User{ID: botID, Username: "mizubot"}
	return &discordgo.Session{State: state}
}

func TestIsReplyDetection(t *testing.T) {
	if isReply(&discordgo.Message{}) {
		t.Fatalf("message without MessageReference should not be a reply")
	}
	if isReply(&discordgo.Message{MessageReference: &discordgo.MessageReference{}}) {
		t.Fatalf("MessageReference without a MessageID should not be a reply")
	}
	if !isReply(&discordgo.Message{MessageReference: &discordgo.MessageReference{MessageID: "123"}}) {
		t.Fatalf("MessageReference with a MessageID should be a reply")
	}
}

func TestBuildConversationHistoryUsesReplyChainWhenReply(t *testing.T) {
	s := newTestSession("bot1")

	msg1 := &discordgo.Message{ID: "msg1", ChannelID: "chan1", GuildID: "guild1", Content: "first message", Author: &discordgo.User{ID: "user1", Username: "account1"}}
	msg2 := &discordgo.Message{
		ID: "msg2", ChannelID: "chan1", GuildID: "guild1", Content: "second message",
		Author:           &discordgo.User{ID: "bot1", Username: "mizubot"},
		MessageReference: &discordgo.MessageReference{ChannelID: "chan1", MessageID: "msg1"},
	}
	msg3 := &discordgo.Message{
		ID: "msg3", ChannelID: "chan1", GuildID: "guild1", Content: "third message (current)",
		Author:            &discordgo.User{ID: "user2", Username: "account2"},
		MessageReference:  &discordgo.MessageReference{ChannelID: "chan1", MessageID: "msg2"},
		ReferencedMessage: msg2,
	}

	fetcher := &stubHistoryFetcher{messagesByID: map[string]*discordgo.Message{"msg1": msg1}}

	history := buildConversationHistory(s, fetcher, msg3)

	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2: %#v", len(history), history)
	}
	if history[0].Content != "first message" || history[0].Author != "Alice" {
		t.Fatalf("history[0] = %#v, want first message from Alice", history[0])
	}
	if history[1].Content != "second message" || !history[1].IsBot {
		t.Fatalf("history[1] = %#v, want second message marked as bot", history[1])
	}
	if len(fetcher.channelMessageCalls) != 1 || fetcher.channelMessageCalls[0].messageID != "msg1" {
		t.Fatalf("expected exactly one ChannelMessage fetch for msg1, got %#v", fetcher.channelMessageCalls)
	}
	if len(fetcher.channelMessagesCalls) != 0 {
		t.Fatalf("channel buffer path should not be used, got calls: %#v", fetcher.channelMessagesCalls)
	}
}

func TestHistoryFromReplyChainCapsHops(t *testing.T) {
	s := newTestSession("bot1")

	// Build a chain of 15 ancestors, each referencing the previous one, all
	// resolved via the fetcher (no gateway-populated ReferencedMessage).
	const total = 15
	byID := make(map[string]*discordgo.Message, total)
	for i := 1; i <= total; i++ {
		id := idFor(i)
		msg := &discordgo.Message{
			ID:        id,
			ChannelID: "chan1",
			GuildID:   "guild1",
			Content:   "message " + id,
			Author:    &discordgo.User{ID: "user1", Username: "account1"},
		}
		if i > 1 {
			msg.MessageReference = &discordgo.MessageReference{ChannelID: "chan1", MessageID: idFor(i - 1)}
		}
		byID[id] = msg
	}
	current := &discordgo.Message{
		ID:               "current",
		ChannelID:        "chan1",
		GuildID:          "guild1",
		Content:          "current message",
		Author:           &discordgo.User{ID: "user2", Username: "account2"},
		MessageReference: &discordgo.MessageReference{ChannelID: "chan1", MessageID: idFor(total)},
	}

	fetcher := &stubHistoryFetcher{messagesByID: byID}
	history := buildConversationHistory(s, fetcher, current)

	if len(history) != maxReplyChainHistory {
		t.Fatalf("history length = %d, want %d", len(history), maxReplyChainHistory)
	}
	// Oldest-first: the last maxReplyChainHistory ancestors, in ascending order.
	firstExpected := "message " + idFor(total-maxReplyChainHistory+1)
	lastExpected := "message " + idFor(total)
	if history[0].Content != firstExpected {
		t.Fatalf("history[0].Content = %q, want %q", history[0].Content, firstExpected)
	}
	if history[len(history)-1].Content != lastExpected {
		t.Fatalf("history[last].Content = %q, want %q", history[len(history)-1].Content, lastExpected)
	}
}

func idFor(i int) string {
	return "msg" + string(rune('a'+i))
}

func TestBuildConversationHistoryUsesChannelBufferWhenNotReply(t *testing.T) {
	s := newTestSession("bot1")

	current := &discordgo.Message{ID: "current", ChannelID: "chan1", GuildID: "guild1", Content: "hi", Author: &discordgo.User{ID: "user1"}}

	// ChannelMessages returns newest-first, as Discord does.
	fetcher := &stubHistoryFetcher{
		bufferReturn: []*discordgo.Message{
			{ID: "m3", ChannelID: "chan1", Content: "newest", Author: &discordgo.User{ID: "user2", Username: "account2"}},
			{ID: "m2", ChannelID: "chan1", Content: "other bot chatter", Author: &discordgo.User{ID: "otherbot", Username: "otherbot", Bot: true}},
			{ID: "m1", ChannelID: "chan1", Content: "oldest", Author: &discordgo.User{ID: "bot1", Username: "mizubot", Bot: true}},
		},
	}

	history := buildConversationHistory(s, fetcher, current)

	if len(fetcher.channelMessagesCalls) != 1 {
		t.Fatalf("expected exactly one ChannelMessages call, got %d", len(fetcher.channelMessagesCalls))
	}
	call := fetcher.channelMessagesCalls[0]
	if call.channelID != "chan1" || call.limit != maxChannelBufferHistory || call.beforeID != "current" {
		t.Fatalf("ChannelMessages called with %#v, want channelID=chan1 limit=%d beforeID=current", call, maxChannelBufferHistory)
	}
	if len(fetcher.channelMessageCalls) != 0 {
		t.Fatalf("reply-chain path should not be used, got calls: %#v", fetcher.channelMessageCalls)
	}

	// other-bot message should be filtered out; this bot's own message and
	// the user message should remain, in chronological order.
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2: %#v", len(history), history)
	}
	if history[0].Content != "oldest" || !history[0].IsBot {
		t.Fatalf("history[0] = %#v, want oldest marked as this bot", history[0])
	}
	if history[1].Content != "newest" || history[1].IsBot {
		t.Fatalf("history[1] = %#v, want newest from a user", history[1])
	}
}

func TestChannelBufferIncludesPlainChatterNotJustBotRelatedMessages(t *testing.T) {
	s := newTestSession("bot1")

	current := &discordgo.Message{ID: "current", ChannelID: "chan1", GuildID: "guild1", Content: "<@bot1> and now?", Author: &discordgo.User{ID: "user1"}}

	// Mirrors the reported scenario: plain human chatter with no bot
	// mention, a message that @mentions the bot, this bot's own reply, and
	// another bot's message — all in the same channel buffer.
	fetcher := &stubHistoryFetcher{
		bufferReturn: []*discordgo.Message{
			{ID: "m4", ChannelID: "chan1", Content: "on it!", Author: &discordgo.User{ID: "bot1", Username: "mizubot", Bot: true}},
			{ID: "m3", ChannelID: "chan1", Content: "<@bot1> can you check this?", Author: &discordgo.User{ID: "user1", Username: "account1"}},
			{ID: "m2", ChannelID: "chan1", Content: "other bot chatter", Author: &discordgo.User{ID: "otherbot", Username: "otherbot", Bot: true}},
			{ID: "m1", ChannelID: "chan1", Content: "Need to see if it can read message", Author: &discordgo.User{ID: "user2", Username: "account2"}},
		},
	}

	history := buildConversationHistory(s, fetcher, current)

	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3 (plain chatter + mention + bot reply, other-bot excluded): %#v", len(history), history)
	}
	if history[0].Content != "Need to see if it can read message" {
		t.Fatalf("history[0] should be the plain human chatter with no bot mention, got %#v", history[0])
	}
	if history[1].Content != "<@bot1> can you check this?" {
		t.Fatalf("history[1] should be the mention message, got %#v", history[1])
	}
	if history[2].Content != "on it!" || !history[2].IsBot {
		t.Fatalf("history[2] should be this bot's own reply, got %#v", history[2])
	}
	for _, h := range history {
		if h.Content == "other bot chatter" {
			t.Fatalf("other bot's message should have been filtered out: %#v", history)
		}
	}
}

func TestHistoryMessagesFromDiscordTruncatesLongContent(t *testing.T) {
	s := newTestSession("bot1")
	longContent := strings.Repeat("x", maxHistoryMessageChars+200)
	messages := []*discordgo.Message{
		{ID: "m1", Content: longContent, Author: &discordgo.User{ID: "user1", Username: "account1"}},
	}

	history := historyMessagesFromDiscord(s, "guild1", messages, false)

	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	if got := len([]rune(history[0].Content)); got > maxHistoryMessageChars+1 {
		t.Fatalf("history content length = %d, want <= %d", got, maxHistoryMessageChars+1)
	}
	if !strings.HasSuffix(history[0].Content, "…") {
		t.Fatalf("truncated content should end with an ellipsis: %q", history[0].Content)
	}
}

func TestBuildConversationHistoryHandlesNilFetcher(t *testing.T) {
	if got := buildConversationHistory(nil, nil, &discordgo.Message{ID: "m1"}); got != nil {
		t.Fatalf("expected nil history with nil fetcher, got %#v", got)
	}
}

func TestHistorySourcePath(t *testing.T) {
	if got := historySourcePath(&discordgo.Message{}); got != "channel-buffer" {
		t.Fatalf("historySourcePath = %q, want channel-buffer", got)
	}
	replyMsg := &discordgo.Message{MessageReference: &discordgo.MessageReference{MessageID: "123"}}
	if got := historySourcePath(replyMsg); got != "reply-chain" {
		t.Fatalf("historySourcePath = %q, want reply-chain", got)
	}
}

func captureLogOutput(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	orig := log.Writer()
	origFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(orig)
		log.SetFlags(origFlags)
	}()
	fn()
	return buf.String()
}

func TestDebugLogHistorySkipsWhenDisabled(t *testing.T) {
	out := captureLogOutput(t, func() {
		debugLogHistory(false, "chan1", "msg1", "reply-chain", []llm.HistoryMessage{
			{Author: "Alice", Content: "hello"},
		})
	})
	if out != "" {
		t.Fatalf("expected no log output when disabled, got %q", out)
	}
}

func TestDebugLogHistoryLogsPathCountAndMessagesWhenEnabled(t *testing.T) {
	out := captureLogOutput(t, func() {
		debugLogHistory(true, "chan1", "msg1", "reply-chain", []llm.HistoryMessage{
			{Author: "Alice", Content: "hello there", IsBot: false},
			{Author: "MizuBot", Content: "hi Alice", IsBot: true},
		})
	})
	if !strings.Contains(out, "path=reply-chain") {
		t.Fatalf("log output missing path: %q", out)
	}
	if !strings.Contains(out, "count=2") {
		t.Fatalf("log output missing count: %q", out)
	}
	if !strings.Contains(out, "author=Alice") || !strings.Contains(out, "hello there") {
		t.Fatalf("log output missing first history entry: %q", out)
	}
	if !strings.Contains(out, "role=bot") || !strings.Contains(out, "hi Alice") {
		t.Fatalf("log output missing second history entry marked as bot: %q", out)
	}
}

func TestDebugLogHistoryTruncatesLongContent(t *testing.T) {
	longContent := strings.Repeat("x", maxDebugHistoryContentChars+100)
	out := captureLogOutput(t, func() {
		debugLogHistory(true, "chan1", "msg1", "channel-buffer", []llm.HistoryMessage{
			{Author: "Alice", Content: longContent},
		})
	})
	if strings.Contains(out, longContent) {
		t.Fatalf("expected log output to truncate long content")
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("expected truncated content to end with ellipsis: %q", out)
	}
}
