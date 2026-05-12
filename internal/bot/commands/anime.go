package commands

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"mizubot-go/internal/animefeed"

	"github.com/bwmarrin/discordgo"
)

const animeEmbedColor = 0x2E8B57
const animeListPageSize = 5

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
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Optional per-follow override channel", Required: false},
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
					Name:        "feed",
					Description: "Show your generated anime feed URL",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "list",
					Description: "List your anime follows",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionInteger, Name: "page", Description: "Page number", Required: false},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "show",
					Description: "Show details for one follow entry",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "Follow entry name", Required: true},
					},
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
	case "feed":
		m.handleFeed(responder, i)
	case "list":
		m.handleList(responder, i)
	case "show":
		m.handleShow(responder, i)
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

	channelValue := "Default channel"
	if entry.ChannelID != "" {
		channelValue = "<#" + entry.ChannelID + ">"
	}

	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title: "Anime Follow Added",
		Color: animeEmbedColor,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Name", Value: entry.Name, Inline: true},
			{Name: "Channel", Value: channelValue, Inline: true},
			{Name: "Keywords", Value: strings.Join(entry.Keywords, ", "), Inline: false},
		},
	}, true)
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

	channelValue := "Default channel"
	if channelID != "" {
		channelValue = "<#" + channelID + ">"
	}

	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title: "Anime Follow Channel Updated",
		Color: animeEmbedColor,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Follow", Value: name, Inline: true},
			{Name: "Effective Channel", Value: channelValue, Inline: true},
		},
	}, true)
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

	channelValue := "Not set"
	if settings.DefaultChannelID != "" {
		channelValue = "<#" + settings.DefaultChannelID + ">"
	}

	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title: "Default Anime Channel Updated",
		Color: animeEmbedColor,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Default Channel", Value: channelValue, Inline: true},
		},
	}, true)
}

func (m *AnimeModule) handleFeed(responder Responder, i *discordgo.InteractionCreate) {
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user.", true)
		return
	}

	feedURL := m.service.FeedURL(userID)
	if feedURL == "" {
		responder.RespondEmbed(i, &discordgo.MessageEmbed{
			Title:       "Anime Feed URL",
			Color:       animeEmbedColor,
			Description: "Public feed URL is not configured yet.",
		}, true)
		return
	}

	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title:       "Anime Feed URL",
		Color:       animeEmbedColor,
		Description: "[Open feed](" + feedURL + ")",
		Footer:      &discordgo.MessageEmbedFooter{Text: "Use this in your RSS reader"},
	}, true)
}

func (m *AnimeModule) handleList(responder Responder, i *discordgo.InteractionCreate) {
	userID := userIDFromInteraction(i)
	if userID == "" {
		responder.Respond(i, "Unable to identify the user.", true)
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

	feedURL := m.service.FeedURL(userID)
	description := fmt.Sprintf("Default channel: %s", renderChannelOrFallback(settings.DefaultChannelID, "Not set"))
	if feedURL != "" {
		description += "\nFeed: [Open feed](" + feedURL + ")"
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Anime Follows",
		Color:       animeEmbedColor,
		Description: description,
	}

	if len(entries) == 0 {
		embed.Description += "\n\nNo follows configured."
		responder.RespondEmbed(i, embed, true)
		return
	}

	totalPages := (len(entries) + animeListPageSize - 1) / animeListPageSize
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * animeListPageSize
	end := start + animeListPageSize
	if end > len(entries) {
		end = len(entries)
	}

	fields := make([]*discordgo.MessageEmbedField, 0, end-start)
	for _, entry := range entries[start:end] {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   entry.Name,
			Value:  formatAnimeListEntryField(entry, settings.DefaultChannelID),
			Inline: false,
		})
	}
	embed.Fields = fields
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: fmt.Sprintf("Page %d/%d • Use /anime show name:<name> for full details", page, totalPages),
	}
	responder.RespondEmbed(i, embed, true)
}

func (m *AnimeModule) handleShow(responder Responder, i *discordgo.InteractionCreate) {
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

	entries, err := m.service.ListFollows(context.Background(), userID)
	if err != nil {
		log.Printf("show anime follow error: %v", err)
		responder.Respond(i, "Failed to load anime follow.", true)
		return
	}
	settings, err := m.service.GetSettings(context.Background(), userID)
	if err != nil {
		log.Printf("get anime settings error: %v", err)
		responder.Respond(i, "Failed to load anime settings.", true)
		return
	}

	var match *animefeed.Entry
	for idx := range entries {
		if strings.EqualFold(entries[idx].Name, strings.TrimSpace(name)) {
			match = &entries[idx]
			break
		}
	}
	if match == nil {
		responder.Respond(i, "Follow entry not found.", true)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       match.Name,
		Color:       animeEmbedColor,
		Description: formatAnimeShowEntry(match, settings.DefaultChannelID, m.service.FeedURL(userID)),
	}
	responder.RespondEmbed(i, embed, true)
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

	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title: "Anime Follow Removed",
		Color: animeEmbedColor,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Name", Value: name, Inline: true},
		},
	}, true)
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

func renderChannelOrFallback(channelID, fallback string) string {
	if channelID == "" {
		return fallback
	}
	return "<#" + channelID + ">"
}

func formatAnimeListEntryField(entry animefeed.Entry, defaultChannelID string) string {
	var b strings.Builder
	if entry.ChannelID != "" {
		fmt.Fprintf(&b, "Channel: %s (override)", renderChannelOrFallback(entry.ChannelID, "Not set"))
	} else {
		fmt.Fprintf(&b, "Channel: %s", renderChannelOrFallback(defaultChannelID, "Not set"))
	}
	if entry.LatestTitle != "" {
		b.WriteString("\n")
		if entry.LatestLink != "" {
			fmt.Fprintf(&b, "Latest: [%s](%s)", trimForField(entry.LatestTitle, 120), entry.LatestLink)
		} else {
			fmt.Fprintf(&b, "Latest: %s", trimForField(entry.LatestTitle, 120))
		}
	}
	if entry.LatestPublishedAt != nil {
		b.WriteString("\n")
		fmt.Fprintf(&b, "Published: %s", entry.LatestPublishedAt.UTC().Format(time.RFC3339))
	}
	return b.String()
}

func formatAnimeShowEntry(entry *animefeed.Entry, defaultChannelID, feedURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Keywords: `%s`", strings.Join(entry.Keywords, "`, `"))
	b.WriteString("\n")
	if entry.ChannelID != "" {
		fmt.Fprintf(&b, "Channel: %s (override)", renderChannelOrFallback(entry.ChannelID, "Not set"))
	} else {
		fmt.Fprintf(&b, "Channel: %s", renderChannelOrFallback(defaultChannelID, "Not set"))
	}
	if entry.LatestTitle != "" {
		b.WriteString("\n")
		if entry.LatestLink != "" {
			fmt.Fprintf(&b, "Latest: [%s](%s)", trimForField(entry.LatestTitle, 160), entry.LatestLink)
		} else {
			fmt.Fprintf(&b, "Latest: %s", trimForField(entry.LatestTitle, 160))
		}
	}
	if entry.LatestPublishedAt != nil {
		b.WriteString("\n")
		fmt.Fprintf(&b, "Published: %s", entry.LatestPublishedAt.UTC().Format(time.RFC3339))
	}
	if feedURL != "" {
		b.WriteString("\n")
		fmt.Fprintf(&b, "Feed: [Open feed](%s)", feedURL)
	}
	return b.String()
}

func trimForField(v string, limit int) string {
	v = strings.TrimSpace(v)
	if len(v) <= limit {
		return v
	}
	return v[:limit-3] + "..."
}
