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

// Service turns a prompt into a grounded answer: embed → search → read the
// cited bytes out of the bucket → ask the model.
type Service struct {
	logger        *log.Logger
	embedder      *Embedder
	qdrant        *QdrantClient
	storage       *S3Storage
	llm           *LLM
	maxQuoteBytes int64
}

// ServiceConfig holds tuning knobs for the answer pipeline.
type ServiceConfig struct {
	MaxQuoteBytes int64 // hard cap on how much text one citation may pull
}

// NewService creates a new RAG search service.
func NewService(logger *log.Logger, embedder *Embedder, qdrant *QdrantClient, storage *S3Storage, llm *LLM, cfg ServiceConfig) *Service {
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
		llm:           llm,
		maxQuoteBytes: cfg.MaxQuoteBytes,
	}
}

// Source is one citation backing the answer.
type Source struct {
	Ref       int     `json:"ref"`                // the [n] the answer refers to
	FileKey   string  `json:"file_key"`           // text object the quote came from
	SourceKey string  `json:"source_key"`         // what the user actually uploaded
	FileExt   string  `json:"file_ext"`           // extension of SourceKey
	ChunkID   string  `json:"chunk_id"`           // byte range that matched
	Score     float64 `json:"score"`              // vector similarity
	Quote     string  `json:"quote"`              // text read back from the bucket
	StartMS   *int64  `json:"start_ms,omitempty"` // position in a recording
	EndMS     *int64  `json:"end_ms,omitempty"`   // position in a recording
	Timecode  string  `json:"timecode,omitempty"` // human-readable "12:03–14:40"
}

// Answer is the full response: generated prose plus what it stands on.
type Answer struct {
	Answer  string   `json:"answer"`
	Sources []Source `json:"sources"`
}

// Answer runs the whole retrieval-augmented pipeline for one prompt.
func (s *Service) Answer(ctx context.Context, prompt string, topK, adjCount int) (Answer, error) {
	sources, err := s.retrieve(ctx, prompt, topK, adjCount)
	if err != nil {
		return Answer{}, err
	}
	if len(sources) == 0 {
		return Answer{Answer: "", Sources: []Source{}}, nil
	}

	if !s.llm.Configured() {
		s.logger.Println("warning: no LLM configured, returning citations without a generated answer")
		return Answer{Answer: "", Sources: sources}, nil
	}

	generated, err := s.llm.Complete(ctx, answerSystemPrompt, buildUserPrompt(prompt, sources))
	if err != nil {
		return Answer{}, fmt.Errorf("generating answer: %w", err)
	}

	return Answer{Answer: generated, Sources: sources}, nil
}

// retrieve finds the best-matching chunks and reads their text back out of the
// bucket, widening each hit by adjCount neighbouring chunks so the model sees
// the sentences around the match rather than a bare fragment.
func (s *Service) retrieve(ctx context.Context, prompt string, topK, adjCount int) ([]Source, error) {
	vector, err := s.embedder.Embed(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("embedding prompt: %w", err)
	}

	hits, err := s.qdrant.Search(ctx, vector, topK)
	if err != nil {
		return nil, fmt.Errorf("qdrant search: %w", err)
	}
	s.logger.Printf("found %d hits for prompt", len(hits))

	// One scroll per file, reused across hits from the same document.
	chunksByFile := make(map[string][]ScoredPoint)
	// Byte windows already quoted per file, so two nearby hits in the same
	// document do not send the model the same paragraph twice.
	quoted := make(map[string][][2]int64)

	sources := make([]Source, 0, len(hits))

	for _, hit := range hits {
		if hit.Payload == nil {
			continue
		}

		fileKey, _ := hit.Payload["file_key"].(string)
		chunkID, _ := hit.Payload["chunk_id"].(string)
		if fileKey == "" || chunkID == "" {
			s.logger.Printf("warning: hit %s has no file_key/chunk_id — reindex it", hit.ID)
			continue
		}

		start, end, err := DecodeChunkID(chunkID)
		if err != nil {
			s.logger.Printf("warning: hit %s has unusable chunk_id %q: %v", hit.ID, chunkID, err)
			continue
		}

		siblings, ok := chunksByFile[fileKey]
		if !ok {
			siblings, err = s.qdrant.GetChunksByFileKey(ctx, fileKey)
			if err != nil {
				s.logger.Printf("warning: no adjacency for %q: %v", fileKey, err)
				siblings = nil
			}
			sortChunksByStart(siblings)
			chunksByFile[fileKey] = siblings
		}

		// Adjacency is just a wider byte range: neighbours are contiguous in
		// the file, so one Range request covers the match and its context.
		start, end = widenRange(siblings, chunkID, adjCount, start, end)
		if span := end - start + 1; span > s.maxQuoteBytes {
			end = start + s.maxQuoteBytes - 1
		}

		if overlapsQuoted(quoted[fileKey], start, end) {
			continue
		}

		quote, err := s.readQuote(ctx, fileKey, start, end)
		if err != nil {
			s.logger.Printf("warning: cannot read %s bytes=%d-%d: %v", fileKey, start, end, err)
			continue
		}
		if strings.TrimSpace(quote) == "" {
			continue
		}
		quoted[fileKey] = append(quoted[fileKey], [2]int64{start, end})

		sourceKey, _ := hit.Payload["source_key"].(string)
		if sourceKey == "" {
			sourceKey = fileKey
		}
		fileExt, _ := hit.Payload["file_ext"].(string)

		source := Source{
			Ref:       len(sources) + 1,
			FileKey:   fileKey,
			SourceKey: sourceKey,
			FileExt:   fileExt,
			ChunkID:   chunkID,
			Score:     hit.Score,
			Quote:     quote,
		}

		if startMS, ok := payloadInt(hit.Payload, "start_ms"); ok {
			endMS, _ := payloadInt(hit.Payload, "end_ms")
			source.StartMS = &startMS
			source.EndMS = &endMS
			source.Timecode = formatTimecode(startMS) + "–" + formatTimecode(endMS)
		}

		sources = append(sources, source)
	}

	return sources, nil
}

// readQuote pulls a byte range out of the bucket and makes it readable.
func (s *Service) readQuote(ctx context.Context, fileKey string, start, end int64) (string, error) {
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

// widenRange grows [start, end] to also cover count chunks on either side.
func widenRange(sorted []ScoredPoint, chunkID string, count int, start, end int64) (int64, int64) {
	if count <= 0 || len(sorted) == 0 {
		return start, end
	}

	prev, next := findAdjacency(sorted, chunkID, count)
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

	if len(sortedChunks) == 0 {
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
