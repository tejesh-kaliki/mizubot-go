package bot

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"mizubot-go/internal/llm"
)

const (
	toolActionPrefix = "toolact:"
	// Discord allows five buttons in one action row.
	maxToolActionButtons = 5
	maxDiscordMessageLen = 2000
)

// toolStatusLine summarizes the tool calls that really ran, as Discord
// subtext. It is generated from execution records, not LLM text, so it shows
// whether an action actually happened.
func toolStatusLine(runs []llm.ToolRun) string {
	if len(runs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(runs))
	for _, run := range runs {
		mark := "✅"
		if !run.OK {
			mark = "⚠️"
		}
		parts = append(parts, mark+" "+run.Status)
	}
	return "-# " + strings.Join(parts, " · ")
}

// toolActionComponents builds the button row for the actions tools offered.
func toolActionComponents(runs []llm.ToolRun) []discordgo.MessageComponent {
	var buttons []discordgo.MessageComponent
	for _, run := range runs {
		for _, action := range run.Actions {
			customID := toolActionPrefix + run.Tool + ":" + action.ID
			if len(buttons) >= maxToolActionButtons || len(customID) > 100 {
				continue
			}
			style := discordgo.SecondaryButton
			if action.Danger {
				style = discordgo.DangerButton
			}
			buttons = append(buttons, discordgo.Button{Label: action.Label, Style: style, CustomID: customID})
		}
	}
	if len(buttons) == 0 {
		return nil
	}
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: buttons}}
}

// appendToolStatus puts the status line under the last reply part, or in a
// message of its own when that part has no room.
func appendToolStatus(parts []string, status string) []string {
	if status == "" {
		return parts
	}
	if len(parts) == 0 {
		return []string{status}
	}
	last := parts[len(parts)-1]
	if last != "" && len([]rune(last))+2+len([]rune(status)) <= maxDiscordMessageLen {
		parts[len(parts)-1] = last + "\n\n" + status
		return parts
	}
	if last == "" {
		parts[len(parts)-1] = status
		return parts
	}
	return append(parts, status)
}

// handleToolActionInteraction runs the button a tool put under a reply.
// It reports whether the interaction belonged to this feature.
func (b *Bot) handleToolActionInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if i == nil || i.Type != discordgo.InteractionMessageComponent {
		return false
	}
	data, ok := i.Data.(discordgo.MessageComponentInteractionData)
	if !ok || !strings.HasPrefix(data.CustomID, toolActionPrefix) {
		return false
	}
	toolName, actionID, ok := strings.Cut(strings.TrimPrefix(data.CustomID, toolActionPrefix), ":")
	if !ok || b.llm == nil || i.Message == nil {
		return true
	}
	user := i.User
	if i.Member != nil && i.Member.User != nil {
		user = i.Member.User
	}
	if user == nil {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	outcome, err := b.llm.RunToolAction(ctx, llm.ToolContext{
		UserID:    user.ID,
		Username:  user.Username,
		ChannelID: i.ChannelID,
		GuildID:   i.GuildID,
	}, toolName, actionID)
	if err != nil {
		log.Printf("tool action failed: tool=%s action=%s user_id=%s error=%v", toolName, actionID, user.ID, err)
		outcome = "Couldn't do that: " + err.Error()
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    i.Message.Content + "\n-# ↳ " + outcome,
			Components: []discordgo.MessageComponent{},
		},
	}); err != nil {
		log.Printf("tool action response failed: tool=%s error=%v", toolName, err)
	}
	return true
}
