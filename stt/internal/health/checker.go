package health

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"brave/stt/internal/api"
	"brave/stt/internal/transcription"
)

type GPUReader interface {
	GPU(context.Context) (transcription.GPUInfo, error)
}
type FreeBytesFunc func(string) (uint64, error)

type Config struct {
	ModelPath        string
	ModelName        string
	Directories      []string
	Binaries         []string
	MinimumFreeBytes uint64
}

type Checker struct {
	config Config
	gpu    GPUReader
	free   FreeBytesFunc
}

func NewChecker(config Config, gpu GPUReader, free FreeBytesFunc) *Checker {
	if free == nil {
		free = DiskFreeBytes
	}
	return &Checker{config: config, gpu: gpu, free: free}
}

func (c *Checker) Check(ctx context.Context) api.Readiness {
	checks := map[string]bool{"model": false, "directories": true, "binaries": true, "disk": false}
	messages := make([]string, 0)
	if info, err := os.Stat(c.config.ModelPath); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
		checks["model"] = true
	} else {
		messages = append(messages, "brak modelu Whisper")
	}
	for _, dir := range c.config.Directories {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			checks["directories"] = false
			messages = append(messages, "katalog danych jest niedostępny")
			break
		}
	}
	for _, binary := range c.config.Binaries {
		if _, err := exec.LookPath(binary); err != nil {
			checks["binaries"] = false
			messages = append(messages, fmt.Sprintf("brak narzędzia %s", binary))
			break
		}
	}
	// GPU is optional: report it when present (e.g. local CUDA box) but never
	// gate readiness on it, since Railway runs CPU-only.
	var gpu *transcription.GPUInfo
	if c.gpu != nil {
		if info, err := c.gpu.GPU(ctx); err == nil && info.TotalMemoryMiB > 0 {
			gpu = &info
		}
	}
	diskDir := "."
	if len(c.config.Directories) > 0 {
		diskDir = c.config.Directories[0]
	}
	if free, err := c.free(diskDir); err == nil && free >= c.config.MinimumFreeBytes {
		checks["disk"] = true
	} else {
		messages = append(messages, "za mało wolnego miejsca na dysku")
	}
	ready := true
	for _, ok := range checks {
		ready = ready && ok
	}
	message := ""
	if len(messages) > 0 {
		for i, item := range messages {
			if i > 0 {
				message += "; "
			}
			message += item
		}
	}
	return api.Readiness{Ready: ready, Checks: checks, GPU: gpu, Model: c.config.ModelName, Message: message}
}
