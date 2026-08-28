package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// Handler holds HTTP handlers for the search microservice.
type Handler struct {
	service *Service
	draft   *DraftService
	store   *Store
	logger  *log.Logger
}

func NewHandler(service *Service, draft *DraftService, store *Store, logger *log.Logger) *Handler {
	if logger == nil {
		logger = log.Default()
	}
	return &Handler{service: service, draft: draft, store: store, logger: logger}
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
		"status":  "ok",
		"service": "rag-search",
	})
}

type draftRequestPayload struct {
	Question string `json:"question"`
	PageURL  string `json:"page_url"`
}

// HandleDraft turns a question into a grounded, cited draft answer (or
// "no_coverage"/"blocked").
//
// POST /draft
// Body: {"question": "...", "page_url": "..."}
func (h *Handler) HandleDraft(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "only POST is allowed")
		return
	}

	var req draftRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Question == "" {
		h.writeError(w, http.StatusBadRequest, "\"question\" is required")
		return
	}

	h.logger.Printf("POST /draft question=%q", req.Question)

	result, err := h.draft.Draft(r.Context(), req.Question)
	if err != nil {
		h.logger.Printf("Draft failed: %v", err)
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

type feedbackRequestPayload struct {
	AnswerID string `json:"answerId"`
	Vote     int    `json:"vote"`
}

// HandleFeedback records a thumbs up/down (vote: 1 or -1) on an answer.
//
// POST /feedback
// Body: {"answerId": "...", "vote": 1}
func (h *Handler) HandleFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "only POST is allowed")
		return
	}

	var req feedbackRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Vote != 1 && req.Vote != -1 {
		h.writeError(w, http.StatusBadRequest, "\"vote\" must be 1 or -1")
		return
	}

	if err := h.store.SaveFeedback(r.Context(), req.AnswerID, req.Vote); err != nil {
		h.logger.Printf("SaveFeedback failed: %v", err)
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type handoffRequestPayload struct {
	AnswerID string `json:"answerId"`
	Question string `json:"question"`
	To       string `json:"to"`
	Urgent   bool   `json:"urgent"`
}

// HandleHandoff records a request to hand a question off to a human expert.
//
// POST /handoff
// Body: {"answerId": "...", "question": "...", "to": "expert", "urgent": false}
func (h *Handler) HandleHandoff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "only POST is allowed")
		return
	}

	var req handoffRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.To == "" {
		h.writeError(w, http.StatusBadRequest, "\"to\" is required")
		return
	}

	if err := h.store.SaveHandoff(r.Context(), req.AnswerID, req.Question, req.To, req.Urgent); err != nil {
		h.logger.Printf("SaveHandoff failed: %v", err)
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleKbFiles lists the source documents actually present in the index.
//
// GET /kb/files
func (h *Handler) HandleKbFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "only GET is allowed")
		return
	}

	files, err := h.store.KbFiles(r.Context())
	if err != nil {
		h.logger.Printf("KbFiles failed: %v", err)
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{"files": files})
}

// HandleKbStats reports how many source documents are indexed and when the
// most recent one was ingested.
//
// GET /kb/stats
func (h *Handler) HandleKbStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "only GET is allowed")
		return
	}

	stats, err := h.store.KbStats(r.Context())
	if err != nil {
		h.logger.Printf("KbStats failed: %v", err)
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, stats)
}

// withCORS allows the browser extension (chrome-extension://…,
// moz-extension://…) and the companion web app to call this API directly
// from extension pages / the browser, without a same-origin backend proxy.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
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
