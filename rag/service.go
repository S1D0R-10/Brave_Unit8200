package main

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
)

// transcriptSuffix marks a text object whose lines carry "[startMs - endMs]"
// prefixes, which have to come off before the text reaches the model.
const transcriptSuffix = "-transcription.txt"

// transcriptLine matches one line of an stt transcript artifact.
var transcriptLine = regexp.MustCompile(`^\[(\d+) - (\d+)\]\s?(.*)$`)

// Service orchestrates the RAG search pipeline.
//
// Chunk text is not stored in Qdrant: the payload only says which bucket object
// a chunk lives in and which bytes it spans, so a hit is turned back into text
// with an HTTP Range read. Bucket → backend traffic is free, the index stays
// small, and a citation can never drift out of sync with its source.
type Service struct {
	logger        *log.Logger
	embedder      *Embedder
	qdrant        *QdrantClient
	storage       *S3Storage
	maxQuoteBytes int64
}

// ServiceConfig holds tuning knobs for retrieval.
type ServiceConfig struct {
	MaxQuoteBytes int64 // hard cap on how much text one citation may pull
}

// NewService creates a new RAG search service.
func NewService(logger *log.Logger, embedder *Embedder, qdrant *QdrantClient, storage *S3Storage, cfg ServiceConfig) *Service {
	if logger == nil {
		logger = log.Default()
	}
	if cfg.MaxQuoteBytes <= 0 {
		cfg.MaxQuoteBytes = 8000
	}
	return &Service{
		logger:        logger,
		embedder:      embedder,
		qdrant:        qdrant,
		storage:       storage,
		maxQuoteBytes: cfg.MaxQuoteBytes,
	}
}

// SearchResult represents the final structured response.
type SearchResult struct {
	ChunkID      string   `json:"chunk_id"`
	FileID       string   `json:"file_id"`  // mapped to file_hash
	FileKey      string   `json:"file_key"` // text object the excerpt was read from
	FileExt      string   `json:"file_ext"`
	Title        string   `json:"title"`              // source key: what the user uploaded
	Text         string   `json:"text"`               // excerpt, read back from the bucket
	Timecode     string   `json:"timecode,omitempty"` // "12:03–14:40" for recordings
	Score        float64  `json:"score"`
	AdjacentPrev []string `json:"adjacent_prev"` // list of chunk_ids before this one
	AdjacentNext []string `json:"adjacent_next"` // list of chunk_ids after this one
}

// Search embeds the prompt, finds the nearest chunks, and reads each one's text
// back out of the bucket. adjCount widens a hit by that many neighbouring
// chunks on either side — since neighbours are contiguous in the file, that
// costs nothing extra: it is the same single Range request, just wider.
func (s *Service) Search(ctx context.Context, prompt string, topK int, adjCount int) ([]SearchResult, error) {
	vector, err := s.embedder.Embed(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("embedding prompt: %w", err)
	}

	hits, err := s.qdrant.Search(ctx, vector, topK)
	if err != nil {
		return nil, fmt.Errorf("qdrant search: %w", err)
	}
	s.logger.Printf("Found %d hits for prompt", len(hits))

	// One scroll per file, reused across hits from the same document.
	chunksByFile := make(map[string][]ScoredPoint)
	// Byte windows already quoted per file, so two nearby hits in the same
	// document do not return the same paragraph twice.
	quoted := make(map[string][][2]int64)

	var results []SearchResult

	for _, hit := range hits {
		payload := hit.Payload
		if payload == nil {
			continue
		}

		fileHash, _ := payload["file_hash"].(string)
		fileExt, _ := payload["file_ext"].(string)
		chunkID, _ := payload["chunk_id"].(string)
		fileKey, _ := payload["file_key"].(string)

		title, _ := payload["source_key"].(string)
		if title == "" {
			// Points written before byte addressing carried the upload key
			// under "key" instead.
			title, _ = payload["key"].(string)
		}

		if fileHash == "" || chunkID == "" {
			s.logger.Printf("Warning: hit %s missing file_hash or chunk_id", hit.ID)
			continue
		}

		result := SearchResult{
			ChunkID:      chunkID,
			FileID:       fileHash,
			FileKey:      fileKey,
			FileExt:      fileExt,
			Title:        title,
			Score:        hit.Score,
			AdjacentPrev: []string{},
			AdjacentNext: []string{},
		}

		if startMS, ok := payloadInt(payload, "start_ms"); ok {
			endMS, _ := payloadInt(payload, "end_ms")
			result.Timecode = formatTimecode(startMS) + "–" + formatTimecode(endMS)
		}

		if fileKey == "" {
			// A stale point from before byte addressing. It may still carry an
			// inline excerpt; without one it is unusable until re-indexed.
			result.Text, _ = payload["text"].(string)
			if result.Text == "" {
				s.logger.Printf("Warning: hit %s has no file_key and no text — re-index it", hit.ID)
				continue
			}
			results = append(results, result)
			continue
		}

		start, end, err := DecodeChunkID(chunkID)
		if err != nil {
			s.logger.Printf("Warning: hit %s has unusable chunk_id %q: %v", hit.ID, chunkID, err)
			continue
		}

		siblings, ok := chunksByFile[fileKey]
		if !ok {
			siblings, err = s.qdrant.GetChunksByFileKey(ctx, fileKey)
			if err != nil {
				s.logger.Printf("Warning: failed to get chunks for %q: %v", fileKey, err)
				siblings = nil // continue anyway without adjacency
			}
			sortChunksByStart(siblings)
			chunksByFile[fileKey] = siblings
		}

		prev, next := findAdjacency(siblings, chunkID, adjCount)
		result.AdjacentPrev, result.AdjacentNext = prev, next

		start, end = widenRange(prev, next, start, end)
		if end-start+1 > s.maxQuoteBytes {
			end = start + s.maxQuoteBytes - 1
		}

		if overlapsQuoted(quoted[fileKey], start, end) {
			continue
		}

		text, err := s.readExcerpt(ctx, fileKey, start, end)
		if err != nil {
			s.logger.Printf("Warning: cannot read %s bytes=%d-%d: %v", fileKey, start, end, err)
			continue
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		quoted[fileKey] = append(quoted[fileKey], [2]int64{start, end})

		result.Text = text
		results = append(results, result)
	}

	return results, nil
}

// readExcerpt pulls a byte range out of the bucket and makes it readable.
func (s *Service) readExcerpt(ctx context.Context, fileKey string, start, end int64) (string, error) {
	data, err := s.storage.FetchRange(ctx, fileKey, start, end)
	if err != nil {
		return "", err
	}

	text := string(data)
	if strings.HasSuffix(strings.ToLower(fileKey), transcriptSuffix) {
		text = stripTranscriptTimestamps(text)
	}
	return strings.TrimSpace(text), nil
}

// widenRange grows [start, end] to also cover the given neighbouring chunks.
func widenRange(prev, next []string, start, end int64) (int64, int64) {
	if len(prev) > 0 {
		if s, _, err := DecodeChunkID(prev[0]); err == nil && s < start {
			start = s
		}
	}
	if len(next) > 0 {
		if _, e, err := DecodeChunkID(next[len(next)-1]); err == nil && e > end {
			end = e
		}
	}
	return start, end
}

// overlapsQuoted reports whether [start, end] intersects an already-used window.
func overlapsQuoted(windows [][2]int64, start, end int64) bool {
	for _, w := range windows {
		if start <= w[1] && end >= w[0] {
			return true
		}
	}
	return false
}

// stripTranscriptTimestamps removes "[startMs - endMs] " prefixes line by line.
func stripTranscriptTimestamps(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if match := transcriptLine.FindStringSubmatch(strings.TrimRight(line, "\r")); match != nil {
			line = match[3]
		}
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return strings.Join(out, " ")
}

// formatTimecode renders milliseconds as mm:ss, or h:mm:ss past an hour.
func formatTimecode(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	total := ms / 1000
	hours, minutes, seconds := total/3600, (total%3600)/60, total%60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// payloadInt reads a numeric payload field, which JSON hands back as float64.
func payloadInt(payload map[string]interface{}, key string) (int64, bool) {
	switch v := payload[key].(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	default:
		return 0, false
	}
}

// sortChunksByStart sorts Qdrant points by decoding their chunk_id payload.
func sortChunksByStart(chunks []ScoredPoint) {
	sort.Slice(chunks, func(i, j int) bool {
		id1, _ := chunks[i].Payload["chunk_id"].(string)
		id2, _ := chunks[j].Payload["chunk_id"].(string)

		start1, _, err1 := DecodeChunkID(id1)
		start2, _, err2 := DecodeChunkID(id2)

		// Fallback to string comparison if decode fails (shouldn't happen)
		if err1 != nil || err2 != nil {
			return id1 < id2
		}

		return start1 < start2
	})
}

// findAdjacency extracts `count` previous and `count` next chunk_ids from a sorted list.
func findAdjacency(sortedChunks []ScoredPoint, targetChunkID string, count int) (prev []string, next []string) {
	prev = make([]string, 0)
	next = make([]string, 0)

	if len(sortedChunks) == 0 || count <= 0 {
		return prev, next
	}

	// Find index of target chunk
	targetIdx := -1
	for i, chunk := range sortedChunks {
		if id, ok := chunk.Payload["chunk_id"].(string); ok && id == targetChunkID {
			targetIdx = i
			break
		}
	}

	if targetIdx == -1 {
		return prev, next
	}

	// Grab previous `count` items
	for i := targetIdx - count; i < targetIdx; i++ {
		if i >= 0 && i < len(sortedChunks) {
			if id, ok := sortedChunks[i].Payload["chunk_id"].(string); ok {
				prev = append(prev, id)
			}
		}
	}

	// Grab next `count` items
	for i := targetIdx + 1; i <= targetIdx+count; i++ {
		if i < len(sortedChunks) {
			if id, ok := sortedChunks[i].Payload["chunk_id"].(string); ok {
				next = append(next, id)
			}
		}
	}

	return prev, next
}
