package bot

import (
	"fmt"
	"strings"
	"time"

	"mizubot-go/internal/animefeed"
	"mizubot-go/internal/bot/commands"
	"mizubot-go/internal/pagemonitor"
	"mizubot-go/internal/reminders"

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

type Bot struct {
	session   *discordgo.Session
	registrar CommandRegistrar
	modules   []commands.Module
	dryRun    bool
}

func New(token string, store *reminders.Store, animeService *animefeed.Service, monitorService *pagemonitor.Service) (*Bot, error) {
	s, err := discordgo.New(token)
	if err != nil {
		return nil, err
	}

	reminderService := reminders.NewService(store)
	modules := []commands.Module{
		commands.NewRemindModule(reminderService),
	}
	if animeService != nil {
		modules = append(modules, commands.NewAnimeModule(animeService))
	}
	if monitorService != nil {
		modules = append(modules, commands.NewMonitorModule(monitorService))
	}
	b := &Bot{
		session:   s,
		registrar: discordRegistrar{s: s},
		modules:   modules,
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
	if !messageMentionsUser(m.Content, s.State.User.ID) {
		return
	}
	if b.dryRun {
		return
	}
	_, _ = s.ChannelMessageSendReply(m.ChannelID, "Hello", m.Reference())
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
