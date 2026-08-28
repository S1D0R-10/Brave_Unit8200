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
	logger := log.New(os.Stdout, "[embedder] ", log.LstdFlags|log.Lshortfile)

	// --- Config from env ---
	port := envOrDefault("RAG_PORT", "8080")

	s3Cfg := S3Config{
		Endpoint:  envOrDefault("RAG_S3_ENDPOINT", "https://t3.storageapi.dev"),
		Bucket:    envOrDefault("RAG_S3_BUCKET", "wiadro-xuw-on7mmw3fdswei6"),
		Region:    envOrDefault("RAG_S3_REGION", "auto"),
		AccessKey: os.Getenv("RAG_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("RAG_S3_SECRET_KEY"),
	}

	chunkerCfg := ChunkerConfig{
		WordsPerChunk: envOrInt("RAG_CHUNK_WORDS", 500),
		SecsPerChunk:  int64(envOrInt("RAG_CHUNK_SECS", 300)),
	}

	embedCfg := EmbedderConfig{
		Endpoint: envOrDefault("RAG_EMBED_ENDPOINT", "https://api.openai.com/v1/embeddings"),
		APIKey:   os.Getenv("RAG_EMBED_API_KEY"),
		Model:    envOrDefault("RAG_EMBED_MODEL", "text-embedding-3-small"),
		Dim:      envOrInt("RAG_EMBED_DIM", 1536),
	}

	qdrantHost := envOrDefault("RAG_QDRANT_HOST", "qdrant.railway.internal")
	qdrantPort := envOrInt("RAG_QDRANT_PORT", 6333)

	// --- Build dependencies ---
	storage, err := NewS3Storage(s3Cfg, logger)
	if err != nil {
		logger.Fatalf("failed to create S3 storage: %v", err)
	}

	chunker := NewChunker(chunkerCfg)
	embedder := NewEmbedder(embedCfg, logger)

	// Use Qdrant if host is configured, otherwise fall back to LogStore stub.
	var store VectorStore
	if qdrantHost != "" {
		store = NewQdrantStore(QdrantConfig{
			Host:       qdrantHost,
			Port:       qdrantPort,
			Collection: "citations",
			VectorDim:  embedCfg.Dim,
		}, logger)
		logger.Printf("vector store: Qdrant at %s:%d (dim=%d)", qdrantHost, qdrantPort, embedCfg.Dim)
	} else {
		store = NewLogStore(logger)
		logger.Println("vector store: LogStore (stub)")
	}

	processor := NewProcessor(logger, storage, chunker, embedder, store)
	handler := NewHandler(processor, logger)

	// --- Routes ---
	mux := http.NewServeMux()
	mux.HandleFunc("/verity", handler.HandleVerity)
	mux.HandleFunc("/ping", handler.HandlePing)

	// --- Start ---
	addr := fmt.Sprintf(":%s", port)
	logger.Printf("Embedder v%s starting on %s", processor.Version(), addr)
	logger.Printf("S3: %s/%s", s3Cfg.Endpoint, s3Cfg.Bucket)
	logger.Printf("Embed: %s model=%s dim=%d", embedCfg.Endpoint, embedCfg.Model, embedCfg.Dim)
	logger.Printf("Chunks: words=%d secs=%d", chunkerCfg.WordsPerChunk, chunkerCfg.SecsPerChunk)

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

// loadEnv reads a simple .env file and sets environment variables using stdlib only.
func loadEnv(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return // Ignore if .env doesn't exist
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
			// Strip quotes if any
			if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
				val = val[1 : len(val)-1]
			}
			os.Setenv(key, val)
		}
	}
}
