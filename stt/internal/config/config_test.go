package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadUsesRAGReadyDefaults(t *testing.T) {
	t.Setenv("STT_DATA_DIR", t.TempDir())
	t.Setenv("STT_MODEL_PATH", filepath.Join(t.TempDir(), "model.bin"))
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.MaxUploadBytes != 3*1024*1024*1024 {
		t.Fatalf("max upload = %d", config.MaxUploadBytes)
	}
	if config.Language != "pl" || config.ModelName != "large-v3-turbo-q5_0" {
		t.Fatalf("unexpected model defaults: %#v", config)
	}
	if config.Retention != 24*time.Hour || config.MaxWorkers != 2 || config.InitialWorkers != 1 {
		t.Fatalf("unexpected scheduling defaults: %#v", config)
	}
}

func TestLoadRejectsUnsafeWorkerCount(t *testing.T) {
	t.Setenv("STT_MAX_WORKERS", "3")
	if _, err := Load(); err == nil {
		t.Fatal("expected worker validation error")
	}
}
