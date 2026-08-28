package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Chunk is one addressable slice of a *text object* living in the bucket.
//
// Start/End are inclusive BYTE offsets into that object, and that is the whole
// point: rag-search never stores chunk text, it re-reads "bytes=Start-End" from
// the bucket with an HTTP Range request when a query hits this chunk.
type Chunk struct {
	// Text is what gets embedded. For a transcript chunk it is the spoken words
	// with the "[ms - ms]" prefixes stripped, so it deliberately differs from
	// the bytes the range covers. It is never persisted anywhere.
	Text    string
	Start   int64  // inclusive byte offset into the text object
	End     int64  // inclusive byte offset into the text object
	ChunkID string // "{Start}-{End}"

	// Timed is set for transcript chunks, where StartMS/EndMS carry the
	// position in the original recording so a citation can point at a moment.
	Timed   bool
	StartMS int64
	EndMS   int64
}

// ChunkerConfig holds chunking parameters per content kind.
type ChunkerConfig struct {
	WordsPerChunk int   // max words per chunk for prose
	SecsPerChunk  int64 // max wall-clock span per chunk for transcripts
}

// DefaultChunkerConfig returns sensible defaults.
func DefaultChunkerConfig() ChunkerConfig {
	return ChunkerConfig{
		WordsPerChunk: 500,
		SecsPerChunk:  300,
	}
}

// Chunker splits a text object into byte-addressable chunks.
type Chunker struct {
	config ChunkerConfig
}

// NewChunker creates a Chunker with the given config.
func NewChunker(cfg ChunkerConfig) *Chunker {
	return &Chunker{config: cfg}
}

// sentenceEnd matches sentence-ending punctuation followed by whitespace or EOF.
// Handles: ".", "?", "!", and their combinations ("..."), plus Polish quotation marks.
var sentenceEnd = regexp.MustCompile(`[.!?]+[""\)\]]*\s+|[.!?]+[""\)\]]*$`)

// transcriptLine matches one line of a "-transcription.txt" artifact produced
// by the stt service: "[startMs - endMs] spoken words".
var transcriptLine = regexp.MustCompile(`^\[(\d+) - (\d+)\]\s?(.*)$`)

// ChunkText splits prose into sentence-boundary-aware chunks of at most
// WordsPerChunk words. A sentence longer than the limit becomes its own chunk.
//
// Chunks never overlap and each addresses a contiguous byte range of data, so
// data[Start:End+1] is exactly the text the chunk stands for.
func (c *Chunker) ChunkText(data []byte) ([]Chunk, error) {
	if len(data) == 0 {
		return nil, nil
	}

	text := string(data)
	spans := splitSentenceSpans(text)
	if len(spans) == 0 {
		return nil, nil
	}

	maxWords := c.config.WordsPerChunk
	if maxWords <= 0 {
		maxWords = 500
	}

	var chunks []Chunk
	start, end, words := -1, -1, 0

	flush := func() {
		if start < 0 {
			return
		}
		chunks = append(chunks, byteChunk(text, start, end))
		start, end, words = -1, -1, 0
	}

	for _, s := range spans {
		n := countWords(text[s.start:s.end])

		// Adding this sentence would overflow the open chunk: close it first.
		if start >= 0 && words+n > maxWords {
			flush()
		}
		if start < 0 {
			start = s.start
		}
		end = s.end
		words += n

		// Chunk is full, or this single sentence already blew past the limit.
		if words >= maxWords {
			flush()
		}
	}
	flush()

	return chunks, nil
}

// ChunkTranscript splits an stt transcript into chunks spanning at most
// SecsPerChunk of wall-clock time. The byte range covers whole lines, timestamp
// prefixes included, so a Range read returns intact lines; the embedded text
// has the prefixes stripped.
func (c *Chunker) ChunkTranscript(data []byte) ([]Chunk, error) {
	if len(data) == 0 {
		return nil, nil
	}

	maxSpanMS := c.config.SecsPerChunk * 1000
	if maxSpanMS <= 0 {
		maxSpanMS = 300 * 1000
	}

	text := string(data)

	var (
		chunks  []Chunk
		spoken  []string
		start   = -1
		end     = -1
		startMS int64
		endMS   int64
		timed   int
	)

	flush := func() {
		if start < 0 {
			return
		}
		chunk := byteChunk(text, start, end)
		chunk.Text = strings.Join(spoken, " ")
		chunk.Timed = true
		chunk.StartMS = startMS
		chunk.EndMS = endMS
		chunks = append(chunks, chunk)
		spoken = nil
		start, end = -1, -1
	}

	for _, line := range lineSpans(text) {
		match := transcriptLine.FindStringSubmatch(text[line.start:line.end])
		if match == nil {
			// Not a timestamped line. Keep it inside the open chunk so the byte
			// range stays contiguous; skip it if nothing is open yet.
			if start >= 0 {
				end = line.end
			}
			continue
		}
		timed++

		lineStartMS, _ := strconv.ParseInt(match[1], 10, 64)
		lineEndMS, _ := strconv.ParseInt(match[2], 10, 64)

		if start >= 0 && lineEndMS-startMS > maxSpanMS {
			flush()
		}
		if start < 0 {
			start = line.start
			startMS = lineStartMS
		}
		end = line.end
		endMS = lineEndMS
		if words := strings.TrimSpace(match[3]); words != "" {
			spoken = append(spoken, words)
		}
	}
	flush()

	if timed == 0 {
		return nil, fmt.Errorf("no timestamped segments found in transcript")
	}

	return chunks, nil
}

// StripTranscriptTimestamps removes the "[startMs - endMs] " prefix from every
// line, turning a raw transcript range back into readable prose.
func StripTranscriptTimestamps(text string) string {
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

// span is a half-open [start, end) byte range into a string.
type span struct {
	start int
	end   int
}

// byteChunk builds a Chunk covering text[start:end), with inclusive offsets.
func byteChunk(text string, start, end int) Chunk {
	s, e := int64(start), int64(end-1)
	return Chunk{
		Text:    text[start:end],
		Start:   s,
		End:     e,
		ChunkID: EncodeChunkID(s, e),
	}
}

// splitSentenceSpans locates sentence boundaries and returns their byte ranges
// with surrounding whitespace trimmed off. Whitespace between sentences belongs
// to no span, which is fine: a chunk runs from its first span's start to its
// last span's end, so interior gaps stay inside the range.
func splitSentenceSpans(text string) []span {
	var spans []span
	prev := 0
	for _, loc := range sentenceEnd.FindAllStringIndex(text, -1) {
		spans = appendTrimmed(spans, text, prev, loc[1])
		prev = loc[1]
	}
	if prev < len(text) {
		spans = appendTrimmed(spans, text, prev, len(text))
	}
	return spans
}

// lineSpans returns the byte range of every non-empty line, excluding its break.
func lineSpans(text string) []span {
	var spans []span
	start := 0
	for i := 0; i <= len(text); i++ {
		if i == len(text) || text[i] == '\n' {
			end := i
			if end > start && text[end-1] == '\r' {
				end--
			}
			if end > start {
				spans = append(spans, span{start, end})
			}
			start = i + 1
		}
	}
	return spans
}

// appendTrimmed appends text[start:end) with ASCII whitespace trimmed from both
// ends, dropping the span entirely if nothing is left.
func appendTrimmed(spans []span, text string, start, end int) []span {
	for start < end && isASCIISpace(text[start]) {
		start++
	}
	for end > start && isASCIISpace(text[end-1]) {
		end--
	}
	if start >= end {
		return spans
	}
	return append(spans, span{start, end})
}

func isASCIISpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}

// splitSentences splits text into sentences at sentence-ending punctuation.
// Preserves the punctuation with the sentence it ends.
// Returns non-empty trimmed sentences.
func splitSentences(text string) []string {
	// Find all sentence boundary positions.
	indices := sentenceEnd.FindAllStringIndex(text, -1)

	if len(indices) == 0 {
		// No sentence boundaries found — return whole text as one sentence.
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	}

	var sentences []string
	prev := 0
	for _, loc := range indices {
		// Include the punctuation, exclude the trailing whitespace.
		end := loc[1]
		sent := strings.TrimSpace(text[prev:end])
		if sent != "" {
			sentences = append(sentences, sent)
		}
		prev = end
	}

	// Remainder after last sentence boundary.
	if prev < len(text) {
		remainder := strings.TrimSpace(text[prev:])
		if remainder != "" {
			sentences = append(sentences, remainder)
		}
	}

	return sentences
}

// countWords returns the number of whitespace-delimited words in text.
func countWords(text string) int {
	return len(strings.Fields(text))
}

// extractPDFText does a best-effort text extraction from PDF data.
// Looks for text in BT/ET blocks, falls back to extracting printable sequences.
func extractPDFText(data []byte) string {
	content := string(data)

	var extracted strings.Builder
	for i := 0; i < len(content); {
		btIdx := strings.Index(content[i:], "BT")
		if btIdx == -1 {
			break
		}
		btIdx += i
		etIdx := strings.Index(content[btIdx:], "ET")
		if etIdx == -1 {
			break
		}
		etIdx += btIdx

		block := content[btIdx+2 : etIdx]
		text := extractTextOperators(block)
		if text != "" {
			extracted.WriteString(text)
			extracted.WriteString(" ")
		}
		i = etIdx + 2
	}

	if extracted.Len() > 0 {
		return strings.TrimSpace(extracted.String())
	}

	return extractPrintable(data)
}

// extractTextOperators pulls text from Tj and TJ PDF operators.
func extractTextOperators(block string) string {
	var result strings.Builder

	for i := 0; i < len(block); {
		lparen := strings.IndexByte(block[i:], '(')
		if lparen == -1 {
			break
		}
		lparen += i
		depth := 1
		j := lparen + 1
		for j < len(block) && depth > 0 {
			switch block[j] {
			case '\\':
				j++ // skip escaped char
			case '(':
				depth++
			case ')':
				depth--
			}
			j++
		}
		if depth == 0 {
			text := block[lparen+1 : j-1]
			result.WriteString(text)
		}
		i = j
	}

	return result.String()
}

// extractPrintable extracts sequences of printable UTF-8 characters.
func extractPrintable(data []byte) string {
	var result strings.Builder
	var current strings.Builder

	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r != utf8.RuneError && (r == ' ' || r == '\n' || r == '\t' || (r >= 0x20 && r < 0x7F) || r >= 0xA0) {
			current.WriteRune(r)
		} else {
			if current.Len() > 4 {
				result.WriteString(current.String())
				result.WriteString(" ")
			}
			current.Reset()
		}
		i += size
	}
	if current.Len() > 4 {
		result.WriteString(current.String())
	}

	return strings.TrimSpace(result.String())
}
