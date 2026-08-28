package main

import (
	"log"
	"testing"
)

func TestNewProcessor_NotNil(t *testing.T) {
	store := NewLogStore(nil)
	chunker := NewChunker(DefaultChunkerConfig())
	embedder := NewEmbedder(EmbedderConfig{Dim: 1}, nil)
	p := NewProcessor(nil, nil, chunker, embedder, store)
	if p == nil {
		t.Fatal("NewProcessor returned nil")
	}
}

func TestNewProcessor_WithLogger(t *testing.T) {
	logger := log.Default()
	store := NewLogStore(logger)
	chunker := NewChunker(DefaultChunkerConfig())
	embedder := NewEmbedder(EmbedderConfig{Dim: 1}, logger)
	p := NewProcessor(logger, nil, chunker, embedder, store)
	if p == nil {
		t.Fatal("NewProcessor with logger returned nil")
	}
}

func TestVersion(t *testing.T) {
	store := NewLogStore(nil)
	chunker := NewChunker(DefaultChunkerConfig())
	embedder := NewEmbedder(EmbedderConfig{Dim: 1}, nil)
	p := NewProcessor(nil, nil, chunker, embedder, store)
	v := p.Version()
	if v == "" {
		t.Fatal("Version() returned empty string")
	}
	if v != "0.1.0" {
		t.Errorf("expected version 0.1.0, got %s", v)
	}
}

func TestPing(t *testing.T) {
	store := NewLogStore(nil)
	chunker := NewChunker(DefaultChunkerConfig())
	embedder := NewEmbedder(EmbedderConfig{Dim: 1}, nil)
	p := NewProcessor(nil, nil, chunker, embedder, store)
	if !p.Ping() {
		t.Fatal("Ping() returned false")
	}
}
