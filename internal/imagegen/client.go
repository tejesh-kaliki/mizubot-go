// Package imagegen talks to a local image-generation HTTP service (an
// mflux/MLX wrapper running natively on the Docker host, since Metal/MLX
// needs GPU access a Linux container can't get). See imagegen/ in the repo
// root for that service.
package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL reaches a host-native image-generation service through the
// Docker host gateway (see prod/docker-compose.yml's extra_hosts entry),
// mirroring how the Bifrost LLM gateway is reached at host.docker.internal.
const DefaultBaseURL = "http://host.docker.internal:8081"

// Config configures a Client for the image-generation HTTP service.
type Config struct {
	BaseURL    string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// Client generates images by calling a local HTTP service.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient returns a Client. cfg.BaseURL and cfg.Timeout fall back to
// DefaultBaseURL and 120s (local diffusion generation, including on-demand
// model load, is slow).
func NewClient(cfg Config) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{baseURL: baseURL, http: httpClient}
}

// Generate asks the image-generation service to render prompt, returning
// the resulting PNG bytes.
func (c *Client) Generate(ctx context.Context, prompt string) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("image generation is not configured")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	body, err := json.Marshal(struct {
		Prompt string `json:"prompt"`
	}{Prompt: prompt})
	if err != nil {
		return nil, fmt.Errorf("encode image generation request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build image generation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call image generation service: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read image generation response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("image generation service returned %d: %s", resp.StatusCode, msg)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("image generation service returned an empty image")
	}
	return data, nil
}
