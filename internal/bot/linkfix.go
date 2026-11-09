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

const (
	linkRewriteEditID             = "linkfix:edit"
	linkRewriteDeleteID           = "linkfix:delete"
	linkRewriteEditModalID        = "linkfix:edit-modal"
	linkRewriteBodyInputID        = "linkfix:body"
	linkRewriteDeleteConfirmPref  = "linkfix:del:"
	linkRewriteDeleteCancelID     = "linkfix:del-cancel"
	linkRewriteNotOwnerText       = "Only the person who shared this can edit or delete it."
	linkRewriteEmptyEditText      = "The message cannot be empty. Use Delete to remove it."
	linkRewriteTooLongText        = "That message is too long to post."
	linkRewriteMissingMessageText = "I couldn't find that message."
)

// linkRewriteMessageRE matches a repost this bot made. Replies to those posts
// are not treated as a request for the bot unless they also @mention it.
var linkRewriteMessageRE = regexp.MustCompile(`(?s)^Shared by <@!?\d+>\n.*https://(?:fixupx\.com/\w{1,15}/status/\d+/en|vxinstagram\.com/)`)

// linkRewriteAuthorRE is the attribution line. The edit box never includes it,
// so saving an edit cannot drop the mention of the person who shared the link.
var linkRewriteAuthorRE = regexp.MustCompile(`^Shared by <@!?(\d+)>\n`)

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
	// The leading "Shared by" mention is allowed, so that user ping stays.
	// Role, @everyone, and @here mentions are left in the text and do not notify.
	_, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content:         content,
		Components:      linkRewriteComponents(),
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

// linkRewriteAllowedMentions lets the repost ping users and only users.
// The sharer is included so the leading "Shared by" mention notifies them.
// Role, @everyone, and @here mentions stay in the text and do not notify.
// A reply pings its target only when the original reply ping was left on,
// which is when Discord includes that author in Mentions.
func linkRewriteAllowedMentions(m *discordgo.Message) *discordgo.MessageAllowedMentions {
	allowed := &discordgo.MessageAllowedMentions{}
	if m == nil {
		return allowed
	}
	allowed.Users = mentionUserIDs(m.Mentions)
	if m.Author != nil {
		allowed.Users = appendUniqueID(allowed.Users, m.Author.ID)
	}
	if m.ReferencedMessage != nil && m.ReferencedMessage.Author != nil {
		allowed.RepliedUser = mentionsInList(m.Mentions, m.ReferencedMessage.Author.ID)
	}
	return allowed
}

// linkRewriteComponents are the controls under a repost. Only the sharer
// can use them; other clicks get an ephemeral refusal.
func linkRewriteComponents() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Edit", Style: discordgo.SecondaryButton, CustomID: linkRewriteEditID},
			discordgo.Button{Label: "Delete", Style: discordgo.DangerButton, CustomID: linkRewriteDeleteID},
		}},
	}
}

func linkRewriteDeleteConfirmID(channelID, messageID string) string {
	return linkRewriteDeleteConfirmPref + channelID + ":" + messageID
}

func parseLinkRewriteDeleteID(customID string) (channelID, messageID string, ok bool) {
	rest, found := strings.CutPrefix(customID, linkRewriteDeleteConfirmPref)
	if !found {
		return "", "", false
	}
	channelID, messageID, found = strings.Cut(rest, ":")
	if !found || !discordSnowflake(channelID) || !discordSnowflake(messageID) {
		return "", "", false
	}
	return channelID, messageID, true
}

func discordSnowflake(id string) bool {
	if id == "" || len(id) > 20 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func linkRewriteAuthorID(content string) (string, bool) {
	m := linkRewriteAuthorRE.FindStringSubmatch(content)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func linkRewriteBody(content string) string {
	loc := linkRewriteAuthorRE.FindStringIndex(content)
	if loc == nil {
		return ""
	}
	return content[loc[1]:]
}

func linkRewriteOwnedBy(m *discordgo.Message, userID string) bool {
	if m == nil || userID == "" {
		return false
	}
	owner, ok := linkRewriteAuthorID(m.Content)
	return ok && owner == userID
}

// composeLinkRewriteEdit rebuilds a repost from the modal text. The attribution
// line is added back here, and any new tweet or Instagram links are cleaned.
// The modal has no user picker, so an edit does not try to parse or send pings.
func composeLinkRewriteEdit(ownerID, submitted string) (string, string) {
	body := strings.TrimSpace(submitted)
	if body == "" {
		return "", linkRewriteEmptyEditText
	}
	body, _ = rewriteEmbedLinks(body)
	content := formatLinkRewrite(ownerID, body)
	if len(content) > 2000 {
		return "", linkRewriteTooLongText
	}
	return content, ""
}

func appendUniqueID(ids []string, id string) []string {
	if id == "" {
		return ids
	}
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func linkRewriteBodyMax(ownerID string) int {
	max := 2000 - len(formatLinkRewrite(ownerID, ""))
	if max > 4000 {
		max = 4000
	}
	if max < 1 {
		return 1
	}
	return max
}

// handleLinkRewriteInteraction handles the Edit and Delete buttons under a
// repost. It reports whether the interaction belonged to this feature.
func (b *Bot) handleLinkRewriteInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if i == nil {
		return false
	}
	switch i.Type {
	case discordgo.InteractionMessageComponent:
		data, ok := i.Data.(discordgo.MessageComponentInteractionData)
		if !ok {
			return false
		}
		switch {
		case data.CustomID == linkRewriteEditID:
			b.openLinkRewriteEdit(s, i)
		case data.CustomID == linkRewriteDeleteID:
			b.confirmLinkRewriteDelete(s, i)
		case data.CustomID == linkRewriteDeleteCancelID:
			b.updateLinkRewriteInteraction(s, i, "Canceled.", nil)
		case strings.HasPrefix(data.CustomID, linkRewriteDeleteConfirmPref):
			b.deleteLinkRewrite(s, i, data.CustomID)
		default:
			return false
		}
		return true
	case discordgo.InteractionModalSubmit:
		data, ok := i.Data.(discordgo.ModalSubmitInteractionData)
		if !ok || data.CustomID != linkRewriteEditModalID {
			return false
		}
		b.submitLinkRewriteEdit(s, i, data)
		return true
	default:
		return false
	}
}

func (b *Bot) openLinkRewriteEdit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ownerID, ok := b.requireLinkRewriteOwner(s, i)
	if !ok {
		return
	}
	body := linkRewriteBody(i.Message.Content)
	max := linkRewriteBodyMax(ownerID)
	if len(body) > max || len(body) > 4000 {
		b.respondLinkRewrite(s, i, linkRewriteTooLongText, true, nil)
		return
	}
	s = b.interactionSession(s)
	if s == nil {
		log.Printf("link rewrite edit dropped: no session")
		return
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: linkRewriteEditModalID,
			Title:    "Edit message",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:  linkRewriteBodyInputID,
						Label:     "Message",
						Style:     discordgo.TextInputParagraph,
						Value:     body,
						Required:  true,
						MaxLength: max,
					},
				}},
			},
		},
	})
	if err != nil {
		log.Printf("link rewrite edit modal failed: channel_id=%s message_id=%s error=%v", i.ChannelID, linkRewriteMessageID(i), err)
	}
}

func (b *Bot) submitLinkRewriteEdit(s *discordgo.Session, i *discordgo.InteractionCreate, data discordgo.ModalSubmitInteractionData) {
	ownerID, ok := b.requireLinkRewriteOwner(s, i)
	if !ok {
		return
	}
	s = b.interactionSession(s)
	content, errText := composeLinkRewriteEdit(ownerID, modalTextInput(data, linkRewriteBodyInputID))
	if errText != "" {
		b.respondLinkRewrite(s, i, errText, true, nil)
		return
	}
	if s == nil {
		log.Printf("link rewrite edit dropped: no session")
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); err != nil {
		log.Printf("link rewrite edit ack failed: channel_id=%s message_id=%s error=%v", i.ChannelID, linkRewriteMessageID(i), err)
		return
	}
	components := linkRewriteComponents()
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:    &content,
		Components: &components,
	}); err != nil {
		log.Printf("link rewrite edit failed: channel_id=%s message_id=%s error=%v", i.ChannelID, linkRewriteMessageID(i), err)
		if _, followErr := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "I couldn't update that message.",
			Flags:   discordgo.MessageFlagsEphemeral,
		}); followErr != nil {
			log.Printf("link rewrite edit followup failed: channel_id=%s message_id=%s error=%v", i.ChannelID, linkRewriteMessageID(i), followErr)
		}
	}
}

func (b *Bot) confirmLinkRewriteDelete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if _, ok := b.requireLinkRewriteOwner(s, i); !ok {
		return
	}
	channelID := i.Message.ChannelID
	if channelID == "" {
		channelID = i.ChannelID
	}
	if !discordSnowflake(channelID) || !discordSnowflake(i.Message.ID) {
		b.respondLinkRewrite(s, i, linkRewriteMissingMessageText, true, nil)
		return
	}
	b.respondLinkRewrite(s, i, "Delete this shared message?", true, []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Delete",
				Style:    discordgo.DangerButton,
				CustomID: linkRewriteDeleteConfirmID(channelID, i.Message.ID),
			},
			discordgo.Button{Label: "Cancel", Style: discordgo.SecondaryButton, CustomID: linkRewriteDeleteCancelID},
		}},
	})
}

func (b *Bot) deleteLinkRewrite(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) {
	channelID, messageID, ok := parseLinkRewriteDeleteID(customID)
	if !ok {
		b.respondLinkRewrite(s, i, linkRewriteMissingMessageText, true, nil)
		return
	}
	s = b.interactionSession(s)
	if s == nil {
		log.Printf("link rewrite delete dropped: no session")
		return
	}
	msg, err := s.ChannelMessage(channelID, messageID)
	if err != nil {
		log.Printf("link rewrite delete lookup failed: channel_id=%s message_id=%s error=%v", channelID, messageID, err)
		b.respondLinkRewrite(s, i, linkRewriteMissingMessageText, true, nil)
		return
	}
	if !linkRewriteMessageOwnedBy(s, msg, interactionUserID(i)) {
		b.respondLinkRewrite(s, i, linkRewriteNotOwnerText, true, nil)
		return
	}
	if err := s.ChannelMessageDelete(channelID, messageID); err != nil {
		log.Printf("link rewrite delete failed: channel_id=%s message_id=%s error=%v", channelID, messageID, err)
		b.respondLinkRewrite(s, i, "I couldn't delete that message.", true, nil)
		return
	}
	b.updateLinkRewriteInteraction(s, i, "Deleted.", nil)
}

func linkRewriteMessageOwnedBy(s *discordgo.Session, m *discordgo.Message, userID string) bool {
	if !linkRewriteOwnedBy(m, userID) {
		return false
	}
	if s != nil && s.State != nil && s.State.User != nil && m.Author != nil && m.Author.ID != "" && m.Author.ID != s.State.User.ID {
		return false
	}
	return true
}

func (b *Bot) requireLinkRewriteOwner(s *discordgo.Session, i *discordgo.InteractionCreate) (string, bool) {
	if i == nil || i.Message == nil {
		b.respondLinkRewrite(s, i, linkRewriteMissingMessageText, true, nil)
		return "", false
	}
	ownerID, ok := linkRewriteAuthorID(i.Message.Content)
	if !ok || ownerID != interactionUserID(i) {
		b.respondLinkRewrite(s, i, linkRewriteNotOwnerText, true, nil)
		return "", false
	}
	return ownerID, true
}

func (b *Bot) interactionSession(s *discordgo.Session) *discordgo.Session {
	if s != nil {
		return s
	}
	return b.session
}

func (b *Bot) respondLinkRewrite(s *discordgo.Session, i *discordgo.InteractionCreate, content string, ephemeral bool, components []discordgo.MessageComponent) {
	s = b.interactionSession(s)
	if s == nil || i == nil || i.Interaction == nil {
		log.Printf("link rewrite interaction dropped: no session")
		return
	}
	var flags discordgo.MessageFlags
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Components: components, Flags: flags},
	}); err != nil {
		log.Printf("link rewrite interaction respond failed: content=%q error=%v", content, err)
	}
}

func (b *Bot) updateLinkRewriteInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	s = b.interactionSession(s)
	if s == nil || i == nil || i.Interaction == nil {
		log.Printf("link rewrite interaction dropped: no session")
		return
	}
	if components == nil {
		components = []discordgo.MessageComponent{}
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{Content: content, Components: components},
	}); err != nil {
		log.Printf("link rewrite interaction update failed: content=%q error=%v", content, err)
	}
}

func interactionUserID(i *discordgo.InteractionCreate) string {
	if i == nil {
		return ""
	}
	if i.Member != nil && i.Member.User != nil && i.Member.User.ID != "" {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func linkRewriteMessageID(i *discordgo.InteractionCreate) string {
	if i != nil && i.Message != nil {
		return i.Message.ID
	}
	return ""
}

func modalTextInput(data discordgo.ModalSubmitInteractionData, customID string) string {
	for _, row := range data.Components {
		actionsRow, ok := row.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, component := range actionsRow.Components {
			input, ok := component.(*discordgo.TextInput)
			if ok && input.CustomID == customID {
				return input.Value
			}
		}
	}
	return ""
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
