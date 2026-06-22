package commands

import (
	"context"
	"fmt"

	"mizubot-go/internal/usersettings"

	"github.com/bwmarrin/discordgo"
)

const settingsEmbedColor = 0x95A5A6

type SettingsModule struct {
	service *usersettings.Service
}

func NewSettingsModule(service *usersettings.Service) *SettingsModule {
	return &SettingsModule{service: service}
}

func (m *SettingsModule) Definitions() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "settings",
			Description: "Manage your MizuBot settings",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
					Name:        "timezone",
					Description: "Manage your timezone",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionSubCommand,
							Name:        "get",
							Description: "Show your configured timezone",
						},
						{
							Type:        discordgo.ApplicationCommandOptionSubCommand,
							Name:        "set",
							Description: "Set your timezone",
							Options: []*discordgo.ApplicationCommandOption{
								{Type: discordgo.ApplicationCommandOptionString, Name: "timezone", Description: "Timezone, e.g. Asia/Kolkata, India, or New York", Required: true},
							},
						},
					},
				},
			},
		},
	}
}

func (m *SettingsModule) Handle(responder Responder, _ *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if i.ApplicationCommandData().Name != "settings" {
		return false
	}

	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		responder.Respond(i, "Missing settings group.", true)
		return true
	}
	if options[0].Name != "timezone" {
		responder.Respond(i, "Unknown settings group.", true)
		return true
	}
	if len(options[0].Options) == 0 {
		responder.Respond(i, "Missing timezone subcommand.", true)
		return true
	}

	switch options[0].Options[0].Name {
	case "get":
		m.handleTimezoneGet(responder, i)
	case "set":
		m.handleTimezoneSet(responder, i, options[0].Options[0])
	default:
		responder.Respond(i, "Unknown timezone subcommand.", true)
	}
	return true
}

func (m *SettingsModule) handleTimezoneGet(responder Responder, i *discordgo.InteractionCreate) {
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user.", true)
		return
	}

	timezone, configured, err := m.service.GetTimezone(context.Background(), userID)
	if err != nil {
		responder.Respond(i, "Failed to load timezone.", true)
		return
	}
	description := fmt.Sprintf("Your timezone is `%s`.", timezone)
	if !configured {
		description = fmt.Sprintf("No timezone is configured. I will use `%s` by default.", timezone)
	}
	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title:       "Timezone",
		Color:       settingsEmbedColor,
		Description: description,
	}, true)
}

func (m *SettingsModule) handleTimezoneSet(responder Responder, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user.", true)
		return
	}

	var timezone string
	for _, opt := range sub.Options {
		if opt.Name == "timezone" {
			timezone = opt.StringValue()
		}
	}

	settings, err := m.service.SetTimezone(context.Background(), userID, timezone)
	if err != nil {
		responder.Respond(i, err.Error(), true)
		return
	}
	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title: "Timezone Updated",
		Color: settingsEmbedColor,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Timezone", Value: "`" + settings.Timezone + "`", Inline: true},
		},
	}, true)
}
