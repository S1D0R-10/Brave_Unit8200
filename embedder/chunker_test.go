package main

import (
	"strings"
	"testing"
)

func TestChunkText_SentenceBoundary(t *testing.T) {
	// 3 sentences: 6 words, 5 words, 4 words = 15 words total.
	// With max 10 words per chunk, first two sentences (11 words) exceed limit,
	// so chunk 1 = sentence 1 (6 words), chunk 2 = sentences 2+3 (9 words).
	cfg := ChunkerConfig{WordsPerChunk: 10}
	c := NewChunker(cfg)

	data := []byte("This is the first sentence. Here is sentence two. And sentence three.")
	chunks, err := c.ChunkFile(data, ".txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// First chunk should contain the first sentence.
	if !strings.Contains(string(chunks[0].Content), "first sentence") {
		t.Errorf("chunk[0] should contain first sentence, got: %q", string(chunks[0].Content))
	}
}

func TestChunkText_SingleSentencePerChunk(t *testing.T) {
	cfg := ChunkerConfig{WordsPerChunk: 5}
	c := NewChunker(cfg)

	// Two sentences, each with ~4-6 words.
	data := []byte("Hello world today is great. Tomorrow will be better.")
	chunks, err := c.ChunkFile(data, ".txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should produce at least 2 chunks (one per sentence).
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestChunkText_Empty(t *testing.T) {
	c := NewChunker(DefaultChunkerConfig())
	chunks, err := c.ChunkFile([]byte(""), ".txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}

func TestChunkText_SingleWord(t *testing.T) {
	cfg := ChunkerConfig{WordsPerChunk: 500}
	c := NewChunker(cfg)

	chunks, err := c.ChunkFile([]byte("hello"), ".txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].ChunkID != "0-0" {
		t.Errorf("chunk id = %q, want %q", chunks[0].ChunkID, "0-0")
	}
}

func TestChunkText_MarkdownExtension(t *testing.T) {
	cfg := ChunkerConfig{WordsPerChunk: 5}
	c := NewChunker(cfg)

	data := []byte("# Hello World. Some text here.")
	chunks, err := c.ChunkFile(data, ".md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk for markdown")
	}
}

func TestChunkText_HTMLExtension(t *testing.T) {
	c := NewChunker(ChunkerConfig{WordsPerChunk: 10})
	_, err := c.ChunkFile([]byte("<html><body>test content here.</body></html>"), ".html")
	if err != nil {
		t.Fatalf("unexpected error for .html: %v", err)
	}
}

func TestChunkMedia_UsesSeconds(t *testing.T) {
	cfg := ChunkerConfig{SecsPerChunk: 300}
	c := NewChunker(cfg)

	// ~128kbps for 600 seconds = 16000 bytes/sec * 600 = 9,600,000 bytes
	data := make([]byte, 9600000)
	chunks, err := c.ChunkFile(data, ".mp4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 600 seconds / 300 per chunk = 2 chunks
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks for 600s media, got %d", len(chunks))
	}

	// First chunk should be 0-300 (seconds, not milliseconds).
	if chunks[0].Start != 0 {
		t.Errorf("first chunk start = %d, want 0", chunks[0].Start)
	}
	if chunks[0].End != 300 {
		t.Errorf("first chunk end = %d, want 300", chunks[0].End)
	}
	if chunks[0].ChunkID != "0-300" {
		t.Errorf("first chunk id = %q, want %q", chunks[0].ChunkID, "0-300")
	}

	// Content should be nil for media (metadata-only).
	if chunks[0].Content != nil {
		t.Errorf("media chunk content should be nil")
	}
}

func TestChunkFile_UnsupportedExtension(t *testing.T) {
	c := NewChunker(DefaultChunkerConfig())
	_, err := c.ChunkFile([]byte("data"), ".xyz")
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
}

func TestChunkText_Deterministic(t *testing.T) {
	cfg := ChunkerConfig{WordsPerChunk: 10}
	c := NewChunker(cfg)

	data := []byte("The quick brown fox jumps over the lazy dog. Today is a good day for coding.")

	chunks1, _ := c.ChunkFile(data, ".txt")
	chunks2, _ := c.ChunkFile(data, ".txt")

	if len(chunks1) != len(chunks2) {
		t.Fatalf("chunk count differs: %d vs %d", len(chunks1), len(chunks2))
	}

	for i := range chunks1 {
		if chunks1[i].ChunkID != chunks2[i].ChunkID {
			t.Errorf("chunk[%d] id not deterministic: %q vs %q", i, chunks1[i].ChunkID, chunks2[i].ChunkID)
		}
	}
}

func TestChunkText_LargeInput(t *testing.T) {
	cfg := ChunkerConfig{WordsPerChunk: 100}
	c := NewChunker(cfg)

	// Generate text with 10 sentences, each ~50 words.
	var sb strings.Builder
	for i := 0; i < 10; i++ {
		words := make([]string, 50)
		for j := range words {
			words[j] = "word"
		}
		sb.WriteString(strings.Join(words, " "))
		sb.WriteString(". ")
	}

	chunks, err := c.ChunkFile([]byte(sb.String()), ".txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 10 sentences × 50 words = 500 words. At 100 words per chunk,
	// each chunk gets 2 sentences (100 words) → 5 chunks.
	if len(chunks) != 5 {
		t.Errorf("expected 5 chunks, got %d", len(chunks))
	}
}

func TestSplitSentences(t *testing.T) {
	tests := []struct {
		input    string
		minCount int
	}{
		{"Hello world.", 1},
		{"First sentence. Second sentence.", 2},
		{"Question? Answer! Done.", 3},
		{"No punctuation", 1},
		{"", 0},
		{"One. Two. Three.", 3},
	}

	for _, tt := range tests {
		sentences := splitSentences(tt.input)
		if len(sentences) < tt.minCount {
			t.Errorf("splitSentences(%q) = %d sentences, want at least %d: %v",
				tt.input, len(sentences), tt.minCount, sentences)
		}
	}
}
