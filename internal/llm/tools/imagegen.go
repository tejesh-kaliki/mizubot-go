package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"mizubot-go/internal/llm"
)

var imageGenKeywords = []string{"image", "picture", "draw", "generate", "art"}

// ImageGenerator is the slice of the image generation client the tool uses.
type ImageGenerator interface {
	Generate(ctx context.Context, prompt string) ([]byte, error)
}

type imageGenTools struct {
	generator ImageGenerator
}

// NewImageGenTools returns the image generation tool. generator may be nil,
// in which case no tool is registered.
func NewImageGenTools(generator ImageGenerator) []llm.Tool {
	if generator == nil {
		return nil
	}
	t := &imageGenTools{generator: generator}
	return []llm.Tool{
		{
			Name:        "generate_image",
			Description: "Generate an image from a text description and attach it to the reply. Use when the user asks to draw, create, generate, or make an image or picture of something.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string","description":"A clear, descriptive prompt of the image to generate, in English, expanded from the user's request with useful visual detail."}},"required":["prompt"],"additionalProperties":false}`),
			Keywords:    imageGenKeywords,
			ClassifierHint: "The user wants an image, picture, drawing, or piece of art created or generated for them, " +
				"e.g. \"draw me a cat\", \"generate an image of a sunset\", \"make a picture of...\".",
			Execute: t.generate,
		},
	}
}

type imageGenArgs struct {
	Prompt string `json:"prompt"`
}

func (t *imageGenTools) generate(ctx context.Context, _ llm.ToolContext, raw json.RawMessage) (llm.ToolResult, error) {
	var args imageGenArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return llm.ToolResult{}, fmt.Errorf("invalid generate_image arguments: %w", err)
		}
	}
	args.Prompt = strings.TrimSpace(args.Prompt)
	if args.Prompt == "" {
		return llm.ToolResult{}, fmt.Errorf("provide a prompt describing the image to generate")
	}

	data, err := t.generator.Generate(ctx, args.Prompt)
	if err != nil {
		return llm.ToolResult{}, fmt.Errorf("generate image: %w", err)
	}

	return llm.ToolResult{
		Content: "An image was generated for the prompt " + fmt.Sprintf("%q", args.Prompt) + " and is attached to your reply automatically. Tell the user it's ready; do not describe the image's contents, you have not seen it.",
		Attachments: []llm.Attachment{
			{Filename: "generated.png", Data: data, ContentType: "image/png"},
		},
	}, nil
}
