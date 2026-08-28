package main

import (
	"context"
	"fmt"
	"log"
	"sort"
)

// Service orchestrates the RAG search pipeline.
type Service struct {
	logger   *log.Logger
	embedder *Embedder
	qdrant   *QdrantClient
}

// NewService creates a new RAG search service.
func NewService(logger *log.Logger, embedder *Embedder, qdrant *QdrantClient) *Service {
	if logger == nil {
		logger = log.Default()
	}
	return &Service{
		logger:   logger,
		embedder: embedder,
		qdrant:   qdrant,
	}
}

// SearchResult represents the final structured response.
type SearchResult struct {
	ChunkID      string   `json:"chunk_id"`
	FileID       string   `json:"file_id"` // mapped to file_hash
	FileExt      string   `json:"file_ext"`
	Title        string   `json:"title"` // source key (filename), if the point was indexed with one
	Text         string   `json:"text"`  // chunk text, if the point was indexed with one
	Score        float64  `json:"score"`
	AdjacentPrev []string `json:"adjacent_prev"` // list of chunk_ids before this one
	AdjacentNext []string `json:"adjacent_next"` // list of chunk_ids after this one
}

// Search executes the vector search and resolves adjacent chunks.
func (s *Service) Search(ctx context.Context, prompt string, topK int, adjCount int) ([]SearchResult, error) {
	// 1. Embed the prompt
	vector, err := s.embedder.Embed(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("embedding prompt: %w", err)
	}

	// 2. Search Qdrant for top-K matches
	hits, err := s.qdrant.Search(ctx, vector, topK)
	if err != nil {
		return nil, fmt.Errorf("qdrant search: %w", err)
	}

	s.logger.Printf("Found %d hits for prompt", len(hits))

	// Cache chunks per file_hash to avoid redundant Qdrant scroll requests
	fileChunksCache := make(map[string][]ScoredPoint)
	var results []SearchResult

	for _, hit := range hits {
		payload := hit.Payload
		if payload == nil {
			continue
		}

		fileHash, _ := payload["file_hash"].(string)
		fileExt, _ := payload["file_ext"].(string)
		chunkID, _ := payload["chunk_id"].(string)
		title, _ := payload["key"].(string)
		text, _ := payload["text"].(string)

		if fileHash == "" || chunkID == "" {
			s.logger.Printf("Warning: hit %s missing file_hash or chunk_id", hit.ID)
			continue
		}

		// 3. Get all chunks for this file
		allChunks, ok := fileChunksCache[fileHash]
		if !ok {
			chunks, err := s.qdrant.GetChunksByFile(ctx, fileHash)
			if err != nil {
				s.logger.Printf("Warning: failed to get chunks for file %s: %v", fileHash, err)
				chunks = nil // continue anyway without adjacency
			}
			// Sort chunks by numeric start index
			sortChunksByStart(chunks)
			fileChunksCache[fileHash] = chunks
			allChunks = chunks
		}

		// 4. Find adjacent chunks
		prev, next := findAdjacency(allChunks, chunkID, adjCount)

		results = append(results, SearchResult{
			ChunkID:      chunkID,
			FileID:       fileHash,
			FileExt:      fileExt,
			Title:        title,
			Text:         text,
			Score:        hit.Score,
			AdjacentPrev: prev,
			AdjacentNext: next,
		})
	}

	return results, nil
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
