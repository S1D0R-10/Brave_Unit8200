package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// LLM talks to an OpenAI-compatible chat completions endpoint — the same
// provider the embeddings already go through, so one API key covers both.
type LLM struct {
	endpoint    string // e.g. "https://api.openai.com/v1/chat/completions"
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
	logger      *log.Logger
}

// LLMConfig holds chat completion settings.
type LLMConfig struct {
	Endpoint    string
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float64
}

// NewLLM creates a new chat completions client.
func NewLLM(cfg LLMConfig, logger *log.Logger) *LLM {
	if logger == nil {
		logger = log.Default()
	}
	return &LLM{
		endpoint:    cfg.Endpoint,
		apiKey:      cfg.APIKey,
		model:       cfg.Model,
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
		httpClient:  &http.Client{},
		logger:      logger,
	}
}

// Configured reports whether the client has enough settings to be usable.
// When it does not, the service still answers with retrieved citations and
// simply leaves the generated prose out.
func (l *LLM) Configured() bool {
	return l != nil && l.endpoint != "" && l.apiKey != "" && l.model != ""
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Complete sends a system + user prompt pair and returns the generated text.
func (l *LLM) Complete(ctx context.Context, system, user string) (string, error) {
	if !l.Configured() {
		return "", fmt.Errorf("LLM is not configured")
	}

	body := chatRequest{
		Model: l.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		MaxTokens:   l.maxTokens,
		Temperature: l.temperature,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.apiKey)

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling chat API: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading chat response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chat API returned %d: %s", resp.StatusCode, string(raw))
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decoding chat response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("chat API error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("chat API returned no choices")
	}

	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}
