package transcription

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteArtifactsAtomic writes the transcript as a single plain-text file named
// "<mp4-base>-transcription.txt". Each line is "[startMs - endMs] text" where the
// timestamps are integer milliseconds.
func WriteArtifactsAtomic(dir string, result Result) (ArtifactPaths, error) {
	if err := validateResult(result); err != nil {
		return ArtifactPaths{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ArtifactPaths{}, err
	}

	var txt bytes.Buffer
	for _, segment := range result.Segments {
		fmt.Fprintf(&txt, "[%d - %d] %s\n", segment.StartMS, segment.EndMS, strings.TrimSpace(segment.Text))
	}

	name := transcriptFileName(result.Source.FileName)
	tmp, err := os.CreateTemp(dir, "."+name+"-*")
	if err != nil {
		return ArtifactPaths{}, err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(txt.Bytes()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return ArtifactPaths{}, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return ArtifactPaths{}, err
	}
	final := filepath.Join(dir, name)
	if err := os.Rename(tmpPath, final); err != nil {
		_ = os.Remove(tmpPath)
		return ArtifactPaths{}, err
	}
	return ArtifactPaths{TXT: final}, nil
}

// transcriptFileName turns "clip.mp4" into "clip-transcription.txt".
func transcriptFileName(sourceName string) string {
	base := filepath.Base(sourceName)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if base == "" || base == "." {
		base = "transcript"
	}
	return base + "-transcription.txt"
}

func validateResult(result Result) error {
	if result.SchemaVersion == "" || result.TranscriptionID == "" {
		return errors.New("missing result identity")
	}
	var previousStart int64 = -1
	for _, segment := range result.Segments {
		if segment.StartMS < 0 || segment.EndMS < segment.StartMS || segment.StartMS < previousStart {
			return fmt.Errorf("invalid timestamps for segment %d", segment.Index)
		}
		previousStart = segment.StartMS
	}
	return nil
}
