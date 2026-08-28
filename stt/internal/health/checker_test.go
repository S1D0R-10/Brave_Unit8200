package health

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"brave/stt/internal/transcription"
)

type fakeGPUReader struct {
	info transcription.GPUInfo
	err  error
}

func (f fakeGPUReader) GPU(context.Context) (transcription.GPUInfo, error) { return f.info, f.err }

func TestCheckerReportsReadyWhenRuntimeIsAvailable(t *testing.T) {
	root := t.TempDir()
	model := filepath.Join(root, "model.bin")
	if err := os.WriteFile(model, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	checker := NewChecker(Config{ModelPath: model, Directories: []string{root}, Binaries: []string{os.Args[0]}, MinimumFreeBytes: 1}, fakeGPUReader{info: transcription.GPUInfo{Name: "RTX", TotalMemoryMiB: 6144}}, func(string) (uint64, error) { return 100, nil })
	result := checker.Check(context.Background())
	if !result.Ready || result.GPU == nil || !result.Checks["disk"] {
		t.Fatalf("unexpected readiness: %#v", result)
	}
}

func TestCheckerReportsMissingModel(t *testing.T) {
	root := t.TempDir()
	checker := NewChecker(Config{ModelPath: filepath.Join(root, "missing.bin"), Directories: []string{root}, Binaries: []string{os.Args[0]}}, fakeGPUReader{info: transcription.GPUInfo{Name: "RTX"}}, func(string) (uint64, error) { return 100, nil })
	result := checker.Check(context.Background())
	if result.Ready || result.Checks["model"] {
		t.Fatalf("missing model reported ready: %#v", result)
	}
}
