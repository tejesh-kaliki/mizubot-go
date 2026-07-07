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

func TestMessageTextWithDisplayNamesPreservesMentionsAsNames(t *testing.T) {
	state := discordgo.NewState()
	if err := state.GuildAdd(&discordgo.Guild{
		ID: "guild",
		Members: []*discordgo.Member{
			{User: &discordgo.User{ID: "bot", Username: "MizuBot"}, Nick: "Mizu"},
			{User: &discordgo.User{ID: "user", Username: "account-name"}, Nick: "Server Name"},
			{User: &discordgo.User{ID: "other-bot", Username: "helper"}, Nick: "Helper Bot"},
		},
	}); err != nil {
		t.Fatalf("GuildAdd: %v", err)
	}
	session := &discordgo.Session{State: state}
	message := &discordgo.Message{
		GuildID: "guild",
		Content: "<@bot> ask <@!user> and <@other-bot>",
		Mentions: []*discordgo.User{
			{ID: "bot", Username: "MizuBot", Bot: true},
			{ID: "user", Username: "account-name"},
			{ID: "other-bot", Username: "helper", Bot: true},
		},
	}

	got := messageTextWithDisplayNames(session, message)
	if got != "@Mizu ask @Server Name and @Helper Bot" {
		t.Fatalf("message text = %q", got)
	}
}

func TestGuildDisplayNameUsesMessageMemberNickname(t *testing.T) {
	user := &discordgo.User{ID: "user", Username: "account-name"}
	member := &discordgo.Member{User: user, Nick: "Server Name"}

	if got := guildDisplayName(nil, "guild", user, member); got != "Server Name" {
		t.Fatalf("display name = %q, want Server Name", got)
	}
}

func TestNewRequestsMessageContentIntent(t *testing.T) {
	b, err := New("Bot faketoken", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if b.session.Identify.Intents&discordgo.IntentMessageContent == 0 {
		t.Fatalf("session should request the message content intent, got intents=%d", b.session.Identify.Intents)
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
