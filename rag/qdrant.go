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

// EnsureCollection creates the given collection (with a tiny 2-dim vector,
// since it's only used as a JSON document store, not for similarity search)
// if it doesn't already exist.
func (q *QdrantClient) EnsureCollection(ctx context.Context, collection string) error {
	url := fmt.Sprintf("%s/collections/%s", q.baseURL, collection)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("checking collection: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     2,
			"distance": "Cosine",
		},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling create request: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("creating collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("creating collection %q: %d: %s", collection, resp.StatusCode, string(respBody))
	}

	q.logger.Printf("[qdrant] created collection %q", collection)
	return nil
}

// UpsertDoc stores a single JSON document (with a throwaway vector) in the
// given collection. Used for append-only stores like feedback/handoff, where
// we only ever need to write and later scroll through everything.
func (q *QdrantClient) UpsertDoc(ctx context.Context, collection string, id string, payload map[string]interface{}) error {
	body := map[string]interface{}{
		"points": []qdrantPoint{
			{ID: id, Vector: []float32{0, 0}, Payload: payload},
		},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling upsert: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points", q.baseURL, collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating upsert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upserting doc: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upsert failed: %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// qdrantPoint is a single point for the Qdrant upsert API.
type qdrantPoint struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

// ScrollCollection pages through every point in a collection, calling visit
// for each one, until exhausted or the collection doesn't exist yet.
func (q *QdrantClient) ScrollCollection(ctx context.Context, collection string, visit func(ScoredPoint)) error {
	var offset interface{}

	for {
		body := map[string]interface{}{
			"limit":        250,
			"with_payload": true,
			"with_vector":  false,
		}
		if offset != nil {
			body["offset"] = offset
		}

		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling scroll request: %w", err)
		}

		url := fmt.Sprintf("%s/collections/%s/points/scroll", q.baseURL, collection)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
		if err != nil {
			return fmt.Errorf("creating scroll request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := q.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("calling scroll API: %w", err)
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return nil // collection doesn't exist yet — nothing to scroll
		}
		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("scroll API returned %d: %s", resp.StatusCode, string(respBody))
		}

		var result struct {
			Result struct {
				Points         []ScoredPoint `json:"points"`
				NextPageOffset interface{}   `json:"next_page_offset"`
			} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return fmt.Errorf("decoding scroll response: %w", err)
		}
		resp.Body.Close()

		for _, p := range result.Result.Points {
			visit(p)
		}

		if result.Result.NextPageOffset == nil || len(result.Result.Points) == 0 {
			return nil
		}
		offset = result.Result.NextPageOffset
	}
}

// GetChunksByFileKey fetches all point metadata (payloads) for one text object.
// Grouping by file_key rather than file_hash matters because the byte offsets
// in a chunk_id are only meaningful against that exact object.
func (q *QdrantClient) GetChunksByFileKey(ctx context.Context, fileKey string) ([]ScoredPoint, error) {
	body := scrollRequest{
		Filter: filterClause{
			Must: []matchCondition{
				{
					Key:   "file_key",
					Match: matchString{Value: fileKey},
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
