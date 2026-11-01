package bot

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// tweetLinkRE matches x.com / twitter.com status links. Everything after the
// status id (media suffixes, query string, fragment) is swallowed so it can be
// dropped when rewriting.
var tweetLinkRE = regexp.MustCompile(`https?://(?:www\.|mobile\.)?(?:x|twitter)\.com/(\w{1,15})/status/(\d+)[^\s<>|)]*`)

// rewriteTweetLinks swaps every tweet link for its tracking-free fixupx.com
// equivalent with a /en suffix. Links wrapped in <...> are left alone since the
// author deliberately suppressed their embed.
func rewriteTweetLinks(content string) (string, bool) {
	matches := tweetLinkRE.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content, false
	}
	var sb strings.Builder
	last := 0
	changed := false
	for _, m := range matches {
		start, end := m[0], m[1]
		if start > 0 && content[start-1] == '<' {
			continue
		}
		user, id := content[m[2]:m[3]], content[m[4]:m[5]]
		sb.WriteString(content[last:start])
		sb.WriteString(fmt.Sprintf("https://fixupx.com/%s/status/%s/en", user, id))
		last = end
		changed = true
	}
	if !changed {
		return content, false
	}
	sb.WriteString(content[last:])
	return sb.String(), true
}

// fixTweetLinks reposts a message with cleaned tweet links, credits the
// original author, and deletes the original. It reports whether the message
// was handled.
func (b *Bot) fixTweetLinks(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	if m.Author.Bot || len(m.Attachments) > 0 || len(m.StickerItems) > 0 {
		// Reposting would drop attachments/stickers; leave those messages alone.
		return false
	}
	fixed, ok := rewriteTweetLinks(m.Content)
	if !ok {
		return false
	}
	content := fmt.Sprintf("Shared by <@%s>\n%s", m.Author.ID, fixed)
	if len(content) > 2000 {
		return false
	}
	if b.dryRun {
		return true
	}
	_, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content:         content,
		Reference:       m.MessageReference,
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}, Users: nil, RepliedUser: false},
	})
	if err != nil {
		log.Printf("link fix repost failed: channel_id=%s message_id=%s error=%v", m.ChannelID, m.ID, err)
		return true
	}
	if err := s.ChannelMessageDelete(m.ChannelID, m.ID); err != nil {
		log.Printf("link fix delete failed (needs Manage Messages): channel_id=%s message_id=%s error=%v", m.ChannelID, m.ID, err)
	}
	return true
}
