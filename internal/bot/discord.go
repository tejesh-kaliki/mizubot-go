package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"mizubot-go/internal/animefeed"
	"mizubot-go/internal/bot/commands"
	"mizubot-go/internal/llm"
	"mizubot-go/internal/llmstats"
	"mizubot-go/internal/pagemonitor"
	"mizubot-go/internal/reminders"
	"mizubot-go/internal/usersettings"

	"github.com/bwmarrin/discordgo"
)

type CommandRegistrar interface {
	RegisterCommands(appID, guildID string, cmds []*discordgo.ApplicationCommand) error
}

type discordRegistrar struct{ s *discordgo.Session }

func (r discordRegistrar) RegisterCommands(appID, guildID string, cmds []*discordgo.ApplicationCommand) error {
	_, err := r.s.ApplicationCommandBulkOverwrite(appID, guildID, cmds)
	return err
}

type SenderFunc func(channelID, content string) error

type llmMessageLogger interface {
	Create(ctx context.Context, params llmstats.CreateMessageLogParams) (llmstats.MessageLog, error)
}

type Bot struct {
	session      *discordgo.Session
	registrar    CommandRegistrar
	modules      []commands.Module
	dryRun       bool
	debugHistory bool
	llm          *llm.Service
	llmLogger    llmMessageLogger
	userSettings *usersettings.Service
}

func New(token string, store *reminders.Store, animeService *animefeed.Service, monitorService *pagemonitor.Service, llmService *llm.Service, userSettingsService *usersettings.Service, llmLogger llmMessageLogger) (*Bot, error) {
	s, err := discordgo.New(token)
	if err != nil {
		return nil, err
	}
	// Message Content is a privileged intent: without it, Discord returns an
	// empty Content field for any message that doesn't mention this bot,
	// wasn't sent by it, or isn't a DM — both over the gateway and via REST
	// endpoints like ChannelMessages, which the conversation-history buffer
	// depends on. Must also be enabled for this application in the Discord
	// Developer Portal (Bot > Privileged Gateway Intents), or the gateway
	// will reject the connection with a disallowed-intents error.
	s.Identify.Intents |= discordgo.IntentMessageContent

	reminderService := reminders.NewService(store)
	modules := []commands.Module{
		commands.NewRemindModule(reminderService, userSettingsService),
	}
	if animeService != nil {
		modules = append(modules, commands.NewAnimeModule(animeService))
	}
	if monitorService != nil {
		modules = append(modules, commands.NewMonitorModule(monitorService))
	}
	if userSettingsService != nil {
		modules = append(modules, commands.NewSettingsModule(userSettingsService))
	}
	b := &Bot{
		session:      s,
		registrar:    discordRegistrar{s: s},
		modules:      modules,
		llm:          llmService,
		llmLogger:    llmLogger,
		userSettings: userSettingsService,
	}
	s.AddHandler(b.onInteractionCreate)
	s.AddHandler(b.onMessageCreate)
	return b, nil
}

func (b *Bot) Open() error {
	return b.session.Open()
}

func (b *Bot) Close() error {
	return b.session.Close()
}

func (b *Bot) SendChannelMessage(channelID, content string) error {
	if b.dryRun {
		return nil
	}
	_, err := b.session.ChannelMessageSend(channelID, content)
	return err
}

func (b *Bot) SendReminder(reminder reminders.Reminder) error {
	if b.dryRun {
		return nil
	}
	_, err := b.session.ChannelMessageSendComplex(reminder.ChannelID, &discordgo.MessageSend{
		Content: "<@" + reminder.UserID + ">\n\n" + reminder.Message,
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Users: []string{reminder.UserID},
		},
	})
	return err
}

func (b *Bot) SendAnimeNotification(channelID string, embed animefeed.AnimeNotificationEmbed) error {
	if b.dryRun {
		return nil
	}

	msgEmbed := &discordgo.MessageEmbed{
		Title: embed.Title,
		URL:   embed.Link,
		Color: 0x2E8B57,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Follow", Value: embed.FollowName, Inline: true},
			{Name: "User", Value: "<@" + embed.UserID + ">", Inline: true},
			{Name: "Release", Value: "[Open on Nyaa](" + embed.Link + ")", Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "New anime feed match"},
	}
	if embed.PublishedAt != nil {
		msgEmbed.Timestamp = embed.PublishedAt.UTC().Format(time.RFC3339)
	}

	_, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: "<@" + embed.UserID + ">",
		Embeds:  []*discordgo.MessageEmbed{msgEmbed},
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Users: []string{embed.UserID},
		},
	})
	return err
}

func (b *Bot) SendPageMonitorNotification(channelID string, m pagemonitor.Monitor, oldContent, newContent string) error {
	if b.dryRun {
		return nil
	}
	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("Page changed: %s", m.Label),
		URL:   m.URL,
		Color: 0xF39C12,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Before", Value: truncate(oldContent, 300), Inline: false},
			{Name: "After", Value: truncate(newContent, 300), Inline: false},
			{Name: "URL", Value: m.URL, Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "Page Monitor"},
	}
	userMention := "<@" + m.UserID + ">"
	_, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: userMention,
		Embeds:  []*discordgo.MessageEmbed{embed},
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Users: []string{m.UserID},
		},
	})
	return err
}

func truncate(s string, max int) string {
	if s == "" {
		return "(empty)"
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func (b *Bot) RegisterCommandsGlobal() error {
	if b.registrar == nil {
		return nil
	}
	if b.session == nil || b.session.State == nil || b.session.State.User == nil {
		return fmt.Errorf("discord session not ready: open the session before registering commands")
	}
	return b.registrar.RegisterCommands(b.session.State.User.ID, "", b.commandDefinitions())
}

func (b *Bot) RegisterCommandsForGuild(guildID string) error {
	if b.registrar == nil {
		return nil
	}
	if b.session == nil || b.session.State == nil || b.session.State.User == nil {
		return fmt.Errorf("discord session not ready: open the session before registering commands")
	}
	return b.registrar.RegisterCommands(b.session.State.User.ID, guildID, b.commandDefinitions())
}

func (b *Bot) SetDryRun(d bool) { b.dryRun = d }

// SetDebugHistory toggles verbose logging of the conversation history
// (path used, message count, and each history entry) built for LLM requests.
func (b *Bot) SetDebugHistory(d bool) { b.debugHistory = d }

func (b *Bot) commandDefinitions() []*discordgo.ApplicationCommand {
	defs := make([]*discordgo.ApplicationCommand, 0, len(b.modules))
	for _, module := range b.modules {
		defs = append(defs, module.Definitions()...)
	}
	return defs
}

func (b *Bot) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	for _, module := range b.modules {
		if module.Handle(b, s, i) {
			return
		}
	}
}

func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if s == nil || s.State == nil || s.State.User == nil || m == nil || m.Author == nil {
		return
	}
	if m.Author.ID == s.State.User.ID || m.Content == "" {
		return
	}
	if !shouldTriggerLLM(s, m.Message) {
		return
	}
	if b.dryRun {
		return
	}

	response := "Hello"
	if b.llm != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		stopTyping := b.startTyping(ctx, s, m.ChannelID)

		log.Printf("generating llm response: channel_id=%s user_id=%s message_id=%s", m.ChannelID, m.Author.ID, m.ID)
		startedAt := time.Now()
		timezone := b.userTimezoneForMessage(ctx, m.Author.ID)
		history := buildConversationHistory(s, s, m.Message)
		debugLogHistory(b.debugHistory, m.ChannelID, m.ID, historySourcePath(m.Message), history)
		generated, err := b.llm.GenerateResponseWithMetrics(ctx, llm.Message{
			UserID:    m.Author.ID,
			Username:  guildDisplayName(s, m.GuildID, m.Author, m.Member),
			BotName:   guildDisplayName(s, m.GuildID, s.State.User, nil),
			ChannelID: m.ChannelID,
			GuildID:   m.GuildID,
			Content:   messageTextWithDisplayNames(s, m.Message),
			Timezone:  timezone,
			Now:       startedAt,
			History:   history,
		})
		latency := time.Since(startedAt)
		if err != nil {
			log.Printf("llm response generation failed: channel_id=%s user_id=%s message_id=%s error=%v", m.ChannelID, m.Author.ID, m.ID, err)
			response = "I couldn't generate a response right now."
			b.logLLMMessage(ctx, m, llm.Response{}, latency, llmstats.StatusError, err.Error())
		} else if generated.Content != "" {
			response = generated.Content
			b.logLLMMessage(ctx, m, generated, latency, llmstats.StatusSuccess, "")
		} else {
			log.Printf("llm returned empty response: channel_id=%s user_id=%s message_id=%s", m.ChannelID, m.Author.ID, m.ID)
			b.logLLMMessage(ctx, m, generated, latency, llmstats.StatusSuccess, "")
		}
		cancel()
		stopTyping()
	} else {
		log.Printf("llm service not configured; using fallback response: channel_id=%s user_id=%s message_id=%s", m.ChannelID, m.Author.ID, m.ID)
	}
	responses := splitDiscordMessages(response)
	for idx, part := range responses {
		var err error
		if idx == 0 {
			_, err = s.ChannelMessageSendReply(m.ChannelID, part, m.Reference())
		} else {
			_, err = s.ChannelMessageSend(m.ChannelID, part)
		}
		if err != nil {
			log.Printf("discord reply send failed: channel_id=%s user_id=%s message_id=%s part=%d total_parts=%d error=%v", m.ChannelID, m.Author.ID, m.ID, idx+1, len(responses), err)
			return
		}
	}
}

func (b *Bot) userTimezoneForMessage(ctx context.Context, userID string) string {
	if b.userSettings == nil {
		return usersettings.DefaultTimezone
	}
	timezone, _, err := b.userSettings.GetTimezone(ctx, userID)
	if err != nil {
		log.Printf("load user timezone failed: user_id=%s error=%v", userID, err)
		return usersettings.DefaultTimezone
	}
	return timezone
}

func (b *Bot) logLLMMessage(ctx context.Context, m *discordgo.MessageCreate, response llm.Response, latency time.Duration, status, errText string) {
	if b.llmLogger == nil || m == nil || m.Author == nil {
		return
	}
	_, err := b.llmLogger.Create(ctx, llmstats.CreateMessageLogParams{
		GuildID:          m.GuildID,
		ChannelID:        m.ChannelID,
		UserID:           m.Author.ID,
		MessageID:        m.ID,
		PromptTokens:     response.Usage.PromptTokens,
		CompletionTokens: response.Usage.CompletionTokens,
		TotalTokens:      response.Usage.TotalTokens(),
		LLMTurns:         response.LLMTurns,
		ToolCalls:        response.ToolCalls,
		Latency:          latency,
		Status:           status,
		Error:            errText,
	})
	if err != nil {
		log.Printf("llm message log failed: channel_id=%s user_id=%s message_id=%s error=%v", m.ChannelID, m.Author.ID, m.ID, err)
	}
}

func (b *Bot) startTyping(ctx context.Context, s *discordgo.Session, channelID string) func() {
	if s == nil || channelID == "" {
		return func() {}
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	if err := s.ChannelTyping(channelID); err != nil {
		log.Printf("discord typing indicator error: %v", err)
	}
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := s.ChannelTyping(channelID); err != nil {
					log.Printf("discord typing indicator refresh error: %v", err)
				}
			case <-ctx.Done():
				return
			case <-done:
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
		})
		<-stopped
	}
}

func (b *Bot) Respond(i *discordgo.InteractionCreate, content string, ephemeral bool) {
	var flags discordgo.MessageFlags
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	_ = b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: flags},
	})
}

func (b *Bot) RespondEmbed(i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, ephemeral bool) {
	var flags discordgo.MessageFlags
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	_ = b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  flags,
		},
	})
}

func messageMentionsUser(content, userID string) bool {
	return strings.Contains(content, "<@"+userID+">") || strings.Contains(content, "<@!"+userID+">")
}

func messageTextWithDisplayNames(s *discordgo.Session, message *discordgo.Message) string {
	if message == nil {
		return ""
	}
	content := message.Content
	for _, user := range message.Mentions {
		if user == nil || user.ID == "" {
			continue
		}
		name := guildDisplayName(s, message.GuildID, user, nil)
		content = strings.NewReplacer(
			"<@"+user.ID+">", "@"+name,
			"<@!"+user.ID+">", "@"+name,
		).Replace(content)
	}
	return strings.TrimSpace(content)
}

func guildDisplayName(s *discordgo.Session, guildID string, user *discordgo.User, member *discordgo.Member) string {
	if member != nil && strings.TrimSpace(member.Nick) != "" {
		return strings.TrimSpace(member.Nick)
	}
	if s != nil && s.State != nil && guildID != "" && user != nil {
		if guildMember, err := s.State.Member(guildID, user.ID); err == nil && strings.TrimSpace(guildMember.Nick) != "" {
			return strings.TrimSpace(guildMember.Nick)
		}
	}
	if user != nil && strings.TrimSpace(user.Username) != "" {
		return strings.TrimSpace(user.Username)
	}
	return "Discord user"
}

func splitDiscordMessages(content string) []string {
	const maxDiscordMessageLength = 2000
	content = strings.TrimSpace(content)
	if content == "" {
		return []string{"Hello"}
	}

	var parts []string
	remaining := content
	for len([]rune(remaining)) > maxDiscordMessageLength {
		chunk, rest := splitDiscordMessageChunk(remaining, maxDiscordMessageLength)
		parts = append(parts, chunk)
		remaining = strings.TrimSpace(rest)
	}
	parts = append(parts, remaining)
	return parts
}

func splitDiscordMessageChunk(content string, maxRunes int) (string, string) {
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return strings.TrimSpace(content), ""
	}

	window := string(runes[:maxRunes])
	breakAt := lastBreakIndex(window, "\n\n")
	if breakAt < maxRunes/2 {
		breakAt = lastBreakIndex(window, "\n")
	}
	if breakAt < maxRunes/2 {
		breakAt = lastBreakIndex(window, " ")
	}
	if breakAt < maxRunes/2 {
		breakAt = maxRunes
	}

	chunk := strings.TrimSpace(string(runes[:breakAt]))
	rest := strings.TrimSpace(string(runes[breakAt:]))
	return chunk, rest
}

func lastBreakIndex(content, sep string) int {
	byteIdx := strings.LastIndex(content, sep)
	if byteIdx < 0 {
		return -1
	}
	return len([]rune(content[:byteIdx+len(sep)]))
}
