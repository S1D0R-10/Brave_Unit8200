package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"brave/stt/internal/transcription"
)

type Config struct {
	InboxDir       string
	WorkDir        string
	OutputDir      string
	MaxUploadBytes int64
	Ready          func(context.Context) Readiness
	Index          http.Handler
	Logger         *slog.Logger

	// Store and Notify wire this service into the pipeline. Without a Store,
	// POST /api/v1/ingest reports 501 and the panel keeps working on its own.
	Store  ObjectStore
	Notify func(ctx context.Context, sourceKey string) error
}

type Readiness struct {
	Ready   bool                   `json:"ready"`
	Checks  map[string]bool        `json:"checks,omitempty"`
	GPU     *transcription.GPUInfo `json:"gpu,omitempty"`
	Model   string                 `json:"model,omitempty"`
	Workers int                    `json:"workers,omitempty"`
	Message string                 `json:"message,omitempty"`
}

type InboxFile struct {
	Name       string    `json:"name"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type Server struct {
	manager *transcription.Manager
	config  Config
	mux     *http.ServeMux
}

func NewServer(manager *transcription.Manager, config Config) *Server {
	if config.MaxUploadBytes <= 0 {
		config.MaxUploadBytes = 3 * 1024 * 1024 * 1024
	}
	server := &Server{manager: manager, config: config, mux: http.NewServeMux()}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return requestIDMiddleware(recoverMiddleware(s.mux))
}

func (s *Server) routes() {
	if s.config.Index != nil {
		s.mux.Handle("GET /", s.config.Index)
	}
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /readyz", s.ready)
	s.mux.HandleFunc("GET /api/v1/inbox", s.listInbox)
	s.mux.HandleFunc("POST /api/v1/transcriptions", s.upload)
	s.mux.HandleFunc("POST /api/v1/ingest", s.ingest)
	s.mux.HandleFunc("POST /api/v1/batches", s.createBatch)
	s.mux.HandleFunc("GET /api/v1/transcriptions", s.listJobs)
	s.mux.HandleFunc("GET /api/v1/transcriptions/{id}", s.getJob)
	s.mux.HandleFunc("POST /api/v1/transcriptions/{id}/actions/cancel", s.cancelJob)
	s.mux.HandleFunc("GET /api/v1/transcriptions/{id}/result", s.getResult)
	s.mux.HandleFunc("GET /api/v1/transcriptions/{id}/artifacts/{format}", s.getArtifact)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	readiness := Readiness{Ready: true}
	if s.config.Ready != nil {
		readiness = s.config.Ready(r.Context())
	}
	readiness.Workers = s.manager.WorkerLimit()
	status := http.StatusOK
	if !readiness.Ready {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, readiness)
}

func (s *Server) listInbox(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.config.InboxDir)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "inbox_unavailable", "Nie można odczytać folderu wejściowego.", nil)
		return
	}
	items := make([]InboxFile, 0)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".mp4") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			continue
		}
		items = append(items, InboxFile{Name: entry.Name(), SizeBytes: info.Size(), ModifiedAt: info.ModTime().UTC()})
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	limit := parseLimit(r, 25, 100)
	cursor := decodeCursor(r.URL.Query().Get("cursor"))
	start := 0
	if cursor != "" {
		for i := range items {
			if items[i].Name == cursor {
				start = i + 1
				break
			}
		}
	}
	end := min(start+limit, len(items))
	next := ""
	if end < len(items) && end > start {
		next = encodeCursor(items[end-1].Name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items[start:end], "nextCursor": next})
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_multipart", "Żądanie musi zawierać plik multipart.", nil)
		return
	}
	part, err := nextFilePart(reader)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "file_required", "Pole file jest wymagane.", nil)
		return
	}
	defer part.Close()
	name := filepath.Base(part.FileName())
	if !strings.EqualFold(filepath.Ext(name), ".mp4") {
		writeError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Obsługiwane są wyłącznie pliki MP4.", nil)
		return
	}
	tmp, err := os.CreateTemp(s.config.WorkDir, "upload-*.mp4")
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "storage_unavailable", "Nie można zapisać pliku roboczego.", nil)
		return
	}
	tmpPath := tmp.Name()
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(tmpPath)
		}
	}()
	written, copyErr := io.Copy(tmp, io.LimitReader(part, s.config.MaxUploadBytes+1))
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		writeError(w, r, http.StatusServiceUnavailable, "upload_failed", "Nie udało się zapisać pliku.", nil)
		return
	}
	if written > s.config.MaxUploadBytes {
		writeError(w, r, http.StatusRequestEntityTooLarge, "upload_too_large", "Plik przekracza limit 3 GB.", map[string]any{"maxBytes": s.config.MaxUploadBytes})
		return
	}
	job, err := s.manager.Enqueue(transcription.EnqueueRequest{FileName: name, SizeBytes: written, SourcePath: tmpPath, OutputRoot: s.config.OutputDir, DeleteInput: true})
	if err != nil {
		s.writeEnqueueError(w, r, err)
		return
	}
	remove = false
	w.Header().Set("Location", "/api/v1/transcriptions/"+job.ID)
	writeJSON(w, http.StatusAccepted, jobView(job))
}

func nextFilePart(reader *multipart.Reader) (*multipart.Part, error) {
	for {
		part, err := reader.NextPart()
		if err != nil {
			return nil, err
		}
		if part.FormName() == "file" && part.FileName() != "" {
			return part, nil
		}
		_ = part.Close()
	}
}

func (s *Server) createBatch(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Files []string `json:"files"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || len(payload.Files) == 0 || len(payload.Files) > 20 {
		writeError(w, r, http.StatusUnprocessableEntity, "invalid_batch", "Podaj od 1 do 20 plików z inboxa.", nil)
		return
	}
	batchID, _ := randomID()
	type batchSource struct {
		name string
		path string
		info os.FileInfo
	}
	sources := make([]batchSource, 0, len(payload.Files))
	for _, name := range payload.Files {
		path, info, err := s.safeInboxFile(name)
		if err != nil {
			writeError(w, r, http.StatusUnprocessableEntity, "invalid_inbox_file", "Nieprawidłowy plik w folderze wejściowym.", map[string]any{"file": name})
			return
		}
		sources = append(sources, batchSource{name: name, path: path, info: info})
	}
	jobs := make([]any, 0, len(payload.Files))
	for _, source := range sources {
		job, err := s.manager.Enqueue(transcription.EnqueueRequest{BatchID: batchID, FileName: source.name, SizeBytes: source.info.Size(), SourcePath: source.path, OutputRoot: s.config.OutputDir})
		if err != nil {
			s.writeEnqueueError(w, r, err)
			return
		}
		jobs = append(jobs, jobView(job))
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"batchId": batchID, "jobs": jobs})
}

func (s *Server) safeInboxFile(name string) (string, os.FileInfo, error) {
	if name == "" || filepath.Base(name) != name || !strings.EqualFold(filepath.Ext(name), ".mp4") || strings.ContainsAny(name, `/\`) {
		return "", nil, errors.New("unsafe file name")
	}
	path := filepath.Join(s.config.InboxDir, name)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", nil, errors.New("file unavailable")
	}
	return path, info, nil
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs := s.manager.List()
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].CreatedAt.After(jobs[j].CreatedAt) })
	limit := parseLimit(r, 25, 100)
	cursor := decodeCursor(r.URL.Query().Get("cursor"))
	start := 0
	if cursor != "" {
		for i := range jobs {
			if jobs[i].ID == cursor {
				start = i + 1
				break
			}
		}
	}
	end := min(start+limit, len(jobs))
	items := make([]any, 0, end-start)
	for _, job := range jobs[start:end] {
		items = append(items, jobView(job))
	}
	next := ""
	if end < len(jobs) && end > start {
		next = encodeCursor(jobs[end-1].ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next, "summary": summarize(jobs, s.manager.WorkerLimit())})
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.manager.Get(r.PathValue("id"))
	if !ok {
		writeError(w, r, http.StatusNotFound, "transcription_not_found", "Nie znaleziono transkrypcji.", nil)
		return
	}
	writeJSON(w, http.StatusOK, jobView(job))
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.Cancel(r.PathValue("id")); err != nil {
		writeError(w, r, http.StatusNotFound, "transcription_not_found", "Nie znaleziono transkrypcji.", nil)
		return
	}
	job, _ := s.manager.Get(r.PathValue("id"))
	writeJSON(w, http.StatusOK, jobView(job))
}

func (s *Server) getResult(w http.ResponseWriter, r *http.Request) {
	job, ok := s.manager.Get(r.PathValue("id"))
	if !ok {
		writeError(w, r, http.StatusNotFound, "transcription_not_found", "Nie znaleziono transkrypcji.", nil)
		return
	}
	if job.Status != transcription.StatusSucceeded || job.Result == nil {
		writeError(w, r, http.StatusConflict, "transcription_not_ready", "Transkrypcja nie jest jeszcze gotowa.", map[string]any{"status": job.Status})
		return
	}
	writeJSON(w, http.StatusOK, job.Result)
}

func (s *Server) getArtifact(w http.ResponseWriter, r *http.Request) {
	job, ok := s.manager.Get(r.PathValue("id"))
	if !ok {
		writeError(w, r, http.StatusNotFound, "transcription_not_found", "Nie znaleziono transkrypcji.", nil)
		return
	}
	if job.Status != transcription.StatusSucceeded || job.Artifacts == nil {
		writeError(w, r, http.StatusConflict, "transcription_not_ready", "Artefakty nie są jeszcze gotowe.", nil)
		return
	}
	if strings.ToLower(r.PathValue("format")) != "txt" || job.Artifacts.TXT == "" {
		writeError(w, r, http.StatusNotFound, "artifact_not_found", "Nieznany format artefaktu.", nil)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(job.Artifacts.TXT)))
	http.ServeFile(w, r, job.Artifacts.TXT)
}

func (s *Server) writeEnqueueError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, transcription.ErrQueueFull) {
		w.Header().Set("Retry-After", "30")
		writeError(w, r, http.StatusServiceUnavailable, "queue_full", "Kolejka transkrypcji jest pełna.", nil)
		return
	}
	writeError(w, r, http.StatusInternalServerError, "enqueue_failed", "Nie udało się utworzyć zadania.", nil)
}

func jobView(job transcription.Job) map[string]any {
	view := map[string]any{"id": job.ID, "status": job.Status, "progress": job.Progress, "source": job.Source, "createdAt": job.CreatedAt}
	if job.BatchID != "" {
		view["batchId"] = job.BatchID
	}
	if job.StartedAt != nil {
		view["startedAt"] = job.StartedAt
	}
	if job.FinishedAt != nil {
		view["finishedAt"] = job.FinishedAt
	}
	if job.Error != nil {
		view["error"] = job.Error
	}
	if job.Status == transcription.StatusSucceeded {
		base := "/api/v1/transcriptions/" + job.ID
		view["resultUrl"] = base + "/result"
		view["artifacts"] = map[string]string{"txt": base + "/artifacts/txt"}
	}
	return view
}

func summarize(jobs []transcription.Job, workers int) map[string]any {
	counts := map[transcription.Status]int{}
	var runningETA, queuedETA int64
	for _, job := range jobs {
		counts[job.Status]++
		if job.Progress.ETASeconds != nil {
			switch job.Status {
			case transcription.StatusRunning:
				runningETA = max(runningETA, *job.Progress.ETASeconds)
			case transcription.StatusQueued:
				queuedETA += *job.Progress.ETASeconds
			}
		}
	}
	if workers < 1 {
		workers = 1
	}
	eta := runningETA + (queuedETA+int64(workers)-1)/int64(workers)
	return map[string]any{"queued": counts[transcription.StatusQueued], "running": counts[transcription.StatusRunning], "succeeded": counts[transcription.StatusSucceeded], "failed": counts[transcription.StatusFailed], "canceled": counts[transcription.StatusCanceled], "workers": workers, "etaSeconds": eta}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	payload := map[string]any{"code": code, "message": message, "requestId": requestID(r.Context())}
	if details != nil {
		payload["details"] = details
	}
	writeJSON(w, status, map[string]any{"error": payload})
}

func parseLimit(r *http.Request, fallback, maximum int) int {
	value, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || value < 1 {
		return fallback
	}
	return min(value, maximum)
}

func encodeCursor(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
func decodeCursor(value string) string {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return ""
	}
	return string(decoded)
}

type contextKey string

const requestIDKey contextKey = "request-id"

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := randomID()
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func requestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}
func randomID() (string, error) { return fmt.Sprintf("%d", time.Now().UnixNano()), nil }

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				writeError(w, r, http.StatusInternalServerError, "internal_error", "Wewnętrzny błąd serwera.", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
