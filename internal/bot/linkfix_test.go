package bot

import (
	"reflect"
	"strings"
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
		Author: &discordgo.User{ID: "owner"},
		Mentions: []*discordgo.User{
			{ID: "user-b"},
			nil,
			{ID: ""},
			{ID: "user-b"},
			{ID: "user-c"},
		},
		MentionRoles: []string{"role-1"},
		Content:      "@everyone <@&role-1>",
		ReferencedMessage: &discordgo.Message{
			Author: &discordgo.User{ID: "user-b"},
		},
	}
	got := linkRewriteAllowedMentions(msg)
	wantUsers := []string{"user-b", "user-c", "owner"}
	if !reflect.DeepEqual(got.Users, wantUsers) {
		t.Fatalf("users = %#v, want %#v", got.Users, wantUsers)
	}
	if len(got.Roles) != 0 {
		t.Fatalf("roles = %#v, want none", got.Roles)
	}
	if !got.RepliedUser {
		t.Fatal("reply ping that was left on should stay on")
	}
	if len(got.Parse) != 0 {
		t.Fatalf("parse = %#v, want none so role and @everyone mentions stay silent", got.Parse)
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

func TestComposeLinkRewriteEditKeepsAttribution(t *testing.T) {
	submitted := "hey <@&99> <@7> @everyone https://x.com/a/status/1?s=1"
	content, errText := composeLinkRewriteEdit("42", submitted)
	if errText != "" {
		t.Fatal(errText)
	}
	if !strings.HasPrefix(content, "Shared by <@42>\n") {
		t.Fatalf("content = %q, want the owner mention kept at the top", content)
	}
	if !strings.Contains(content, "https://fixupx.com/a/status/1/en") {
		t.Fatalf("content = %q, want the tweet link rewritten", content)
	}
	if !strings.Contains(content, "hey <@&99> <@7> @everyone ") {
		t.Fatalf("content = %q, want the edited text left as typed", content)
	}

	if _, errText := composeLinkRewriteEdit("42", "   "); errText != linkRewriteEmptyEditText {
		t.Fatalf("empty edit error = %q", errText)
	}
}

func TestLinkRewriteControls(t *testing.T) {
	owner, ok := linkRewriteAuthorID("Shared by <@!42>\nhello")
	if !ok || owner != "42" || linkRewriteBody("Shared by <@!42>\nhello") != "hello" {
		t.Fatalf("author parse failed: %q %v body %q", owner, ok, linkRewriteBody("Shared by <@!42>\nhello"))
	}
	row := linkRewriteComponents()[0].(discordgo.ActionsRow)
	edit := row.Components[0].(discordgo.Button)
	del := row.Components[1].(discordgo.Button)
	if edit.CustomID != linkRewriteEditID || edit.Style != discordgo.SecondaryButton {
		t.Fatalf("edit button = %#v", edit)
	}
	if del.CustomID != linkRewriteDeleteID || del.Style != discordgo.DangerButton {
		t.Fatalf("delete button = %#v", del)
	}
	customID := linkRewriteDeleteConfirmID("111", "222")
	channelID, messageID, ok := parseLinkRewriteDeleteID(customID)
	if !ok || channelID != "111" || messageID != "222" {
		t.Fatalf("parse %q = %q %q %v", customID, channelID, messageID, ok)
	}
	if _, _, ok := parseLinkRewriteDeleteID("linkfix:del:nope"); ok {
		t.Fatal("non-snowflake delete id should be rejected")
	}

	msg := &discordgo.Message{Content: formatLinkRewrite("42", "https://fixupx.com/a/status/1/en")}
	if !linkRewriteOwnedBy(msg, "42") || linkRewriteOwnedBy(msg, "7") {
		t.Fatal("ownership check failed")
	}
}

func TestLinkRewriteInteractionClaimsOnlyItsButtons(t *testing.T) {
	b := &Bot{}
	other := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type: discordgo.InteractionMessageComponent,
		Data: discordgo.MessageComponentInteractionData{CustomID: "flags-add"},
	}}
	if b.handleLinkRewriteInteraction(nil, other) {
		t.Fatal("claimed an unrelated component")
	}
	untyped := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type: discordgo.InteractionMessageComponent,
	}}
	if b.handleLinkRewriteInteraction(nil, untyped) {
		t.Fatal("claimed a component interaction with no data")
	}

	edit := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type:   discordgo.InteractionMessageComponent,
		Data:   discordgo.MessageComponentInteractionData{CustomID: linkRewriteEditID},
		Member: &discordgo.Member{User: &discordgo.User{ID: "7"}},
		Message: &discordgo.Message{
			Content: formatLinkRewrite("42", "https://fixupx.com/a/status/1/en"),
		},
	}}
	if !b.handleLinkRewriteInteraction(nil, edit) {
		t.Fatal("edit button should be claimed")
	}
}
