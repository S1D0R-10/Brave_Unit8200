package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"brave/stt/internal/api"
	"brave/stt/internal/config"
	"brave/stt/internal/health"
	"brave/stt/internal/objectstore"
	"brave/stt/internal/pipeline"
	"brave/stt/internal/transcription"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	for _, dir := range []string{cfg.InboxDir, cfg.WorkDir, cfg.OutputDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			logger.Error("cannot create data directory", "path", dir, "error", err)
			os.Exit(1)
		}
	}

	backend := transcription.NewCLIBackend(transcription.CLIBackendConfig{
		FFprobeBinary: cfg.FFprobeBinary, FFmpegBinary: cfg.FFmpegBinary,
		WhisperBinary: cfg.WhisperBinary, NvidiaSMI: cfg.NvidiaSMI,
		ModelPath: cfg.ModelPath, Threads: cfg.Threads,
	})
	var manager *transcription.Manager
	runner := transcription.NewProcessRunner(backend, transcription.ProcessRunnerConfig{
		Model: cfg.ModelName, EngineVersion: cfg.WhisperVersion,
		CalibrationDuration: cfg.CalibrationDuration, MaxWorkers: cfg.MaxWorkers,
		GPUHeadroomMiB: cfg.GPUHeadroomMiB,
	}, func(info transcription.CalibrationInfo) {
		manager.SetWorkerLimit(info.SuggestedWorkers)
		logger.Info("calibration completed", "workers", info.SuggestedWorkers, "realtimeFactor", info.RealtimeFactor, "baselineVramMiB", info.BaselineVRAMMiB, "peakVramMiB", info.PeakVRAMMiB)
	})
	manager = transcription.NewManager(runner, transcription.ManagerConfig{
		MaxQueue: cfg.MaxQueue, MaxWorkers: cfg.MaxWorkers,
		InitialWorkers: cfg.InitialWorkers, Retention: cfg.Retention,
	})
	rootContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	manager.Start(rootContext)
	defer manager.Stop()

	checker := health.NewChecker(health.Config{
		ModelPath: cfg.ModelPath, ModelName: cfg.ModelName,
		Directories:      []string{cfg.WorkDir, cfg.OutputDir, cfg.InboxDir},
		Binaries:         []string{cfg.FFprobeBinary, cfg.FFmpegBinary, cfg.WhisperBinary},
		MinimumFreeBytes: cfg.MinimumFreeBytes,
	}, backend, nil)
	// Pipeline wiring is optional: without bucket credentials the panel and
	// the upload endpoints keep working, only /api/v1/ingest goes dark.
	apiConfig := api.Config{
		InboxDir: cfg.InboxDir, WorkDir: cfg.WorkDir, OutputDir: cfg.OutputDir,
		MaxUploadBytes: cfg.MaxUploadBytes, Ready: checker.Check, Index: api.IndexHandler(),
		Logger: logger,
	}
	if store, storeErr := objectstore.New(objectstore.Config{
		Endpoint: cfg.S3Endpoint, Bucket: cfg.S3Bucket, Region: cfg.S3Region,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
	}); storeErr != nil {
		logger.Warn("object store not configured, /api/v1/ingest is disabled", "error", storeErr)
	} else {
		apiConfig.Store = store
		logger.Info("object store ready", "endpoint", cfg.S3Endpoint, "bucket", cfg.S3Bucket)
	}
	if notifier := pipeline.NewEmbedderNotifier(cfg.EmbedderURL); notifier != nil {
		apiConfig.Notify = notifier.Notify
		logger.Info("embedder handoff ready", "url", cfg.EmbedderURL)
	} else {
		logger.Warn("EMBEDDER_URL not set, transcripts will be uploaded but not indexed")
	}

	apiServer := api.NewServer(manager, apiConfig)
	httpServer := &http.Server{
		Addr: cfg.Address, Handler: apiServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		logger.Info("STT service listening", "address", cfg.Address, "model", cfg.ModelName)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server stopped unexpectedly", "error", err)
			stopSignals()
		}
	}()
	<-rootContext.Done()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		logger.Error("HTTP shutdown failed", "error", err)
	}
}
