package commands

import (
	"context"
	"fmt"
	"log"
	"strings"

	"mizubot-go/internal/reminders"

	"github.com/bwmarrin/discordgo"
)

type RemindModule struct {
	service *reminders.Service
}

func NewRemindModule(service *reminders.Service) *RemindModule {
	return &RemindModule{service: service}
}

func (m *RemindModule) Definitions() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
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

	guildID := i.GuildID
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user for this reminder.", true)
		return
	}

	reminder, err := m.service.CreateReminder(context.Background(), reminders.CreateReminderInput{
		UserID:    userID,
		ChannelID: i.ChannelID,
		GuildID:   guildID,
		Message:   message,
		Schedule:  scheduleStr,
		At:        at,
	})
	if err != nil {
		responder.Respond(i, err.Error(), true)
		return
	}

	responder.Respond(i, fmt.Sprintf("Created reminder %d scheduled at %s", reminder.ID, reminder.NextRun.Format("2006-01-02T15:04:05Z07:00")), true)
}

func (m *RemindModule) handleList(responder Responder, i *discordgo.InteractionCreate) {
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user for this reminder.", true)
		return
	}

	list, err := m.service.ListUserReminders(context.Background(), userID)
	if err != nil {
		log.Printf("list reminders error: %v", err)
		responder.Respond(i, "Failed to list reminders.", true)
		return
	}
	if len(list) == 0 {
		responder.Respond(i, "You have no reminders.", true)
		return
	}

	var bld strings.Builder
	for _, r := range list {
		fmt.Fprintf(&bld, "ID %d - %s - next %s - %s\n", r.ID, r.Schedule, r.NextRun.Format("2006-01-02T15:04:05Z07:00"), r.Message)
	}
	responder.Respond(i, bld.String(), true)
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

	ok, err := m.service.DeleteReminder(context.Background(), id, userID)
	if err != nil {
		log.Printf("delete reminder error: %v", err)
		responder.Respond(i, "Failed to delete.", true)
		return
	}
	if !ok {
		responder.Respond(i, "Reminder not found.", true)
		return
	}
	responder.Respond(i, "Deleted.", true)
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
