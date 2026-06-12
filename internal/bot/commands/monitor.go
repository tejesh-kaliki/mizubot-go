package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"mizubot-go/internal/pagemonitor"

	"github.com/bwmarrin/discordgo"
)

type MonitorModule struct {
	service *pagemonitor.Service
}

func NewMonitorModule(service *pagemonitor.Service) *MonitorModule {
	return &MonitorModule{service: service}
}

func (m *MonitorModule) Definitions() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "monitor",
			Description: "Monitor a webpage for content changes",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "add",
					Description: "Add a URL to monitor",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "url",
							Description: "The URL to monitor (e.g. https://www.amazon.in/dp/...)",
							Required:    true,
						},
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "selector",
							Description: "CSS selector for the element to watch (e.g. #availability). Omit to auto-detect.",
							Required:    false,
						},
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "label",
							Description: "Friendly name for this monitor",
							Required:    false,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "list",
					Description: "List your active monitors",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "remove",
					Description: "Remove a monitor by ID",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionInteger,
							Name:        "id",
							Description: "Monitor ID (from /monitor list)",
							Required:    true,
						},
					},
				},
			},
		},
	}
}

func (m *MonitorModule) Handle(r Responder, _ *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if i.ApplicationCommandData().Name != "monitor" {
		return false
	}
	sub := i.ApplicationCommandData().Options[0]
	switch sub.Name {
	case "add":
		m.handleAdd(r, i, sub)
	case "list":
		m.handleList(r, i)
	case "remove":
		m.handleRemove(r, i, sub)
	}
	return true
}

func (m *MonitorModule) handleAdd(r Responder, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optionMap(sub.Options)
	rawURL := strings.TrimSpace(opts["url"].StringValue())

	selector := ""
	if o, ok := opts["selector"]; ok {
		selector = strings.TrimSpace(o.StringValue())
	}
	label := ""
	if o, ok := opts["label"]; ok {
		label = strings.TrimSpace(o.StringValue())
	}

	mon, err := m.service.AddMonitor(context.Background(), pagemonitor.AddMonitorInput{
		UserID:    i.Member.User.ID,
		ChannelID: i.ChannelID,
		GuildID:   i.GuildID,
		URL:       rawURL,
		Label:     label,
		Selector:  selector,
	})
	if err != nil {
		r.Respond(i, "Error: "+err.Error(), true)
		return
	}

	selectorNote := "auto-detect"
	if mon.Selector != "" {
		selectorNote = "`" + mon.Selector + "`"
	}
	r.Respond(i, fmt.Sprintf("Monitoring **%s** (ID: %d, selector: %s). I'll notify you here when the content changes.", mon.Label, mon.ID, selectorNote), true)
}

func (m *MonitorModule) handleList(r Responder, i *discordgo.InteractionCreate) {
	monitors, err := m.service.ListMonitors(context.Background(), i.Member.User.ID)
	if err != nil {
		r.Respond(i, "Failed to fetch monitors.", true)
		return
	}
	if len(monitors) == 0 {
		r.Respond(i, "You have no active monitors. Use `/monitor add` to start one.", true)
		return
	}
	var sb strings.Builder
	sb.WriteString("**Your monitors:**\n")
	for _, mon := range monitors {
		sel := "auto"
		if mon.Selector != "" {
			sel = mon.Selector
		}
		fmt.Fprintf(&sb, "`%d` **%s** `%s` — selector: `%s` — <%s>\n", mon.ID, mon.Label, mon.LastStatus, sel, mon.URL)
	}
	r.Respond(i, sb.String(), true)
}

func (m *MonitorModule) handleRemove(r Responder, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optionMap(sub.Options)
	id := opts["id"].IntValue()

	ok, err := m.service.RemoveMonitor(context.Background(), id, i.Member.User.ID)
	if err != nil {
		r.Respond(i, "Failed to remove monitor.", true)
		return
	}
	if !ok {
		r.Respond(i, "Monitor not found or doesn't belong to you.", true)
		return
	}
	r.Respond(i, "Monitor #"+strconv.FormatInt(id, 10)+" removed.", true)
}

func optionMap(opts []*discordgo.ApplicationCommandInteractionDataOption) map[string]*discordgo.ApplicationCommandInteractionDataOption {
	m := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opts))
	for _, o := range opts {
		m[o.Name] = o
	}
	return m
}
