package commands

import (
	"context"
	"errors"
	"testing"

	"mizubot-go/internal/guildflags"

	"github.com/bwmarrin/discordgo"
)

type stubFlagStore struct {
	flags     []guildflags.Flag
	nextID    int64
	listErr   error
	createErr error
	updateErr error
	deleteErr error

	createdGuild, createdName, createdDesc, createdGuidance string
	updatedID                                               int64
	deletedID                                               int64
}

func (s *stubFlagStore) Create(_ context.Context, guildID, name, description, guidance string) (guildflags.Flag, error) {
	if s.createErr != nil {
		return guildflags.Flag{}, s.createErr
	}
	s.nextID++
	s.createdGuild, s.createdName, s.createdDesc, s.createdGuidance = guildID, name, description, guidance
	flag := guildflags.Flag{ID: s.nextID, GuildID: guildID, Name: name, Description: description, Guidance: guidance}
	s.flags = append(s.flags, flag)
	return flag, nil
}

func (s *stubFlagStore) ListByGuild(_ context.Context, guildID string) ([]guildflags.Flag, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	var out []guildflags.Flag
	for _, flag := range s.flags {
		if flag.GuildID == guildID {
			out = append(out, flag)
		}
	}
	return out, nil
}

func (s *stubFlagStore) Update(_ context.Context, id int64, guildID, description, guidance string) (guildflags.Flag, error) {
	if s.updateErr != nil {
		return guildflags.Flag{}, s.updateErr
	}
	s.updatedID = id
	for idx, flag := range s.flags {
		if flag.ID == id && flag.GuildID == guildID {
			s.flags[idx].Description = description
			s.flags[idx].Guidance = guidance
			return s.flags[idx], nil
		}
	}
	return guildflags.Flag{}, errors.New("not found")
}

func (s *stubFlagStore) Delete(_ context.Context, id int64, guildID string) (bool, error) {
	if s.deleteErr != nil {
		return false, s.deleteErr
	}
	s.deletedID = id
	for idx, flag := range s.flags {
		if flag.ID == id && flag.GuildID == guildID {
			s.flags = append(s.flags[:idx], s.flags[idx+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func flagsCommandInteraction(userID string, permissions int64) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type:    discordgo.InteractionApplicationCommand,
		GuildID: "guild-1",
		Member: &discordgo.Member{
			User:        &discordgo.User{ID: userID},
			Permissions: permissions,
		},
		Data: discordgo.ApplicationCommandInteractionData{Name: flagsCommandName},
	}}
}

func flagsComponentInteraction(customID string, values []string, userID string, permissions int64) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type:    discordgo.InteractionMessageComponent,
		GuildID: "guild-1",
		Member: &discordgo.Member{
			User:        &discordgo.User{ID: userID},
			Permissions: permissions,
		},
		Data: discordgo.MessageComponentInteractionData{CustomID: customID, Values: values},
	}}
}

func flagsModalInteraction(customID string, values map[string]string, userID string, permissions int64) *discordgo.InteractionCreate {
	var components []discordgo.MessageComponent
	for id, value := range values {
		components = append(components, &discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			&discordgo.TextInput{CustomID: id, Value: value},
		}})
	}
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type:    discordgo.InteractionModalSubmit,
		GuildID: "guild-1",
		Member: &discordgo.Member{
			User:        &discordgo.User{ID: userID},
			Permissions: permissions,
		},
		Data: discordgo.ModalSubmitInteractionData{CustomID: customID, Components: components},
	}}
}

func TestFlagsDefinitions(t *testing.T) {
	defs := NewFlagsModule(&stubFlagStore{}, "").Definitions()
	if len(defs) != 1 || defs[0].Name != flagsCommandName {
		t.Fatalf("definitions = %+v, want a single flags command", defs)
	}
}

func TestFlagsHandleIgnoresOtherInteractions(t *testing.T) {
	m := NewFlagsModule(&stubFlagStore{}, "")
	responder := &stubResponder{}

	other := flagsCommandInteraction("user-1", discordgo.PermissionAdministrator)
	other.Data = discordgo.ApplicationCommandInteractionData{Name: "remind"}
	if m.Handle(responder, nil, other) {
		t.Fatalf("Handle claimed a /remind interaction")
	}
	if responder.respondents != 0 {
		t.Fatalf("responder should not have been used")
	}
}

func TestFlagsListShowsEmptyState(t *testing.T) {
	m := NewFlagsModule(&stubFlagStore{}, "")
	responder := &stubResponder{}

	i := flagsCommandInteraction("user-1", discordgo.PermissionAdministrator)
	if !m.Handle(responder, nil, i) {
		t.Fatalf("Handle did not claim /flags")
	}
	if responder.embed == nil || responder.embed.Title != "Content Flags" {
		t.Fatalf("embed = %+v, want Content Flags", responder.embed)
	}
	if !responder.ephemeral {
		t.Fatalf("list response should be ephemeral")
	}
}

func TestFlagsListDeniedWithoutPermission(t *testing.T) {
	m := NewFlagsModule(&stubFlagStore{}, "")
	responder := &stubResponder{}

	i := flagsCommandInteraction("user-1", 0)
	if !m.Handle(responder, nil, i) {
		t.Fatalf("Handle did not claim /flags")
	}
	if responder.embed == nil || responder.embed.Title != "Not allowed" {
		t.Fatalf("embed = %+v, want Not allowed", responder.embed)
	}
}

func TestFlagsAddModalCreatesFlag(t *testing.T) {
	store := &stubFlagStore{}
	m := NewFlagsModule(store, "")
	responder := &stubResponder{}

	submit := flagsModalInteraction(flagsAddModalID, map[string]string{
		flagsNameInputID:  "spoilers",
		flagsDescInputID:  "Message discusses spoilers",
		flagsGuideInputID: "Refuse to discuss spoilers",
	}, "user-1", discordgo.PermissionAdministrator)
	if !m.Handle(responder, nil, submit) {
		t.Fatalf("Handle did not claim the add-flag modal submit")
	}
	if store.createdName != "spoilers" || store.createdDesc != "Message discusses spoilers" || store.createdGuidance != "Refuse to discuss spoilers" {
		t.Fatalf("store not called with expected values: %+v", store)
	}
	if responder.embed == nil || responder.embed.Title != "Flag Added" {
		t.Fatalf("embed = %+v, want Flag Added", responder.embed)
	}
}

func TestFlagsSelectShowsDetailAndEditUpdates(t *testing.T) {
	store := &stubFlagStore{}
	store.flags = []guildflags.Flag{{ID: 1, GuildID: "guild-1", Name: "spoilers", Description: "desc", Guidance: "guide"}}
	m := NewFlagsModule(store, "")
	responder := &stubResponder{}

	sel := flagsComponentInteraction(flagsSelectID, []string{"1"}, "user-1", discordgo.PermissionAdministrator)
	if !m.Handle(responder, nil, sel) {
		t.Fatalf("Handle did not claim the select interaction")
	}
	if !responder.updated || responder.embed == nil || responder.embed.Title != "spoilers" {
		t.Fatalf("detail view = %+v, want spoilers detail via UpdateMessage", responder.embed)
	}

	edit := flagsModalInteraction("flags-edit-modal:1", map[string]string{
		flagsDescInputID:  "new desc",
		flagsGuideInputID: "new guide",
	}, "user-1", discordgo.PermissionAdministrator)
	responder = &stubResponder{}
	if !m.Handle(responder, nil, edit) {
		t.Fatalf("Handle did not claim the edit-flag modal submit")
	}
	if store.updatedID != 1 || store.flags[0].Description != "new desc" || store.flags[0].Guidance != "new guide" {
		t.Fatalf("update not applied: %+v", store.flags)
	}
	if responder.embed == nil || responder.embed.Title != "Flag Updated" {
		t.Fatalf("embed = %+v, want Flag Updated", responder.embed)
	}
}

func TestFlagsDeleteConfirmFlow(t *testing.T) {
	store := &stubFlagStore{}
	store.flags = []guildflags.Flag{{ID: 1, GuildID: "guild-1", Name: "spoilers", Description: "desc", Guidance: "guide"}}
	m := NewFlagsModule(store, "")

	confirm := flagsComponentInteraction("flags-delete-confirm:1", nil, "user-1", discordgo.PermissionAdministrator)
	responder := &stubResponder{}
	if !m.Handle(responder, nil, confirm) {
		t.Fatalf("Handle did not claim the delete-confirm interaction")
	}
	if store.deletedID != 1 {
		t.Fatalf("delete not called for id 1, got %d", store.deletedID)
	}
	if !responder.updated || responder.embed == nil || responder.embed.Title != "Content Flags" {
		t.Fatalf("expected to land back on the list view, got %+v", responder.embed)
	}
}

func TestFlagsOwnerCanEditWithoutPermissions(t *testing.T) {
	store := &stubFlagStore{}
	m := NewFlagsModule(store, "owner-1")
	responder := &stubResponder{}

	i := flagsCommandInteraction("owner-1", 0)
	if !m.Handle(responder, nil, i) {
		t.Fatalf("Handle did not claim /flags")
	}
	if responder.embed == nil || responder.embed.Title != "Content Flags" {
		t.Fatalf("owner override failed: embed = %+v", responder.embed)
	}
}
