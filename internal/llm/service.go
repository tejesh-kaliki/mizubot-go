package llm

import (
	"context"
	"strings"
)

type Message struct {
	UserID    string
	Username  string
	ChannelID string
	Content   string
}

type Generator interface {
	GenerateResponse(ctx context.Context, message Message) (string, error)
}

type Service struct {
	generator Generator
}

func NewService(generator Generator) *Service {
	return &Service{generator: generator}
}

func (s *Service) GenerateResponse(ctx context.Context, message Message) (string, error) {
	if s == nil || s.generator == nil {
		return "", nil
	}
	message.Content = strings.TrimSpace(message.Content)
	if message.Content == "" {
		return "", nil
	}
	return s.generator.GenerateResponse(ctx, message)
}
