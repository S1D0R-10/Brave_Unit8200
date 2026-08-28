package transcription

import (
	"strings"
	"testing"
)

func TestParseProbeAcceptsMP4WithAudio(t *testing.T) {
	payload := `{"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"12.345"},"streams":[{"codec_type":"video"},{"codec_type":"audio"}]}`
	info, err := ParseProbe(strings.NewReader(payload), "film.mp4", 42)
	if err != nil {
		t.Fatal(err)
	}
	if info.FileName != "film.mp4" || info.SizeBytes != 42 || info.DurationMS != 12345 {
		t.Fatalf("unexpected source info: %#v", info)
	}
}

func TestParseProbeRejectsFileWithoutAudio(t *testing.T) {
	payload := `{"format":{"format_name":"mov,mp4","duration":"12"},"streams":[{"codec_type":"video"}]}`
	if _, err := ParseProbe(strings.NewReader(payload), "silent.mp4", 42); err == nil {
		t.Fatal("expected missing audio error")
	}
}

func TestParseWhisperJSONUsesMillisecondOffsets(t *testing.T) {
	payload := `{
      "result":{"language":"pl"},
      "transcription":[
        {"timestamps":{"from":"00:00:00,000","to":"00:00:02,500"},"offsets":{"from":0,"to":2500},"text":" Dzień dobry."},
        {"timestamps":{"from":"00:00:02,500","to":"00:00:06,000"},"offsets":{"from":2500,"to":6000},"text":" Test."}
      ]
    }`
	segments, text, language, err := ParseWhisperJSON(strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if language != "pl" || text != "Dzień dobry. Test." || len(segments) != 2 {
		t.Fatalf("unexpected parse result: %q %q %#v", language, text, segments)
	}
	if segments[1].StartMS != 2500 || segments[1].EndMS != 6000 {
		t.Fatalf("offsets were not preserved: %#v", segments[1])
	}
}

func TestIsOutOfMemoryError(t *testing.T) {
	if !IsOutOfMemoryError("ggml_backend_cuda_buffer_type_alloc_buffer: allocating 123 MiB failed: out of memory") {
		t.Fatal("CUDA OOM was not detected")
	}
	if IsOutOfMemoryError("invalid media file") {
		t.Fatal("ordinary error detected as OOM")
	}
}
