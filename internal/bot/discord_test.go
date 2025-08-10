package bot

import (
	"errors"
	"testing"

	"github.com/bwmarrin/discordgo"
)

type stubRegistrar struct {
	lastApp   string
	lastGuild string
	called    bool
}

func (s *stubRegistrar) RegisterCommands(appID, guildID string, cmds []*discordgo.ApplicationCommand) error {
	s.lastApp = appID
	s.lastGuild = guildID
	s.called = true
	if appID == "" {
		return errors.New("empty appID")
	}
	return nil
}

func TestRegisterCommandsRequiresOpen(t *testing.T) {
	b := &Bot{session: &discordgo.Session{}, registrar: &stubRegistrar{}}

	if err := b.RegisterCommandsGlobal(); err == nil {
		t.Fatalf("expected error when session not ready")
	}

	// Simulate session open by populating State.User
	b.session.State = &discordgo.State{}
	b.session.State.User = &discordgo.User{ID: "app"}

	if err := b.RegisterCommandsGlobal(); err != nil {
		t.Fatalf("unexpected error after session ready: %v", err)
	}
}
