package transcription

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type GPUInfo struct {
	Name           string `json:"name"`
	TotalMemoryMiB int64  `json:"totalMemoryMiB"`
	UsedMemoryMiB  int64  `json:"usedMemoryMiB"`
}

type BackendTranscription struct {
	JSONPath         string
	PeakGPUMemoryMiB int64
}

type Backend interface {
	Probe(context.Context, string, string, int64) (SourceInfo, error)
	Extract(context.Context, string, string, time.Duration) error
	Transcribe(context.Context, string, string, func(int)) (BackendTranscription, error)
	GPU(context.Context) (GPUInfo, error)
}

type ProcessRunnerConfig struct {
	Model               string
	EngineVersion       string
	CalibrationDuration time.Duration
	MaxWorkers          int
	GPUHeadroomMiB      int64
}

type CalibrationInfo struct {
	SuggestedWorkers int     `json:"suggestedWorkers"`
	RealtimeFactor   float64 `json:"realtimeFactor"`
	BaselineVRAMMiB  int64   `json:"baselineVramMiB"`
	PeakVRAMMiB      int64   `json:"peakVramMiB"`
}

type ProcessRunner struct {
	backend      Backend
	config       ProcessRunnerConfig
	onCalibrated func(CalibrationInfo)
	mu           sync.Mutex
	calibrated   bool
	calibration  CalibrationInfo
}

func NewProcessRunner(backend Backend, config ProcessRunnerConfig, onCalibrated func(CalibrationInfo)) *ProcessRunner {
	if config.Model == "" {
		config.Model = "large-v3-turbo-q5_0"
	}
	if config.EngineVersion == "" {
		config.EngineVersion = "1.9.1"
	}
	if config.CalibrationDuration <= 0 {
		config.CalibrationDuration = 5 * time.Minute
	}
	if config.MaxWorkers < 1 {
		config.MaxWorkers = 1
	}
	if config.GPUHeadroomMiB <= 0 {
		config.GPUHeadroomMiB = 1024
	}
	return &ProcessRunner{backend: backend, config: config, onCalibrated: onCalibrated}
}

func (r *ProcessRunner) Run(ctx context.Context, request RunRequest, update func(ProgressUpdate)) (RunOutput, error) {
	started := time.Now()
	workDir := filepath.Join(request.OutputDir, ".work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return RunOutput{}, fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	update(ProgressUpdate{Stage: StageProbing, Percent: 0})
	source, err := r.backend.Probe(ctx, request.SourcePath, request.FileName, request.SizeBytes)
	if err != nil {
		return RunOutput{}, err
	}
	calibration, err := r.ensureCalibrated(ctx, request.SourcePath, workDir, source, update)
	if err != nil {
		return RunOutput{}, fmt.Errorf("calibration failed: %w", err)
	}
	initialETA := time.Duration(float64(source.DurationMS) / 1000 / calibration.RealtimeFactor * float64(time.Second))
	initialRTF := calibration.RealtimeFactor

	wavPath := filepath.Join(workDir, "audio.wav")
	update(ProgressUpdate{Stage: StageExtracting, Percent: 0, ETA: &initialETA, RealtimeFactor: &initialRTF})
	if err := r.backend.Extract(ctx, request.SourcePath, wavPath, 0); err != nil {
		return RunOutput{}, fmt.Errorf("extract audio: %w", err)
	}

	estimator := NewProgressEstimator(30*time.Second, 3)
	outputBase := filepath.Join(workDir, "whisper")
	update(ProgressUpdate{Stage: StageTranscribing, Percent: 0, ETA: &initialETA, RealtimeFactor: &initialRTF})
	transcribed, err := r.backend.Transcribe(ctx, wavPath, outputBase, func(percent int) {
		estimate := estimator.Observe(time.Now(), percent, time.Duration(source.DurationMS)*time.Millisecond)
		update(ProgressUpdate{Stage: StageTranscribing, Percent: percent, ETA: estimate.ETA, RealtimeFactor: estimate.RealtimeFactor})
	})
	if err != nil {
		return RunOutput{}, err
	}
	file, err := os.Open(transcribed.JSONPath)
	if err != nil {
		return RunOutput{}, fmt.Errorf("open whisper output: %w", err)
	}
	segments, text, language, parseErr := ParseWhisperJSON(file)
	_ = file.Close()
	if parseErr != nil {
		return RunOutput{}, parseErr
	}
	if language == "" {
		language = "pl"
	}

	wall := time.Since(started)
	rtf := RealtimeFactor(source.DurationMS, wall)
	result := Result{
		SchemaVersion: "1.0", TranscriptionID: request.ID, Source: source,
		Language: language, Text: text, Segments: segments,
		Engine:     EngineInfo{Name: "whisper.cpp", Version: r.config.EngineVersion, Model: r.config.Model},
		Processing: ProcessingInfo{WallTimeMS: wall.Milliseconds(), RealtimeFactor: rtf},
		CreatedAt:  time.Now().UTC(),
	}
	update(ProgressUpdate{Stage: StageWriting, Percent: 99})
	artifacts, err := WriteArtifactsAtomic(request.OutputDir, result)
	if err != nil {
		return RunOutput{}, fmt.Errorf("write artifacts: %w", err)
	}
	if request.DeleteInput {
		_ = os.Remove(request.SourcePath)
	}
	return RunOutput{Result: result, Artifacts: artifacts}, nil
}

func (r *ProcessRunner) ensureCalibrated(ctx context.Context, sourcePath, workDir string, source SourceInfo, update func(ProgressUpdate)) (CalibrationInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.calibrated {
		return r.calibration, nil
	}
	update(ProgressUpdate{Stage: StageCalibrating, Percent: 0})
	// GPU telemetry is best-effort: on a CPU-only host (Railway) nvidia-smi is
	// absent, so we calibrate real-time factor without any VRAM-based scaling.
	baseline, gpuErr := r.backend.GPU(ctx)
	cpuOnly := gpuErr != nil || baseline.TotalMemoryMiB == 0
	calibrationWAV := filepath.Join(workDir, "calibration.wav")
	limit := r.config.CalibrationDuration
	if source.DurationMS > 0 && time.Duration(source.DurationMS)*time.Millisecond < limit {
		limit = time.Duration(source.DurationMS) * time.Millisecond
	}
	if err := r.backend.Extract(ctx, sourcePath, calibrationWAV, limit); err != nil {
		return CalibrationInfo{}, err
	}
	started := time.Now()
	transcribed, err := r.backend.Transcribe(ctx, calibrationWAV, filepath.Join(workDir, "calibration"), func(percent int) {
		update(ProgressUpdate{Stage: StageCalibrating, Percent: percent})
	})
	if err != nil {
		return CalibrationInfo{}, err
	}
	wall := time.Since(started)
	rtf := 0.0
	if wall > 0 {
		rtf = limit.Seconds() / wall.Seconds()
	}
	suggested := 1
	if cpuOnly {
		// No VRAM budget to reason about; a single CPU worker is the safe
		// default. Operators can still raise it via STT_MAX_WORKERS.
		suggested = r.config.MaxWorkers
	} else {
		delta := transcribed.PeakGPUMemoryMiB - baseline.UsedMemoryMiB
		if delta < 0 {
			delta = 0
		}
		for workers := 2; workers <= r.config.MaxWorkers; workers++ {
			projected := baseline.UsedMemoryMiB + int64(workers)*delta + r.config.GPUHeadroomMiB
			if projected <= baseline.TotalMemoryMiB {
				suggested = workers
			}
		}
	}
	r.calibration = CalibrationInfo{SuggestedWorkers: suggested, RealtimeFactor: rtf, BaselineVRAMMiB: baseline.UsedMemoryMiB, PeakVRAMMiB: transcribed.PeakGPUMemoryMiB}
	r.calibrated = true
	if r.onCalibrated != nil {
		r.onCalibrated(r.calibration)
	}
	return r.calibration, nil
}
