package transcription

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct {
	mu      sync.Mutex
	block   chan struct{}
	started chan string
	err     error
	errors  []error
	calls   int
}

type concurrentOOMRunner struct {
	started  chan string
	releaseA chan struct{}
	mu       sync.Mutex
	calls    map[string]int
}

func (r *concurrentOOMRunner) Run(ctx context.Context, request RunRequest, _ func(ProgressUpdate)) (RunOutput, error) {
	r.mu.Lock()
	r.calls[request.FileName]++
	call := r.calls[request.FileName]
	r.mu.Unlock()
	r.started <- request.FileName
	if request.FileName == "a.mp4" {
		select {
		case <-r.releaseA:
		case <-ctx.Done():
			return RunOutput{}, ctx.Err()
		}
	}
	if request.FileName == "b.mp4" && call == 1 {
		return RunOutput{}, errors.New("CUDA out of memory")
	}
	result := sampleResult()
	result.TranscriptionID = request.ID
	result.Source.FileName = request.FileName
	return RunOutput{Result: result}, nil
}

func (r *fakeRunner) Run(ctx context.Context, request RunRequest, update func(ProgressUpdate)) (RunOutput, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if r.started != nil {
		r.started <- request.ID
	}
	update(ProgressUpdate{Stage: StageTranscribing, Percent: 50})
	if r.block != nil {
		select {
		case <-r.block:
		case <-ctx.Done():
			return RunOutput{}, ctx.Err()
		}
	}
	if call <= len(r.errors) && r.errors[call-1] != nil {
		return RunOutput{}, r.errors[call-1]
	}
	if r.err != nil {
		return RunOutput{}, r.err
	}
	result := sampleResult()
	result.TranscriptionID = request.ID
	result.Source.FileName = request.FileName
	return RunOutput{Result: result, Artifacts: ArtifactPaths{TXT: filepath.Join(request.OutputDir, "transcript-transcription.txt")}}, nil
}

func TestManagerRetriesOOMOnceAndReducesWorkers(t *testing.T) {
	runner := &fakeRunner{errors: []error{errors.New("CUDA out of memory")}}
	manager := NewManager(runner, ManagerConfig{MaxQueue: 4, MaxWorkers: 2, InitialWorkers: 2, Retention: time.Hour})
	manager.Start(context.Background())
	t.Cleanup(manager.Stop)

	job, err := manager.Enqueue(EnqueueRequest{FileName: "film.mp4", SourcePath: "film.mp4", OutputRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	waitForJob(t, manager, job.ID, StatusSucceeded)
	runner.mu.Lock()
	calls := runner.calls
	runner.mu.Unlock()
	if calls != 2 {
		t.Fatalf("runner calls = %d; want 2", calls)
	}
	if manager.WorkerLimit() != 1 {
		t.Fatalf("worker limit = %d; want 1 after OOM", manager.WorkerLimit())
	}
}

func TestManagerWaitsForOtherGPUWorkBeforeOOMRetry(t *testing.T) {
	runner := &concurrentOOMRunner{started: make(chan string, 4), releaseA: make(chan struct{}), calls: map[string]int{}}
	manager := NewManager(runner, ManagerConfig{MaxQueue: 4, MaxWorkers: 2, InitialWorkers: 2, Retention: time.Hour})
	manager.Start(context.Background())
	t.Cleanup(manager.Stop)

	a, err := manager.Enqueue(EnqueueRequest{FileName: "a.mp4", SourcePath: "a.mp4", OutputRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if started := <-runner.started; started != "a.mp4" {
		t.Fatalf("first started job = %s", started)
	}
	b, err := manager.Enqueue(EnqueueRequest{FileName: "b.mp4", SourcePath: "b.mp4", OutputRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if started := <-runner.started; started != "b.mp4" {
		t.Fatalf("second started job = %s", started)
	}
	select {
	case started := <-runner.started:
		t.Fatalf("OOM retry %s started while another GPU job was active", started)
	case <-time.After(150 * time.Millisecond):
	}
	close(runner.releaseA)
	waitForJob(t, manager, a.ID, StatusSucceeded)
	waitForJob(t, manager, b.ID, StatusSucceeded)
}

func TestManagerRunsQueuedJobAndPublishesResult(t *testing.T) {
	runner := &fakeRunner{}
	manager := NewManager(runner, ManagerConfig{MaxQueue: 4, MaxWorkers: 2, InitialWorkers: 1, Retention: time.Hour})
	manager.Start(context.Background())
	t.Cleanup(manager.Stop)

	job, err := manager.Enqueue(EnqueueRequest{FileName: "film.mp4", SourcePath: "film.mp4", OutputRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	completed := waitForJob(t, manager, job.ID, StatusSucceeded)
	if completed.Progress.Percent != 100 || completed.Result == nil || completed.Result.TranscriptionID != job.ID {
		t.Fatalf("unexpected completed job: %#v", completed)
	}
}

func TestManagerCancelsRunningJob(t *testing.T) {
	runner := &fakeRunner{block: make(chan struct{}), started: make(chan string, 1)}
	manager := NewManager(runner, ManagerConfig{MaxQueue: 4, MaxWorkers: 1, InitialWorkers: 1, Retention: time.Hour})
	manager.Start(context.Background())
	t.Cleanup(manager.Stop)

	job, err := manager.Enqueue(EnqueueRequest{FileName: "film.mp4", SourcePath: "film.mp4", OutputRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	<-runner.started
	if err := manager.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	waitForJob(t, manager, job.ID, StatusCanceled)
}

func TestManagerDeletesUploadedInputOnCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{block: make(chan struct{}), started: make(chan string, 1)}
	manager := NewManager(runner, ManagerConfig{MaxQueue: 4, MaxWorkers: 1, InitialWorkers: 1, Retention: time.Hour})
	manager.Start(context.Background())
	t.Cleanup(manager.Stop)
	job, err := manager.Enqueue(EnqueueRequest{FileName: "upload.mp4", SourcePath: path, OutputRoot: t.TempDir(), DeleteInput: true})
	if err != nil {
		t.Fatal(err)
	}
	<-runner.started
	if err := manager.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("canceled upload was not deleted: %v", err)
	}
}

func TestManagerDeletesFailedUploadWhenRetentionExpires(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(&fakeRunner{err: errors.New("invalid media")}, ManagerConfig{MaxQueue: 4, MaxWorkers: 1, InitialWorkers: 1, Retention: time.Millisecond})
	manager.Start(context.Background())
	t.Cleanup(manager.Stop)
	job, err := manager.Enqueue(EnqueueRequest{FileName: "upload.mp4", SourcePath: path, OutputRoot: t.TempDir(), DeleteInput: true})
	if err != nil {
		t.Fatal(err)
	}
	failed := waitForJob(t, manager, job.ID, StatusFailed)
	manager.Prune(failed.FinishedAt.Add(time.Millisecond))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired failed upload was not deleted: %v", err)
	}
}

func TestManagerRejectsFullQueue(t *testing.T) {
	runner := &fakeRunner{block: make(chan struct{}), started: make(chan string, 1)}
	manager := NewManager(runner, ManagerConfig{MaxQueue: 1, MaxWorkers: 1, InitialWorkers: 1, Retention: time.Hour})
	manager.Start(context.Background())
	t.Cleanup(manager.Stop)

	_, _ = manager.Enqueue(EnqueueRequest{FileName: "one.mp4", SourcePath: "one.mp4", OutputRoot: t.TempDir()})
	<-runner.started
	_, _ = manager.Enqueue(EnqueueRequest{FileName: "two.mp4", SourcePath: "two.mp4", OutputRoot: t.TempDir()})
	if _, err := manager.Enqueue(EnqueueRequest{FileName: "three.mp4", SourcePath: "three.mp4", OutputRoot: t.TempDir()}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
}

func waitForJob(t *testing.T, manager *Manager, id string, status Status) Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := manager.Get(id)
		if ok && job.Status == status {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, _ := manager.Get(id)
	t.Fatalf("job %s did not reach %s; last state: %#v", id, status, job)
	return Job{}
}
