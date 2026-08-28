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

// Embedder calls an OpenAI-compatible embeddings API to convert text into vectors.
type Embedder struct {
	endpoint   string // e.g. "https://api.openai.com/v1/embeddings"
	apiKey     string
	model      string // e.g. "text-embedding-3-small"
	dim        int    // expected vector dimension (for Qdrant collection setup)
	httpClient *http.Client
	logger     *log.Logger
}

// EmbedderConfig holds embedding API configuration.
type EmbedderConfig struct {
	Endpoint string // full URL to embeddings endpoint
	APIKey   string
	Model    string
	Dim      int // vector dimension
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
		dim:        cfg.Dim,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

// Dim returns the configured vector dimension.
func (e *Embedder) Dim() int {
	return e.dim
}

// embeddingRequest is the OpenAI-compatible request body.
type embeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

// embeddingResponse is the OpenAI-compatible response body.
type embeddingResponse struct {
	Data []embeddingData `json:"data"`
}

type embeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

// Embed generates embedding vectors for a batch of texts.
// Returns vectors in the same order as the input texts.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body := embeddingRequest{
		Input: texts,
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

	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("embedding API returned %d vectors for %d inputs", len(result.Data), len(texts))
	}

	// Sort by index to ensure correct ordering.
	vectors := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("invalid embedding index %d", d.Index)
		}
		vectors[d.Index] = d.Embedding
	}

	e.logger.Printf("[embed] generated %d vectors (dim=%d)", len(vectors), len(vectors[0]))
	return vectors, nil
}

// EmbedBatched processes texts in batches to avoid API limits.
// maxBatch controls how many texts per API call (0 = send all at once).
func (e *Embedder) EmbedBatched(ctx context.Context, texts []string, maxBatch int) ([][]float32, error) {
	if maxBatch <= 0 || maxBatch >= len(texts) {
		return e.Embed(ctx, texts)
	}

	allVectors := make([][]float32, len(texts))

	for i := 0; i < len(texts); i += maxBatch {
		end := i + maxBatch
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		vectors, err := e.Embed(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("batch %d-%d: %w", i, end, err)
		}

		copy(allVectors[i:end], vectors)
	}

	return allVectors, nil
}
