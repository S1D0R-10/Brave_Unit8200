package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// Handler holds HTTP handlers for the microservice.
type Handler struct {
	processor *Processor
	logger    *log.Logger
}

// NewHandler creates a new Handler.
func NewHandler(processor *Processor, logger *log.Logger) *Handler {
	if logger == nil {
		logger = log.Default()
	}
	return &Handler{processor: processor, logger: logger}
}

// verityRequest is the expected JSON body for POST /verity.
type verityRequest struct {
	Key string `json:"key"` // S3 object key (e.g. "Dojrzewanie-bez-wstydu.pdf")
}

// HandleVerity processes a file from the S3 bucket: download → chunk → embed → store.
// Returns only a status — no chunk data leaves the backend.
//
// POST /verity
// Body: {"key": "filename.pdf"}
// Response: 200 {"status": "ok"} or 4xx/5xx {"error": "..."}
func (h *Handler) HandleVerity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "only POST is allowed")
		return
	}

	var req verityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if req.Key == "" {
		h.writeError(w, http.StatusBadRequest, "\"key\" is required")
		return
	}

	h.logger.Printf("POST /verity key=%q", req.Key)

	if err := h.processor.ProcessFile(r.Context(), req.Key); err != nil {
		h.logger.Printf("error processing %q: %v", req.Key, err)
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandlePing is a health check endpoint.
//
// GET /ping
// Response: {"status": "ok", "version": "0.1.0"}
func (h *Handler) HandlePing(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": h.processor.Version(),
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
