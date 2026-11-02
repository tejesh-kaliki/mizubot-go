package bot

import (
	"reflect"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestRewriteTweetLinks(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		changed bool
	}{
		{
			in:      "https://x.com/thdxr/status/2116735229792522518?a=7xJcZuXh2-RPvE_2rXY&s=19",
			want:    "https://fixupx.com/thdxr/status/2116735229792522518/en",
			changed: true,
		},
		{
			in:      "look at this https://twitter.com/a_b/status/1/photo/1?s=20 lol",
			want:    "look at this https://fixupx.com/a_b/status/1/en lol",
			changed: true,
		},
		{in: "<https://x.com/a/status/1?s=1>", want: "<https://x.com/a/status/1?s=1>"},
		{in: "https://example.com/x.com/a/status/1", want: "https://example.com/x.com/a/status/1"},
		{in: "no links", want: "no links"},
	}
	for _, tt := range tests {
		got, changed := rewriteTweetLinks(tt.in)
		if got != tt.want || changed != tt.changed {
			t.Errorf("rewriteTweetLinks(%q) = %q, %v; want %q, %v", tt.in, got, changed, tt.want, tt.changed)
		}
	}
}

func TestIsLinkRewriteMessage(t *testing.T) {
	body, changed := rewriteTweetLinks("hey https://x.com/thdxr/status/2116735229792522518?s=19")
	if !changed {
		t.Fatal("expected the sample tweet link to be rewritten")
	}
	if !isLinkRewriteMessage(formatLinkRewrite("42", body)) {
		t.Fatalf("repost %q should be recognized as a link rewrite", formatLinkRewrite("42", body))
	}
	if isLinkRewriteMessage("here's the answer") || isLinkRewriteMessage("Shared by <@42>\nno tweet here") {
		t.Fatal("ordinary bot messages should not be recognized as link rewrites")
	}
}

func TestTweetLinkAllowedMentions(t *testing.T) {
	msg := &discordgo.Message{
		Mentions: []*discordgo.User{
			{ID: "user-b"},
			nil,
			{ID: ""},
			{ID: "user-b"},
			{ID: "user-c"},
		},
		MentionRoles: []string{"role-1"},
		ReferencedMessage: &discordgo.Message{
			Author: &discordgo.User{ID: "user-b"},
		},
	}
	got := tweetLinkAllowedMentions(msg)
	wantUsers := []string{"user-b", "user-c"}
	if !reflect.DeepEqual(got.Users, wantUsers) {
		t.Fatalf("users = %#v, want %#v", got.Users, wantUsers)
	}
	if !reflect.DeepEqual(got.Roles, []string{"role-1"}) {
		t.Fatalf("roles = %#v", got.Roles)
	}
	if !got.RepliedUser {
		t.Fatal("reply ping that was left on should stay on")
	}
	if len(got.Parse) != 0 {
		t.Fatalf("parse = %#v, want none so the attribution line stays silent", got.Parse)
	}

	pingOff := &discordgo.Message{
		Mentions: []*discordgo.User{{ID: "user-c"}},
		ReferencedMessage: &discordgo.Message{
			Author: &discordgo.User{ID: "user-b"},
		},
	}
	if tweetLinkAllowedMentions(pingOff).RepliedUser {
		t.Fatal("reply with the ping off should not ping the replied user")
	}

	if tweetLinkAllowedMentions(nil).RepliedUser || tweetLinkAllowedMentions(&discordgo.Message{}).Users != nil {
		t.Fatal("messages with no mentions should allow none")
	}
}
