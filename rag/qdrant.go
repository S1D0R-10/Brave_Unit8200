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

// QdrantClient is a REST client for searching Qdrant points.
type QdrantClient struct {
	baseURL    string // e.g. "http://qdrant.railway.internal:6333"
	collection string // e.g. "citations"
	httpClient *http.Client
	logger     *log.Logger
}

type QdrantConfig struct {
	Host       string
	Port       int
	Collection string
}

func NewQdrantClient(cfg QdrantConfig, logger *log.Logger) *QdrantClient {
	if cfg.Collection == "" {
		cfg.Collection = "citations"
	}
	if cfg.Port == 0 {
		cfg.Port = 6333
	}
	if logger == nil {
		logger = log.Default()
	}

	return &QdrantClient{
		baseURL:    fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port),
		collection: cfg.Collection,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

// ScoredPoint represents a search result from Qdrant.
type ScoredPoint struct {
	ID      string                 `json:"id"`
	Score   float64                `json:"score"`
	Payload map[string]interface{} `json:"payload"`
}

// searchRequest represents the Qdrant search API body.
type searchRequest struct {
	Vector      []float32 `json:"vector"`
	Limit       int       `json:"limit"`
	WithPayload bool      `json:"with_payload"`
}

type searchResponse struct {
	Result []ScoredPoint `json:"result"`
}

// Search finds the k-nearest neighbors to the given vector.
func (q *QdrantClient) Search(ctx context.Context, vector []float32, limit int) ([]ScoredPoint, error) {
	body := searchRequest{
		Vector:      vector,
		Limit:       limit,
		WithPayload: true,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling search request: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points/search", q.baseURL, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling search API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding search response: %w", err)
	}

	q.logger.Printf("[qdrant] search returned %d hits", len(result.Result))
	return result.Result, nil
}

// scrollRequest represents the Qdrant scroll API body with filters.
type scrollRequest struct {
	Filter      filterClause `json:"filter"`
	Limit       int          `json:"limit"`
	WithPayload bool         `json:"with_payload"`
}

type filterClause struct {
	Must []matchCondition `json:"must"`
}

type matchCondition struct {
	Key   string      `json:"key"`
	Match matchString `json:"match"`
}

type matchString struct {
	Value string `json:"value"`
}

type scrollResponse struct {
	Result scrollResult `json:"result"`
}

type scrollResult struct {
	Points []ScoredPoint `json:"points"`
}

// GetChunksByFile fetches all point metadata (payloads) for a given file_hash.
func (q *QdrantClient) GetChunksByFile(ctx context.Context, fileHash string) ([]ScoredPoint, error) {
	// Qdrant scroll request filtering by file_hash
	body := scrollRequest{
		Filter: filterClause{
			Must: []matchCondition{
				{
					Key:   "file_hash",
					Match: matchString{Value: fileHash},
				},
			},
		},
		Limit:       10000, // retrieve up to 10k chunks for a single file
		WithPayload: true,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling scroll request: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points/scroll", q.baseURL, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating scroll request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling scroll API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("scroll API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result scrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding scroll response: %w", err)
	}

	return result.Result.Points, nil
}
