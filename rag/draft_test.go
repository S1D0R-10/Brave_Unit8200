package main

import "testing"

func TestIsCrisis(t *testing.T) {
	cases := []struct {
		question string
		want     bool
	}{
		{"Syn ma myśli samobójcze, co robić?", true},
		{"Córka się samookalecza", true},
		{"Nie chce już żyć, chce umrzeć", true},
		{"Jak rozmawiać o zmieniającym się ciele?", false},
		{"", false},
	}

	for _, c := range cases {
		if got := isCrisis(c.question); got != c.want {
			t.Errorf("isCrisis(%q) = %v, want %v", c.question, got, c.want)
		}
	}
}

func TestKindFromExt(t *testing.T) {
	cases := map[string]string{
		".pdf":  "PDF",
		".PDF":  "PDF",
		".mp3":  "WEBINAR",
		".mp4":  "WEBINAR",
		".txt":  "BLOG",
		".md":   "BLOG",
		".docx": "NEWSLETTER",
	}
	for ext, want := range cases {
		if got := kindFromExt(ext); got != want {
			t.Errorf("kindFromExt(%q) = %q, want %q", ext, got, want)
		}
	}
}

func TestLocatorFromChunk(t *testing.T) {
	cases := []struct {
		chunkID string
		ext     string
		want    string
	}{
		{"0-99", ".pdf", "znaki 0–99"},
		{"0-99", ".txt", "słowa 0–99"},
		{"65-125", ".mp3", "01:05–02:05"},
		{"not-a-chunk-id", ".pdf", ""},
	}
	for _, c := range cases {
		if got := locatorFromChunk(c.chunkID, c.ext); got != c.want {
			t.Errorf("locatorFromChunk(%q, %q) = %q, want %q", c.chunkID, c.ext, got, c.want)
		}
	}
}

func TestSourceIDStableAndUnique(t *testing.T) {
	a := sourceID(SearchResult{FileID: "hash1", ChunkID: "0-99"})
	b := sourceID(SearchResult{FileID: "hash1", ChunkID: "0-99"})
	c := sourceID(SearchResult{FileID: "hash1", ChunkID: "100-199"})

	if a != b {
		t.Errorf("sourceID should be deterministic: %q != %q", a, b)
	}
	if a == c {
		t.Errorf("sourceID should differ for different chunks: %q == %q", a, c)
	}
}

func TestNearMissesFromCapsAtTwo(t *testing.T) {
	hits := []SearchResult{
		{Title: "a.pdf", Score: 0.4},
		{Title: "b.pdf", Score: 0.3},
		{Title: "c.pdf", Score: 0.2},
	}

	got := nearMissesFrom(hits)

	if len(got) != 2 {
		t.Fatalf("expected 2 near misses, got %d", len(got))
	}
	if got[0].Title != "a.pdf" || got[0].Match != 40 {
		t.Errorf("unexpected first near miss: %+v", got[0])
	}
}
