package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

// crisisKeywords mirrors the keyword list the browser extension used to run
// client-side (src/lib/mock.ts). Moving it server-side means every draft
// request — not just the mocked ones — gets the same safety gate, and it's
// no longer bypassable by calling the API directly.
var crisisKeywords = []string{
	"samookalec",
	"samobój",
	"samobojs",
	"chce umrzeć",
	"chce umrzec",
}

// DraftService turns a raw question into a grounded, cited draft answer.
type DraftService struct {
	logger              *log.Logger
	search              *Service
	chat                *ChatClient
	noCoverageThreshold float64
	topK                int
}

func NewDraftService(logger *log.Logger, search *Service, chat *ChatClient, noCoverageThreshold float64, topK int) *DraftService {
	if logger == nil {
		logger = log.Default()
	}
	return &DraftService{
		logger:              logger,
		search:              search,
		chat:                chat,
		noCoverageThreshold: noCoverageThreshold,
		topK:                topK,
	}
}

// --- response types, matching extension/src/lib/types.ts ---

type DraftSentence struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	SourceID string `json:"sourceId,omitempty"`
	Weak     bool   `json:"weak"`
	Quote    string `json:"quote,omitempty"`
}

type DraftSource struct {
	ID       string `json:"id"`
	Num      int    `json:"num"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Locator  string `json:"locator"`
	DeepLink string `json:"deepLink"`
	Match    int    `json:"match"`

	// excerpt is the chunk text used to ground generation. Unexported so it
	// never leaves the backend in the JSON response.
	excerpt string
}

type DraftNearMiss struct {
	Title string `json:"title"`
	Match int    `json:"match"`
}

type DraftResult struct {
	Status     string          `json:"status"` // "answer" | "no_coverage" | "blocked"
	AnswerID   string          `json:"answerId,omitempty"`
	Sentences  []DraftSentence `json:"sentences,omitempty"`
	Sources    []DraftSource   `json:"sources,omitempty"`
	NearMisses []DraftNearMiss `json:"nearMisses,omitempty"`
}

func isCrisis(question string) bool {
	lower := strings.ToLower(question)
	for _, kw := range crisisKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// Draft runs the full pipeline: crisis gate -> vector search -> grounded
// generation.
func (d *DraftService) Draft(ctx context.Context, question string) (*DraftResult, error) {
	if isCrisis(question) {
		return &DraftResult{Status: "blocked"}, nil
	}

	hits, err := d.search.Search(ctx, question, d.topK, 0)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })

	var covered []SearchResult
	for _, h := range hits {
		if h.Score >= d.noCoverageThreshold && h.Text != "" {
			covered = append(covered, h)
		}
	}

	if len(covered) == 0 {
		return &DraftResult{Status: "no_coverage", NearMisses: nearMissesFrom(hits)}, nil
	}

	sources := buildSources(covered)
	sentences, sufficient, err := d.generate(ctx, question, sources)
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}

	if !sufficient || len(sentences) == 0 {
		return &DraftResult{Status: "no_coverage", NearMisses: nearMissesFrom(hits)}, nil
	}

	return &DraftResult{Status: "answer", AnswerID: randomID(), Sentences: sentences, Sources: sources}, nil
}

func nearMissesFrom(hits []SearchResult) []DraftNearMiss {
	var out []DraftNearMiss
	for _, h := range hits {
		if len(out) >= 2 {
			break
		}
		title := h.Title
		if title == "" {
			title = h.FileID
		}
		out = append(out, DraftNearMiss{Title: title, Match: int(h.Score * 100)})
	}
	return out
}

func buildSources(hits []SearchResult) []DraftSource {
	sources := make([]DraftSource, len(hits))
	for i, h := range hits {
		sources[i] = DraftSource{
			ID:       sourceID(h),
			Num:      i + 1,
			Kind:     kindFromExt(h.FileExt),
			Title:    h.Title,
			Locator:  locatorFor(h),
			DeepLink: "",
			Match:    int(h.Score * 100),
			excerpt:  h.Text,
		}
	}
	return sources
}

func sourceID(h SearchResult) string {
	sum := sha256.Sum256([]byte(h.FileID + ":" + h.ChunkID))
	return fmt.Sprintf("src-%x", sum[:6])
}

func kindFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".pdf":
		return "PDF"
	case ".mp4", ".mp3", ".wav", ".webm", ".ogg":
		return "WEBINAR"
	case ".txt", ".md", ".html", ".htm", ".blog":
		return "BLOG"
	default:
		return "NEWSLETTER"
	}
}

// locatorFor describes where in the source document a chunk sits. For a
// recording that is the timecode its transcript segment carries; for anything
// else it is the byte range the chunk actually occupies in the indexed text
// object — never a fabricated page number we don't actually know.
func locatorFor(h SearchResult) string {
	if h.Timecode != "" {
		return h.Timecode
	}
	start, end, err := DecodeChunkID(h.ChunkID)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("bajty %d–%d", start, end)
}

// --- LLM generation ---

type llmSentence struct {
	Text      string `json:"text"`
	SourceNum int    `json:"source_num"`
	Weak      bool   `json:"weak"`
	Quote     string `json:"quote"`
}

type llmResponse struct {
	Sufficient bool          `json:"sufficient"`
	Sentences  []llmSentence `json:"sentences"`
}

const draftSystemPrompt = `Jesteś asystentem, który pomaga rodzicom i nauczycielom odpowiadać na pytania, korzystając WYŁĄCZNIE z dostarczonych fragmentów źródeł. Zasady:
- Pisz po polsku, ciepłym, konkretnym tonem, bez pouczania.
- Każde zdanie odpowiedzi musi opierać się na jednym z ponumerowanych fragmentów źródeł. Podaj jego numer w "source_num".
- Dla każdego zdania, w polu "quote" zwróć dosłowny cytat z fragmentu źródłowego, który potwierdza to zdanie. Jeśli zdanie nie opiera się na konkretnym cytacie, zostaw puste.
- Jeśli jakieś zdanie jest ogólną radą, którą tylko luźno wspierają źródła, ustaw "weak": true.
- Jeśli fragmenty źródeł NIE zawierają wystarczających informacji, aby odpowiedzieć na pytanie, ustaw "sufficient": false i zwróć pustą listę zdań. Nie zmyślaj odpowiedzi.
- Odpowiedz WYŁĄCZNIE obiektem JSON w formacie: {"sufficient": true|false, "sentences": [{"text": "...", "source_num": 1, "weak": false, "quote": "..."}]}`

func (d *DraftService) generate(ctx context.Context, question string, sources []DraftSource) ([]DraftSentence, bool, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Pytanie: %s\n\nFragmenty źródeł:\n", question)
	for _, s := range sources {
		fmt.Fprintf(&b, "[%d] (%s, %s) %s\n\n", s.Num, s.Title, s.Kind, sourceText(s))
	}

	raw, err := d.chat.CompleteJSON(ctx, draftSystemPrompt, b.String())
	if err != nil {
		return nil, false, err
	}

	var parsed llmResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, false, fmt.Errorf("parsing model reply: %w (raw=%q)", err, raw)
	}

	if !parsed.Sufficient {
		return nil, false, nil
	}

	sentences := make([]DraftSentence, 0, len(parsed.Sentences))
	for i, s := range parsed.Sentences {
		var sourceID string
		if s.SourceNum >= 1 && s.SourceNum <= len(sources) {
			sourceID = sources[s.SourceNum-1].ID
		} else {
			s.Weak = true
		}
		sentences = append(sentences, DraftSentence{
			ID:       fmt.Sprintf("s%d", i+1),
			Text:     s.Text,
			SourceID: sourceID,
			Weak:     s.Weak,
			Quote:    s.Quote,
		})
	}

	return sentences, true, nil
}

// sourceText returns the chunk excerpt used to ground generation, capped so
// a handful of large chunks can't blow up the prompt.
func sourceText(s DraftSource) string {
	const maxChars = 1500
	if len(s.excerpt) > maxChars {
		return s.excerpt[:maxChars] + "…"
	}
	return s.excerpt
}
