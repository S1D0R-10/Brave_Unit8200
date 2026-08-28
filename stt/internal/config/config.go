package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

type Config struct {
	Address             string
	DataDir             string
	InboxDir            string
	WorkDir             string
	OutputDir           string
	ModelPath           string
	ModelName           string
	Language            string
	WhisperVersion      string
	WhisperBinary       string
	FFmpegBinary        string
	FFprobeBinary       string
	NvidiaSMI           string
	Threads             int
	MaxUploadBytes      int64
	MaxQueue            int
	MaxWorkers          int
	InitialWorkers      int
	Retention           time.Duration
	CalibrationDuration time.Duration
	GPUHeadroomMiB      int64
	MinimumFreeBytes    uint64

	// Pipeline wiring. The RAG_S3_* names are shared with the embedder and
	// rag-search so all three services read one set of bucket credentials.
	S3Endpoint  string
	S3Bucket    string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	EmbedderURL string
}

func Load() (Config, error) {
	dataDir := env("STT_DATA_DIR", "./data")
	config := Config{
		Address: listenAddress(), DataDir: dataDir,
		InboxDir: filepath.Join(dataDir, "inbox"), WorkDir: filepath.Join(dataDir, "work"), OutputDir: filepath.Join(dataDir, "output"),
		ModelPath: env("STT_MODEL_PATH", "./models/ggml-large-v3-turbo-q5_0.bin"),
		ModelName: env("STT_MODEL_NAME", "large-v3-turbo-q5_0"), Language: "pl", WhisperVersion: "1.9.1",
		WhisperBinary: env("STT_WHISPER_BINARY", "whisper-cli"), FFmpegBinary: env("STT_FFMPEG_BINARY", "ffmpeg"),
		FFprobeBinary: env("STT_FFPROBE_BINARY", "ffprobe"), NvidiaSMI: env("STT_NVIDIA_SMI", "nvidia-smi"),
		Threads: max(1, runtime.NumCPU()/2), MaxUploadBytes: 3 * 1024 * 1024 * 1024, MaxQueue: 20,
		MaxWorkers: 2, InitialWorkers: 1, Retention: 24 * time.Hour, CalibrationDuration: 5 * time.Minute,
		GPUHeadroomMiB: 1024, MinimumFreeBytes: 1 * 1024 * 1024 * 1024,
		S3Endpoint:  env("RAG_S3_ENDPOINT", "https://t3.storageapi.dev"),
		S3Bucket:    os.Getenv("RAG_S3_BUCKET"),
		S3Region:    env("RAG_S3_REGION", "auto"),
		S3AccessKey: os.Getenv("RAG_S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("RAG_S3_SECRET_KEY"),
		EmbedderURL: os.Getenv("EMBEDDER_URL"),
	}
	var err error
	if minFree, minErr := envInt64("STT_MIN_FREE_BYTES", int64(config.MinimumFreeBytes)); minErr != nil {
		return Config{}, minErr
	} else if minFree >= 0 {
		config.MinimumFreeBytes = uint64(minFree)
	}
	if config.Threads, err = envInt("STT_THREADS", config.Threads); err != nil {
		return Config{}, err
	}
	if config.MaxQueue, err = envInt("STT_MAX_QUEUE", config.MaxQueue); err != nil {
		return Config{}, err
	}
	if config.MaxWorkers, err = envInt("STT_MAX_WORKERS", config.MaxWorkers); err != nil {
		return Config{}, err
	}
	if config.InitialWorkers, err = envInt("STT_INITIAL_WORKERS", config.InitialWorkers); err != nil {
		return Config{}, err
	}
	if config.MaxUploadBytes, err = envInt64("STT_MAX_UPLOAD_BYTES", config.MaxUploadBytes); err != nil {
		return Config{}, err
	}
	if config.GPUHeadroomMiB, err = envInt64("STT_GPU_HEADROOM_MIB", config.GPUHeadroomMiB); err != nil {
		return Config{}, err
	}
	if config.MaxWorkers < 1 || config.MaxWorkers > 2 {
		return Config{}, fmt.Errorf("STT_MAX_WORKERS must be 1 or 2")
	}
	if config.InitialWorkers < 1 || config.InitialWorkers > config.MaxWorkers {
		return Config{}, fmt.Errorf("STT_INITIAL_WORKERS must be between 1 and STT_MAX_WORKERS")
	}
	if config.MaxQueue < 1 || config.MaxUploadBytes < 1 || config.Threads < 1 {
		return Config{}, fmt.Errorf("numeric STT settings must be positive")
	}
	return config, nil
}

// listenAddress honours STT_ADDRESS, then Railway's injected PORT, then :8000.
func listenAddress() string {
	if value := os.Getenv("STT_ADDRESS"); value != "" {
		return value
	}
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8000"
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func envInt(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return parsed, nil
}
func envInt64(name string, fallback int64) (int64, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return parsed, nil
}
