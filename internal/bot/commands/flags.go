package commands

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"mizubot-go/internal/guildflags"

	"github.com/bwmarrin/discordgo"
)

const (
	flagsEmbedColor = 0x3498DB

	flagsCommandName = "flags"

	flagsSelectID         = "flags-select"
	flagsAddButtonID      = "flags-add"
	flagsBackButtonID     = "flags-back"
	flagsEditPrefix       = "flags-edit:"
	flagsDeletePrefix     = "flags-delete:"
	flagsConfirmPrefix    = "flags-delete-confirm:"
	flagsCancelDeletePref = "flags-cancel-delete:"
	flagsAddModalID       = "flags-add-modal"
	flagsEditModalPref    = "flags-edit-modal:"
	flagsNameInputID      = "flags-name-input"
	flagsDescInputID      = "flags-desc-input"
	flagsGuideInputID     = "flags-guidance-input"
	flagsNameMaxLength    = 100
	flagsFieldMaxLength   = 1000
)

// GuildFlagEditor is the subset of the guild flag store the flags command
// needs.
type GuildFlagEditor interface {
	Create(ctx context.Context, guildID, name, description, guidance string) (guildflags.Flag, error)
	ListByGuild(ctx context.Context, guildID string) ([]guildflags.Flag, error)
	Update(ctx context.Context, id int64, guildID, description, guidance string) (guildflags.Flag, error)
	Delete(ctx context.Context, id int64, guildID string) (bool, error)
}

type FlagsModule struct {
	store GuildFlagEditor
	// ownerDiscordID may always edit flags in any server, whatever their
	// permissions there. Empty means no such override.
	ownerDiscordID string
}

func NewFlagsModule(store GuildFlagEditor, ownerDiscordID string) *FlagsModule {
	return &FlagsModule{store: store, ownerDiscordID: strings.TrimSpace(ownerDiscordID)}
}

func (m *FlagsModule) Definitions() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        flagsCommandName,
			Description: "View or edit this server's content flags",
		},
	}
}

func (m *FlagsModule) Handle(responder Responder, _ *discordgo.Session, i *discordgo.InteractionCreate) bool {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		if i.ApplicationCommandData().Name != flagsCommandName {
			return false
		}
		if !m.authorized(responder, i, false) {
			return true
		}
		m.showList(responder, i, false)
		return true

	case discordgo.InteractionMessageComponent:
		customID := i.MessageComponentData().CustomID
		if !strings.HasPrefix(customID, "flags-") {
			return false
		}
		if !m.authorized(responder, i, true) {
			return true
		}
		m.handleComponent(responder, i, customID)
		return true

	case discordgo.InteractionModalSubmit:
		customID := i.ModalSubmitData().CustomID
		if customID != flagsAddModalID && !strings.HasPrefix(customID, flagsEditModalPref) {
			return false
		}
		if !m.authorized(responder, i, false) {
			return true
		}
		m.handleModalSubmit(responder, i, customID)
		return true

	default:
		return false
	}
}

// authorized replies with the reason and returns false when the caller may
// not manage this server's flags. Permissions can change between opening a
// view and interacting with it, so every entry point re-checks. update
// selects UpdateMessage (editing the panel a button/select is attached to)
// vs a fresh reply (slash command or modal submit).
func (m *FlagsModule) authorized(responder Responder, i *discordgo.InteractionCreate, update bool) bool {
	if i.GuildID == "" {
		responder.Respond(i, "This command only works inside a server.", true)
		return false
	}
	if !canEditGuildConfig(i, m.ownerDiscordID) {
		respondNotAllowed(responder, i, update)
		return false
	}
	return true
}

func (m *FlagsModule) showList(responder Responder, i *discordgo.InteractionCreate, update bool) {
	flags, err := m.store.ListByGuild(context.Background(), i.GuildID)
	if err != nil {
		log.Printf("flags list failed: guild_id=%s error=%v", i.GuildID, err)
		respondOrUpdate(responder, i, update, notAllowedEmbed("Failed to load this server's flags."), nil)
		return
	}

	embed, components := buildListView(flags)
	respondOrUpdate(responder, i, update, embed, components)
}

func buildListView(flags []guildflags.Flag) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	description := "No content flags configured yet. Use **+ Add Flag** to create one."
	if len(flags) > 0 {
		var b strings.Builder
		for _, flag := range flags {
			fmt.Fprintf(&b, "**%s**\n%s\n\n", flag.Name, truncate(flag.Description, 150))
		}
		description = strings.TrimSpace(b.String())
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Content Flags",
		Color:       flagsEmbedColor,
		Description: description,
	}

	components := []discordgo.MessageComponent{}
	if len(flags) > 0 {
		options := make([]discordgo.SelectMenuOption, 0, len(flags))
		for _, flag := range flags {
			options = append(options, discordgo.SelectMenuOption{
				Label:       truncate(flag.Name, 100),
				Value:       strconv.FormatInt(flag.ID, 10),
				Description: truncate(flag.Description, 100),
			})
		}
		components = append(components, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    flagsSelectID,
					Placeholder: "View a flag...",
					Options:     options,
				},
			},
		})
	}
	components = append(components, discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "+ Add Flag",
				Style:    discordgo.SuccessButton,
				CustomID: flagsAddButtonID,
			},
		},
	})
	return embed, components
}

func (m *FlagsModule) handleComponent(responder Responder, i *discordgo.InteractionCreate, customID string) {
	switch {
	case customID == flagsSelectID:
		values := i.MessageComponentData().Values
		if len(values) == 0 {
			m.showList(responder, i, true)
			return
		}
		id, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil {
			m.showList(responder, i, true)
			return
		}
		m.showDetail(responder, i, id)

	case customID == flagsAddButtonID:
		m.openAddModal(responder, i)

	case customID == flagsBackButtonID:
		m.showList(responder, i, true)

	case strings.HasPrefix(customID, flagsEditPrefix):
		id, err := strconv.ParseInt(strings.TrimPrefix(customID, flagsEditPrefix), 10, 64)
		if err != nil {
			m.showList(responder, i, true)
			return
		}
		m.openEditModal(responder, i, id)

	case strings.HasPrefix(customID, flagsDeletePrefix):
		id, err := strconv.ParseInt(strings.TrimPrefix(customID, flagsDeletePrefix), 10, 64)
		if err != nil {
			m.showList(responder, i, true)
			return
		}
		m.showDeleteConfirm(responder, i, id)

	case strings.HasPrefix(customID, flagsConfirmPrefix):
		id, err := strconv.ParseInt(strings.TrimPrefix(customID, flagsConfirmPrefix), 10, 64)
		if err != nil {
			m.showList(responder, i, true)
			return
		}
		m.deleteFlag(responder, i, id)

	case strings.HasPrefix(customID, flagsCancelDeletePref):
		id, err := strconv.ParseInt(strings.TrimPrefix(customID, flagsCancelDeletePref), 10, 64)
		if err != nil {
			m.showList(responder, i, true)
			return
		}
		m.showDetail(responder, i, id)

	default:
		m.showList(responder, i, true)
	}
}

func (m *FlagsModule) findFlag(guildID string, id int64) (guildflags.Flag, bool, error) {
	flags, err := m.store.ListByGuild(context.Background(), guildID)
	if err != nil {
		return guildflags.Flag{}, false, err
	}
	for _, flag := range flags {
		if flag.ID == id {
			return flag, true, nil
		}
	}
	return guildflags.Flag{}, false, nil
}

func (m *FlagsModule) showDetail(responder Responder, i *discordgo.InteractionCreate, id int64) {
	flag, ok, err := m.findFlag(i.GuildID, id)
	if err != nil {
		log.Printf("flags detail load failed: guild_id=%s id=%d error=%v", i.GuildID, id, err)
		_ = responder.UpdateMessage(i, notAllowedEmbed("Failed to load that flag."), backOnlyComponents())
		return
	}
	if !ok {
		m.showList(responder, i, true)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: flag.Name,
		Color: flagsEmbedColor,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Trigger description", Value: truncate(flag.Description, 1024)},
			{Name: "Guidance when triggered", Value: truncate(flag.Guidance, 1024)},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Updated " + flag.UpdatedAt.Format("2006-01-02 15:04 UTC"),
		},
	}
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Edit", Style: discordgo.PrimaryButton, CustomID: flagsEditPrefix + strconv.FormatInt(id, 10)},
				discordgo.Button{Label: "Delete", Style: discordgo.DangerButton, CustomID: flagsDeletePrefix + strconv.FormatInt(id, 10)},
				discordgo.Button{Label: "Back", Style: discordgo.SecondaryButton, CustomID: flagsBackButtonID},
			},
		},
	}
	_ = responder.UpdateMessage(i, embed, components)
}

func (m *FlagsModule) showDeleteConfirm(responder Responder, i *discordgo.InteractionCreate, id int64) {
	flag, ok, err := m.findFlag(i.GuildID, id)
	if err != nil || !ok {
		m.showList(responder, i, true)
		return
	}
	embed := &discordgo.MessageEmbed{
		Title:       "Delete flag?",
		Color:       flagsEmbedColor,
		Description: fmt.Sprintf("Delete **%s**? This cannot be undone.", flag.Name),
	}
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Yes, delete", Style: discordgo.DangerButton, CustomID: flagsConfirmPrefix + strconv.FormatInt(id, 10)},
				discordgo.Button{Label: "Cancel", Style: discordgo.SecondaryButton, CustomID: flagsCancelDeletePref + strconv.FormatInt(id, 10)},
			},
		},
	}
	_ = responder.UpdateMessage(i, embed, components)
}

func (m *FlagsModule) deleteFlag(responder Responder, i *discordgo.InteractionCreate, id int64) {
	_, err := m.store.Delete(context.Background(), id, i.GuildID)
	if err != nil {
		log.Printf("flags delete failed: guild_id=%s id=%d error=%v", i.GuildID, id, err)
	}
	m.showList(responder, i, true)
}

func (m *FlagsModule) openAddModal(responder Responder, i *discordgo.InteractionCreate) {
	err := responder.RespondModal(i, flagsAddModalID, "Add Content Flag", []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.TextInput{
				CustomID:  flagsNameInputID,
				Label:     "Flag name",
				Style:     discordgo.TextInputShort,
				Required:  true,
				MaxLength: flagsNameMaxLength,
			},
		}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.TextInput{
				CustomID:    flagsDescInputID,
				Label:       "Trigger description",
				Style:       discordgo.TextInputParagraph,
				Placeholder: "When should this flag apply to a message?",
				Required:    true,
				MaxLength:   flagsFieldMaxLength,
			},
		}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.TextInput{
				CustomID:    flagsGuideInputID,
				Label:       "Guidance when triggered",
				Style:       discordgo.TextInputParagraph,
				Placeholder: "How should the bot respond when this flag fires?",
				Required:    true,
				MaxLength:   flagsFieldMaxLength,
			},
		}},
	})
	if err != nil {
		log.Printf("flags add modal open failed: guild_id=%s error=%v", i.GuildID, err)
		responder.Respond(i, "Failed to open the add-flag form.", true)
	}
}

func (m *FlagsModule) openEditModal(responder Responder, i *discordgo.InteractionCreate, id int64) {
	flag, ok, err := m.findFlag(i.GuildID, id)
	if err != nil {
		log.Printf("flags edit modal load failed: guild_id=%s id=%d error=%v", i.GuildID, id, err)
		responder.Respond(i, "Failed to load that flag.", true)
		return
	}
	if !ok {
		responder.Respond(i, "That flag no longer exists.", true)
		return
	}

	err = responder.RespondModal(i, flagsEditModalPref+strconv.FormatInt(id, 10), "Edit "+truncate(flag.Name, 40), []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.TextInput{
				CustomID:  flagsDescInputID,
				Label:     "Trigger description",
				Style:     discordgo.TextInputParagraph,
				Value:     flag.Description,
				Required:  true,
				MaxLength: flagsFieldMaxLength,
			},
		}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.TextInput{
				CustomID:  flagsGuideInputID,
				Label:     "Guidance when triggered",
				Style:     discordgo.TextInputParagraph,
				Value:     flag.Guidance,
				Required:  true,
				MaxLength: flagsFieldMaxLength,
			},
		}},
	})
	if err != nil {
		log.Printf("flags edit modal open failed: guild_id=%s id=%d error=%v", i.GuildID, id, err)
		responder.Respond(i, "Failed to open the edit form.", true)
	}
}

func (m *FlagsModule) handleModalSubmit(responder Responder, i *discordgo.InteractionCreate, customID string) {
	data := i.ModalSubmitData()
	description := strings.TrimSpace(modalInputValue(data, flagsDescInputID))
	guidance := strings.TrimSpace(modalInputValue(data, flagsGuideInputID))

	if customID == flagsAddModalID {
		name := strings.TrimSpace(modalInputValue(data, flagsNameInputID))
		flag, err := m.store.Create(context.Background(), i.GuildID, name, description, guidance)
		if err != nil {
			log.Printf("flags add save failed: guild_id=%s error=%v", i.GuildID, err)
			responder.Respond(i, "Failed to save the flag: "+err.Error(), true)
			return
		}
		responder.RespondEmbed(i, &discordgo.MessageEmbed{
			Title:       "Flag Added",
			Color:       flagsEmbedColor,
			Description: fmt.Sprintf("Added **%s**. Run `/%s` again to see the updated list.", flag.Name, flagsCommandName),
		}, true)
		return
	}

	id, err := strconv.ParseInt(strings.TrimPrefix(customID, flagsEditModalPref), 10, 64)
	if err != nil {
		responder.Respond(i, "Something went wrong identifying that flag.", true)
		return
	}
	flag, err := m.store.Update(context.Background(), id, i.GuildID, description, guidance)
	if err != nil {
		log.Printf("flags edit save failed: guild_id=%s id=%d error=%v", i.GuildID, id, err)
		responder.Respond(i, "Failed to save the flag: "+err.Error(), true)
		return
	}
	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title:       "Flag Updated",
		Color:       flagsEmbedColor,
		Description: fmt.Sprintf("Updated **%s**. Run `/%s` again to see the updated list.", flag.Name, flagsCommandName),
	}, true)
}

func respondNotAllowed(responder Responder, i *discordgo.InteractionCreate, update bool) {
	respondOrUpdate(responder, i, update, notAllowedEmbed("Editing content flags needs one of: "+guildModPermissionNames()+"."), backOnlyComponents())
}

func notAllowedEmbed(description string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "Not allowed",
		Color:       flagsEmbedColor,
		Description: description,
	}
}

func backOnlyComponents() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Back", Style: discordgo.SecondaryButton, CustomID: flagsBackButtonID},
		}},
	}
}

func respondOrUpdate(responder Responder, i *discordgo.InteractionCreate, update bool, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) {
	if update {
		_ = responder.UpdateMessage(i, embed, components)
		return
	}
	responder.RespondEmbedWithComponents(i, embed, components, true)
}

// modalInputValue pulls a named text input's value out of a modal submit's
// nested ActionsRow > TextInput component tree.
func modalInputValue(data discordgo.ModalSubmitInteractionData, customID string) string {
	for _, row := range data.Components {
		actionsRow, ok := row.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, component := range actionsRow.Components {
			input, ok := component.(*discordgo.TextInput)
			if ok && input.CustomID == customID {
				return input.Value
			}
		}
	}
	return ""
}

func truncate(s string, max int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= max {
		return string(runes)
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}
