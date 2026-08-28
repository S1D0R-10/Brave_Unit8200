package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"path/filepath"
	"strings"
)

const version = "0.1.0"

// Processor is the core RAG engine that orchestrates downloading, chunking,
// embedding, and storing file chunks.
type Processor struct {
	logger   *log.Logger
	storage  *S3Storage
	chunker  *Chunker
	embedder *Embedder
	store    VectorStore
}

// NewProcessor creates a new Processor instance.
func NewProcessor(logger *log.Logger, storage *S3Storage, chunker *Chunker, embedder *Embedder, store VectorStore) *Processor {
	if logger == nil {
		logger = log.Default()
	}
	return &Processor{
		logger:   logger,
		storage:  storage,
		chunker:  chunker,
		embedder: embedder,
		store:    store,
	}
}

// Version returns the current version string.
func (p *Processor) Version() string {
	return version
}

// Ping is a health-check method — returns true when the engine is alive.
func (p *Processor) Ping() bool {
	p.logger.Println("ping")
	return true
}

// ProcessFile downloads a file from S3, chunks it, generates embeddings,
// and stores vectors + metadata directly in the vector DB.
// No chunk content is returned or stored — only vectors and payload metadata.
func (p *Processor) ProcessFile(ctx context.Context, key string) error {
	// 1. Download from S3 (bucket → backend = free egress).
	data, err := p.storage.Download(ctx, key)
	if err != nil {
		return fmt.Errorf("download %q: %w", key, err)
	}

	// 2. Compute file hash.
	hash := sha256.Sum256(data)
	fileHash := fmt.Sprintf("sha256:%x", hash)

	// 3. Determine file extension.
	fileExt := strings.ToLower(filepath.Ext(key))
	if fileExt == "" {
		return fmt.Errorf("cannot determine file extension for %q", key)
	}

	// 4. Chunk the file.
	chunks, err := p.chunker.ChunkFile(data, fileExt)
	if err != nil {
		return fmt.Errorf("chunking %q: %w", key, err)
	}

	p.logger.Printf("chunked %q → %d chunks (hash=%s, ext=%s)", key, len(chunks), fileHash, fileExt)

	// 5. Collect chunk texts for embedding.
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		if c.Content != nil {
			texts[i] = string(c.Content)
		} else {
			// Media metadata-only chunks: use the chunk_id as a placeholder text.
			texts[i] = fmt.Sprintf("media segment %s of %s", c.ChunkID, key)
		}
	}

	// 6. Generate embeddings (batched to avoid API limits).
	vectors, err := p.embedder.EmbedBatched(ctx, texts, 64)
	if err != nil {
		return fmt.Errorf("embedding %q: %w", key, err)
	}

	// 7. Build records and store vectors + payload (including chunk text and
	//    source key) directly in Qdrant, so answers can be grounded and
	//    cited against the real source document.
	records := make([]VectorRecord, len(chunks))
	for i, c := range chunks {
		records[i] = VectorRecord{
			FileHash: fileHash,
			FileExt:  fileExt,
			ChunkID:  c.ChunkID,
			Key:      key,
			Text:     texts[i],
			Vector:   vectors[i],
		}
	}

	if err := p.store.EnsureCollection(ctx); err != nil {
		return fmt.Errorf("ensure collection: %w", err)
	}

	if err := p.store.SaveChunks(ctx, records); err != nil {
		return fmt.Errorf("save chunks: %w", err)
	}

	p.logger.Printf("stored %d vectors for %q in Qdrant", len(records), key)
	return nil
}
