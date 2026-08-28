package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"path"
	"strings"
)

const version = "0.2.0"

// Suffixes of the derived text objects the pipeline keeps in the bucket.
const (
	transcriptSuffix = "-transcription.txt"
	extractedSuffix  = "-extracted.txt"
)

// mediaExts never get embedded directly — stt turns them into a transcript
// first, and the transcript is what this service indexes.
var mediaExts = map[string]bool{
	".mp4": true, ".mov": true, ".webm": true, ".mkv": true,
	".mp3": true, ".wav": true, ".m4a": true, ".ogg": true, ".flac": true,
}

// textExts can be chunked straight from the uploaded bytes.
var textExts = map[string]bool{
	".txt": true, ".md": true, ".html": true, ".htm": true, ".blog": true,
}

// Processor is the core ingest engine: it resolves an uploaded key to the text
// object that actually gets indexed, chunks it, embeds it, and stores vectors.
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

// ingestPlan says which object holds the indexable text for an uploaded key,
// and how that text has to be read.
type ingestPlan struct {
	sourceKey  string // what the user uploaded, kept for citations
	textKey    string // what gets chunked and later Range-read by rag-search
	sourceExt  string // extension of sourceKey, e.g. ".mp4"
	transcript bool   // textKey is an stt transcript, chunk it by timestamps
	extractPDF bool   // textKey does not exist yet; extract it from the PDF
}

// planFor maps an uploaded object key onto its text object.
//
//	notes.txt   → notes.txt                   (indexed as-is)
//	raport.pdf  → raport-extracted.txt        (extracted here, then uploaded)
//	film.mp4    → film-transcription.txt      (produced earlier by stt)
func planFor(key string) (ingestPlan, error) {
	ext := strings.ToLower(path.Ext(key))
	base := strings.TrimSuffix(key, path.Ext(key))

	switch {
	case ext == "":
		return ingestPlan{}, fmt.Errorf("cannot determine file extension for %q", key)

	// A transcript uploaded (or handed over) directly.
	case strings.HasSuffix(strings.ToLower(key), transcriptSuffix):
		return ingestPlan{sourceKey: key, textKey: key, sourceExt: ext, transcript: true}, nil

	case mediaExts[ext]:
		return ingestPlan{sourceKey: key, textKey: base + transcriptSuffix, sourceExt: ext, transcript: true}, nil

	case ext == ".pdf":
		return ingestPlan{sourceKey: key, textKey: base + extractedSuffix, sourceExt: ext, extractPDF: true}, nil

	case textExts[ext]:
		return ingestPlan{sourceKey: key, textKey: key, sourceExt: ext}, nil

	default:
		return ingestPlan{}, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// ProcessFile indexes an uploaded object: resolve → fetch text → chunk → embed
// → store. Only vectors and metadata are persisted; chunk text stays in the
// bucket and is re-read at query time via byte ranges.
func (p *Processor) ProcessFile(ctx context.Context, key string) error {
	plan, err := planFor(key)
	if err != nil {
		return err
	}

	textData, err := p.textObject(ctx, plan)
	if err != nil {
		return err
	}
	if len(textData) == 0 {
		return fmt.Errorf("text object %q is empty", plan.textKey)
	}

	// The hash identifies the text object we chunked, which is what the byte
	// offsets are valid against.
	sum := sha256.Sum256(textData)
	fileHash := fmt.Sprintf("sha256:%x", sum)

	chunks, err := p.chunk(textData, plan)
	if err != nil {
		return fmt.Errorf("chunking %q: %w", plan.textKey, err)
	}
	if len(chunks) == 0 {
		return fmt.Errorf("no chunks produced for %q", plan.textKey)
	}

	p.logger.Printf("chunked %q → %d chunks (source=%s hash=%s)",
		plan.textKey, len(chunks), plan.sourceKey, fileHash[:19])

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}

	vectors, err := p.embedder.EmbedBatched(ctx, texts, 64)
	if err != nil {
		return fmt.Errorf("embedding %q: %w", plan.textKey, err)
	}

	records := make([]VectorRecord, len(chunks))
	for i, c := range chunks {
		records[i] = VectorRecord{
			FileHash:  fileHash,
			FileKey:   plan.textKey,
			SourceKey: plan.sourceKey,
			FileExt:   plan.sourceExt,
			ChunkID:   c.ChunkID,
			Timed:     c.Timed,
			StartMS:   c.StartMS,
			EndMS:     c.EndMS,
			Vector:    vectors[i],
		}
	}

	if err := p.store.EnsureCollection(ctx); err != nil {
		return fmt.Errorf("ensure collection: %w", err)
	}

	// Re-indexing the same key invalidates the old byte offsets, so drop the
	// previous points before writing the new ones.
	if err := p.store.DeleteByFileKey(ctx, plan.textKey); err != nil {
		return fmt.Errorf("delete stale chunks for %q: %w", plan.textKey, err)
	}

	if err := p.store.SaveChunks(ctx, records); err != nil {
		return fmt.Errorf("save chunks: %w", err)
	}

	p.logger.Printf("stored %d vectors for %q", len(records), plan.textKey)
	return nil
}

// textObject returns the bytes that get chunked. For PDFs it extracts the text
// and pushes it back to the bucket as a sidecar, because a byte range over a
// raw PDF is meaningless — rag-search has to be able to Range-read plain text.
func (p *Processor) textObject(ctx context.Context, plan ingestPlan) ([]byte, error) {
	if !plan.extractPDF {
		data, err := p.storage.Download(ctx, plan.textKey)
		if err != nil {
			if plan.transcript && plan.textKey != plan.sourceKey {
				return nil, fmt.Errorf("transcript %q for %q not found — did stt run? %w",
					plan.textKey, plan.sourceKey, err)
			}
			return nil, fmt.Errorf("download %q: %w", plan.textKey, err)
		}
		return data, nil
	}

	raw, err := p.storage.Download(ctx, plan.sourceKey)
	if err != nil {
		return nil, fmt.Errorf("download %q: %w", plan.sourceKey, err)
	}

	text := strings.TrimSpace(extractPDFText(raw))
	if text == "" {
		return nil, fmt.Errorf("no text content extracted from %q", plan.sourceKey)
	}

	data := []byte(text)
	if err := p.storage.Upload(ctx, plan.textKey, data, "text/plain; charset=utf-8"); err != nil {
		return nil, fmt.Errorf("upload extracted text %q: %w", plan.textKey, err)
	}
	p.logger.Printf("extracted %d bytes of text from %q → %q", len(data), plan.sourceKey, plan.textKey)

	return data, nil
}

// chunk picks the chunking strategy for the resolved text object.
func (p *Processor) chunk(data []byte, plan ingestPlan) ([]Chunk, error) {
	if !plan.transcript {
		return p.chunker.ChunkText(data)
	}

	chunks, err := p.chunker.ChunkTranscript(data)
	if err == nil {
		return chunks, nil
	}

	// The file is named like a transcript but holds no timestamps. Index it as
	// prose rather than losing the document.
	p.logger.Printf("warning: %q has no timestamps (%v), falling back to plain text chunking", plan.textKey, err)
	return p.chunker.ChunkText(data)
}
