package bot

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"mizubot-go/internal/llm"
)

func TestToolStatusLine(t *testing.T) {
	if got := toolStatusLine(nil); got != "" {
		t.Fatalf("empty runs = %q", got)
	}
	got := toolStatusLine([]llm.ToolRun{
		{Tool: "a", Status: "Created reminder #1", OK: true},
		{Tool: "b", Status: "Failed: b"},
	})
	want := "-# ✅ Created reminder #1 · ⚠️ Failed: b"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestToolActionComponents(t *testing.T) {
	if toolActionComponents([]llm.ToolRun{{Tool: "a", OK: true}}) != nil {
		t.Fatal("no actions should yield no components")
	}
	comps := toolActionComponents([]llm.ToolRun{{Tool: "reminder_create", Actions: []llm.Action{{Label: "Delete", ID: "delete:7", Danger: true}}}})
	row := comps[0].(discordgo.ActionsRow)
	btn := row.Components[0].(discordgo.Button)
	if btn.CustomID != "toolact:reminder_create:delete:7" || btn.Style != discordgo.DangerButton {
		t.Fatalf("button = %+v", btn)
	}
}

func TestAppendToolStatus(t *testing.T) {
	got := appendToolStatus([]string{"hi"}, "-# s")
	if len(got) != 1 || got[0] != "hi\n\n-# s" {
		t.Fatalf("got %q", got)
	}
	full := strings.Repeat("a", 1990)
	got = appendToolStatus([]string{full}, "-# "+strings.Repeat("b", 20))
	if len(got) != 2 {
		t.Fatalf("want separate message, got %d parts", len(got))
	}
}
