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

	qdrantCfg := QdrantConfig{
		Host:       envOrDefault("RAG_QDRANT_HOST", "qdrant.railway.internal"),
		Port:       envOrInt("RAG_QDRANT_PORT", 6333),
		Collection: "citations",
	}

	s3Cfg := S3Config{
		Endpoint:  envOrDefault("RAG_S3_ENDPOINT", "https://t3.storageapi.dev"),
		Bucket:    envOrDefault("RAG_S3_BUCKET", "wiadro-xuw-on7mmw3fdswei6"),
		Region:    envOrDefault("RAG_S3_REGION", "auto"),
		AccessKey: os.Getenv("RAG_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("RAG_S3_SECRET_KEY"),
	}

	// The chat model reuses the embeddings key unless it is given its own.
	llmCfg := LLMConfig{
		Endpoint:    envOrDefault("RAG_LLM_ENDPOINT", "https://api.openai.com/v1/chat/completions"),
		APIKey:      envOrDefault("RAG_LLM_API_KEY", os.Getenv("RAG_EMBED_API_KEY")),
		Model:       envOrDefault("RAG_LLM_MODEL", "gpt-4o-mini"),
		MaxTokens:   envOrInt("RAG_LLM_MAX_TOKENS", 800),
		Temperature: 0,
	}

	embedder := NewEmbedder(embedCfg, logger)
	qdrant := NewQdrantClient(qdrantCfg, logger)

	storage, err := NewS3Storage(s3Cfg, logger)
	if err != nil {
		logger.Fatalf("failed to create S3 storage: %v", err)
	}

	llm := NewLLM(llmCfg, logger)
	if !llm.Configured() {
		logger.Println("warning: LLM not configured — /search will return citations without a generated answer")
	}

	service := NewService(logger, embedder, qdrant, storage, llm, ServiceConfig{
		MaxQuoteBytes: int64(envOrInt("RAG_MAX_QUOTE_BYTES", 8000)),
	})
	handler := NewHandler(service, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/search", handler.HandleSearch)
	mux.HandleFunc("/ping", handler.HandlePing)

	addr := fmt.Sprintf(":%s", port)
	logger.Printf("Starting RAG Search on %s", addr)
	logger.Printf("Embed: %s model=%s", embedCfg.Endpoint, embedCfg.Model)
	logger.Printf("Qdrant: %s:%d", qdrantCfg.Host, qdrantCfg.Port)
	logger.Printf("S3: %s/%s", s3Cfg.Endpoint, s3Cfg.Bucket)
	logger.Printf("LLM: %s model=%s", llmCfg.Endpoint, llmCfg.Model)

	if err := http.ListenAndServe(addr, mux); err != nil {
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
