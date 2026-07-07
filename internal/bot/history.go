package bot

import (
	"log"
	"strings"

	"mizubot-go/internal/llm"

	"github.com/bwmarrin/discordgo"
)

const (
	maxReplyChainHistory    = 10
	maxChannelBufferHistory = 8
	maxHistoryMessageChars  = 500
)

// messageHistoryFetcher covers the discordgo.Session REST methods used to
// resolve conversation history, so tests can supply a stub instead of
// hitting the Discord API.
type messageHistoryFetcher interface {
	ChannelMessages(channelID string, limit int, beforeID, afterID, aroundID string, options ...discordgo.RequestOption) ([]*discordgo.Message, error)
	ChannelMessage(channelID, messageID string, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

// buildConversationHistory resolves prior conversation context for the
// triggering message: if the message is a reply, it walks the reply chain
// up to maxReplyChainHistory hops; otherwise it falls back to the last
// messages in the channel.
func buildConversationHistory(s *discordgo.Session, fetcher messageHistoryFetcher, msg *discordgo.Message) []llm.HistoryMessage {
	if fetcher == nil || msg == nil {
		return nil
	}
	if isReply(msg) {
		return historyFromReplyChain(s, fetcher, msg)
	}
	return historyFromChannelBuffer(s, fetcher, msg)
}

func isReply(msg *discordgo.Message) bool {
	return msg.MessageReference != nil && msg.MessageReference.MessageID != ""
}

// historySourcePath reports which strategy buildConversationHistory used for
// msg, for logging purposes.
func historySourcePath(msg *discordgo.Message) string {
	if isReply(msg) {
		return "reply-chain"
	}
	return "channel-buffer"
}

const maxDebugHistoryContentChars = 200

// debugLogHistory logs the conversation history built for an LLM request
// when enabled: the source path, message count, and each entry's author and
// truncated content. Off by default so normal operation doesn't spam logs.
func debugLogHistory(enabled bool, channelID, messageID, path string, history []llm.HistoryMessage) {
	if !enabled {
		return
	}
	log.Printf("llm history debug: channel_id=%s message_id=%s path=%s count=%d", channelID, messageID, path, len(history))
	for i, h := range history {
		role := "user"
		if h.IsBot {
			role = "bot"
		}
		log.Printf("llm history debug: channel_id=%s message_id=%s idx=%d role=%s author=%s content=%q",
			channelID, messageID, i, role, h.Author, truncate(h.Content, maxDebugHistoryContentChars))
	}
}

func historyFromReplyChain(s *discordgo.Session, fetcher messageHistoryFetcher, msg *discordgo.Message) []llm.HistoryMessage {
	chain := make([]*discordgo.Message, 0, maxReplyChainHistory)
	ref := msg.MessageReference
	resolved := msg.ReferencedMessage
	seen := map[string]bool{msg.ID: true}

	for i := 0; i < maxReplyChainHistory && ref != nil && ref.MessageID != ""; i++ {
		if seen[ref.MessageID] {
			break
		}

		var parent *discordgo.Message
		if resolved != nil && resolved.ID == ref.MessageID {
			parent = resolved
		} else {
			channelID := ref.ChannelID
			if channelID == "" {
				channelID = msg.ChannelID
			}
			fetched, err := fetcher.ChannelMessage(channelID, ref.MessageID)
			if err != nil {
				log.Printf("fetch reply-chain ancestor failed: channel_id=%s message_id=%s error=%v", channelID, ref.MessageID, err)
				break
			}
			parent = fetched
		}
		if parent == nil {
			break
		}

		seen[parent.ID] = true
		chain = append(chain, parent)
		ref = parent.MessageReference
		resolved = parent.ReferencedMessage
	}

	reverseMessages(chain)
	return historyMessagesFromDiscord(s, msg.GuildID, chain, false)
}

func historyFromChannelBuffer(s *discordgo.Session, fetcher messageHistoryFetcher, msg *discordgo.Message) []llm.HistoryMessage {
	fetched, err := fetcher.ChannelMessages(msg.ChannelID, maxChannelBufferHistory, msg.ID, "", "")
	if err != nil {
		log.Printf("fetch channel history failed: channel_id=%s error=%v", msg.ChannelID, err)
		return nil
	}
	reverseMessages(fetched)
	return historyMessagesFromDiscord(s, msg.GuildID, fetched, true)
}

// reverseMessages reverses a slice of messages in place. ChannelMessages and
// ancestor chains are both collected newest-first; the LLM prompt needs
// them in chronological (oldest-first) order.
func reverseMessages(messages []*discordgo.Message) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}

// historyMessagesFromDiscord converts discordgo messages (already in
// chronological order) into llm.HistoryMessage entries. When
// filterOtherBots is set, messages from bots other than this one are
// dropped, while this bot's own prior replies are kept for continuity.
func historyMessagesFromDiscord(s *discordgo.Session, guildID string, messages []*discordgo.Message, filterOtherBots bool) []llm.HistoryMessage {
	var botUserID string
	if s != nil && s.State != nil && s.State.User != nil {
		botUserID = s.State.User.ID
	}

	out := make([]llm.HistoryMessage, 0, len(messages))
	for _, dm := range messages {
		if dm == nil || dm.Author == nil {
			continue
		}
		isBotAuthor := botUserID != "" && dm.Author.ID == botUserID
		if filterOtherBots && dm.Author.Bot && !isBotAuthor {
			continue
		}

		content := strings.TrimSpace(messageTextWithDisplayNames(s, dm))
		if content == "" {
			continue
		}

		out = append(out, llm.HistoryMessage{
			Author:  guildDisplayName(s, guildID, dm.Author, dm.Member),
			Content: truncate(content, maxHistoryMessageChars),
			IsBot:   isBotAuthor,
		})
	}
	return out
}
