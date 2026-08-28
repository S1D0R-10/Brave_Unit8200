package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"brave/stt/internal/transcription"
)

type instantRunner struct{}

func (instantRunner) Run(_ context.Context, request transcription.RunRequest, update func(transcription.ProgressUpdate)) (transcription.RunOutput, error) {
	update(transcription.ProgressUpdate{Stage: transcription.StageTranscribing, Percent: 50})
	result := transcription.Result{
		SchemaVersion: "1.0", TranscriptionID: request.ID,
		Source:   transcription.SourceInfo{FileName: request.FileName, SizeBytes: request.SizeBytes, DurationMS: 2000},
		Language: "pl", Text: "Test.", Segments: []transcription.Segment{{Index: 0, StartMS: 0, EndMS: 2000, Text: "Test."}},
		Engine: transcription.EngineInfo{Name: "whisper.cpp", Version: "1.9.1", Model: "large-v3-turbo-q5_0"}, CreatedAt: time.Now().UTC(),
	}
	paths, err := transcription.WriteArtifactsAtomic(request.OutputDir, result)
	return transcription.RunOutput{Result: result, Artifacts: paths}, err
}

type testApp struct {
	handler http.Handler
	manager *transcription.Manager
	inbox   string
	work    string
	output  string
}

func newTestApp(t *testing.T, maxUpload int64) testApp {
	t.Helper()
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	work := filepath.Join(root, "work")
	output := filepath.Join(root, "output")
	for _, dir := range []string{inbox, work, output} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	manager := transcription.NewManager(instantRunner{}, transcription.ManagerConfig{MaxQueue: 20, MaxWorkers: 1, InitialWorkers: 1, Retention: time.Hour})
	manager.Start(context.Background())
	t.Cleanup(manager.Stop)
	server := NewServer(manager, Config{InboxDir: inbox, WorkDir: work, OutputDir: output, MaxUploadBytes: maxUpload, Ready: func(context.Context) Readiness { return Readiness{Ready: true} }})
	return testApp{handler: server.Handler(), manager: manager, inbox: inbox, work: work, output: output}
}

func TestInboxListsOnlySafeMP4Files(t *testing.T) {
	app := newTestApp(t, 1024)
	_ = os.WriteFile(filepath.Join(app.inbox, "b.MP4"), []byte("b"), 0o600)
	_ = os.WriteFile(filepath.Join(app.inbox, "a.mp4"), []byte("a"), 0o600)
	_ = os.WriteFile(filepath.Join(app.inbox, "notes.txt"), []byte("x"), 0o600)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/inbox?limit=1", nil)
	response := httptest.NewRecorder()
	app.handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Items      []InboxFile `json:"items"`
		NextCursor string      `json:"nextCursor"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 || payload.Items[0].Name != "a.mp4" || payload.NextCursor == "" {
		t.Fatalf("unexpected inbox page: %#v", payload)
	}
}

func TestInboxFilesAreNotSubjectToHTTPUploadLimit(t *testing.T) {
	app := newTestApp(t, 4)
	path := filepath.Join(app.inbox, "long-recording.mp4")
	if err := os.WriteFile(path, []byte("larger than upload limit"), 0o600); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	app.handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/inbox", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "long-recording.mp4") {
		t.Fatalf("large inbox file should be listed: %d %s", response.Code, response.Body.String())
	}

	batch := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/batches", strings.NewReader(`{"files":["long-recording.mp4"]}`))
	request.Header.Set("Content-Type", "application/json")
	app.handler.ServeHTTP(batch, request)
	if batch.Code != http.StatusAccepted {
		t.Fatalf("large inbox file should be accepted: %d %s", batch.Code, batch.Body.String())
	}
}

func TestBatchRejectsPathTraversal(t *testing.T) {
	app := newTestApp(t, 1024)
	body := strings.NewReader(`{"files":["../secret.mp4"]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/batches", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	assertErrorCode(t, response.Body.Bytes(), "invalid_inbox_file")
}

func TestBatchDoesNotEnqueuePartialJobsWhenAnyFileIsInvalid(t *testing.T) {
	app := newTestApp(t, 1024)
	if err := os.WriteFile(filepath.Join(app.inbox, "valid.mp4"), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/batches", strings.NewReader(`{"files":["valid.mp4","../invalid.mp4"]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if jobs := app.manager.List(); len(jobs) != 0 {
		t.Fatalf("batch was partially enqueued: %#v", jobs)
	}
}

func TestBatchCreatesJobsForInboxFiles(t *testing.T) {
	app := newTestApp(t, 1024)
	_ = os.WriteFile(filepath.Join(app.inbox, "film.mp4"), []byte("video"), 0o600)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/batches", strings.NewReader(`{"files":["film.mp4"]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		BatchID string              `json:"batchId"`
		Jobs    []transcription.Job `json:"jobs"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.BatchID == "" || len(payload.Jobs) != 1 {
		t.Fatalf("unexpected batch: %#v", payload)
	}
}

func TestUploadStreamsFileAndEnforcesLimit(t *testing.T) {
	app := newTestApp(t, 8)
	request := multipartRequest(t, "/api/v1/transcriptions", "film.mp4", []byte("123456789"))
	response := httptest.NewRecorder()
	app.handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	assertErrorCode(t, response.Body.Bytes(), "upload_too_large")
}

func TestCompletedJobExposesResultAndArtifact(t *testing.T) {
	app := newTestApp(t, 1024)
	request := multipartRequest(t, "/api/v1/transcriptions", "film.mp4", []byte("mp4"))
	response := httptest.NewRecorder()
	app.handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var accepted transcription.Job
	if err := json.NewDecoder(response.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}
	waitJob(t, app.manager, accepted.ID)

	resultResponse := httptest.NewRecorder()
	app.handler.ServeHTTP(resultResponse, httptest.NewRequest(http.MethodGet, "/api/v1/transcriptions/"+accepted.ID+"/result", nil))
	if resultResponse.Code != http.StatusOK || !bytes.Contains(resultResponse.Body.Bytes(), []byte(`"schemaVersion":"1.0"`)) {
		t.Fatalf("unexpected result: %d %s", resultResponse.Code, resultResponse.Body.String())
	}

	artifactResponse := httptest.NewRecorder()
	app.handler.ServeHTTP(artifactResponse, httptest.NewRequest(http.MethodGet, "/api/v1/transcriptions/"+accepted.ID+"/artifacts/txt", nil))
	if artifactResponse.Code != http.StatusOK || !strings.Contains(artifactResponse.Body.String(), "[0 - 2000] Test.") {
		t.Fatalf("unexpected artifact: %d %s", artifactResponse.Code, artifactResponse.Body.String())
	}
	if cd := artifactResponse.Header().Get("Content-Disposition"); !strings.Contains(cd, "film-transcription.txt") {
		t.Fatalf("unexpected content-disposition: %s", cd)
	}
}

func TestQueueETAMatchesSlowestParallelWorker(t *testing.T) {
	longETA, shortETA := int64(100), int64(10)
	jobs := []transcription.Job{
		{Status: transcription.StatusRunning, Progress: transcription.Progress{ETASeconds: &longETA}},
		{Status: transcription.StatusRunning, Progress: transcription.Progress{ETASeconds: &shortETA}},
	}
	summary := summarize(jobs, 2)
	if got := summary["etaSeconds"]; got != int64(100) {
		t.Fatalf("parallel queue ETA = %v; want 100", got)
	}
}

func multipartRequest(t *testing.T, target, name string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func assertErrorCode(t *testing.T, body []byte, code string) {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != code {
		t.Fatalf("error code = %q; want %q; body=%s", payload.Error.Code, code, body)
	}
}

func waitJob(t *testing.T, manager *transcription.Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if job, ok := manager.Get(id); ok && job.Status == transcription.StatusSucceeded {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not complete")
}
