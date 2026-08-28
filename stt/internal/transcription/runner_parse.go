package transcription

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var (
	ErrNotMP4   = errors.New("source is not an MP4 container")
	ErrNoAudio  = errors.New("source has no audio stream")
	ErrBadMedia = errors.New("cannot read media metadata")
)

func ParseProbe(reader io.Reader, fileName string, sizeBytes int64) (SourceInfo, error) {
	var payload struct {
		Format struct {
			Name     string `json:"format_name"`
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType string `json:"codec_type"`
		} `json:"streams"`
	}
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return SourceInfo{}, fmt.Errorf("%w: %v", ErrBadMedia, err)
	}
	if !strings.Contains(strings.ToLower(payload.Format.Name), "mp4") {
		return SourceInfo{}, ErrNotMP4
	}
	hasAudio := false
	for _, stream := range payload.Streams {
		if stream.CodecType == "audio" {
			hasAudio = true
			break
		}
	}
	if !hasAudio {
		return SourceInfo{}, ErrNoAudio
	}
	durationSeconds, err := strconv.ParseFloat(payload.Format.Duration, 64)
	if err != nil || durationSeconds <= 0 {
		return SourceInfo{}, fmt.Errorf("%w: invalid duration", ErrBadMedia)
	}
	return SourceInfo{FileName: fileName, SizeBytes: sizeBytes, DurationMS: int64(durationSeconds*1000 + .5)}, nil
}

func ParseWhisperJSON(reader io.Reader) ([]Segment, string, string, error) {
	var payload struct {
		Result struct {
			Language string `json:"language"`
		} `json:"result"`
		Transcription []struct {
			Offsets struct {
				From int64 `json:"from"`
				To   int64 `json:"to"`
			} `json:"offsets"`
			Text string `json:"text"`
		} `json:"transcription"`
	}
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return nil, "", "", fmt.Errorf("decode whisper JSON: %w", err)
	}
	segments := make([]Segment, 0, len(payload.Transcription))
	texts := make([]string, 0, len(payload.Transcription))
	for _, raw := range payload.Transcription {
		text := strings.TrimSpace(raw.Text)
		if text == "" {
			continue
		}
		segment := Segment{Index: len(segments), StartMS: raw.Offsets.From, EndMS: raw.Offsets.To, Text: text}
		if segment.StartMS < 0 || segment.EndMS < segment.StartMS {
			return nil, "", "", fmt.Errorf("invalid whisper segment %d", segment.Index)
		}
		segments = append(segments, segment)
		texts = append(texts, text)
	}
	if len(segments) == 0 {
		return nil, "", "", errors.New("whisper returned no speech segments")
	}
	return segments, strings.Join(texts, " "), payload.Result.Language, nil
}

func IsOutOfMemoryError(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "out of memory") || strings.Contains(lower, "cuda_error_out_of_memory") || strings.Contains(lower, "failed to allocate cuda")
}
