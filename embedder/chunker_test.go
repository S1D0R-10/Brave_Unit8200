package main

import (
	"strings"
	"testing"
)

// assertByteExact is the property the whole pipeline leans on: a chunk's byte
// range, read back from the object, must return that chunk's own bytes.
func assertByteExact(t *testing.T, data []byte, chunks []Chunk) {
	t.Helper()
	for i, c := range chunks {
		if c.Start < 0 || c.End < c.Start || c.End >= int64(len(data)) {
			t.Fatalf("chunk[%d] range %d-%d out of bounds for %d bytes", i, c.Start, c.End, len(data))
		}
		start, end, err := DecodeChunkID(c.ChunkID)
		if err != nil {
			t.Fatalf("chunk[%d] id %q does not decode: %v", i, c.ChunkID, err)
		}
		if start != c.Start || end != c.End {
			t.Errorf("chunk[%d] id %q disagrees with range %d-%d", i, c.ChunkID, c.Start, c.End)
		}
		if i > 0 && c.Start <= chunks[i-1].End {
			t.Errorf("chunk[%d] starts at %d, overlapping previous chunk ending at %d",
				i, c.Start, chunks[i-1].End)
		}
	}
}

func TestChunkText_SentenceBoundary(t *testing.T) {
	// 3 sentences: 5 words, 4 words, 3 words. With max 10 words per chunk the
	// first two fit together (9 words) and the third opens a new chunk.
	c := NewChunker(ChunkerConfig{WordsPerChunk: 10})

	data := []byte("This is the first sentence. Here is sentence two. And sentence three.")
	chunks, err := c.ChunkText(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
	assertByteExact(t, data, chunks)

	if !strings.Contains(chunks[0].Text, "first sentence") {
		t.Errorf("chunk[0] should contain the first sentence, got: %q", chunks[0].Text)
	}
}

func TestChunkText_RangeMatchesText(t *testing.T) {
	c := NewChunker(ChunkerConfig{WordsPerChunk: 6})

	data := []byte("Ala ma kota. Kot ma Ale. Oboje maja sie dobrze.")
	chunks, err := c.ChunkText(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertByteExact(t, data, chunks)

	for i, chunk := range chunks {
		// This is what rag-search does with a Range response.
		fromBucket := string(data[chunk.Start : chunk.End+1])
		if fromBucket != chunk.Text {
			t.Errorf("chunk[%d]: bytes %d-%d read back as %q, want %q",
				i, chunk.Start, chunk.End, fromBucket, chunk.Text)
		}
	}
}

func TestChunkText_Empty(t *testing.T) {
	c := NewChunker(DefaultChunkerConfig())
	chunks, err := c.ChunkText([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}

func TestChunkText_SingleWord(t *testing.T) {
	c := NewChunker(ChunkerConfig{WordsPerChunk: 500})
	data := []byte("Hello")
	chunks, err := c.ChunkText(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].ChunkID != "0-4" {
		t.Errorf("chunk id = %q, want %q", chunks[0].ChunkID, "0-4")
	}
}

func TestChunkText_LongSentenceGetsOwnChunk(t *testing.T) {
	c := NewChunker(ChunkerConfig{WordsPerChunk: 3})

	long := strings.Repeat("word ", 20) + "."
	data := []byte(long + " Short one.")
	chunks, err := c.ChunkText(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	assertByteExact(t, data, chunks)
}

func TestChunkText_Deterministic(t *testing.T) {
	c := NewChunker(ChunkerConfig{WordsPerChunk: 10})
	data := []byte("The quick brown fox jumps over the lazy dog. Today is a good day for coding.")

	first, _ := c.ChunkText(data)
	second, _ := c.ChunkText(data)

	if len(first) != len(second) {
		t.Fatalf("chunk count differs: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ChunkID != second[i].ChunkID {
			t.Errorf("chunk[%d] id not deterministic: %q vs %q", i, first[i].ChunkID, second[i].ChunkID)
		}
	}
}

func TestChunkText_LargeInput(t *testing.T) {
	c := NewChunker(ChunkerConfig{WordsPerChunk: 100})

	var sb strings.Builder
	for i := 0; i < 10; i++ {
		sb.WriteString(strings.TrimSpace(strings.Repeat("word ", 50)))
		sb.WriteString(". ")
	}
	data := []byte(sb.String())

	chunks, err := c.ChunkText(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 10 sentences x 50 words = 500 words, 100 per chunk => 5 chunks.
	if len(chunks) != 5 {
		t.Errorf("expected 5 chunks, got %d", len(chunks))
	}
	assertByteExact(t, data, chunks)
}

const sampleTranscript = "[0 - 4000] Dzien dobry, zaczynamy.\n" +
	"[4000 - 9000] Dzisiaj o retrieval augmented generation.\n" +
	"[9000 - 14000] Najpierw indeksujemy dokumenty.\n" +
	"[14000 - 20000] Potem szukamy wektorowo.\n"

func TestChunkTranscript_SplitsByTime(t *testing.T) {
	// 10s per chunk over a 20s transcript.
	c := NewChunker(ChunkerConfig{SecsPerChunk: 10})
	data := []byte(sampleTranscript)

	chunks, err := c.ChunkTranscript(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	assertByteExact(t, data, chunks)

	if !chunks[0].Timed {
		t.Error("transcript chunk should be marked as timed")
	}
	if chunks[0].StartMS != 0 || chunks[0].EndMS != 9000 {
		t.Errorf("chunk[0] span = %d-%d ms, want 0-9000", chunks[0].StartMS, chunks[0].EndMS)
	}
	if chunks[2].EndMS != 20000 {
		t.Errorf("last chunk should end at 20000 ms, got %d", chunks[2].EndMS)
	}
	// No chunk may cover more wall-clock time than the configured limit.
	for i, chunk := range chunks {
		if span := chunk.EndMS - chunk.StartMS; span > 10_000 {
			t.Errorf("chunk[%d] spans %d ms, over the 10000 ms limit", i, span)
		}
		if i > 0 && chunk.StartMS < chunks[i-1].EndMS {
			t.Errorf("chunk[%d] starts at %d ms, before the previous chunk ended at %d",
				i, chunk.StartMS, chunks[i-1].EndMS)
		}
	}
}

func TestChunkTranscript_EmbeddedTextDropsTimestamps(t *testing.T) {
	c := NewChunker(ChunkerConfig{SecsPerChunk: 300})
	chunks, err := c.ChunkTranscript([]byte(sampleTranscript))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if strings.Contains(chunks[0].Text, "[") {
		t.Errorf("embedded text still carries timestamps: %q", chunks[0].Text)
	}
	if !strings.HasPrefix(chunks[0].Text, "Dzien dobry") {
		t.Errorf("embedded text = %q, want it to start with the spoken words", chunks[0].Text)
	}
}

func TestChunkTranscript_RangeCoversWholeLines(t *testing.T) {
	c := NewChunker(ChunkerConfig{SecsPerChunk: 10})
	data := []byte(sampleTranscript)

	chunks, err := c.ChunkTranscript(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, chunk := range chunks {
		raw := string(data[chunk.Start : chunk.End+1])
		if !strings.HasPrefix(raw, "[") {
			t.Errorf("chunk[%d] range starts mid-line: %q", i, raw)
		}
		if strings.HasSuffix(raw, "\n") {
			t.Errorf("chunk[%d] range should stop before the line break: %q", i, raw)
		}
		if got := StripTranscriptTimestamps(raw); strings.Contains(got, "[") {
			t.Errorf("chunk[%d] stripped text still has a timestamp: %q", i, got)
		}
	}
}

func TestChunkTranscript_NoTimestamps(t *testing.T) {
	c := NewChunker(DefaultChunkerConfig())
	if _, err := c.ChunkTranscript([]byte("just prose, no segments at all")); err == nil {
		t.Fatal("expected an error for a transcript without timestamps")
	}
}

func TestStripTranscriptTimestamps(t *testing.T) {
	got := StripTranscriptTimestamps("[0 - 100] jeden\n[100 - 200] dwa\n")
	if got != "jeden dwa" {
		t.Errorf("StripTranscriptTimestamps = %q, want %q", got, "jeden dwa")
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

func TestPlanFor(t *testing.T) {
	tests := []struct {
		key        string
		textKey    string
		transcript bool
		extractPDF bool
		wantErr    bool
	}{
		{key: "notes.txt", textKey: "notes.txt"},
		{key: "post.md", textKey: "post.md"},
		{key: "raport.pdf", textKey: "raport-extracted.txt", extractPDF: true},
		{key: "film.mp4", textKey: "film-transcription.txt", transcript: true},
		{key: "wyklad.MP4", textKey: "wyklad-transcription.txt", transcript: true},
		{key: "film-transcription.txt", textKey: "film-transcription.txt", transcript: true},
		{key: "archiwum.zip", wantErr: true},
		{key: "noext", wantErr: true},
	}

	for _, tt := range tests {
		plan, err := planFor(tt.key)
		if tt.wantErr {
			if err == nil {
				t.Errorf("planFor(%q) should have failed", tt.key)
			}
			continue
		}
		if err != nil {
			t.Errorf("planFor(%q): unexpected error: %v", tt.key, err)
			continue
		}
		if plan.textKey != tt.textKey {
			t.Errorf("planFor(%q).textKey = %q, want %q", tt.key, plan.textKey, tt.textKey)
		}
		if plan.transcript != tt.transcript {
			t.Errorf("planFor(%q).transcript = %v, want %v", tt.key, plan.transcript, tt.transcript)
		}
		if plan.extractPDF != tt.extractPDF {
			t.Errorf("planFor(%q).extractPDF = %v, want %v", tt.key, plan.extractPDF, tt.extractPDF)
		}
		if plan.sourceKey != tt.key {
			t.Errorf("planFor(%q).sourceKey = %q, want the key itself", tt.key, plan.sourceKey)
		}
	}
}
