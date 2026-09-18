package commands

import (
	"context"
	"fmt"
	"log"
	"strings"

	"mizubot-go/internal/guildinstructions"

	"github.com/bwmarrin/discordgo"
)

const (
	promptEmbedColor = 0x9B59B6

	// promptModalID is the modal's custom ID; the modal submit interaction
	// carries it back so Handle can tell our modal apart from any other.
	promptModalID = "edit-prompt-modal"
	// promptInputID identifies the text input row inside that modal.
	promptInputID = "edit-prompt-input"

	// Discord caps a modal text input at 4000 characters, so that is also the
	// longest prompt this command can round-trip.
	promptMaxLength = 4000
	// Embed descriptions are capped at 4096; leave room for the code fence.
	promptPreviewLimit = 3900
)

// GuildInstructionEditor is the subset of the guild instruction store the
// prompt command needs.
type GuildInstructionEditor interface {
	Get(ctx context.Context, guildID string) (guildinstructions.Instruction, bool, error)
	Upsert(ctx context.Context, guildID, instructions string) (guildinstructions.Instruction, error)
	Delete(ctx context.Context, guildID string) (bool, error)
}

type PromptModule struct {
	store GuildInstructionEditor
	// ownerDiscordID may always edit the prompt in any server, whatever their
	// permissions there. Empty means no such override.
	ownerDiscordID string
}

func NewPromptModule(store GuildInstructionEditor, ownerDiscordID string) *PromptModule {
	return &PromptModule{store: store, ownerDiscordID: strings.TrimSpace(ownerDiscordID)}
}

func (m *PromptModule) Definitions() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "edit-prompt",
			Description: "View or edit this server's custom system prompt",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "view",
					Description: "Show the system prompt currently used in this server",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "edit",
					Description: "Open an editor to change this server's system prompt",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "reset",
					Description: "Remove this server's custom system prompt",
				},
			},
		},
	}
}

func (m *PromptModule) Handle(responder Responder, _ *discordgo.Session, i *discordgo.InteractionCreate) bool {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		if i.ApplicationCommandData().Name != "edit-prompt" {
			return false
		}
	case discordgo.InteractionModalSubmit:
		if i.ModalSubmitData().CustomID != promptModalID {
			return false
		}
		m.handleModalSubmit(responder, i)
		return true
	default:
		return false
	}

	if i.GuildID == "" {
		responder.Respond(i, "This command only works inside a server.", true)
		return true
	}
	if !m.authorize(responder, i) {
		return true
	}

	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		responder.Respond(i, "Missing subcommand.", true)
		return true
	}

	switch options[0].Name {
	case "view":
		m.handleView(responder, i)
	case "edit":
		m.handleEdit(responder, i)
	case "reset":
		m.handleReset(responder, i)
	default:
		responder.Respond(i, "Unknown subcommand.", true)
	}
	return true
}

// authorize replies with the reason and returns false when the caller may not
// touch this server's prompt.
func (m *PromptModule) authorize(responder Responder, i *discordgo.InteractionCreate) bool {
	if m.canEditPrompt(i) {
		return true
	}
	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title:       "Not allowed",
		Color:       promptEmbedColor,
		Description: "Editing the server prompt needs one of: " + guildModPermissionNames() + ".",
	}, true)
	return false
}

func (m *PromptModule) canEditPrompt(i *discordgo.InteractionCreate) bool {
	// Discord computes Member.Permissions for the invoking channel on every
	// interaction, so this already accounts for role and channel overwrites.
	return canEditGuildConfig(i, m.ownerDiscordID)
}

func (m *PromptModule) handleView(responder Responder, i *discordgo.InteractionCreate) {
	instruction, ok, err := m.loadInstruction(i.GuildID)
	if err != nil {
		responder.Respond(i, "Failed to load the server prompt.", true)
		return
	}
	if !ok {
		responder.RespondEmbed(i, &discordgo.MessageEmbed{
			Title:       "Server Prompt",
			Color:       promptEmbedColor,
			Description: "No custom prompt is set for this server. Use `/edit-prompt edit` to add one.",
		}, true)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Server Prompt",
		Color:       promptEmbedColor,
		Description: promptCodeBlock(instruction.Instructions),
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("%d characters · updated %s", len([]rune(instruction.Instructions)), instruction.UpdatedAt.Format("2006-01-02 15:04 UTC")),
		},
	}
	responder.RespondEmbed(i, embed, true)
}

func (m *PromptModule) handleEdit(responder Responder, i *discordgo.InteractionCreate) {
	instruction, _, err := m.loadInstruction(i.GuildID)
	if err != nil {
		responder.Respond(i, "Failed to load the server prompt.", true)
		return
	}

	// A prompt seeded from config can be longer than a modal can hold. Opening
	// the modal anyway would truncate it and then save the truncation back.
	if len([]rune(instruction.Instructions)) > promptMaxLength {
		responder.Respond(i, fmt.Sprintf(
			"This server's prompt is %d characters, longer than the %d Discord allows in an editor. Use `/edit-prompt reset` first, or shorten it in the bot config.",
			len([]rune(instruction.Instructions)), promptMaxLength), true)
		return
	}

	err = responder.RespondModal(i, promptModalID, "Edit Server Prompt", []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    promptInputID,
					Label:       "System prompt for this server",
					Style:       discordgo.TextInputParagraph,
					Placeholder: "Extra instructions the bot should follow in this server.",
					Value:       instruction.Instructions,
					Required:    true,
					MaxLength:   promptMaxLength,
				},
			},
		},
	})
	if err != nil {
		log.Printf("edit-prompt modal open failed: guild_id=%s error=%v", i.GuildID, err)
		responder.Respond(i, "Failed to open the prompt editor.", true)
	}
}

func (m *PromptModule) handleModalSubmit(responder Responder, i *discordgo.InteractionCreate) {
	if i.GuildID == "" {
		responder.Respond(i, "This command only works inside a server.", true)
		return
	}
	// Re-check on submit: permissions can change between opening the modal and
	// submitting it, and a modal submit is a fresh interaction either way.
	if !m.authorize(responder, i) {
		return
	}
	if m.store == nil {
		responder.Respond(i, "Server prompts are not configured.", true)
		return
	}

	instructions := strings.TrimSpace(promptModalValue(i.ModalSubmitData()))
	if instructions == "" {
		responder.Respond(i, "The prompt cannot be empty. Use `/edit-prompt reset` to remove it instead.", true)
		return
	}

	saved, err := m.store.Upsert(context.Background(), i.GuildID, instructions)
	if err != nil {
		log.Printf("edit-prompt save failed: guild_id=%s error=%v", i.GuildID, err)
		responder.Respond(i, "Failed to save the server prompt.", true)
		return
	}

	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title:       "Server Prompt Updated",
		Color:       promptEmbedColor,
		Description: promptCodeBlock(saved.Instructions),
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("%d characters · applies to new messages", len([]rune(saved.Instructions))),
		},
	}, true)
}

func (m *PromptModule) handleReset(responder Responder, i *discordgo.InteractionCreate) {
	if m.store == nil {
		responder.Respond(i, "Server prompts are not configured.", true)
		return
	}
	removed, err := m.store.Delete(context.Background(), i.GuildID)
	if err != nil {
		log.Printf("edit-prompt reset failed: guild_id=%s error=%v", i.GuildID, err)
		responder.Respond(i, "Failed to reset the server prompt.", true)
		return
	}
	description := "Removed this server's custom prompt. The bot's default personality applies again."
	if !removed {
		description = "This server had no custom prompt to remove."
	}
	responder.RespondEmbed(i, &discordgo.MessageEmbed{
		Title:       "Server Prompt Reset",
		Color:       promptEmbedColor,
		Description: description,
	}, true)
}

func (m *PromptModule) loadInstruction(guildID string) (guildinstructions.Instruction, bool, error) {
	if m.store == nil {
		return guildinstructions.Instruction{}, false, nil
	}
	return m.store.Get(context.Background(), guildID)
}

// promptModalValue pulls the prompt text out of the modal's nested
// ActionsRow > TextInput component tree.
func promptModalValue(data discordgo.ModalSubmitInteractionData) string {
	for _, row := range data.Components {
		actionsRow, ok := row.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, component := range actionsRow.Components {
			input, ok := component.(*discordgo.TextInput)
			if ok && input.CustomID == promptInputID {
				return input.Value
			}
		}
	}
	return ""
}

func promptCodeBlock(instructions string) string {
	runes := []rune(instructions)
	if len(runes) > promptPreviewLimit {
		instructions = string(runes[:promptPreviewLimit]) + "\n…(truncated)"
	}
	// Guard the fence: a prompt containing ``` would otherwise break the block.
	instructions = strings.ReplaceAll(instructions, "```", "'''")
	return "```\n" + instructions + "\n```"
}
