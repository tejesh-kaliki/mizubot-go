package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"mizubot-go/internal/llm"
	"mizubot-go/internal/usersettings"
)

var timezoneToolKeywords = []string{"timezone", "time zone", "tz", "local time"}

func NewUserSettingsTools(service *usersettings.Service) []llm.Tool {
	if service == nil {
		return nil
	}
	return []llm.Tool{
		{
			Name:        "user_timezone_set",
			Description: "Set the current Discord user's timezone. Use IANA timezone names like Asia/Kolkata or America/Los_Angeles.",
			Parameters:  json.RawMessage(`{"type":"object","required":["timezone"],"properties":{"timezone":{"type":"string","description":"IANA timezone name, for example Asia/Kolkata."}},"additionalProperties":false}`),
			Keywords:    timezoneToolKeywords,
			Execute:     setUserTimezone(service),
		},
	}
}

type setUserTimezoneArgs struct {
	Timezone string `json:"timezone"`
}

func setUserTimezone(service *usersettings.Service) llm.ToolHandler {
	return func(ctx context.Context, toolCtx llm.ToolContext, raw json.RawMessage) (llm.ToolResult, error) {
		var args setUserTimezoneArgs
		if err := json.Unmarshal(raw, &args); err != nil {
			return llm.ToolResult{}, fmt.Errorf("invalid timezone arguments: %w", err)
		}
		settings, err := service.SetTimezone(ctx, toolCtx.UserID, args.Timezone)
		if err != nil {
			return llm.ToolResult{}, err
		}
		return llm.ToolResult{Content: fmt.Sprintf("Timezone set to %s.", settings.Timezone)}, nil
	}
}
