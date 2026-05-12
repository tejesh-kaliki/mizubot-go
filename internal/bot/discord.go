package bot

import (
	"fmt"

	"mizubot-go/internal/bot/commands"
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

func New(token string, store *reminders.Store) (*Bot, error) {
	s, err := discordgo.New(token)
	if err != nil {
		return nil, err
	}

	reminderService := reminders.NewService(store)
	b := &Bot{
		session:   s,
		registrar: discordRegistrar{s: s},
		modules: []commands.Module{
			commands.NewRemindModule(reminderService),
		},
	}
	s.AddHandler(b.onInteractionCreate)
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
