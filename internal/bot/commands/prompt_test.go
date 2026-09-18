package commands

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"mizubot-go/internal/guildinstructions"

	"github.com/bwmarrin/discordgo"
)

type stubStore struct {
	instruction guildinstructions.Instruction
	exists      bool
	deleted     bool
	getErr      error
	upsertErr   error
	deleteErr   error

	upsertedGuild string
	upsertedText  string
	deletedGuild  string
}

func (s *stubStore) Get(_ context.Context, guildID string) (guildinstructions.Instruction, bool, error) {
	if s.getErr != nil {
		return guildinstructions.Instruction{}, false, s.getErr
	}
	if !s.exists || s.instruction.GuildID != guildID {
		return guildinstructions.Instruction{}, false, nil
	}
	return s.instruction, true, nil
}

func (s *stubStore) Upsert(_ context.Context, guildID, instructions string) (guildinstructions.Instruction, error) {
	if s.upsertErr != nil {
		return guildinstructions.Instruction{}, s.upsertErr
	}
	s.upsertedGuild = guildID
	s.upsertedText = instructions
	s.instruction = guildinstructions.Instruction{GuildID: guildID, Instructions: instructions, UpdatedAt: time.Unix(0, 0).UTC()}
	s.exists = true
	return s.instruction, nil
}

func (s *stubStore) Delete(_ context.Context, guildID string) (bool, error) {
	if s.deleteErr != nil {
		return false, s.deleteErr
	}
	s.deletedGuild = guildID
	existed := s.exists
	s.exists = false
	s.deleted = true
	return existed, nil
}

type stubResponder struct {
	content     string
	embed       *discordgo.MessageEmbed
	components  []discordgo.MessageComponent
	ephemeral   bool
	modalID     string
	modalTitle  string
	modalRows   []discordgo.MessageComponent
	modalErr    error
	updateErr   error
	updated     bool
	respondents int
}

func (r *stubResponder) Respond(_ *discordgo.InteractionCreate, content string, ephemeral bool) {
	r.content = content
	r.ephemeral = ephemeral
	r.respondents++
}

func (r *stubResponder) RespondEmbed(_ *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, ephemeral bool) {
	r.embed = embed
	r.ephemeral = ephemeral
	r.respondents++
}

func (r *stubResponder) RespondEmbedWithComponents(_ *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, ephemeral bool) {
	r.embed = embed
	r.components = components
	r.ephemeral = ephemeral
	r.respondents++
}

func (r *stubResponder) RespondModal(_ *discordgo.InteractionCreate, customID, title string, components []discordgo.MessageComponent) error {
	if r.modalErr != nil {
		return r.modalErr
	}
	r.modalID = customID
	r.modalTitle = title
	r.modalRows = components
	r.respondents++
	return nil
}

func (r *stubResponder) UpdateMessage(_ *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.embed = embed
	r.components = components
	r.updated = true
	r.respondents++
	return nil
}

// commandInteraction builds an /edit-prompt <sub> invocation from a member
// with the given computed permissions.
func commandInteraction(sub, userID string, permissions int64) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type:    discordgo.InteractionApplicationCommand,
		GuildID: "guild-1",
		Member: &discordgo.Member{
			User:        &discordgo.User{ID: userID},
			Permissions: permissions,
		},
		Data: discordgo.ApplicationCommandInteractionData{
			Name:    "edit-prompt",
			Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: sub}},
		},
	}}
}

func modalInteraction(value, userID string, permissions int64) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type:    discordgo.InteractionModalSubmit,
		GuildID: "guild-1",
		Member: &discordgo.Member{
			User:        &discordgo.User{ID: userID},
			Permissions: permissions,
		},
		Data: discordgo.ModalSubmitInteractionData{
			CustomID: promptModalID,
			Components: []discordgo.MessageComponent{
				&discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					&discordgo.TextInput{CustomID: promptInputID, Value: value},
				}},
			},
		},
	}}
}

func TestPromptDefinitions(t *testing.T) {
	defs := NewPromptModule(&stubStore{}, "").Definitions()
	if len(defs) != 1 || defs[0].Name != "edit-prompt" {
		t.Fatalf("definitions = %+v, want a single edit-prompt command", defs)
	}
	var names []string
	for _, opt := range defs[0].Options {
		if opt.Type != discordgo.ApplicationCommandOptionSubCommand {
			t.Fatalf("option %q type = %v, want subcommand", opt.Name, opt.Type)
		}
		names = append(names, opt.Name)
	}
	if got := strings.Join(names, ","); got != "view,edit,reset" {
		t.Fatalf("subcommands = %q, want view,edit,reset", got)
	}
}

func TestPromptHandleIgnoresOtherInteractions(t *testing.T) {
	m := NewPromptModule(&stubStore{}, "")
	responder := &stubResponder{}

	other := commandInteraction("view", "user-1", discordgo.PermissionAdministrator)
	other.Data = discordgo.ApplicationCommandInteractionData{Name: "remind"}
	if m.Handle(responder, nil, other) {
		t.Fatalf("Handle claimed a /remind interaction")
	}

	otherModal := modalInteraction("x", "user-1", discordgo.PermissionAdministrator)
	otherModal.Data = discordgo.ModalSubmitInteractionData{CustomID: "some-other-modal"}
	if m.Handle(responder, nil, otherModal) {
		t.Fatalf("Handle claimed an unrelated modal submit")
	}

	if responder.respondents != 0 {
		t.Fatalf("responder used %d times for unclaimed interactions", responder.respondents)
	}
}

func TestPromptAuthorization(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		permissions int64
		owner       string
		wantAllowed bool
	}{
		{name: "configured owner without permissions", userID: "owner", permissions: 0, owner: "owner", wantAllowed: true},
		{name: "configured owner is trimmed", userID: "owner", permissions: 0, owner: "  owner  ", wantAllowed: true},
		{name: "server administrator", userID: "user-1", permissions: discordgo.PermissionAdministrator, wantAllowed: true},
		{name: "manage server", userID: "user-1", permissions: discordgo.PermissionManageServer, wantAllowed: true},
		{name: "moderate members", userID: "user-1", permissions: discordgo.PermissionModerateMembers, wantAllowed: true},
		{name: "manage messages", userID: "user-1", permissions: discordgo.PermissionManageMessages, wantAllowed: true},
		{name: "kick members", userID: "user-1", permissions: discordgo.PermissionKickMembers, wantAllowed: true},
		{name: "ban members", userID: "user-1", permissions: discordgo.PermissionBanMembers, wantAllowed: true},
		{name: "plain member", userID: "user-1", permissions: discordgo.PermissionSendMessages, wantAllowed: false},
		{name: "no permissions at all", userID: "user-1", permissions: 0, wantAllowed: false},
		{name: "other user is not the configured owner", userID: "user-1", permissions: 0, owner: "owner", wantAllowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubStore{}
			m := NewPromptModule(store, tt.owner)
			responder := &stubResponder{}

			if !m.Handle(responder, nil, commandInteraction("reset", tt.userID, tt.permissions)) {
				t.Fatalf("Handle did not claim the interaction")
			}
			if got := store.deleted; got != tt.wantAllowed {
				t.Fatalf("store reached = %v, want %v", got, tt.wantAllowed)
			}
			if !tt.wantAllowed {
				if responder.embed == nil || responder.embed.Title != "Not allowed" {
					t.Fatalf("embed = %+v, want a Not allowed denial", responder.embed)
				}
				if !strings.Contains(responder.embed.Description, "Manage Server") {
					t.Fatalf("denial %q does not name the accepted permissions", responder.embed.Description)
				}
			}
			if !responder.ephemeral {
				t.Fatalf("response was not ephemeral")
			}
		})
	}
}

func TestPromptRequiresGuild(t *testing.T) {
	m := NewPromptModule(&stubStore{}, "owner")
	responder := &stubResponder{}

	i := commandInteraction("view", "owner", 0)
	i.GuildID = ""
	i.Member = nil
	i.User = &discordgo.User{ID: "owner"}

	if !m.Handle(responder, nil, i) {
		t.Fatalf("Handle did not claim the interaction")
	}
	if !strings.Contains(responder.content, "inside a server") {
		t.Fatalf("content = %q, want a server-only message", responder.content)
	}
}

func TestPromptViewWithoutInstruction(t *testing.T) {
	m := NewPromptModule(&stubStore{}, "")
	responder := &stubResponder{}

	m.Handle(responder, nil, commandInteraction("view", "user-1", discordgo.PermissionAdministrator))

	if responder.embed == nil || !strings.Contains(responder.embed.Description, "No custom prompt") {
		t.Fatalf("embed = %+v, want an empty-prompt notice", responder.embed)
	}
}

func TestPromptViewShowsInstruction(t *testing.T) {
	store := &stubStore{
		exists: true,
		instruction: guildinstructions.Instruction{
			GuildID:      "guild-1",
			Instructions: "Be extra polite here.",
			UpdatedAt:    time.Date(2026, 3, 4, 5, 6, 0, 0, time.UTC),
		},
	}
	m := NewPromptModule(store, "")
	responder := &stubResponder{}

	m.Handle(responder, nil, commandInteraction("view", "user-1", discordgo.PermissionManageServer))

	if responder.embed == nil {
		t.Fatalf("no embed returned")
	}
	if !strings.Contains(responder.embed.Description, "Be extra polite here.") {
		t.Fatalf("description = %q, want the stored prompt", responder.embed.Description)
	}
	if !strings.Contains(responder.embed.Footer.Text, "2026-03-04 05:06 UTC") {
		t.Fatalf("footer = %q, want the update time", responder.embed.Footer.Text)
	}
	if !responder.ephemeral {
		t.Fatalf("view response was not ephemeral")
	}
}

func TestPromptEditOpensPrefilledModal(t *testing.T) {
	store := &stubStore{
		exists:      true,
		instruction: guildinstructions.Instruction{GuildID: "guild-1", Instructions: "Existing prompt"},
	}
	m := NewPromptModule(store, "")
	responder := &stubResponder{}

	m.Handle(responder, nil, commandInteraction("edit", "user-1", discordgo.PermissionAdministrator))

	if responder.modalID != promptModalID {
		t.Fatalf("modal id = %q, want %q", responder.modalID, promptModalID)
	}
	if len(responder.modalRows) != 1 {
		t.Fatalf("modal rows = %d, want 1", len(responder.modalRows))
	}
	row, ok := responder.modalRows[0].(discordgo.ActionsRow)
	if !ok {
		t.Fatalf("modal row type = %T, want ActionsRow", responder.modalRows[0])
	}
	input, ok := row.Components[0].(discordgo.TextInput)
	if !ok {
		t.Fatalf("component type = %T, want TextInput", row.Components[0])
	}
	if input.Value != "Existing prompt" {
		t.Fatalf("input value = %q, want the stored prompt", input.Value)
	}
	if input.Style != discordgo.TextInputParagraph {
		t.Fatalf("input style = %v, want paragraph", input.Style)
	}
	if input.MaxLength != promptMaxLength {
		t.Fatalf("input max length = %d, want %d", input.MaxLength, promptMaxLength)
	}
}

func TestPromptEditRefusesOversizedPrompt(t *testing.T) {
	store := &stubStore{
		exists:      true,
		instruction: guildinstructions.Instruction{GuildID: "guild-1", Instructions: strings.Repeat("a", promptMaxLength+1)},
	}
	m := NewPromptModule(store, "")
	responder := &stubResponder{}

	m.Handle(responder, nil, commandInteraction("edit", "user-1", discordgo.PermissionAdministrator))

	if responder.modalID != "" {
		t.Fatalf("a modal was opened for an oversized prompt")
	}
	if !strings.Contains(responder.content, "longer than the 4000") {
		t.Fatalf("content = %q, want an over-length explanation", responder.content)
	}
}

func TestPromptEditReportsModalFailure(t *testing.T) {
	m := NewPromptModule(&stubStore{}, "")
	responder := &stubResponder{modalErr: errors.New("discord down")}

	m.Handle(responder, nil, commandInteraction("edit", "user-1", discordgo.PermissionAdministrator))

	if !strings.Contains(responder.content, "Failed to open") {
		t.Fatalf("content = %q, want a modal failure message", responder.content)
	}
}

func TestPromptModalSubmitSaves(t *testing.T) {
	store := &stubStore{}
	m := NewPromptModule(store, "")
	responder := &stubResponder{}

	if !m.Handle(responder, nil, modalInteraction("  Speak only in haiku.  ", "user-1", discordgo.PermissionAdministrator)) {
		t.Fatalf("Handle did not claim the modal submit")
	}
	if store.upsertedGuild != "guild-1" {
		t.Fatalf("upserted guild = %q, want guild-1", store.upsertedGuild)
	}
	if store.upsertedText != "Speak only in haiku." {
		t.Fatalf("upserted text = %q, want the trimmed prompt", store.upsertedText)
	}
	if responder.embed == nil || responder.embed.Title != "Server Prompt Updated" {
		t.Fatalf("embed = %+v, want an update confirmation", responder.embed)
	}
}

func TestPromptModalSubmitRejectsEmpty(t *testing.T) {
	store := &stubStore{}
	m := NewPromptModule(store, "")
	responder := &stubResponder{}

	m.Handle(responder, nil, modalInteraction("   ", "user-1", discordgo.PermissionAdministrator))

	if store.upsertedText != "" {
		t.Fatalf("empty prompt was saved as %q", store.upsertedText)
	}
	if !strings.Contains(responder.content, "cannot be empty") {
		t.Fatalf("content = %q, want an empty-prompt rejection", responder.content)
	}
}

func TestPromptModalSubmitChecksPermissions(t *testing.T) {
	store := &stubStore{}
	m := NewPromptModule(store, "owner")
	responder := &stubResponder{}

	// Someone who could open the modal but lost their role before submitting.
	m.Handle(responder, nil, modalInteraction("sneaky prompt", "user-1", discordgo.PermissionSendMessages))

	if store.upsertedText != "" {
		t.Fatalf("unauthorized submit saved %q", store.upsertedText)
	}
	if responder.embed == nil || responder.embed.Title != "Not allowed" {
		t.Fatalf("embed = %+v, want a denial", responder.embed)
	}
}

func TestPromptModalSubmitReportsSaveFailure(t *testing.T) {
	m := NewPromptModule(&stubStore{upsertErr: errors.New("db gone")}, "")
	responder := &stubResponder{}

	m.Handle(responder, nil, modalInteraction("new prompt", "user-1", discordgo.PermissionAdministrator))

	if !strings.Contains(responder.content, "Failed to save") {
		t.Fatalf("content = %q, want a save failure message", responder.content)
	}
}

func TestPromptReset(t *testing.T) {
	tests := []struct {
		name   string
		exists bool
		want   string
	}{
		{name: "removes existing prompt", exists: true, want: "Removed this server's custom prompt"},
		{name: "reports nothing to remove", exists: false, want: "no custom prompt to remove"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubStore{exists: tt.exists}
			m := NewPromptModule(store, "")
			responder := &stubResponder{}

			m.Handle(responder, nil, commandInteraction("reset", "user-1", discordgo.PermissionAdministrator))

			if store.deletedGuild != "guild-1" {
				t.Fatalf("deleted guild = %q, want guild-1", store.deletedGuild)
			}
			if responder.embed == nil || !strings.Contains(responder.embed.Description, tt.want) {
				t.Fatalf("embed = %+v, want %q", responder.embed, tt.want)
			}
		})
	}
}

func TestPromptUnknownSubcommand(t *testing.T) {
	m := NewPromptModule(&stubStore{}, "")
	responder := &stubResponder{}

	m.Handle(responder, nil, commandInteraction("explode", "user-1", discordgo.PermissionAdministrator))

	if responder.content != "Unknown subcommand." {
		t.Fatalf("content = %q, want an unknown subcommand message", responder.content)
	}
}

func TestPromptCodeBlockEscapesAndTruncates(t *testing.T) {
	block := promptCodeBlock("before ``` after")
	if strings.Count(block, "```") != 2 {
		t.Fatalf("block %q must contain only its own two fences", block)
	}

	long := promptCodeBlock(strings.Repeat("x", promptPreviewLimit+50))
	if !strings.Contains(long, "(truncated)") {
		t.Fatalf("long prompt was not marked truncated")
	}
	if len([]rune(long)) > 4096 {
		t.Fatalf("block is %d runes, over Discord's 4096 embed description limit", len([]rune(long)))
	}
}
