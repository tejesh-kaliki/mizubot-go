package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"mizubot-go/internal/llm"
	"mizubot-go/internal/reminders"
)

func NewReminderTools(service *reminders.Service) []llm.Tool {
	if service == nil {
		return nil
	}
	return []llm.Tool{
		{
			Name:        "reminder_list_active",
			Description: "Load the current Discord user's active reminders.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			Execute:     listReminders(service),
		},
		{
			Name:        "reminder_create",
			Description: "Create a reminder for the current Discord user. Times are UTC. Supported schedules are once, hourly, and daily.",
			Parameters:  json.RawMessage(`{"type":"object","required":["message","schedule"],"properties":{"message":{"type":"string","description":"Reminder text to send."},"schedule":{"type":"string","enum":["once","hourly","daily"]},"at":{"type":"string","description":"For once: 10m, 2h, 3d, RFC3339, or YYYY-MM-DD HH:MM UTC. For daily: HH:MM UTC. For hourly: :MM. Optional."},"channel_id":{"type":"string","description":"Discord channel ID. Optional; defaults to the current channel."}},"additionalProperties":false}`),
			Execute:     createReminder(service),
		},
		{
			Name:        "reminder_delete",
			Description: "Delete one of the current Discord user's reminders by ID.",
			Parameters:  json.RawMessage(`{"type":"object","required":["id"],"properties":{"id":{"type":"integer","description":"Reminder ID to delete."}},"additionalProperties":false}`),
			Execute:     deleteReminder(service),
		},
	}
}

func listReminders(service *reminders.Service) llm.ToolHandler {
	return func(ctx context.Context, toolCtx llm.ToolContext, _ json.RawMessage) (llm.ToolResult, error) {
		list, err := service.ListUserReminders(ctx, toolCtx.UserID)
		if err != nil {
			return llm.ToolResult{}, err
		}
		if len(list) == 0 {
			return llm.ToolResult{Content: "No active reminders."}, nil
		}

		var b strings.Builder
		for _, reminder := range list {
			fmt.Fprintf(&b, "ID %d: %s reminder, next run %s UTC, channel %s, message: %s\n",
				reminder.ID,
				reminder.Schedule,
				reminder.NextRun.UTC().Format("2006-01-02 15:04"),
				reminder.ChannelID,
				reminder.Message,
			)
		}
		return llm.ToolResult{Content: strings.TrimSpace(b.String())}, nil
	}
}

type reminderCreateArgs struct {
	Message   string `json:"message"`
	Schedule  string `json:"schedule"`
	At        string `json:"at"`
	ChannelID string `json:"channel_id"`
}

func createReminder(service *reminders.Service) llm.ToolHandler {
	return func(ctx context.Context, toolCtx llm.ToolContext, raw json.RawMessage) (llm.ToolResult, error) {
		var args reminderCreateArgs
		if err := json.Unmarshal(raw, &args); err != nil {
			return llm.ToolResult{}, fmt.Errorf("invalid create reminder arguments: %w", err)
		}
		channelID := strings.TrimSpace(args.ChannelID)
		if channelID == "" {
			channelID = toolCtx.ChannelID
		}
		created, err := service.CreateReminder(ctx, reminders.CreateReminderInput{
			UserID:    toolCtx.UserID,
			ChannelID: channelID,
			GuildID:   toolCtx.GuildID,
			Message:   args.Message,
			Schedule:  args.Schedule,
			At:        args.At,
		})
		if err != nil {
			return llm.ToolResult{}, err
		}
		return llm.ToolResult{Content: fmt.Sprintf("Created reminder ID %d. Schedule: %s. Next run: %s UTC. Channel: %s. Message: %s",
			created.ID,
			created.Schedule,
			created.NextRun.UTC().Format("2006-01-02 15:04"),
			created.ChannelID,
			created.Message,
		)}, nil
	}
}

type reminderDeleteArgs struct {
	ID int64 `json:"id"`
}

func deleteReminder(service *reminders.Service) llm.ToolHandler {
	return func(ctx context.Context, toolCtx llm.ToolContext, raw json.RawMessage) (llm.ToolResult, error) {
		var args reminderDeleteArgs
		if err := json.Unmarshal(raw, &args); err != nil {
			return llm.ToolResult{}, fmt.Errorf("invalid delete reminder arguments: %w", err)
		}
		if args.ID <= 0 {
			return llm.ToolResult{}, fmt.Errorf("id must be positive")
		}
		ok, err := service.DeleteReminder(ctx, args.ID, toolCtx.UserID)
		if err != nil {
			return llm.ToolResult{}, err
		}
		if !ok {
			return llm.ToolResult{Content: fmt.Sprintf("Reminder ID %d was not found for this user.", args.ID)}, nil
		}
		return llm.ToolResult{Content: fmt.Sprintf("Deleted reminder ID %d.", args.ID)}, nil
	}
}
