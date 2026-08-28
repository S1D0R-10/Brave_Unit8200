package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// S3Storage reads slices of text objects out of the bucket.
//
// rag-search never holds chunk text of its own: the embedder stored byte
// offsets, so answering a query means asking the bucket for exactly those bytes
// with an HTTP Range request. Bucket → backend traffic is free, and a citation
// costs a few kilobytes instead of a whole document.
type S3Storage struct {
	httpClient *http.Client
	endpoint   string
	bucket     string
	region     string
	accessKey  string
	secretKey  string
	logger     *log.Logger
}

// S3Config holds the configuration for connecting to the S3-compatible store.
type S3Config struct {
	Endpoint  string // e.g. "https://t3.storageapi.dev"
	Bucket    string // e.g. "wiadro-xuw-on7mmw3fdswei6"
	Region    string // e.g. "auto"
	AccessKey string
	SecretKey string
}

// NewS3Storage creates a new S3Storage client.
func NewS3Storage(cfg S3Config, logger *log.Logger) (*S3Storage, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 endpoint and bucket are required")
	}
	if logger == nil {
		logger = log.Default()
	}

	return &S3Storage{
		httpClient: &http.Client{},
		endpoint:   strings.TrimRight(cfg.Endpoint, "/"),
		bucket:     cfg.Bucket,
		region:     cfg.Region,
		accessKey:  cfg.AccessKey,
		secretKey:  cfg.SecretKey,
		logger:     logger,
	}, nil
}

// FetchRange returns bytes [start, end] (both inclusive) of the object at key.
func (s *S3Storage) FetchRange(ctx context.Context, key string, start, end int64) ([]byte, error) {
	if start < 0 || end < start {
		return nil, fmt.Errorf("invalid byte range %d-%d for %q", start, end, key)
	}

	url := fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating range request for %q: %w", key, err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	s3Sign(req, s.accessKey, s.secretKey, s.region, "s3")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("S3 GET %q bytes=%d-%d: %w", key, start, end, err)
	}
	defer resp.Body.Close()

	// 206 is the expected answer; 200 means the store ignored the Range header
	// and sent the whole object, which we then have to slice ourselves.
	switch resp.StatusCode {
	case http.StatusPartialContent:
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading range of %q: %w", key, err)
		}
		return data, nil

	case http.StatusOK:
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading %q: %w", key, err)
		}
		s.logger.Printf("warning: %q ignored Range, sliced %d bytes locally", key, len(data))
		if start >= int64(len(data)) {
			return nil, fmt.Errorf("byte range %d-%d is past the end of %q (%d bytes)", start, end, key, len(data))
		}
		if end >= int64(len(data)) {
			end = int64(len(data)) - 1
		}
		return data[start : end+1], nil

	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("S3 GET %q bytes=%d-%d returned %d: %s",
			key, start, end, resp.StatusCode, string(body))
	}
}
