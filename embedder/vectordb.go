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

// VectorRecord represents a single chunk vector to be stored.
// No chunk text is persisted — only the embedding vector and a metadata payload
// that tells rag-search which bytes of which object to re-read.
type VectorRecord struct {
	FileHash  string    // SHA-256 of the text object the offsets are valid against
	FileKey   string    // bucket key to Range-read, e.g. "film-transcription.txt"
	SourceKey string    // what the user uploaded, e.g. "film.mp4"
	FileExt   string    // extension of SourceKey, e.g. ".mp4"
	ChunkID   string    // deterministic chunk ID ("{startByte}-{endByte}")
	Timed     bool      // chunk came from a transcript, so StartMS/EndMS apply
	StartMS   int64     // position in the recording, milliseconds
	EndMS     int64     // position in the recording, milliseconds
	Vector    []float32 // embedding vector
}

// VectorStore is the interface for persisting chunk vectors + metadata.
type VectorStore interface {
	// EnsureCollection creates the citations collection if it doesn't exist.
	EnsureCollection(ctx context.Context) error

	// DeleteByFileKey removes every point belonging to a text object, so a
	// re-index cannot leave stale byte offsets behind.
	DeleteByFileKey(ctx context.Context, fileKey string) error

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

// DeleteByFileKey removes every point whose payload matches file_key.
func (q *QdrantStore) DeleteByFileKey(ctx context.Context, fileKey string) error {
	body := map[string]interface{}{
		"filter": map[string]interface{}{
			"must": []map[string]interface{}{
				{"key": "file_key", "match": map[string]interface{}{"value": fileKey}},
			},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling delete request: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points/delete?wait=true", q.baseURL, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating delete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("deleting points: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed: %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// qdrantPoint is a single point for the Qdrant upsert API.
type qdrantPoint struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

// SaveChunks upserts vector records as points in the Qdrant collection.
// The payload carries only metadata — file_key, chunk_id and friends — never
// chunk text: the text is re-read from the bucket by byte range at query time.
func (q *QdrantStore) SaveChunks(ctx context.Context, records []VectorRecord) error {
	if len(records) == 0 {
		return nil
	}

	q.logger.Printf("[qdrant] upserting %d points to %q", len(records), q.collection)

	points := make([]qdrantPoint, len(records))
	for i, r := range records {
		// Deterministic point ID derived from file_key + chunk_id. Qdrant only
		// accepts unsigned integers or UUIDs, so the digest is shaped into one.
		pointID := uuidFromHash(sha256Hex([]byte(r.FileKey + ":" + r.ChunkID)))

		payload := map[string]interface{}{
			"file_hash":  r.FileHash,
			"file_key":   r.FileKey,
			"source_key": r.SourceKey,
			"file_ext":   r.FileExt,
			"chunk_id":   r.ChunkID,
		}
		if r.Timed {
			payload["start_ms"] = r.StartMS
			payload["end_ms"] = r.EndMS
		}

		points[i] = qdrantPoint{
			ID:      pointID,
			Vector:  r.Vector,
			Payload: payload,
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

// uuidFromHash formats the first 16 bytes of a hex digest as a UUID string,
// which is one of the two point-ID shapes Qdrant accepts.
func uuidFromHash(hexDigest string) string {
	h := hexDigest[:32]
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
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

// DeleteByFileKey is a no-op.
func (s *LogStore) DeleteByFileKey(ctx context.Context, fileKey string) error {
	s.logger.Printf("[stub] DeleteByFileKey(%q)", fileKey)
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
		s.logger.Printf("  [%d] key=%s chunk=%s hash=%s vec_dim=%d",
			i, r.FileKey, r.ChunkID, truncHash(r.FileHash), vecLen)
	}
	return nil
}

func truncHash(h string) string {
	if len(h) > 20 {
		return h[:20] + "…"
	}
	return h
}
