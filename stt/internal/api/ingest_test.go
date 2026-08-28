package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"brave/stt/internal/transcription"
)

// fakeStore stands in for the bucket: DownloadTo writes a placeholder file,
// UploadFile records what would have been pushed back.
type fakeStore struct {
	mu         sync.Mutex
	downloaded []string
	uploaded   map[string]string // key → transcript contents
	downloadTo func(key, destination string) error
}

func newFakeStore() *fakeStore {
	return &fakeStore{uploaded: map[string]string{}}
}

func (f *fakeStore) DownloadTo(_ context.Context, key, destination string) (int64, error) {
	f.mu.Lock()
	f.downloaded = append(f.downloaded, key)
	f.mu.Unlock()

	if f.downloadTo != nil {
		if err := f.downloadTo(key, destination); err != nil {
			return 0, err
		}
		info, err := os.Stat(destination)
		if err != nil {
			return 0, err
		}
		return info.Size(), nil
	}

	body := []byte("fake mp4 bytes")
	if err := os.WriteFile(destination, body, 0o644); err != nil {
		return 0, err
	}
	return int64(len(body)), nil
}

func (f *fakeStore) UploadFile(_ context.Context, key, source, _ string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploaded[key] = string(data)
	return nil
}

func (f *fakeStore) uploadedKeys() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]string, len(f.uploaded))
	for k, v := range f.uploaded {
		out[k] = v
	}
	return out
}

type ingestApp struct {
	handler  http.Handler
	store    *fakeStore
	notified chan string
}

func newIngestApp(t *testing.T, store *fakeStore, notify func(context.Context, string) error) ingestApp {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	output := filepath.Join(root, "output")
	for _, dir := range []string{work, output} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	manager := transcription.NewManager(instantRunner{}, transcription.ManagerConfig{
		MaxQueue: 20, MaxWorkers: 1, InitialWorkers: 1, Retention: time.Hour,
	})
	manager.Start(context.Background())
	t.Cleanup(manager.Stop)

	config := Config{
		InboxDir: root, WorkDir: work, OutputDir: output,
		Ready:  func(context.Context) Readiness { return Readiness{Ready: true} },
		Notify: notify,
	}
	if store != nil {
		config.Store = store
	}

	return ingestApp{handler: NewServer(manager, config).Handler(), store: store}
}

func postIngest(t *testing.T, app ingestApp, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	app.handler.ServeHTTP(recorder, request)
	return recorder
}

func TestIngestWithoutStoreIsNotImplemented(t *testing.T) {
	app := newIngestApp(t, nil, nil)
	if got := postIngest(t, app, `{"key":"film.mp4"}`).Code; got != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", got, http.StatusNotImplemented)
	}
}

func TestIngestRejectsNonMP4(t *testing.T) {
	app := newIngestApp(t, newFakeStore(), nil)

	if got := postIngest(t, app, `{"key":"raport.pdf"}`).Code; got != http.StatusUnsupportedMediaType {
		t.Errorf("pdf status = %d, want %d", got, http.StatusUnsupportedMediaType)
	}
	if got := postIngest(t, app, `{"key":""}`).Code; got != http.StatusBadRequest {
		t.Errorf("empty key status = %d, want %d", got, http.StatusBadRequest)
	}
	if got := postIngest(t, app, `not json`).Code; got != http.StatusBadRequest {
		t.Errorf("bad body status = %d, want %d", got, http.StatusBadRequest)
	}
}

// The whole point of the endpoint: hand it a key, and a transcript shows up in
// the bucket next to the recording, with the embedder told about the recording
// (not the transcript — it derives that name itself).
func TestIngestUploadsTranscriptAndNotifies(t *testing.T) {
	store := newFakeStore()
	notified := make(chan string, 1)
	app := newIngestApp(t, store, func(_ context.Context, key string) error {
		notified <- key
		return nil
	})

	recorder := postIngest(t, app, `{"key":"kurs/film.mp4"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}

	var accepted struct {
		Status        string `json:"status"`
		Key           string `json:"key"`
		TranscriptKey string `json:"transcriptKey"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if accepted.TranscriptKey != "kurs/film-transcription.txt" {
		t.Errorf("transcriptKey = %q, want %q", accepted.TranscriptKey, "kurs/film-transcription.txt")
	}

	select {
	case key := <-notified:
		if key != "kurs/film.mp4" {
			t.Errorf("embedder was told about %q, want the recording key", key)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the embedder was never notified")
	}

	uploaded := store.uploadedKeys()
	transcript, ok := uploaded["kurs/film-transcription.txt"]
	if !ok {
		t.Fatalf("no transcript uploaded, got keys: %v", uploaded)
	}
	if !strings.HasPrefix(transcript, "[0 - 2000] Test.") {
		t.Errorf("transcript = %q, want the timestamped segment format", transcript)
	}
}

func TestIngestDoesNotNotifyWhenDownloadFails(t *testing.T) {
	store := newFakeStore()
	store.downloadTo = func(string, string) error { return os.ErrNotExist }

	notified := make(chan string, 1)
	app := newIngestApp(t, store, func(_ context.Context, key string) error {
		notified <- key
		return nil
	})

	if got := postIngest(t, app, `{"key":"film.mp4"}`).Code; got != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", got, http.StatusAccepted)
	}

	// A failed download must not push a transcript or wake the embedder.
	select {
	case key := <-notified:
		t.Fatalf("embedder was notified about %q despite a failed download", key)
	case <-time.After(500 * time.Millisecond):
	}
	if uploaded := store.uploadedKeys(); len(uploaded) != 0 {
		t.Errorf("uploaded %v after a failed download, want nothing", uploaded)
	}
}
