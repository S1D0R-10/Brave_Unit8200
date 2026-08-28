package transcription

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleResult() Result {
	return Result{
		SchemaVersion:   "1.0",
		TranscriptionID: "job-1",
		Source:          SourceInfo{FileName: "wyklad.mp4", SizeBytes: 1234, DurationMS: 6000},
		Language:        "pl",
		Text:            "Dzień dobry. To jest test.",
		Segments: []Segment{
			{Index: 0, StartMS: 0, EndMS: 2500, Text: "Dzień dobry."},
			{Index: 1, StartMS: 2500, EndMS: 6000, Text: "To jest test."},
		},
		Engine:     EngineInfo{Name: "whisper.cpp", Version: "1.9.1", Model: "large-v3-turbo-q5_0"},
		Processing: ProcessingInfo{WallTimeMS: 3000, RealtimeFactor: 2},
		CreatedAt:  time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC),
	}
}

func TestWriteArtifactsCreatesMillisecondText(t *testing.T) {
	dir := t.TempDir()
	result := sampleResult()

	paths, err := WriteArtifactsAtomic(dir, result)
	if err != nil {
		t.Fatal(err)
	}

	if got := filepath.Base(paths.TXT); got != "wyklad-transcription.txt" {
		t.Fatalf("unexpected artifact name: %s", got)
	}
	if filepath.Dir(paths.TXT) != dir {
		t.Fatalf("artifact escaped output directory: %s", paths.TXT)
	}

	txt, _ := os.ReadFile(paths.TXT)
	if string(txt) != "[0 - 2500] Dzień dobry.\n[2500 - 6000] To jest test.\n" {
		t.Fatalf("unexpected TXT body: %q", txt)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one artifact, got %v", entries)
	}
}

func TestWriteArtifactsRejectsInvalidSegmentsWithoutPublishingFiles(t *testing.T) {
	dir := t.TempDir()
	result := sampleResult()
	result.Segments[1].StartMS = -1

	if _, err := WriteArtifactsAtomic(dir, result); err == nil {
		t.Fatal("expected invalid timestamp error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial artifacts were published: %v", entries)
	}
}
