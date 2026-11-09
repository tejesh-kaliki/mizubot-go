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
	got := linkRewriteAllowedMentions(msg)
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
	if linkRewriteAllowedMentions(pingOff).RepliedUser {
		t.Fatal("reply with the ping off should not ping the replied user")
	}

	if linkRewriteAllowedMentions(nil).RepliedUser || linkRewriteAllowedMentions(&discordgo.Message{}).Users != nil {
		t.Fatal("messages with no mentions should allow none")
	}
}

func TestRewriteInstagramLinks(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		changed bool
	}{
		{
			in:      "https://www.instagram.com/p/CxAbCdEfGhI/?igsh=abc&utm_source=ig_web_copy_link",
			want:    "https://vxinstagram.com/p/CxAbCdEfGhI",
			changed: true,
		},
		{
			in:      "look https://instagram.com/reel/CxAbCdEfGhI/?igsh=abc lol",
			want:    "look https://vxinstagram.com/reel/CxAbCdEfGhI lol",
			changed: true,
		},
		{
			in:      "https://m.instagram.com/reels/CxAbCdEfGhI/",
			want:    "https://vxinstagram.com/reels/CxAbCdEfGhI",
			changed: true,
		},
		{
			in:      "https://www.instagram.com/p/CxAbCdEfGhI/?img_index=2&igsh=abc",
			want:    "https://vxinstagram.com/p/CxAbCdEfGhI/2",
			changed: true,
		},
		{
			in:      "https://www.instagram.com/p/CxAbCdEfGhI/3/?igsh=abc",
			want:    "https://vxinstagram.com/p/CxAbCdEfGhI/3",
			changed: true,
		},
		{
			in:      "https://www.instagram.com/some.user/reel/CxAbCdEfGhI/",
			want:    "https://vxinstagram.com/some.user/reel/CxAbCdEfGhI",
			changed: true,
		},
		{
			in:      "https://instagram.com/stories/some.user/123456789012345/",
			want:    "https://vxinstagram.com/stories/some.user/123456789012345",
			changed: true,
		},
		{
			in:      "https://www.instagram.com/stories/some.user/",
			want:    "https://vxinstagram.com/stories/some.user",
			changed: true,
		},
		{
			in:      "https://www.instagram.com/share/p/BACxYz123abc/",
			want:    "https://vxinstagram.com/share/p/BACxYz123abc",
			changed: true,
		},
		{
			in:      "https://instagr.am/share/BACxYz123abc?igsh=abc",
			want:    "https://vxinstagram.com/share/BACxYz123abc",
			changed: true,
		},
		{
			in:      "https://www.instagram.com/tv/CxAbCdEfGhI/",
			want:    "https://vxinstagram.com/tv/CxAbCdEfGhI",
			changed: true,
		},
		{
			in:      "see https://www.instagram.com/p/CxAbCdEfGhI/.",
			want:    "see https://vxinstagram.com/p/CxAbCdEfGhI.",
			changed: true,
		},
		{in: "<https://www.instagram.com/p/CxAbCdEfGhI/>", want: "<https://www.instagram.com/p/CxAbCdEfGhI/>"},
		{
			in:      "https://instagram.com/some.user/ and https://instagram.com/p/CxAbCdEfGhI/",
			want:    "https://instagram.com/some.user/ and https://vxinstagram.com/p/CxAbCdEfGhI",
			changed: true,
		},
		{in: "https://www.instagram.com/some.user/", want: "https://www.instagram.com/some.user/"},
		{in: "https://www.instagram.com/explore/tags/cats/", want: "https://www.instagram.com/explore/tags/cats/"},
		{in: "https://www.instagram.com/stories/highlights/123456789/", want: "https://www.instagram.com/stories/highlights/123456789/"},
		{in: "https://example.com/instagram.com/p/CxAbCdEfGhI/", want: "https://example.com/instagram.com/p/CxAbCdEfGhI/"},
		{in: "no links", want: "no links"},
	}
	for _, tt := range tests {
		got, changed := rewriteInstagramLinks(tt.in)
		if got != tt.want || changed != tt.changed {
			t.Errorf("rewriteInstagramLinks(%q) = %q, %v; want %q, %v", tt.in, got, changed, tt.want, tt.changed)
		}
	}
}

func TestRewriteEmbedLinksMixed(t *testing.T) {
	in := "https://x.com/thdxr/status/2116735229792522518?s=19 and https://www.instagram.com/reel/CxAbCdEfGhI/?igsh=abc"
	want := "https://fixupx.com/thdxr/status/2116735229792522518/en and https://vxinstagram.com/reel/CxAbCdEfGhI"
	got, changed := rewriteEmbedLinks(in)
	if !changed || got != want {
		t.Fatalf("rewriteEmbedLinks(%q) = %q, %v; want %q, true", in, got, changed, want)
	}
}

func TestIsInstagramLinkRewriteMessage(t *testing.T) {
	body, changed := rewriteInstagramLinks("https://www.instagram.com/p/CxAbCdEfGhI/?igsh=abc")
	if !changed {
		t.Fatal("expected the sample instagram link to be rewritten")
	}
	if !isLinkRewriteMessage(formatLinkRewrite("42", body)) {
		t.Fatalf("repost %q should be recognized as a link rewrite", formatLinkRewrite("42", body))
	}
}
