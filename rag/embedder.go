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

// Embedder calls an OpenAI-compatible embeddings API to convert text into a vector.
type Embedder struct {
	endpoint   string // e.g. "https://api.openai.com/v1/embeddings"
	apiKey     string
	model      string // e.g. "text-embedding-3-small"
	httpClient *http.Client
	logger     *log.Logger
}

// EmbedderConfig holds embedding API configuration.
type EmbedderConfig struct {
	Endpoint string
	APIKey   string
	Model    string
}

// NewEmbedder creates a new Embedder.
func NewEmbedder(cfg EmbedderConfig, logger *log.Logger) *Embedder {
	if logger == nil {
		logger = log.Default()
	}
	return &Embedder{
		endpoint:   cfg.Endpoint,
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

type embeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

type embeddingResponse struct {
	Data []embeddingData `json:"data"`
}

type embeddingData struct {
	Embedding []float32 `json:"embedding"`
}

// Embed generates an embedding vector for a single prompt.
func (e *Embedder) Embed(ctx context.Context, prompt string) ([]float32, error) {
	if prompt == "" {
		return nil, fmt.Errorf("prompt cannot be empty")
	}

	body := embeddingRequest{
		Input: prompt,
		Model: e.model,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling embedding API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding embedding response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("embedding API returned 0 vectors")
	}

	e.logger.Printf("[embed] generated vector (dim=%d) for prompt", len(result.Data[0].Embedding))
	return result.Data[0].Embedding, nil
}
