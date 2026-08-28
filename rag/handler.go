package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// Handler holds HTTP handlers for the search microservice.
type Handler struct {
	service *Service
	logger  *log.Logger
}

func NewHandler(service *Service, logger *log.Logger) *Handler {
	if logger == nil {
		logger = log.Default()
	}
	return &Handler{service: service, logger: logger}
}

type searchRequestPayload struct {
	Prompt   string `json:"prompt"`
	TopK     int    `json:"top_k"`
	AdjCount int    `json:"adj_count"`
}

type searchResponsePayload struct {
	Status  string         `json:"status"`
	Results []SearchResult `json:"results"`
}

// HandleSearch processes the vector search.
//
// POST /search
// Body: {"prompt": "how to build rag", "top_k": 3, "adj_count": 1}
func (h *Handler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "only POST is allowed")
		return
	}

	var req searchRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Prompt == "" {
		h.writeError(w, http.StatusBadRequest, "\"prompt\" is required")
		return
	}

	if req.TopK <= 0 {
		req.TopK = 5 // default
	}

	// Default adjacency to 1 (1 chunk before, 1 chunk after)
	if req.AdjCount < 0 {
		req.AdjCount = 1
	}

	h.logger.Printf("POST /search prompt=%q topK=%d adj=%d", req.Prompt, req.TopK, req.AdjCount)

	results, err := h.service.Search(r.Context(), req.Prompt, req.TopK, req.AdjCount)
	if err != nil {
		h.logger.Printf("Search failed: %v", err)
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if results == nil {
		results = []SearchResult{} // ensure JSON array instead of null
	}

	h.writeJSON(w, http.StatusOK, searchResponsePayload{
		Status:  "ok",
		Results: results,
	})
}

// HandlePing is a health check endpoint.
func (h *Handler) HandlePing(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"service": "rag-search",
	})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.logger.Printf("error encoding JSON: %v", err)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"error": msg})
}
