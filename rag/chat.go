package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// ChatClient calls an OpenAI-compatible chat completions API (OpenRouter).
type ChatClient struct {
	endpoint   string // e.g. "https://openrouter.ai/api/v1/chat/completions"
	apiKey     string
	model      string
	httpClient *http.Client
	logger     *log.Logger
}

// ChatConfig holds chat completion API configuration.
type ChatConfig struct {
	Endpoint string
	APIKey   string
	Model    string
}

// NewChatClient creates a new ChatClient.
func NewChatClient(cfg ChatConfig, logger *log.Logger) *ChatClient {
	if logger == nil {
		logger = log.Default()
	}
	return &ChatClient{
		endpoint:   cfg.Endpoint,
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string                 `json:"model"`
	Messages       []chatMessage          `json:"messages"`
	Temperature    float64                `json:"temperature"`
	ResponseFormat map[string]interface{} `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// CompleteJSON sends a system+user prompt pair and returns the raw JSON
// object text the model replied with (response_format is set to force a
// JSON object reply).
func (c *ChatClient) CompleteJSON(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature:    0.2,
		ResponseFormat: map[string]interface{}{"type": "json_object"},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling chat API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading chat response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chat API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result chatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decoding chat response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("chat API error: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("chat API returned 0 choices")
	}

	c.logger.Printf("[chat] model=%s reply_len=%d", c.model, len(result.Choices[0].Message.Content))
	return result.Choices[0].Message.Content, nil
}
