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

func TestLocatorFor(t *testing.T) {
	cases := []struct {
		result SearchResult
		want   string
	}{
		// Byte offsets are what a chunk_id means now, for every file type.
		{SearchResult{ChunkID: "0-99", FileExt: ".pdf"}, "bajty 0–99"},
		{SearchResult{ChunkID: "1024-4096", FileExt: ".txt"}, "bajty 1024–4096"},
		// A recording locates by timecode, not by where its transcript sits.
		{SearchResult{ChunkID: "0-99", FileExt: ".mp4", Timecode: "01:05–02:05"}, "01:05–02:05"},
		{SearchResult{ChunkID: "not-a-chunk-id", FileExt: ".pdf"}, ""},
	}
	for _, c := range cases {
		if got := locatorFor(c.result); got != c.want {
			t.Errorf("locatorFor(%+v) = %q, want %q", c.result, got, c.want)
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
