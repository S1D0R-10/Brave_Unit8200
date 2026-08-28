package main

import (
	"strings"
	"testing"
)

// Extracted PDF text often has no sentence punctuation at all; every chunk must
// still respect the byte cap or the embedding API rejects the whole document.
func TestChunkTextEnforcesByteCapWithoutPunctuation(t *testing.T) {
	cfg := DefaultChunkerConfig()
	cfg.MaxBytesPerChunk = 1000
	chunker := NewChunker(cfg)

	var b strings.Builder
	for b.Len() < 50_000 {
		b.WriteString("wyraz ")
	}
	data := []byte(b.String())

	chunks, err := chunker.ChunkText(data)
	if err != nil {
		t.Fatalf("ChunkText: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks, got none")
	}
	for i, ch := range chunks {
		size := ch.End - ch.Start + 1
		if size > int64(cfg.MaxBytesPerChunk) {
			t.Fatalf("chunk %d is %d bytes, cap is %d", i, size, cfg.MaxBytesPerChunk)
		}
		if got := string(data[ch.Start : ch.End+1]); got != ch.Text {
			t.Fatalf("chunk %d text does not match its byte range", i)
		}
	}
}

// A single run without any whitespace (e.g. binary junk that survived
// extraction) must be cut at rune boundaries rather than sent whole.
func TestChunkTextByteCapOnSpacelessRun(t *testing.T) {
	cfg := DefaultChunkerConfig()
	cfg.MaxBytesPerChunk = 999 // deliberately not a multiple of the rune size
	chunker := NewChunker(cfg)

	data := []byte(strings.Repeat("ł", 10_000)) // 2 bytes per rune
	chunks, err := chunker.ChunkText(data)
	if err != nil {
		t.Fatalf("ChunkText: %v", err)
	}
	var covered int64
	for i, ch := range chunks {
		size := ch.End - ch.Start + 1
		if size > int64(cfg.MaxBytesPerChunk) {
			t.Fatalf("chunk %d is %d bytes, cap is %d", i, size, cfg.MaxBytesPerChunk)
		}
		if !strings.HasPrefix(ch.Text, "ł") || !strings.HasSuffix(ch.Text, "ł") {
			t.Fatalf("chunk %d was cut mid-rune", i)
		}
		covered += size
	}
	if covered != int64(len(data)) {
		t.Fatalf("chunks cover %d bytes, want %d", covered, len(data))
	}
}
