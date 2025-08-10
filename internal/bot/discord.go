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
	if b.session == nil || b.session.State == nil || b.session.State.User == nil {
		return fmt.Errorf("discord session not ready: open the session before registering commands")
	}
	return b.registrar.RegisterCommands(b.session.State.User.ID, "", []*discordgo.ApplicationCommand{remindCommand()})
}

func (b *Bot) RegisterCommandsForGuild(guildID string) error {
	if b.registrar == nil {
		return nil
	}
	if b.session == nil || b.session.State == nil || b.session.State.User == nil {
		return fmt.Errorf("discord session not ready: open the session before registering commands")
	}
	return b.registrar.RegisterCommands(b.session.State.User.ID, guildID, []*discordgo.ApplicationCommand{remindCommand()})
}

func (b *Bot) SetDryRun(d bool) { b.dryRun = d }

func remindCommand() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "remind",
		Description: "Create and manage reminders (times are in UTC)",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "add",
				Description: "Add a reminder",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionString, Name: "message", Description: "What should I send?", Required: true},
					{Type: discordgo.ApplicationCommandOptionString, Name: "schedule", Description: "When: once, hourly, or daily", Required: true, Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "once", Value: "once"},
						{Name: "hourly", Value: "hourly"},
						{Name: "daily", Value: "daily"},
					}},
					{Type: discordgo.ApplicationCommandOptionString, Name: "at", Description: "When (UTC). Once: 'YYYY-MM-DD HH:MM' or ISO like '2025-01-31T15:04:05Z'. Daily: 'HH:MM'. Optional.", Required: false},
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
			// default: 5 minutes from now
			nextRun = now.Add(5 * time.Minute)
			atNS = sql.NullString{String: nextRun.Format(time.RFC3339), Valid: true}
		} else {
			// try parsing flexible formats: 'YYYY-MM-DD HH:MM' or RFC3339
			parsed, perr := parseFlexibleTimeUTC(at)
			if perr != nil || !parsed.After(now) {
				b.respond(i, "Invalid time. Use 'YYYY-MM-DD HH:MM' or ISO like '2025-01-31T15:04:05Z' (UTC).", true)
				return
			}
			nextRun = parsed
			atNS = sql.NullString{String: nextRun.Format(time.RFC3339), Valid: true}
		}
	case reminders.ScheduleHourly:
		// default: next minute
		if at == "" {
			nextRun = now.Truncate(time.Minute).Add(time.Minute)
		} else {
			// allow minutes offset like ":15" or "HH:MM"
			parsed, perr := parseHourMinuteUTC(now, at)
			if perr != nil || !parsed.After(now) {
				// fallback to +1h
				nextRun = now.Add(time.Hour)
			} else {
				nextRun = parsed
			}
		}
	case reminders.ScheduleDaily:
		// default: next minute (HH:MM of now+1min)
		hhmm := at
		if hhmm == "" {
			nm := now.Truncate(time.Minute).Add(time.Minute)
			hhmm = fmt.Sprintf("%02d:%02d", nm.Hour(), nm.Minute())
		}
		if len(hhmm) != 5 || hhmm[2] != ':' {
			b.respond(i, "Use HH:MM (UTC), e.g., 09:00.", true)
			return
		}
		hour := (int(hhmm[0]-'0')*10 + int(hhmm[1]-'0'))
		min := (int(hhmm[3]-'0')*10 + int(hhmm[4]-'0'))
		nextRun = time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)
		if !nextRun.After(now) {
			nextRun = nextRun.Add(24 * time.Hour)
		}
		atNS = sql.NullString{String: hhmm, Valid: true}
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
