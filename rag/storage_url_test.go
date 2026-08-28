package main

import (
	"log"
	"net/http"
	"testing"
)

// Keys come from user file names, so "#", spaces and non-ASCII bytes all occur.
// Sent raw, "#" turns the rest of the key into a URL fragment and the store
// sees a truncated key — Range reads of citations then fail with 403s.
func TestObjectURLEncodesSpecialKeyBytes(t *testing.T) {
	s := &S3Storage{endpoint: "https://store.example", bucket: "b", logger: log.Default()}

	url := s.objectURL("dir/Moduł#2 - Lekcja 4-extracted.txt")
	want := "https://store.example/b/dir/Modu%C5%82%232%20-%20Lekcja%204-extracted.txt"
	if url != want {
		t.Fatalf("objectURL = %q, want %q", url, want)
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	if req.URL.Fragment != "" {
		t.Fatalf("key leaked into fragment: %q", req.URL.Fragment)
	}
	if got, wantPath := req.URL.EscapedPath(), "/b/dir/Modu%C5%82%232%20-%20Lekcja%204-extracted.txt"; got != wantPath {
		t.Fatalf("EscapedPath = %q, want %q", got, wantPath)
	}
}
