package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"

	"brave/stt/internal/objectstore"
	"brave/stt/internal/transcription"
)

// TranscriptSuffix is what a recording's transcript is called in the bucket.
// The embedder derives the same name from the recording's key, which is the
// only thing the two services have to agree on.
const TranscriptSuffix = "-transcription.txt"

// ObjectStore is the slice of the bucket this service needs: pull a recording
// down by key, push its transcript back up.
type ObjectStore interface {
	DownloadTo(ctx context.Context, key, destination string) (int64, error)
	UploadFile(ctx context.Context, key, source, contentType string) error
}

type ingestRequestPayload struct {
	Key string `json:"key"`
}

// ingest transcribes a recording that already sits in the bucket.
//
// The work is deliberately not done inside the request: a two-hour MP4 takes
// minutes just to download and hours to transcribe, so the caller gets a 202
// and the pipeline carries itself from there — transcript to the bucket, then
// a nudge to the embedder.
//
// POST /api/v1/ingest
// Body: {"key": "film.mp4"}
func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	if s.config.Store == nil {
		writeError(w, r, http.StatusNotImplemented, "object_store_unavailable",
			"Serwis nie ma skonfigurowanego bucketa.", nil)
		return
	}

	var payload ingestRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "Żądanie musi być poprawnym JSON-em.", nil)
		return
	}

	key := strings.TrimSpace(payload.Key)
	if key == "" {
		writeError(w, r, http.StatusBadRequest, "key_required", "Pole key jest wymagane.", nil)
		return
	}
	if !strings.EqualFold(path.Ext(key), ".mp4") {
		writeError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type",
			"Obsługiwane są wyłącznie pliki MP4.", map[string]any{"key": key})
		return
	}

	transcriptKey := objectstore.SiblingKey(key, TranscriptSuffix)

	// Detached from the request context on purpose: the client is not going to
	// wait around, and cancelling the download when it hangs up would be wrong.
	go s.runIngest(context.WithoutCancel(r.Context()), key, transcriptKey)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":        "accepted",
		"key":           key,
		"transcriptKey": transcriptKey,
	})
}

// runIngest downloads the recording, queues it, and arranges for the finished
// transcript to be pushed back and handed on.
func (s *Server) runIngest(ctx context.Context, key, transcriptKey string) {
	logger := s.logger().With("key", key)

	// Reserve a unique name up front, so re-ingesting a key while an earlier
	// run is still downloading cannot have the two overwrite each other.
	reserved, err := os.CreateTemp(s.config.WorkDir, "ingest-*-"+sanitizeFileName(path.Base(key)))
	if err != nil {
		logger.Error("cannot create a working file", "error", err)
		return
	}
	local := reserved.Name()
	_ = reserved.Close()

	size, err := s.config.Store.DownloadTo(ctx, key, local)
	if err != nil {
		_ = os.Remove(local)
		logger.Error("cannot fetch recording from the bucket", "error", err)
		return
	}
	logger.Info("fetched recording from the bucket", "bytes", size)

	_, err = s.manager.Enqueue(transcription.EnqueueRequest{
		FileName:    path.Base(key),
		SizeBytes:   size,
		SourcePath:  local,
		OutputRoot:  s.config.OutputDir,
		DeleteInput: true,
		OnFinish: func(job transcription.Job) {
			s.publishTranscript(ctx, key, transcriptKey, job)
		},
	})
	if err != nil {
		_ = os.Remove(local)
		logger.Error("cannot queue the recording", "error", err)
	}
}

// publishTranscript uploads a finished transcript and tells the next stage.
func (s *Server) publishTranscript(ctx context.Context, key, transcriptKey string, job transcription.Job) {
	logger := s.logger().With("key", key, "job", job.ID)

	if job.Status != transcription.StatusSucceeded || job.Artifacts == nil || job.Artifacts.TXT == "" {
		reason := "no transcript artifact"
		if job.Error != nil {
			reason = job.Error.Message
		}
		logger.Error("transcription did not produce a transcript", "status", job.Status, "reason", reason)
		return
	}

	if err := s.config.Store.UploadFile(ctx, transcriptKey, job.Artifacts.TXT, "text/plain; charset=utf-8"); err != nil {
		logger.Error("cannot upload the transcript", "transcriptKey", transcriptKey, "error", err)
		return
	}
	logger.Info("uploaded transcript", "transcriptKey", transcriptKey)

	if s.config.Notify == nil {
		return
	}

	// The embedder is handed the *recording's* key, not the transcript's: it
	// resolves the transcript name itself, so this message stays a single key.
	if err := s.config.Notify(ctx, key); err != nil {
		logger.Error("cannot hand the recording on to the embedder", "error", err)
		return
	}
	logger.Info("handed the recording on to the embedder")
}

func (s *Server) logger() *slog.Logger {
	if s.config.Logger != nil {
		return s.config.Logger
	}
	return slog.Default()
}

// sanitizeFileName keeps a bucket key from reaching outside the work directory.
func sanitizeFileName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, name)
	name = strings.TrimLeft(name, ".")
	if name == "" {
		return "recording.mp4"
	}
	return name
}
