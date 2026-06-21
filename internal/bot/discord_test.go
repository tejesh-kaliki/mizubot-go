package bot

import (
	"errors"
	"strings"
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

func TestMessageMentionsUser(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{content: "hello <@123>", want: true},
		{content: "hello <@!123>", want: true},
		{content: "hello 123", want: false},
	}

	for _, tt := range tests {
		if got := messageMentionsUser(tt.content, "123"); got != tt.want {
			t.Fatalf("messageMentionsUser(%q) = %v, want %v", tt.content, got, tt.want)
		}
	}
}

func TestSplitDiscordMessages(t *testing.T) {
	short := splitDiscordMessages(" hello ")
	if len(short) != 1 || short[0] != "hello" {
		t.Fatalf("short message = %#v, want [hello]", short)
	}

	long := strings.Repeat("x", 1990) + " " + strings.Repeat("y", 50)
	got := splitDiscordMessages(long)
	if len(got) != 2 {
		t.Fatalf("parts = %d, want 2", len(got))
	}
	if len([]rune(got[0])) > 2000 || len([]rune(got[1])) > 2000 {
		t.Fatalf("part too long: %d/%d", len([]rune(got[0])), len([]rune(got[1])))
	}
	if strings.HasSuffix(got[0], " ") || strings.HasPrefix(got[1], " ") {
		t.Fatalf("parts were not trimmed: %#v", got)
	}
	if got[1] != strings.Repeat("y", 50) {
		t.Fatalf("second part = %q, want y run", got[1])
	}
}
