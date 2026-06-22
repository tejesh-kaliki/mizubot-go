package commands

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"mizubot-go/internal/reminders"
	"mizubot-go/internal/usersettings"

	"github.com/bwmarrin/discordgo"
)

const (
	reminderEmbedColor   = 0x5865F2
	reminderListPageSize = 5
)

type RemindModule struct {
	service         *reminders.Service
	settingsService *usersettings.Service
}

func NewRemindModule(service *reminders.Service, settingsService ...*usersettings.Service) *RemindModule {
	var settings *usersettings.Service
	if len(settingsService) > 0 {
		settings = settingsService[0]
	}
	return &RemindModule{service: service, settingsService: settings}
}

func (m *RemindModule) Definitions() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "remind",
			Description: "Create and manage reminders",
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
						{Type: discordgo.ApplicationCommandOptionString, Name: "at", Description: "Once: 10m, 2h, or YYYY-MM-DD HH:MM. Daily: HH:MM. Hourly: :MM. Optional.", Required: false},
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel to send this reminder in; defaults to current channel", Required: false},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "list",
					Description: "List your reminders",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionInteger, Name: "page", Description: "Page number", Required: false},
					},
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
		},
	}
}

func (m *RemindModule) Handle(responder Responder, _ *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if i.ApplicationCommandData().Name != "remind" {
		return false
	}

	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		responder.Respond(i, "Missing subcommand.", true)
		return true
	}

	switch options[0].Name {
	case "add":
		m.handleAdd(responder, i)
	case "list":
		m.handleList(responder, i)
	case "delete":
		m.handleDelete(responder, i)
	default:
		responder.Respond(i, "Unknown subcommand.", true)
	}
	return true
}

func (m *RemindModule) handleAdd(responder Responder, i *discordgo.InteractionCreate) {
	opts := i.ApplicationCommandData().Options[0].Options
	var message, scheduleStr, at string
	channelID := i.ChannelID
	for _, o := range opts {
		switch o.Name {
		case "message":
			message = o.StringValue()
		case "schedule":
			scheduleStr = o.StringValue()
		case "at":
			at = o.StringValue()
		case "channel":
			if selected := channelIDFromOption(o); selected != "" {
				channelID = selected
			}
		}
	}

	guildID := i.GuildID
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user for this reminder.", true)
		return
	}
	timezone := m.userTimezone(context.Background(), userID)

	reminder, err := m.service.CreateReminder(context.Background(), reminders.CreateReminderInput{
		UserID:    userID,
		ChannelID: channelID,
		GuildID:   guildID,
		Message:   message,
		Schedule:  scheduleStr,
		At:        at,
		Timezone:  timezone,
	})
	if err != nil {
		responder.Respond(i, err.Error(), true)
		return
	}

	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title: "Reminder Added",
		Color: reminderEmbedColor,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "ID", Value: fmt.Sprintf("%d", reminder.ID), Inline: true},
			{Name: "Schedule", Value: string(reminder.Schedule), Inline: true},
			{Name: "Channel", Value: renderChannelOrFallback(reminder.ChannelID, "Current channel"), Inline: true},
			{Name: "Timezone", Value: "`" + reminder.Timezone + "`", Inline: true},
			{Name: "Next Run", Value: formatReminderTime(reminder.NextRun), Inline: false},
			{Name: "Message", Value: trimForField(reminder.Message, 900), Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "Reminder delivery will mention you only"},
	}, true)
}

func (m *RemindModule) userTimezone(ctx context.Context, userID string) string {
	if m.settingsService == nil {
		return usersettings.DefaultTimezone
	}
	timezone, _, err := m.settingsService.GetTimezone(ctx, userID)
	if err != nil {
		log.Printf("load user timezone error: user_id=%s error=%v", userID, err)
		return usersettings.DefaultTimezone
	}
	return timezone
}

func (m *RemindModule) handleList(responder Responder, i *discordgo.InteractionCreate) {
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user for this reminder.", true)
		return
	}

	page := 1
	for _, opt := range i.ApplicationCommandData().Options[0].Options {
		if opt.Name == "page" {
			page = int(opt.IntValue())
		}
	}
	if page <= 0 {
		page = 1
	}

	list, err := m.service.ListUserReminders(context.Background(), userID)
	if err != nil {
		log.Printf("list reminders error: %v", err)
		responder.Respond(i, "Failed to list reminders.", true)
		return
	}
	embed := &discordgo.MessageEmbed{
		Title: "Reminders",
		Color: reminderEmbedColor,
	}
	if len(list) == 0 {
		embed.Description = "No reminders configured."
		responder.RespondEmbed(i, embed, true)
		return
	}

	totalPages := (len(list) + reminderListPageSize - 1) / reminderListPageSize
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * reminderListPageSize
	end := start + reminderListPageSize
	if end > len(list) {
		end = len(list)
	}

	fields := make([]*discordgo.MessageEmbedField, 0, end-start)
	for _, r := range list[start:end] {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("#%d - %s", r.ID, r.Schedule),
			Value:  formatReminderListEntry(r),
			Inline: false,
		})
	}
	embed.Fields = fields
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: fmt.Sprintf("Page %d/%d • Use /remind delete id:<id> to remove one", page, totalPages),
	}
	responder.RespondEmbed(i, embed, true)
}

func (m *RemindModule) handleDelete(responder Responder, i *discordgo.InteractionCreate) {
	var id int64
	for _, o := range i.ApplicationCommandData().Options[0].Options {
		if o.Name == "id" {
			id = o.IntValue()
		}
	}
	if id <= 0 {
		responder.Respond(i, "Invalid id", true)
		return
	}

	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user for this reminder.", true)
		return
	}

	deleted, ok, err := m.service.DeleteReminderWithDetails(context.Background(), id, userID)
	if err != nil {
		log.Printf("delete reminder error: %v", err)
		responder.Respond(i, "Failed to delete.", true)
		return
	}
	if !ok {
		responder.Respond(i, "Reminder not found.", true)
		return
	}
	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title: "Reminder Deleted",
		Color: reminderEmbedColor,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "ID", Value: fmt.Sprintf("%d", id), Inline: true},
			{Name: "Repeat", Value: formatReminderRepeat(deleted), Inline: true},
			{Name: "Timezone", Value: "`" + deleted.Timezone + "`", Inline: true},
			{Name: "Next Run", Value: formatReminderTime(deleted.NextRun), Inline: false},
			{Name: "Channel", Value: renderChannelOrFallback(deleted.ChannelID, "Unknown"), Inline: true},
			{Name: "Message", Value: trimForField(deleted.Message, 900), Inline: false},
		},
	}, true)
}

func userIDFromInteraction(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func formatReminderListEntry(r reminders.Reminder) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Next: %s", formatReminderTime(r.NextRun))
	b.WriteString("\n")
	fmt.Fprintf(&b, "Timezone: %s", r.Timezone)
	b.WriteString("\n")
	fmt.Fprintf(&b, "Channel: %s", renderChannelOrFallback(r.ChannelID, "Unknown"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "Message: %s", trimForField(r.Message, 220))
	return b.String()
}

func formatReminderTime(t time.Time) string {
	return fmt.Sprintf("<t:%d:F> (<t:%d:R>)", t.UTC().Unix(), t.UTC().Unix())
}

func formatReminderRepeat(r reminders.Reminder) string {
	if r.Once {
		return "once"
	}
	switch r.Schedule {
	case reminders.ScheduleHourly:
		return "hourly"
	case reminders.ScheduleDaily:
		return "daily"
	case reminders.ScheduleCron:
		return "custom"
	default:
		return string(r.Schedule)
	}
}
