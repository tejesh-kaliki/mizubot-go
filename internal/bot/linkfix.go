package bot

import (
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// tweetLinkRE matches x.com / twitter.com status links. Everything after the
// status id (media suffixes, query string, fragment) is swallowed so it can be
// dropped when rewriting.
var tweetLinkRE = regexp.MustCompile(`https?://(?:www\.|mobile\.)?(?:x|twitter)\.com/(\w{1,15})/status/(\d+)[^\s<>|)]*`)

// instagramLinkRE matches instagram.com / instagr.am URLs. The path is checked
// afterwards so profiles, explore pages, and other non-media links stay put.
var instagramLinkRE = regexp.MustCompile(`(?i)https?://(?:www\.|m\.)?(?:instagram\.com|instagr\.am)/[^\s<>|)]+`)

// linkRewriteMessageRE matches a repost this bot made. Replies to those posts
// are not treated as a request for the bot unless they also @mention it.
var linkRewriteMessageRE = regexp.MustCompile(`(?s)^Shared by <@!?\d+>\n.*https://(?:fixupx\.com/\w{1,15}/status/\d+/en|vxinstagram\.com/)`)

var (
	instagramShortcodeRE = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	instagramShareRE     = regexp.MustCompile(`^[A-Za-z0-9_-]{8,}$`)
	instagramUsernameRE  = regexp.MustCompile(`^[A-Za-z0-9._]{1,30}$`)
	instagramStoryIDRE   = regexp.MustCompile(`^[0-9]{5,}$`)
)

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

// rewriteInstagramLinks swaps Instagram media links for vxinstagram.com so
// Discord can embed the photo or video. Tracking query parameters are dropped.
// An album's img_index is kept as a path suffix, which vxinstagram treats as
// 1-based. Links wrapped in <...> are left alone.
func rewriteInstagramLinks(content string) (string, bool) {
	matches := instagramLinkRE.FindAllStringIndex(content, -1)
	if len(matches) == 0 {
		return content, false
	}
	var sb strings.Builder
	last := 0
	changed := false
	for _, m := range matches {
		start, end := m[0], m[1]
		end = trimTrailingPunct(content, start, end)
		if start > 0 && content[start-1] == '<' {
			continue
		}
		path, ok := instagramEmbedPath(content[start:end])
		if !ok {
			continue
		}
		sb.WriteString(content[last:start])
		sb.WriteString("https://vxinstagram.com/")
		sb.WriteString(path)
		last = end
		changed = true
	}
	if !changed {
		return content, false
	}
	sb.WriteString(content[last:])
	return sb.String(), true
}

// rewriteEmbedLinks rewrites tweet and Instagram links. A message that
// contains both is reposted once.
func rewriteEmbedLinks(content string) (string, bool) {
	rewritten, tweetChanged := rewriteTweetLinks(content)
	rewritten, instagramChanged := rewriteInstagramLinks(rewritten)
	return rewritten, tweetChanged || instagramChanged
}

// instagramEmbedPath returns the vxinstagram path for a media URL. Profiles
// and other pages vxinstagram does not embed return false.
func instagramEmbedPath(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	host = strings.TrimPrefix(host, "m.")
	if host != "instagram.com" && host != "instagr.am" {
		return "", false
	}
	segs := strings.FieldsFunc(strings.Trim(u.Path, "/"), func(r rune) bool { return r == '/' })
	if len(segs) == 0 {
		return "", false
	}
	imgIndex := instagramImgIndex(u.Query().Get("img_index"))
	kind := strings.ToLower(segs[0])
	switch kind {
	case "p", "reel", "reels", "tv":
		if len(segs) < 2 || !instagramShortcodeRE.MatchString(segs[1]) {
			return "", false
		}
		return joinInstagramPath(kind, segs[1], mediaIndex(segs, 2, imgIndex)), true
	case "stories":
		if len(segs) < 2 || !instagramUsernameRE.MatchString(segs[1]) || strings.EqualFold(segs[1], "highlights") {
			return "", false
		}
		path := "stories/" + segs[1]
		if len(segs) >= 3 && instagramStoryIDRE.MatchString(segs[2]) {
			path += "/" + segs[2]
		}
		return path, true
	case "share":
		if len(segs) >= 3 && instagramMediaKind(segs[1]) && instagramShareRE.MatchString(segs[2]) {
			return "share/" + strings.ToLower(segs[1]) + "/" + segs[2], true
		}
		if len(segs) >= 2 && instagramShareRE.MatchString(segs[1]) && !instagramMediaKind(segs[1]) {
			return "share/" + segs[1], true
		}
		return "", false
	default:
		if len(segs) < 3 || !instagramUsernameRE.MatchString(segs[0]) || !instagramMediaKind(segs[1]) || !instagramShortcodeRE.MatchString(segs[2]) {
			return "", false
		}
		return joinInstagramPath(segs[0]+"/"+strings.ToLower(segs[1]), segs[2], mediaIndex(segs, 3, imgIndex)), true
	}
}

func instagramMediaKind(seg string) bool {
	switch strings.ToLower(seg) {
	case "p", "reel", "reels", "tv":
		return true
	default:
		return false
	}
}

// mediaIndex prefers an explicit /N suffix and otherwise the img_index query.
func mediaIndex(segs []string, at int, imgIndex string) string {
	if len(segs) > at && instagramIndex(segs[at]) {
		return segs[at]
	}
	return imgIndex
}

func joinInstagramPath(prefix, shortcode, index string) string {
	path := prefix + "/" + shortcode
	if index != "" {
		path += "/" + index
	}
	return path
}

func instagramIndex(seg string) bool {
	n, err := strconv.Atoi(seg)
	return err == nil && n >= 1 && n <= 99 && strconv.Itoa(n) == seg
}

func instagramImgIndex(raw string) string {
	if instagramIndex(raw) {
		return raw
	}
	return ""
}

func trimTrailingPunct(content string, start, end int) int {
	for end > start && strings.ContainsRune(".,;:!", rune(content[end-1])) {
		end--
	}
	return end
}

// fixEmbedLinks reposts a message with cleaned tweet and Instagram links,
// credits the original author, and deletes the original. It reports whether
// the message was handled.
func (b *Bot) fixEmbedLinks(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	if m.Author.Bot || len(m.Attachments) > 0 || len(m.StickerItems) > 0 {
		// Reposting would drop attachments/stickers; leave those messages alone.
		return false
	}
	fixed, ok := rewriteEmbedLinks(m.Content)
	if !ok {
		return false
	}
	content := formatLinkRewrite(m.Author.ID, fixed)
	if len(content) > 2000 {
		return false
	}
	if b.dryRun {
		return true
	}
	// Parse stays empty so "Shared by" does not ping the author. Users and
	// roles list only the pings the original message already made.
	_, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content:         content,
		Reference:       m.MessageReference,
		AllowedMentions: linkRewriteAllowedMentions(m.Message),
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

func formatLinkRewrite(authorID, body string) string {
	return fmt.Sprintf("Shared by <@%s>\n%s", authorID, body)
}

// isLinkRewriteMessage reports whether content is one of this bot's tweet or
// Instagram link reposts.
func isLinkRewriteMessage(content string) bool {
	return linkRewriteMessageRE.MatchString(content)
}

// linkRewriteAllowedMentions lets the repost ping the same users and roles as
// the original message. The sharing user is not added, so the attribution
// line stays silent unless that user was mentioned in the message itself.
// A reply pings its target only when the original reply ping was left on,
// which is when Discord includes that author in Mentions.
func linkRewriteAllowedMentions(m *discordgo.Message) *discordgo.MessageAllowedMentions {
	allowed := &discordgo.MessageAllowedMentions{}
	if m == nil {
		return allowed
	}
	allowed.Users = mentionUserIDs(m.Mentions)
	if len(m.MentionRoles) > 0 {
		allowed.Roles = append([]string(nil), m.MentionRoles...)
	}
	if m.ReferencedMessage != nil && m.ReferencedMessage.Author != nil {
		allowed.RepliedUser = mentionsInList(m.Mentions, m.ReferencedMessage.Author.ID)
	}
	return allowed
}

func mentionUserIDs(users []*discordgo.User) []string {
	if len(users) == 0 {
		return nil
	}
	ids := make([]string, 0, len(users))
	seen := make(map[string]struct{}, len(users))
	for _, user := range users {
		if user == nil || user.ID == "" {
			continue
		}
		if _, ok := seen[user.ID]; ok {
			continue
		}
		seen[user.ID] = struct{}{}
		ids = append(ids, user.ID)
	}
	return ids
}
