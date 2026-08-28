package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// VectorRecord represents a single chunk vector to be stored.
// The chunk text and source key are persisted alongside the vector so the
// RAG search service can ground generated answers and citations in the
// original content.
type VectorRecord struct {
	FileHash string    // SHA-256 hash of the source file
	FileExt  string    // file extension with dot (e.g. ".pdf")
	ChunkID  string    // deterministic chunk ID ("{start}-{end}")
	Key      string    // S3 object key (source filename), used as the citation title
	Text     string    // chunk text content, used to ground generated answers
	Vector   []float32 // embedding vector
}

// VectorStore is the interface for persisting chunk vectors + metadata.
type VectorStore interface {
	// EnsureCollection creates the citations collection if it doesn't exist.
	EnsureCollection(ctx context.Context) error

	// SaveChunks persists a batch of vector records.
	SaveChunks(ctx context.Context, records []VectorRecord) error
}

// ---------------------------------------------------------------------------
// QdrantStore — Qdrant REST API client (port 6333, internal hostname)
// ---------------------------------------------------------------------------

// QdrantStore persists vectors + payload to Qdrant via its REST API.
type QdrantStore struct {
	baseURL    string
	collection string
	vectorDim  int
	httpClient *http.Client
	logger     *log.Logger
}

// QdrantConfig holds Qdrant connection settings.
type QdrantConfig struct {
	Host       string // e.g. "qdrant.railway.internal"
	Port       int    // e.g. 6333
	Collection string // e.g. "citations"
	VectorDim  int    // embedding dimension (e.g. 1536)
}

// NewQdrantStore creates a new QdrantStore.
func NewQdrantStore(cfg QdrantConfig, logger *log.Logger) *QdrantStore {
	if cfg.Collection == "" {
		cfg.Collection = "citations"
	}
	if cfg.Port == 0 {
		cfg.Port = 6333
	}
	if cfg.VectorDim == 0 {
		cfg.VectorDim = 1536
	}
	if logger == nil {
		logger = log.Default()
	}

	return &QdrantStore{
		baseURL:    fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port),
		collection: cfg.Collection,
		vectorDim:  cfg.VectorDim,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

// EnsureCollection creates the citations collection if it doesn't exist.
func (q *QdrantStore) EnsureCollection(ctx context.Context) error {
	url := fmt.Sprintf("%s/collections/%s", q.baseURL, q.collection)

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

	// Create collection with correct vector dimension.
	body := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     q.vectorDim,
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
		return fmt.Errorf("creating collection %q: %d: %s", q.collection, resp.StatusCode, string(respBody))
	}

	q.logger.Printf("[qdrant] created collection %q (dim=%d)", q.collection, q.vectorDim)
	return nil
}

// qdrantPoint is a single point for the Qdrant upsert API.
type qdrantPoint struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

// SaveChunks upserts vector records as points in the Qdrant collection.
// Payload contains file_hash, file_ext, chunk_id, the source key (title) and
// the chunk text, so search results can ground a generated answer and cite
// their real source.
func (q *QdrantStore) SaveChunks(ctx context.Context, records []VectorRecord) error {
	if len(records) == 0 {
		return nil
	}

	q.logger.Printf("[qdrant] upserting %d points to %q", len(records), q.collection)

	indexedAt := time.Now().UTC().Format(time.RFC3339)

	points := make([]qdrantPoint, len(records))
	for i, r := range records {
		// Deterministic point ID: hash of file_hash + chunk_id.
		pointID := sha256Hex([]byte(r.FileHash + ":" + r.ChunkID))

		points[i] = qdrantPoint{
			ID:     pointID,
			Vector: r.Vector,
			Payload: map[string]interface{}{
				"file_hash":  r.FileHash,
				"file_ext":   r.FileExt,
				"chunk_id":   r.ChunkID,
				"key":        r.Key,
				"text":       r.Text,
				"indexed_at": indexedAt,
			},
		}
	}

	body := map[string]interface{}{
		"points": points,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling upsert: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points", q.baseURL, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating upsert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upserting points: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upsert failed: %d: %s", resp.StatusCode, string(respBody))
	}

	q.logger.Printf("[qdrant] upserted %d points", len(records))
	return nil
}

// ---------------------------------------------------------------------------
// LogStore — stub for local dev (no Qdrant)
// ---------------------------------------------------------------------------

// LogStore logs operations to stdout instead of persisting.
type LogStore struct {
	logger *log.Logger
}

// NewLogStore creates a new LogStore.
func NewLogStore(logger *log.Logger) *LogStore {
	if logger == nil {
		logger = log.Default()
	}
	return &LogStore{logger: logger}
}

// EnsureCollection is a no-op.
func (s *LogStore) EnsureCollection(ctx context.Context) error {
	s.logger.Println("[stub] EnsureCollection('citations')")
	return nil
}

// SaveChunks logs metadata for each record.
func (s *LogStore) SaveChunks(ctx context.Context, records []VectorRecord) error {
	s.logger.Printf("[stub] SaveChunks: %d records", len(records))
	for i, r := range records {
		vecLen := 0
		if r.Vector != nil {
			vecLen = len(r.Vector)
		}
		s.logger.Printf("  [%d] hash=%s ext=%s chunk=%s vec_dim=%d",
			i, truncHash(r.FileHash), r.FileExt, r.ChunkID, vecLen)
	}
	return nil
}

func truncHash(h string) string {
	if len(h) > 20 {
		return h[:20] + "…"
	}
	return h
}
