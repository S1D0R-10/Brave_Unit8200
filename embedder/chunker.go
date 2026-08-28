package main

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Chunk represents a single segment of a file.
type Chunk struct {
	Content []byte // raw chunk content (may be empty for media metadata-only chunks)
	Start   int64  // start offset (word index, second, or char offset depending on ext)
	End     int64  // end offset (inclusive)
	ChunkID string // encoded "{start}-{end}"
}

// ChunkerConfig holds chunking parameters per file type.
type ChunkerConfig struct {
	WordsPerChunk int   // max words per chunk for text files (.txt, .md, .html)
	SecsPerChunk  int64 // max seconds per chunk for media files (.mp4, .mp3, .wav)
	CharsPerChunk int   // max chars per chunk for PDF files
}

// DefaultChunkerConfig returns sensible defaults.
func DefaultChunkerConfig() ChunkerConfig {
	return ChunkerConfig{
		WordsPerChunk: 500,
		SecsPerChunk:  300,
		CharsPerChunk: 3000,
	}
}

// Chunker splits file content into chunks based on file extension.
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

// ChunkFile splits data into chunks appropriate for the given file extension.
// The extension should include the leading dot (e.g. ".txt", ".pdf", ".mp4").
func (c *Chunker) ChunkFile(data []byte, ext string) ([]Chunk, error) {
	ext = strings.ToLower(ext)

	switch ext {
	case ".txt", ".md", ".html", ".htm", ".blog":
		return c.chunkText(data)
	case ".pdf":
		return c.chunkPDF(data)
	case ".mp4", ".mp3", ".wav", ".webm", ".ogg":
		return c.chunkMedia(data)
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// chunkText splits text into sentence-boundary-aware chunks.
// Each chunk contains the maximum number of full sentences that fit
// within WordsPerChunk words.
// Start/End in the chunk_id represent word indices.
func (c *Chunker) chunkText(data []byte) ([]Chunk, error) {
	if len(data) == 0 {
		return nil, nil
	}

	sentences := splitSentences(string(data))
	if len(sentences) == 0 {
		return nil, nil
	}

	maxWords := c.config.WordsPerChunk
	if maxWords <= 0 {
		maxWords = 500
	}

	var chunks []Chunk
	wordOffset := int64(0) // running word index

	var currentSentences []string
	currentWordCount := 0

	for _, sent := range sentences {
		sentWords := countWords(sent)

		// If a single sentence exceeds maxWords, it becomes its own chunk.
		if sentWords > maxWords && len(currentSentences) == 0 {
			content := strings.TrimSpace(sent)
			startIdx := wordOffset
			endIdx := wordOffset + int64(sentWords) - 1

			chunks = append(chunks, Chunk{
				Content: []byte(content),
				Start:   startIdx,
				End:     endIdx,
				ChunkID: EncodeChunkID(startIdx, endIdx),
			})

			wordOffset += int64(sentWords)
			continue
		}

		// Would adding this sentence exceed the limit?
		if currentWordCount+sentWords > maxWords && len(currentSentences) > 0 {
			// Flush current chunk.
			content := strings.TrimSpace(strings.Join(currentSentences, " "))
			startIdx := wordOffset - int64(currentWordCount)
			endIdx := wordOffset - 1

			chunks = append(chunks, Chunk{
				Content: []byte(content),
				Start:   startIdx,
				End:     endIdx,
				ChunkID: EncodeChunkID(startIdx, endIdx),
			})

			currentSentences = nil
			currentWordCount = 0
		}

		currentSentences = append(currentSentences, strings.TrimSpace(sent))
		currentWordCount += sentWords
		wordOffset += int64(sentWords)
	}

	// Flush remaining sentences.
	if len(currentSentences) > 0 {
		content := strings.TrimSpace(strings.Join(currentSentences, " "))
		startIdx := wordOffset - int64(currentWordCount)
		endIdx := wordOffset - 1

		chunks = append(chunks, Chunk{
			Content: []byte(content),
			Start:   startIdx,
			End:     endIdx,
			ChunkID: EncodeChunkID(startIdx, endIdx),
		})
	}

	return chunks, nil
}

// chunkPDF extracts text from PDF data and chunks by character offsets
// using sentence boundaries.
func (c *Chunker) chunkPDF(data []byte) ([]Chunk, error) {
	text := extractPDFText(data)
	if len(text) == 0 {
		return nil, fmt.Errorf("no text content extracted from PDF")
	}

	maxChars := c.config.CharsPerChunk
	if maxChars <= 0 {
		maxChars = 3000
	}

	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return nil, fmt.Errorf("no sentences found in PDF text")
	}

	var chunks []Chunk
	charOffset := int64(0)

	var currentSentences []string
	currentCharCount := 0

	for _, sent := range sentences {
		sentChars := utf8.RuneCountInString(sent)

		// Single sentence exceeding limit gets its own chunk.
		if sentChars > maxChars && len(currentSentences) == 0 {
			content := strings.TrimSpace(sent)
			startIdx := charOffset
			endIdx := charOffset + int64(sentChars) - 1

			chunks = append(chunks, Chunk{
				Content: []byte(content),
				Start:   startIdx,
				End:     endIdx,
				ChunkID: EncodeChunkID(startIdx, endIdx),
			})

			charOffset += int64(sentChars)
			continue
		}

		if currentCharCount+sentChars > maxChars && len(currentSentences) > 0 {
			content := strings.TrimSpace(strings.Join(currentSentences, " "))
			startIdx := charOffset - int64(currentCharCount)
			endIdx := charOffset - 1

			chunks = append(chunks, Chunk{
				Content: []byte(content),
				Start:   startIdx,
				End:     endIdx,
				ChunkID: EncodeChunkID(startIdx, endIdx),
			})

			currentSentences = nil
			currentCharCount = 0
		}

		currentSentences = append(currentSentences, strings.TrimSpace(sent))
		currentCharCount += sentChars
		charOffset += int64(sentChars)
	}

	if len(currentSentences) > 0 {
		content := strings.TrimSpace(strings.Join(currentSentences, " "))
		startIdx := charOffset - int64(currentCharCount)
		endIdx := charOffset - 1

		chunks = append(chunks, Chunk{
			Content: []byte(content),
			Start:   startIdx,
			End:     endIdx,
			ChunkID: EncodeChunkID(startIdx, endIdx),
		})
	}

	return chunks, nil
}

// chunkMedia generates metadata-only chunks for media files.
// Start/End in the chunk_id represent seconds.
//
// Actual content extraction requires ffprobe/ffmpeg (future work).
// For now, duration is estimated from file size (~128 kbps average).
func (c *Chunker) chunkMedia(data []byte) ([]Chunk, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty media file")
	}

	// Rough estimate: ~128 kbps average bitrate → 16 bytes/ms → 16000 bytes/s.
	bytesPerSec := 16000.0
	estimatedDurationSecs := int64(float64(len(data)) / bytesPerSec)

	secsPerChunk := c.config.SecsPerChunk
	if secsPerChunk <= 0 {
		secsPerChunk = 300
	}

	if estimatedDurationSecs <= 0 {
		estimatedDurationSecs = secsPerChunk
	}

	var chunks []Chunk
	for start := int64(0); start < estimatedDurationSecs; start += secsPerChunk {
		end := start + secsPerChunk
		if end > estimatedDurationSecs {
			end = estimatedDurationSecs
		}

		chunks = append(chunks, Chunk{
			Content: nil, // metadata-only — no actual content extraction
			Start:   start,
			End:     end,
			ChunkID: EncodeChunkID(start, end),
		})
	}

	return chunks, nil
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
