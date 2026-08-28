package transcription

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CLIBackendConfig struct {
	FFprobeBinary string
	FFmpegBinary  string
	WhisperBinary string
	NvidiaSMI     string
	ModelPath     string
	Threads       int
}

type CLIBackend struct{ config CLIBackendConfig }

func NewCLIBackend(config CLIBackendConfig) *CLIBackend {
	if config.FFprobeBinary == "" {
		config.FFprobeBinary = "ffprobe"
	}
	if config.FFmpegBinary == "" {
		config.FFmpegBinary = "ffmpeg"
	}
	if config.WhisperBinary == "" {
		config.WhisperBinary = "whisper-cli"
	}
	if config.NvidiaSMI == "" {
		config.NvidiaSMI = "nvidia-smi"
	}
	if config.Threads < 1 {
		config.Threads = 6
	}
	return &CLIBackend{config: config}
}

func (b *CLIBackend) Probe(ctx context.Context, path, fileName string, sizeBytes int64) (SourceInfo, error) {
	cmd := exec.CommandContext(ctx, b.config.FFprobeBinary, "-v", "error", "-show_entries", "format=format_name,duration:stream=codec_type", "-of", "json", path)
	output, err := cmd.Output()
	if err != nil {
		return SourceInfo{}, fmt.Errorf("ffprobe failed: %w", err)
	}
	return ParseProbe(bytes.NewReader(output), fileName, sizeBytes)
}

func (b *CLIBackend) Extract(ctx context.Context, source, destination string, limit time.Duration) error {
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-i", source}
	if limit > 0 {
		args = append(args, "-t", strconv.FormatFloat(limit.Seconds(), 'f', 3, 64))
	}
	args = append(args, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", destination)
	output, err := exec.CommandContext(ctx, b.config.FFmpegBinary, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %w: %s", err, truncate(string(output), 4096))
	}
	return nil
}

func (b *CLIBackend) Transcribe(ctx context.Context, wavPath, outputBase string, onProgress func(int)) (BackendTranscription, error) {
	args := []string{"-m", b.config.ModelPath, "-f", wavPath, "-l", "pl", "-t", strconv.Itoa(b.config.Threads), "-ojf", "-of", outputBase, "-pp", "-fa"}
	cmd := exec.CommandContext(ctx, b.config.WhisperBinary, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return BackendTranscription{}, err
	}
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		return BackendTranscription{}, fmt.Errorf("start whisper: %w", err)
	}

	baseline, _ := b.GPU(ctx)
	peak := baseline.UsedMemoryMiB
	var peakMu sync.Mutex
	sampleCtx, cancelSamples := context.WithCancel(ctx)
	var samples sync.WaitGroup
	samples.Add(1)
	go func() {
		defer samples.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-sampleCtx.Done():
				return
			case <-ticker.C:
				if gpu, sampleErr := b.GPU(sampleCtx); sampleErr == nil {
					peakMu.Lock()
					if gpu.UsedMemoryMiB > peak {
						peak = gpu.UsedMemoryMiB
					}
					peakMu.Unlock()
				}
			}
		}
	}()

	var log bytes.Buffer
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if log.Len() < 64*1024 {
			log.WriteString(line)
			log.WriteByte('\n')
		}
		if percent, ok := ParseWhisperProgress(line); ok {
			onProgress(percent)
		}
	}
	waitErr := cmd.Wait()
	cancelSamples()
	samples.Wait()
	if scanErr := scanner.Err(); scanErr != nil && waitErr == nil {
		waitErr = scanErr
	}
	if waitErr != nil {
		return BackendTranscription{}, fmt.Errorf("whisper failed: %w: %s", waitErr, truncate(log.String(), 8192))
	}
	peakMu.Lock()
	finalPeak := peak
	peakMu.Unlock()
	return BackendTranscription{JSONPath: outputBase + ".json", PeakGPUMemoryMiB: finalPeak}, nil
}

func (b *CLIBackend) GPU(ctx context.Context) (GPUInfo, error) {
	output, err := exec.CommandContext(ctx, b.config.NvidiaSMI, "--query-gpu=name,memory.total,memory.used", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return GPUInfo{}, fmt.Errorf("nvidia-smi failed: %w", err)
	}
	line := strings.Split(strings.TrimSpace(string(output)), "\n")[0]
	parts := strings.Split(line, ",")
	if len(parts) < 3 {
		return GPUInfo{}, fmt.Errorf("unexpected nvidia-smi output: %s", line)
	}
	total, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return GPUInfo{}, err
	}
	used, err := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
	if err != nil {
		return GPUInfo{}, err
	}
	return GPUInfo{Name: strings.TrimSpace(parts[0]), TotalMemoryMiB: total, UsedMemoryMiB: used}, nil
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[len(value)-max:]
}
