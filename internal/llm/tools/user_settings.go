package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"mizubot-go/internal/llm"
	"mizubot-go/internal/usersettings"
)

func NewUserSettingsTools(service *usersettings.Service) []llm.Tool {
	if service == nil {
		return nil
	}
	return []llm.Tool{
		{
			Name:        "user_timezone_get",
			Description: "Get the current Discord user's configured timezone. Returns UTC when no timezone has been explicitly set.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			Execute:     getUserTimezone(service),
		},
		{
			Name:        "user_timezone_set",
			Description: "Set the current Discord user's timezone. Use IANA timezone names like Asia/Kolkata or America/Los_Angeles.",
			Parameters:  json.RawMessage(`{"type":"object","required":["timezone"],"properties":{"timezone":{"type":"string","description":"IANA timezone name, for example Asia/Kolkata."}},"additionalProperties":false}`),
			Execute:     setUserTimezone(service),
		},
	}
}

func getUserTimezone(service *usersettings.Service) llm.ToolHandler {
	return func(ctx context.Context, toolCtx llm.ToolContext, _ json.RawMessage) (llm.ToolResult, error) {
		timezone, configured, err := service.GetTimezone(ctx, toolCtx.UserID)
		if err != nil {
			return llm.ToolResult{}, err
		}
		if !configured {
			return llm.ToolResult{Content: fmt.Sprintf("No timezone is configured. Default timezone is %s.", timezone)}, nil
		}
		return llm.ToolResult{Content: fmt.Sprintf("Configured timezone is %s.", timezone)}, nil
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
