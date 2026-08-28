package transcription

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

type Stage string

const (
	StageQueued       Stage = "queued"
	StageCalibrating  Stage = "calibrating"
	StageProbing      Stage = "probing"
	StageExtracting   Stage = "extracting"
	StageTranscribing Stage = "transcribing"
	StageWriting      Stage = "writing"
	StageDone         Stage = "done"
)

var (
	ErrQueueFull = errors.New("transcription queue is full")
	ErrNotFound  = errors.New("transcription not found")
)

type Progress struct {
	Stage          Stage    `json:"stage"`
	Percent        int      `json:"percent"`
	ETASeconds     *int64   `json:"etaSeconds"`
	RealtimeFactor *float64 `json:"realtimeFactor"`
}

type JobError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Job struct {
	ID          string         `json:"id"`
	BatchID     string         `json:"batchId,omitempty"`
	Status      Status         `json:"status"`
	Progress    Progress       `json:"progress"`
	Source      SourceInfo     `json:"source"`
	CreatedAt   time.Time      `json:"createdAt"`
	StartedAt   *time.Time     `json:"startedAt,omitempty"`
	FinishedAt  *time.Time     `json:"finishedAt,omitempty"`
	Result      *Result        `json:"result,omitempty"`
	Artifacts   *ArtifactPaths `json:"artifacts,omitempty"`
	Error       *JobError      `json:"error,omitempty"`
	sourcePath  string
	outputDir   string
	deleteInput bool
	retryCount  int
	cancel      context.CancelFunc
}

type EnqueueRequest struct {
	BatchID     string
	FileName    string
	SizeBytes   int64
	SourcePath  string
	OutputRoot  string
	DeleteInput bool
}

type RunRequest struct {
	ID          string
	FileName    string
	SizeBytes   int64
	SourcePath  string
	OutputDir   string
	DeleteInput bool
}

type ProgressUpdate struct {
	Stage          Stage
	Percent        int
	ETA            *time.Duration
	RealtimeFactor *float64
}

type RunOutput struct {
	Result    Result
	Artifacts ArtifactPaths
}

type Runner interface {
	Run(context.Context, RunRequest, func(ProgressUpdate)) (RunOutput, error)
}

type ManagerConfig struct {
	MaxQueue       int
	MaxWorkers     int
	InitialWorkers int
	Retention      time.Duration
}

type Manager struct {
	mu           sync.RWMutex
	runner       Runner
	config       ManagerConfig
	jobs         map[string]*Job
	queue        chan string
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	workers      int
	workerLimit  int
	activeRuns   int
	oomWaiters   int
	pauseWorkers bool
	activity     *sync.Cond
	retryMu      sync.Mutex
	startOnce    sync.Once
	stopOnce     sync.Once
}

func NewManager(runner Runner, config ManagerConfig) *Manager {
	if config.MaxQueue < 1 {
		config.MaxQueue = 20
	}
	if config.MaxWorkers < 1 {
		config.MaxWorkers = 1
	}
	if config.InitialWorkers < 1 {
		config.InitialWorkers = 1
	}
	if config.InitialWorkers > config.MaxWorkers {
		config.InitialWorkers = config.MaxWorkers
	}
	if config.Retention <= 0 {
		config.Retention = 24 * time.Hour
	}
	manager := &Manager{runner: runner, config: config, jobs: make(map[string]*Job), queue: make(chan string, config.MaxQueue)}
	manager.activity = sync.NewCond(&manager.mu)
	return manager
}

func (m *Manager) Start(parent context.Context) {
	m.startOnce.Do(func() {
		m.ctx, m.cancel = context.WithCancel(parent)
		m.SetWorkerLimit(m.config.InitialWorkers)
		m.wg.Add(1)
		go m.cleanupLoop()
	})
}

func (m *Manager) Stop() {
	m.stopOnce.Do(func() {
		if m.cancel != nil {
			m.cancel()
		}
		m.mu.Lock()
		for _, job := range m.jobs {
			if job.cancel != nil {
				job.cancel()
			}
		}
		m.mu.Unlock()
		m.activity.Broadcast()
		m.wg.Wait()
	})
}

func (m *Manager) SetWorkerLimit(limit int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx == nil {
		return
	}
	if limit > m.config.MaxWorkers {
		limit = m.config.MaxWorkers
	}
	if limit < 1 {
		limit = 1
	}
	m.workerLimit = limit
	for m.workers < limit {
		index := m.workers
		m.workers++
		m.wg.Add(1)
		go m.worker(index)
	}
}

func (m *Manager) WorkerLimit() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.workerLimit
}

func (m *Manager) Enqueue(request EnqueueRequest) (Job, error) {
	id, err := newID()
	if err != nil {
		return Job{}, err
	}
	job := &Job{
		ID: id, BatchID: request.BatchID, Status: StatusQueued,
		Progress:  Progress{Stage: StageQueued},
		Source:    SourceInfo{FileName: request.FileName, SizeBytes: request.SizeBytes},
		CreatedAt: time.Now().UTC(), sourcePath: request.SourcePath,
		outputDir: request.OutputRoot + stringPathSeparator() + id, deleteInput: request.DeleteInput,
	}
	m.mu.Lock()
	m.jobs[id] = job
	queuedView := cloneJob(job)
	m.mu.Unlock()
	select {
	case m.queue <- id:
		return queuedView, nil
	default:
		m.mu.Lock()
		delete(m.jobs, id)
		m.mu.Unlock()
		return Job{}, ErrQueueFull
	}
}

func (m *Manager) Get(id string) (Job, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[id]
	if !ok {
		return Job{}, false
	}
	copy := cloneJob(job)
	return copy, true
}

func (m *Manager) List() []Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		result = append(result, cloneJob(job))
	}
	return result
}

func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return ErrNotFound
	}
	if job.Status == StatusSucceeded || job.Status == StatusFailed || job.Status == StatusCanceled {
		return nil
	}
	job.Status = StatusCanceled
	job.Progress.Stage = StageDone
	job.Progress.Percent = 0
	job.Progress.ETASeconds = nil
	now := time.Now().UTC()
	job.FinishedAt = &now
	if job.cancel != nil {
		job.cancel()
	}
	m.activity.Broadcast()
	if job.deleteInput {
		_ = os.Remove(job.sourcePath)
	}
	return nil
}

func (m *Manager) Prune(now time.Time) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for id, job := range m.jobs {
		if job.FinishedAt != nil && now.Sub(*job.FinishedAt) >= m.config.Retention {
			if job.deleteInput && job.Status != StatusSucceeded {
				_ = os.Remove(job.sourcePath)
			}
			delete(m.jobs, id)
			removed++
		}
	}
	return removed
}

func (m *Manager) worker(index int) {
	defer m.wg.Done()
	for {
		if !m.workerEnabled(index) {
			select {
			case <-m.ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		select {
		case <-m.ctx.Done():
			return
		case id := <-m.queue:
			if !m.workerEnabled(index) {
				select {
				case m.queue <- id:
				case <-m.ctx.Done():
					return
				}
				continue
			}
			m.run(id)
		}
	}
}

func (m *Manager) workerEnabled(index int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return !m.pauseWorkers && index < m.workerLimit
}

func (m *Manager) run(id string) {
	m.mu.Lock()
	job, ok := m.jobs[id]
	if !ok || job.Status == StatusCanceled {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	job.cancel = cancel
	job.Status = StatusRunning
	job.Progress.Stage = StageProbing
	m.activeRuns++
	now := time.Now().UTC()
	job.StartedAt = &now
	request := RunRequest{ID: id, FileName: job.Source.FileName, SizeBytes: job.Source.SizeBytes, SourcePath: job.sourcePath, OutputDir: job.outputDir, DeleteInput: job.deleteInput}
	m.mu.Unlock()

	output, err := m.runner.Run(ctx, request, func(update ProgressUpdate) { m.updateProgress(id, update) })
	m.mu.Lock()
	m.activeRuns--
	job, ok = m.jobs[id]
	shouldRetry := ok && job.Status == StatusRunning && job.retryCount == 0 && err != nil && IsOutOfMemoryError(err.Error()) && ctx.Err() == nil
	if shouldRetry {
		job.retryCount++
		job.Progress.Stage = StageQueued
		job.Progress.Percent = 0
		m.workerLimit = 1
		m.oomWaiters++
		m.pauseWorkers = true
	}
	m.activity.Broadcast()
	m.mu.Unlock()
	if shouldRetry {
		m.retryMu.Lock()
		m.mu.Lock()
		for m.activeRuns > 0 && ctx.Err() == nil && job.Status == StatusRunning {
			m.activity.Wait()
		}
		canRetry := ctx.Err() == nil && job.Status == StatusRunning
		if canRetry {
			m.activeRuns++
			job.Progress.Stage = StageProbing
		}
		m.mu.Unlock()
		if canRetry {
			output, err = m.runner.Run(ctx, request, func(update ProgressUpdate) { m.updateProgress(id, update) })
			m.mu.Lock()
			m.activeRuns--
			m.activity.Broadcast()
			m.mu.Unlock()
		}
		m.mu.Lock()
		m.oomWaiters--
		m.pauseWorkers = m.oomWaiters > 0
		m.activity.Broadcast()
		m.mu.Unlock()
		m.retryMu.Unlock()
	}
	cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok = m.jobs[id]
	if !ok || job.Status == StatusCanceled {
		return
	}
	finished := time.Now().UTC()
	job.FinishedAt = &finished
	job.cancel = nil
	if err != nil {
		job.Status = StatusFailed
		job.Progress.Stage = StageDone
		job.Progress.ETASeconds = nil
		job.Error = &JobError{Code: runnerErrorCode(err), Message: err.Error()}
		return
	}
	job.Status = StatusSucceeded
	job.Progress.Stage = StageDone
	job.Progress.Percent = 100
	job.Progress.ETASeconds = nil
	job.Result = &output.Result
	job.Source = output.Result.Source
	job.Artifacts = &output.Artifacts
}

func (m *Manager) updateProgress(id string, update ProgressUpdate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok || job.Status != StatusRunning {
		return
	}
	job.Progress.Stage = update.Stage
	if update.Percent >= 0 && update.Percent <= 100 {
		job.Progress.Percent = update.Percent
	}
	if update.ETA != nil {
		seconds := int64(update.ETA.Seconds())
		job.Progress.ETASeconds = &seconds
	}
	if update.RealtimeFactor != nil {
		job.Progress.RealtimeFactor = update.RealtimeFactor
	}
}

func (m *Manager) cleanupLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case now := <-ticker.C:
			m.Prune(now)
		}
	}
}

func runnerErrorCode(err error) string {
	if errors.Is(err, context.Canceled) {
		return "transcription_canceled"
	}
	if IsOutOfMemoryError(err.Error()) {
		return "transcriber_out_of_memory"
	}
	return "transcription_failed"
}

func cloneJob(job *Job) Job {
	copy := *job
	copy.cancel = nil
	return copy
}

func newID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("create job ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
