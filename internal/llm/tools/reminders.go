package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mizubot-go/internal/llm"
	"mizubot-go/internal/reminders"
	"mizubot-go/internal/usersettings"
)

func NewReminderTools(service *reminders.Service, settingsService ...*usersettings.Service) []llm.Tool {
	if service == nil {
		return nil
	}
	var settings *usersettings.Service
	if len(settingsService) > 0 {
		settings = settingsService[0]
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
			Description: "Create a reminder for the current Discord user. Infer a concise reminder message from the user's request unless they explicitly provide exact reminder text. LLM callers must provide normalized scheduling: cron_expr for repeated reminders, or once=true plus run_at for one-time reminders.",
			Parameters:  json.RawMessage(`{"type":"object","required":["message","once"],"properties":{"message":{"type":"string","description":"Concise reminder text to send later. Infer the actual thing to remember, not the full user command. For example, 'remind me to take meds tomorrow' should use 'take meds'. If the user quotes or explicitly states exact reminder text, preserve it."},"once":{"type":"boolean","description":"true for a one-time reminder; false for a repeated reminder."},"run_at":{"type":"string","description":"Required when once=true. Use a duration like 10m, 2h, 3d, RFC3339, or YYYY-MM-DD HH:MM in the selected timezone."},"cron_expr":{"type":"string","description":"Required when once=false. Five-field cron expression in the selected timezone."},"timezone":{"type":"string","description":"Optional IANA timezone name. Defaults to the user's configured timezone, then UTC."},"channel_id":{"type":"string","description":"Discord channel ID. Optional; defaults to the current channel."}},"additionalProperties":false}`),
			Execute:     createReminder(service, settings),
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
			fmt.Fprintf(&b, "Reminder ID %d\nMessage: %s\nNext run: %s\nChannel: %s\nTimezone: %s\nRepeat: %s\n\n",
				reminder.ID,
				reminder.Message,
				discordTimestamp(reminder.NextRun),
				reminder.ChannelID,
				reminder.Timezone,
				humanSchedule(reminder),
			)
		}
		return llm.ToolResult{Content: strings.TrimSpace(b.String())}, nil
	}
}

type reminderCreateArgs struct {
	Message   string `json:"message"`
	Once      bool   `json:"once"`
	RunAt     string `json:"run_at"`
	CronExpr  string `json:"cron_expr"`
	Timezone  string `json:"timezone"`
	ChannelID string `json:"channel_id"`
}

func createReminder(service *reminders.Service, settingsService *usersettings.Service) llm.ToolHandler {
	return func(ctx context.Context, toolCtx llm.ToolContext, raw json.RawMessage) (llm.ToolResult, error) {
		var args reminderCreateArgs
		if err := json.Unmarshal(raw, &args); err != nil {
			return llm.ToolResult{}, fmt.Errorf("invalid create reminder arguments: %w", err)
		}
		channelID := strings.TrimSpace(args.ChannelID)
		if channelID == "" {
			channelID = toolCtx.ChannelID
		}
		timezone := strings.TrimSpace(args.Timezone)
		if timezone == "" {
			timezone = userTimezone(ctx, settingsService, toolCtx.UserID)
		}
		schedule := string(reminders.ScheduleCron)
		at := strings.TrimSpace(args.CronExpr)
		if args.Once {
			schedule = string(reminders.ScheduleOnce)
			at = strings.TrimSpace(args.RunAt)
			if at == "" {
				return llm.ToolResult{}, fmt.Errorf("run_at is required when once=true")
			}
		} else if at == "" {
			return llm.ToolResult{}, fmt.Errorf("cron_expr is required when once=false")
		}
		created, err := service.CreateReminder(ctx, reminders.CreateReminderInput{
			UserID:    toolCtx.UserID,
			ChannelID: channelID,
			GuildID:   toolCtx.GuildID,
			Message:   args.Message,
			Schedule:  schedule,
			At:        at,
			CronExpr:  args.CronExpr,
			Timezone:  timezone,
		})
		if err != nil {
			return llm.ToolResult{}, err
		}
		return llm.ToolResult{Content: fmt.Sprintf("Created reminder ID %d.\nMessage: %s\nNext run: %s\nChannel: %s\nTimezone: %s\nRepeat: %s",
			created.ID,
			created.Message,
			discordTimestamp(created.NextRun),
			created.ChannelID,
			created.Timezone,
			humanSchedule(*created),
		)}, nil
	}
}

func humanSchedule(reminder reminders.Reminder) string {
	if reminder.Once {
		return "once"
	}
	switch reminder.Schedule {
	case reminders.ScheduleHourly:
		return "hourly"
	case reminders.ScheduleDaily:
		return "daily"
	case reminders.ScheduleCron:
		return "custom recurring schedule"
	default:
		return string(reminder.Schedule)
	}
}

func userTimezone(ctx context.Context, settingsService *usersettings.Service, userID string) string {
	if settingsService == nil {
		return usersettings.DefaultTimezone
	}
	timezone, _, err := settingsService.GetTimezone(ctx, userID)
	if err != nil {
		return usersettings.DefaultTimezone
	}
	return timezone
}

func discordTimestamp(t time.Time) string {
	unix := t.UTC().Unix()
	return fmt.Sprintf("<t:%d:F> (<t:%d:R>)", unix, unix)
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
		deleted, ok, err := service.DeleteReminderWithDetails(ctx, args.ID, toolCtx.UserID)
		if err != nil {
			return llm.ToolResult{}, err
		}
		if !ok {
			return llm.ToolResult{Content: fmt.Sprintf("Reminder ID %d was not found for this user.", args.ID)}, nil
		}
		return llm.ToolResult{Content: fmt.Sprintf("Deleted reminder ID %d.\nMessage: %s\nNext run: %s\nChannel: %s\nTimezone: %s\nRepeat: %s",
			deleted.ID,
			deleted.Message,
			discordTimestamp(deleted.NextRun),
			deleted.ChannelID,
			deleted.Timezone,
			humanSchedule(deleted),
		)}, nil
	}
}
