package commands

import (
	"context"
	"fmt"
	"log"
	"strings"

	"mizubot-go/internal/animefeed"

	"github.com/bwmarrin/discordgo"
)

type AnimeModule struct {
	service *animefeed.Service
}

func NewAnimeModule(service *animefeed.Service) *AnimeModule {
	return &AnimeModule{service: service}
}

func (m *AnimeModule) Definitions() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "anime",
			Description: "Manage anime follows for the Nyaa RSS feed",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "follow",
					Description: "Follow an anime by name and keyword list",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "Label for this follow entry", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "keywords", Description: "Comma-separated keywords to match in the Nyaa title", Required: true},
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Optional channel for notifications", Required: false},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "channel",
					Description: "Set or clear the per-follow notification channel override",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "Follow entry name", Required: true},
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel to notify; omit to clear", Required: false},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "default-channel",
					Description: "Set or clear your default notification channel",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Default channel; omit to clear", Required: false},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "list",
					Description: "List your anime follows, channels, and latest match",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "unfollow",
					Description: "Delete a follow entry",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "Follow entry name", Required: true},
					},
				},
			},
		},
	}
}

func (m *AnimeModule) Handle(responder Responder, _ *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if i.ApplicationCommandData().Name != "anime" {
		return false
	}

	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		responder.Respond(i, "Missing subcommand.", true)
		return true
	}

	switch options[0].Name {
	case "follow":
		m.handleFollow(responder, i)
	case "channel":
		m.handleChannel(responder, i)
	case "default-channel":
		m.handleDefaultChannel(responder, i)
	case "list":
		m.handleList(responder, i)
	case "unfollow":
		m.handleUnfollow(responder, i)
	default:
		responder.Respond(i, "Unknown subcommand.", true)
	}
	return true
}

func (m *AnimeModule) handleFollow(responder Responder, i *discordgo.InteractionCreate) {
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user.", true)
		return
	}

	var name string
	var keywordsRaw string
	var channelID string
	for _, opt := range i.ApplicationCommandData().Options[0].Options {
		switch opt.Name {
		case "name":
			name = opt.StringValue()
		case "keywords":
			keywordsRaw = opt.StringValue()
		case "channel":
			channelID = channelIDFromOption(opt)
		}
	}

	entry, err := m.service.Follow(context.Background(), animefeed.FollowInput{
		UserID:    userID,
		Name:      name,
		Keywords:  strings.Split(keywordsRaw, ","),
		ChannelID: channelID,
	})
	if err != nil {
		responder.Respond(i, err.Error(), true)
		return
	}

	channelMsg := "using your default notification channel"
	if entry.ChannelID != "" {
		channelMsg = fmt.Sprintf("override channel <#%s>", entry.ChannelID)
	}
	responder.Respond(i, fmt.Sprintf("Created follow `%s` with keywords `%s` and %s.", entry.Name, strings.Join(entry.Keywords, ", "), channelMsg), true)
}

func (m *AnimeModule) handleChannel(responder Responder, i *discordgo.InteractionCreate) {
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user.", true)
		return
	}

	var name string
	var channelID string
	for _, opt := range i.ApplicationCommandData().Options[0].Options {
		switch opt.Name {
		case "name":
			name = opt.StringValue()
		case "channel":
			channelID = channelIDFromOption(opt)
		}
	}

	ok, err := m.service.SetChannel(context.Background(), userID, name, channelID)
	if err != nil {
		responder.Respond(i, err.Error(), true)
		return
	}
	if !ok {
		responder.Respond(i, "Follow entry not found.", true)
		return
	}

	if channelID == "" {
		responder.Respond(i, fmt.Sprintf("Cleared per-follow channel override for `%s`. It will use your default channel.", name), true)
		return
	}
	responder.Respond(i, fmt.Sprintf("Set per-follow channel override for `%s` to <#%s>.", name, channelID), true)
}

func (m *AnimeModule) handleDefaultChannel(responder Responder, i *discordgo.InteractionCreate) {
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user.", true)
		return
	}

	var channelID string
	for _, opt := range i.ApplicationCommandData().Options[0].Options {
		if opt.Name == "channel" {
			channelID = channelIDFromOption(opt)
		}
	}

	settings, err := m.service.SetDefaultChannel(context.Background(), userID, channelID)
	if err != nil {
		responder.Respond(i, err.Error(), true)
		return
	}
	if settings.DefaultChannelID == "" {
		responder.Respond(i, "Cleared your default anime notification channel.", true)
		return
	}
	responder.Respond(i, fmt.Sprintf("Set your default anime notification channel to <#%s>.", settings.DefaultChannelID), true)
}

func (m *AnimeModule) handleList(responder Responder, i *discordgo.InteractionCreate) {
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user.", true)
		return
	}

	entries, err := m.service.ListFollows(context.Background(), userID)
	if err != nil {
		log.Printf("list anime follows error: %v", err)
		responder.Respond(i, "Failed to list anime follows.", true)
		return
	}
	settings, err := m.service.GetSettings(context.Background(), userID)
	if err != nil {
		log.Printf("get anime settings error: %v", err)
		responder.Respond(i, "Failed to load anime settings.", true)
		return
	}
	if len(entries) == 0 {
		if settings.DefaultChannelID == "" {
			responder.Respond(i, "You have no anime follows and no default channel set.", true)
			return
		}
		responder.Respond(i, fmt.Sprintf("You have no anime follows. Default channel: <#%s>.", settings.DefaultChannelID), true)
		return
	}

	var b strings.Builder
	if settings.DefaultChannelID != "" {
		fmt.Fprintf(&b, "Default channel: <#%s>\n", settings.DefaultChannelID)
	} else {
		b.WriteString("Default channel: not set\n")
	}
	for _, entry := range entries {
		fmt.Fprintf(&b, "%s - keywords: %s", entry.Name, strings.Join(entry.Keywords, ", "))
		if entry.ChannelID != "" {
			fmt.Fprintf(&b, " - override: <#%s>", entry.ChannelID)
		} else if settings.DefaultChannelID != "" {
			fmt.Fprintf(&b, " - channel: default <#%s>", settings.DefaultChannelID)
		}
		if entry.LatestTitle != "" {
			fmt.Fprintf(&b, " - latest: %s", entry.LatestTitle)
			if entry.LatestLink != "" {
				fmt.Fprintf(&b, " (%s)", entry.LatestLink)
			}
		}
		b.WriteString("\n")
	}

	responder.Respond(i, strings.TrimSpace(b.String()), true)
}

func (m *AnimeModule) handleUnfollow(responder Responder, i *discordgo.InteractionCreate) {
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user.", true)
		return
	}

	var name string
	for _, opt := range i.ApplicationCommandData().Options[0].Options {
		if opt.Name == "name" {
			name = opt.StringValue()
		}
	}

	ok, err := m.service.Unfollow(context.Background(), userID, name)
	if err != nil {
		log.Printf("unfollow anime error: %v", err)
		responder.Respond(i, "Failed to delete anime follow.", true)
		return
	}
	if !ok {
		responder.Respond(i, "Follow entry not found.", true)
		return
	}

	responder.Respond(i, fmt.Sprintf("Deleted follow `%s`.", name), true)
}

func channelIDFromOption(opt *discordgo.ApplicationCommandInteractionDataOption) string {
	if opt == nil || opt.Value == nil {
		return ""
	}
	v, ok := opt.Value.(string)
	if !ok {
		return ""
	}
	return v
}
