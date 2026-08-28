// Package pipeline holds the one message stt sends onwards: "this key is
// ready, index it". Everything else the services need they work out from the
// key itself, which is what keeps the coupling between them this thin.
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// EmbedderNotifier pokes the embedder's /verity endpoint.
type EmbedderNotifier struct {
	baseURL    string
	httpClient *http.Client
}

// NewEmbedderNotifier returns a notifier, or nil when no embedder URL is set —
// a nil notifier means the transcript still lands in the bucket, it just is not
// handed on automatically.
func NewEmbedderNotifier(baseURL string) *EmbedderNotifier {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	return &EmbedderNotifier{
		baseURL: baseURL,
		// Indexing a long transcript takes a while: chunking, embedding, and
		// the round trip to the vector store all happen inside this call.
		httpClient: &http.Client{Timeout: 10 * time.Minute},
	}
}

// Notify asks the embedder to index the object at sourceKey.
func (n *EmbedderNotifier) Notify(ctx context.Context, sourceKey string) error {
	body, err := json.Marshal(map[string]string{"key": sourceKey})
	if err != nil {
		return fmt.Errorf("marshaling notify body: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+"/verity", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating notify request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := n.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("calling embedder: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("embedder returned %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}

	return nil
}
