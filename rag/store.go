package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"time"
)

// Store persists feedback, handoff requests, and reports KB stats. It reuses
// the already-provisioned Qdrant instance as a plain JSON document store
// (separate collections, throwaway vectors) instead of standing up a
// dedicated database.
type Store struct {
	logger *log.Logger
	qdrant *QdrantClient
}

const (
	feedbackCollection  = "feedback"
	handoffCollection   = "handoff"
	citationsCollection = "citations"
)

func NewStore(logger *log.Logger, qdrant *QdrantClient) *Store {
	if logger == nil {
		logger = log.Default()
	}
	return &Store{logger: logger, qdrant: qdrant}
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SaveFeedback records a thumbs up/down on a drafted answer.
func (s *Store) SaveFeedback(ctx context.Context, answerID string, vote int) error {
	if err := s.qdrant.EnsureCollection(ctx, feedbackCollection); err != nil {
		return fmt.Errorf("ensure feedback collection: %w", err)
	}
	return s.qdrant.UpsertDoc(ctx, feedbackCollection, randomID(), map[string]interface{}{
		"answer_id":  answerID,
		"vote":       vote,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// SaveHandoff records a request to hand a question off to a human expert.
func (s *Store) SaveHandoff(ctx context.Context, answerID, question, to string, urgent bool) error {
	if err := s.qdrant.EnsureCollection(ctx, handoffCollection); err != nil {
		return fmt.Errorf("ensure handoff collection: %w", err)
	}
	return s.qdrant.UpsertDoc(ctx, handoffCollection, randomID(), map[string]interface{}{
		"answer_id":  answerID,
		"question":   question,
		"to":         to,
		"urgent":     urgent,
		"resolved":   false,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// KbStats reports how many distinct source documents are indexed and when
// the most recent one was ingested, computed by scanning the citations
// collection (there's no dedicated metadata table).
type KbStats struct {
	DocCount int    `json:"docCount"`
	SyncedAt string `json:"syncedAt"`
}

// KbFile is one distinct source document present in the index.
type KbFile struct {
	Key       string `json:"key"`
	IndexedAt string `json:"indexedAt,omitempty"`
}

// KbFiles lists the source documents actually present in the index, so the
// web app can show a truthful per-file status instead of assuming everything
// in the bucket has been indexed.
func (s *Store) KbFiles(ctx context.Context) ([]KbFile, error) {
	latest := make(map[string]time.Time)

	err := s.qdrant.ScrollCollection(ctx, citationsCollection, func(p ScoredPoint) {
		key, _ := p.Payload["source_key"].(string)
		if key == "" {
			// Points from before source_key existed: fall back to the text object.
			key, _ = p.Payload["file_key"].(string)
		}
		if key == "" {
			return
		}
		ts := latest[key]
		if raw, ok := p.Payload["indexed_at"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, raw); err == nil && parsed.After(ts) {
				ts = parsed
			}
		}
		latest[key] = ts
	})
	if err != nil {
		return nil, fmt.Errorf("scanning citations: %w", err)
	}

	files := make([]KbFile, 0, len(latest))
	for key, ts := range latest {
		file := KbFile{Key: key}
		if !ts.IsZero() {
			file.IndexedAt = ts.Format(time.RFC3339)
		}
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Key < files[j].Key })
	return files, nil
}

func (s *Store) KbStats(ctx context.Context) (*KbStats, error) {
	docs := make(map[string]struct{})
	var latest time.Time

	err := s.qdrant.ScrollCollection(ctx, citationsCollection, func(p ScoredPoint) {
		if hash, ok := p.Payload["file_hash"].(string); ok && hash != "" {
			docs[hash] = struct{}{}
		}
		if ts, ok := p.Payload["indexed_at"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, ts); err == nil && parsed.After(latest) {
				latest = parsed
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("scanning citations: %w", err)
	}

	stats := &KbStats{DocCount: len(docs)}
	if !latest.IsZero() {
		stats.SyncedAt = latest.Format(time.RFC3339)
	}
	return stats, nil
}
