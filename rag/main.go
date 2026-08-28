package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func main() {
	loadEnv(".env")
	logger := log.New(os.Stdout, "[rag-search] ", log.LstdFlags|log.Lshortfile)

	port := envOrDefault("SEARCH_PORT", "8081")

	embedCfg := EmbedderConfig{
		Endpoint: envOrDefault("RAG_EMBED_ENDPOINT", "https://api.openai.com/v1/embeddings"),
		APIKey:   os.Getenv("RAG_EMBED_API_KEY"),
		Model:    envOrDefault("RAG_EMBED_MODEL", "text-embedding-3-small"),
	}

	// Chat completions reuse the same OpenRouter key as embeddings —
	// RAG_EMBED_ENDPOINT already points at openrouter.ai/api/v1/embeddings,
	// so RAG_EMBED_API_KEY is an OpenRouter key and works for chat too.
	chatCfg := ChatConfig{
		Endpoint: envOrDefault("RAG_CHAT_ENDPOINT", "https://openrouter.ai/api/v1/chat/completions"),
		APIKey:   envOrDefault("RAG_CHAT_API_KEY", os.Getenv("RAG_EMBED_API_KEY")),
		Model:    envOrDefault("RAG_CHAT_MODEL", "openai/gpt-4o-mini"),
	}

	qdrantCfg := QdrantConfig{
		Host:       envOrDefault("RAG_QDRANT_HOST", "qdrant.railway.internal"),
		Port:       envOrInt("RAG_QDRANT_PORT", 6333),
		Collection: "citations",
	}

	// The bucket is where chunk text actually lives: Qdrant holds byte offsets,
	// rag-search reads the cited bytes back with a Range request.
	s3Cfg := S3Config{
		Endpoint:  envOrDefault("RAG_S3_ENDPOINT", "https://t3.storageapi.dev"),
		Bucket:    envOrDefault("RAG_S3_BUCKET", "wiadro-xuw-on7mmw3fdswei6"),
		Region:    envOrDefault("RAG_S3_REGION", "auto"),
		AccessKey: os.Getenv("RAG_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("RAG_S3_SECRET_KEY"),
	}

	noCoverageThreshold := envOrFloat("RAG_NO_COVERAGE_THRESHOLD", 0.5)
	topK := envOrInt("RAG_DRAFT_TOP_K", 5)

	storage, err := NewS3Storage(s3Cfg, logger)
	if err != nil {
		logger.Fatalf("failed to create S3 storage: %v", err)
	}

	embedder := NewEmbedder(embedCfg, logger)
	qdrant := NewQdrantClient(qdrantCfg, logger)
	chat := NewChatClient(chatCfg, logger)
	service := NewService(logger, embedder, qdrant, storage, ServiceConfig{
		MaxQuoteBytes: int64(envOrInt("RAG_MAX_QUOTE_BYTES", 8000)),
	})
	draftService := NewDraftService(logger, service, chat, noCoverageThreshold, topK)
	store := NewStore(logger, qdrant)
	handler := NewHandler(service, draftService, store, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/search", handler.HandleSearch)
	mux.HandleFunc("/draft", handler.HandleDraft)
	mux.HandleFunc("/feedback", handler.HandleFeedback)
	mux.HandleFunc("/handoff", handler.HandleHandoff)
	mux.HandleFunc("/kb/stats", handler.HandleKbStats)
	mux.HandleFunc("/kb/files", handler.HandleKbFiles)
	mux.HandleFunc("/kb/files/chunks", handler.HandleKbFileChunks)
	mux.HandleFunc("/ping", handler.HandlePing)

	addr := fmt.Sprintf(":%s", port)
	logger.Printf("Starting RAG Search on %s", addr)
	logger.Printf("Embed: %s model=%s", embedCfg.Endpoint, embedCfg.Model)
	logger.Printf("Chat: %s model=%s", chatCfg.Endpoint, chatCfg.Model)
	logger.Printf("Qdrant: %s:%d", qdrantCfg.Host, qdrantCfg.Port)
	logger.Printf("S3: %s/%s", s3Cfg.Endpoint, s3Cfg.Bucket)

	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		logger.Fatalf("server error: %v", err)
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envOrInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func envOrFloat(key string, defaultVal float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultVal
	}
	return n
}

func loadEnv(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
				val = val[1 : len(val)-1]
			}
			os.Setenv(key, val)
		}
	}
}
