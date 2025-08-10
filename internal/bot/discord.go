package bot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

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
	store     *reminders.Store
	registrar CommandRegistrar
	dryRun    bool
}

func New(token string, store *reminders.Store) (*Bot, error) {
	s, err := discordgo.New(token)
	if err != nil {
		return nil, err
	}
	b := &Bot{session: s, store: store, registrar: discordRegistrar{s: s}}
	s.AddHandler(b.onInteractionCreate)
	return b, nil
}

func (b *Bot) Open() error {
	if err := b.session.Open(); err != nil {
		return err
	}
	return nil
}

func (b *Bot) Close() error {
	return b.session.Close()
}

func (b *Bot) SendReminderMessage(channelID, content string) error {
	if b.dryRun {
		log.Printf("DRY RUN send to %s: %s", channelID, content)
		return nil
	}
	_, err := b.session.ChannelMessageSend(channelID, content)
	return err
}

func (b *Bot) RegisterCommandsGlobal() error {
	if b.registrar == nil {
		return nil
	}
	return b.registrar.RegisterCommands(b.session.State.User.ID, "", []*discordgo.ApplicationCommand{remindCommand()})
}

func (b *Bot) RegisterCommandsForGuild(guildID string) error {
	if b.registrar == nil {
		return nil
	}
	return b.registrar.RegisterCommands(b.session.State.User.ID, guildID, []*discordgo.ApplicationCommand{remindCommand()})
}

func (b *Bot) SetDryRun(d bool) { b.dryRun = d }

func remindCommand() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "remind",
		Description: "Create and manage reminders",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "add",
				Description: "Add a reminder",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionString, Name: "message", Description: "Message to send", Required: true},
					{Type: discordgo.ApplicationCommandOptionString, Name: "schedule", Description: "once | hourly | daily", Required: true, Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "once", Value: "once"},
						{Name: "hourly", Value: "hourly"},
						{Name: "daily", Value: "daily"},
					}},
					{Type: discordgo.ApplicationCommandOptionString, Name: "at", Description: "RFC3339 for once, HH:MM for daily", Required: false},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "List your reminders",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "delete",
				Description: "Delete a reminder by id",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionInteger, Name: "id", Description: "Reminder ID", Required: true},
				},
			},
		},
	}
}

func (b *Bot) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if i.ApplicationCommandData().Name != "remind" {
		return
	}

	switch i.ApplicationCommandData().Options[0].Name {
	case "add":
		b.handleAdd(i)
	case "list":
		b.handleList(i)
	case "delete":
		b.handleDelete(i)
	}
}

func (b *Bot) respond(i *discordgo.InteractionCreate, content string, ephemeral bool) {
	var flags discordgo.MessageFlags
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	_ = b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: flags},
	})
}

func (b *Bot) handleAdd(i *discordgo.InteractionCreate) {
	opts := i.ApplicationCommandData().Options[0].Options
	var message, scheduleStr, at string
	for _, o := range opts {
		switch o.Name {
		case "message":
			message = o.StringValue()
		case "schedule":
			scheduleStr = o.StringValue()
		case "at":
			at = o.StringValue()
		}
	}

	schedule := reminders.Schedule(strings.ToLower(scheduleStr))
	if schedule != reminders.ScheduleOnce && schedule != reminders.ScheduleHourly && schedule != reminders.ScheduleDaily {
		b.respond(i, "Invalid schedule. Use once, hourly, or daily.", true)
		return
	}

	// Determine next_run
	now := time.Now().UTC()
	var nextRun time.Time
	var atNS sql.NullString
	switch schedule {
	case reminders.ScheduleOnce:
		if at == "" {
			b.respond(i, "For once schedule, provide 'at' in RFC3339, e.g. 2025-01-31T15:04:05Z", true)
			return
		}
		t, err := time.Parse(time.RFC3339, at)
		if err != nil || !t.After(now) {
			b.respond(i, "Invalid RFC3339 time or time is in the past.", true)
			return
		}
		nextRun = t.UTC()
		atNS = sql.NullString{String: nextRun.Format(time.RFC3339), Valid: true}
	case reminders.ScheduleHourly:
		nextRun = now.Add(time.Hour)
	case reminders.ScheduleDaily:
		if at == "" {
			b.respond(i, "For daily schedule, provide 'at' in HH:MM (24h) server time.", true)
			return
		}
		if len(at) != 5 || at[2] != ':' {
			b.respond(i, "Invalid HH:MM format.", true)
			return
		}
		hour := (int(at[0]-'0')*10 + int(at[1]-'0'))
		min := (int(at[3]-'0')*10 + int(at[4]-'0'))
		nextRun = time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)
		if !nextRun.After(now) {
			nextRun = nextRun.Add(24 * time.Hour)
		}
		atNS = sql.NullString{String: at, Valid: true}
	}

	guildID := ""
	if i.GuildID != "" {
		guildID = i.GuildID
	}

	r := &reminders.Reminder{
		UserID:    i.Member.User.ID,
		ChannelID: i.ChannelID,
		GuildID:   sql.NullString{String: guildID, Valid: guildID != ""},
		Message:   message,
		Schedule:  schedule,
		AtTime:    atNS,
		NextRun:   nextRun,
	}
	if err := b.store.Create(context.Background(), r); err != nil {
		log.Printf("create reminder error: %v", err)
		b.respond(i, "Failed to create reminder.", true)
		return
	}
	b.respond(i, fmt.Sprintf("Created reminder %d scheduled at %s", r.ID, r.NextRun.Format(time.RFC3339)), true)
}

func (b *Bot) handleList(i *discordgo.InteractionCreate) {
	userID := i.Member.User.ID
	list, err := b.store.ListByUser(context.Background(), userID)
	if err != nil {
		log.Printf("list reminders error: %v", err)
		b.respond(i, "Failed to list reminders.", true)
		return
	}
	if len(list) == 0 {
		b.respond(i, "You have no reminders.", true)
		return
	}
	var bld strings.Builder
	for _, r := range list {
		fmt.Fprintf(&bld, "ID %d — %s — next %s — %s\n", r.ID, r.Schedule, r.NextRun.Format(time.RFC3339), r.Message)
	}
	b.respond(i, bld.String(), true)
}

func (b *Bot) handleDelete(i *discordgo.InteractionCreate) {
	var id int64
	for _, o := range i.ApplicationCommandData().Options[0].Options {
		if o.Name == "id" {
			id = o.IntValue()
		}
	}
	if id <= 0 {
		b.respond(i, "Invalid id", true)
		return
	}
	ok, err := b.store.Delete(context.Background(), id, i.Member.User.ID)
	if err != nil {
		log.Printf("delete reminder error: %v", err)
		b.respond(i, "Failed to delete.", true)
		return
	}
	if !ok {
		b.respond(i, "Reminder not found.", true)
		return
	}
	b.respond(i, "Deleted.", true)
}
