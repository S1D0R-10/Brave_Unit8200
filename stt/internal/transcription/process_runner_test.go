package transcription

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeBackend struct {
	mu             sync.Mutex
	extractLimits  []time.Duration
	transcriptions int
}

func (b *fakeBackend) Probe(context.Context, string, string, int64) (SourceInfo, error) {
	return SourceInfo{FileName: "film.mp4", SizeBytes: 100, DurationMS: 600000}, nil
}

func (b *fakeBackend) Extract(_ context.Context, _ string, destination string, limit time.Duration) error {
	b.mu.Lock()
	b.extractLimits = append(b.extractLimits, limit)
	b.mu.Unlock()
	return os.WriteFile(destination, []byte("wav"), 0o600)
}

func (b *fakeBackend) Transcribe(_ context.Context, _ string, outputBase string, onProgress func(int)) (BackendTranscription, error) {
	b.mu.Lock()
	b.transcriptions++
	call := b.transcriptions
	b.mu.Unlock()
	onProgress(25)
	onProgress(75)
	payload := `{"result":{"language":"pl"},"transcription":[{"offsets":{"from":0,"to":2000},"text":" Test."}]}`
	path := outputBase + ".json"
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		return BackendTranscription{}, err
	}
	peak := int64(3100)
	if call > 1 {
		peak = 3200
	}
	return BackendTranscription{JSONPath: path, PeakGPUMemoryMiB: peak}, nil
}

func (b *fakeBackend) GPU(context.Context) (GPUInfo, error) {
	return GPUInfo{Name: "RTX", TotalMemoryMiB: 6144, UsedMemoryMiB: 1400}, nil
}

func TestProcessRunnerCalibratesOnceAndWritesArtifacts(t *testing.T) {
	backend := &fakeBackend{}
	calibrated := make(chan CalibrationInfo, 1)
	runner := NewProcessRunner(backend, ProcessRunnerConfig{
		Model: "large-v3-turbo-q5_0", EngineVersion: "1.9.1", CalibrationDuration: 5 * time.Minute,
		MaxWorkers: 2, GPUHeadroomMiB: 1024,
	}, func(info CalibrationInfo) { calibrated <- info })

	root := t.TempDir()
	request := RunRequest{ID: "job-1", FileName: "film.mp4", SizeBytes: 100, SourcePath: "film.mp4", OutputDir: filepath.Join(root, "job-1")}
	var stages []Stage
	var initialEstimate *ProgressUpdate
	output, err := runner.Run(context.Background(), request, func(update ProgressUpdate) {
		stages = append(stages, update.Stage)
		if update.ETA != nil && update.RealtimeFactor != nil && initialEstimate == nil {
			copy := update
			initialEstimate = &copy
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	info := <-calibrated
	if info.SuggestedWorkers != 2 || info.RealtimeFactor <= 0 {
		t.Fatalf("unexpected calibration: %#v", info)
	}
	if _, err := os.Stat(output.Artifacts.TXT); err != nil {
		t.Fatalf("transcript not written: %v", err)
	}
	if output.Result.Source.DurationMS != 600000 || output.Result.Language != "pl" {
		t.Fatalf("unexpected result: %#v", output.Result)
	}
	if !containsStage(stages, StageCalibrating) || !containsStage(stages, StageWriting) {
		t.Fatalf("missing pipeline stages: %v", stages)
	}
	if initialEstimate == nil || *initialEstimate.ETA <= 0 || *initialEstimate.RealtimeFactor <= 0 {
		t.Fatalf("calibration did not publish an initial ETA/RTF: %#v", initialEstimate)
	}

	request.ID = "job-2"
	request.OutputDir = filepath.Join(root, "job-2")
	if _, err := runner.Run(context.Background(), request, func(ProgressUpdate) {}); err != nil {
		t.Fatal(err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.extractLimits) != 3 || backend.extractLimits[0] != 5*time.Minute || backend.extractLimits[1] != 0 || backend.extractLimits[2] != 0 {
		t.Fatalf("calibration must happen once, extract calls: %v", backend.extractLimits)
	}
}

func containsStage(stages []Stage, wanted Stage) bool {
	for _, stage := range stages {
		if stage == wanted {
			return true
		}
	}
	return false
}
