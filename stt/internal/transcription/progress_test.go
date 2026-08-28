package transcription

import (
	"testing"
	"time"
)

func TestParseWhisperProgress(t *testing.T) {
	percent, ok := ParseWhisperProgress("whisper_print_progress_callback: progress =  37%")
	if !ok || percent != 37 {
		t.Fatalf("ParseWhisperProgress() = %d, %v; want 37, true", percent, ok)
	}

	if _, ok := ParseWhisperProgress("whisper_model_load: loading model"); ok {
		t.Fatal("non-progress line was accepted")
	}
}

func TestProgressEstimatorWaitsForStableSample(t *testing.T) {
	estimator := NewProgressEstimator(2*time.Second, 2)
	start := time.Unix(100, 0)

	first := estimator.Observe(start.Add(time.Second), 1, 120*time.Minute)
	if first.ETA != nil || first.RealtimeFactor != nil {
		t.Fatalf("first observation must not expose an estimate: %#v", first)
	}

	second := estimator.Observe(start.Add(3*time.Second), 10, 120*time.Minute)
	if second.ETA == nil || *second.ETA <= 0 {
		t.Fatalf("expected positive ETA, got %#v", second.ETA)
	}
	if second.RealtimeFactor == nil || *second.RealtimeFactor <= 0 {
		t.Fatalf("expected positive realtime factor, got %#v", second.RealtimeFactor)
	}
}

func TestRealtimeFactorUsesMilliseconds(t *testing.T) {
	if got := RealtimeFactor(600_000, 5*time.Minute); got != 2 {
		t.Fatalf("RealtimeFactor() = %v; want 2", got)
	}
}
