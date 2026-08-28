package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedIndexContainsWorkingDashboardContract(t *testing.T) {
	handler := IndexHandler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	html := response.Body.String()
	for _, required := range []string{
		`id="inboxList"`, `id="uploadInput"`, `id="jobsList"`,
		`fetch("/readyz")`, `fetch("/api/v1/inbox`, `fetch("/api/v1/transcriptions`,
		`/api/v1/batches`, `actions/cancel`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("index is missing %q", required)
		}
	}
}
