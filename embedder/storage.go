package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// S3Storage wraps a stdlib HTTP client for downloading objects
// from an S3-compatible store (Railway Object Store) using AWS Signature V4.
type S3Storage struct {
	httpClient *http.Client
	endpoint   string // e.g. "https://t3.storageapi.dev"
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

	// Trim trailing slash from endpoint.
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")

	return &S3Storage{
		httpClient: &http.Client{},
		endpoint:   cfg.Endpoint,
		bucket:     cfg.Bucket,
		region:     cfg.Region,
		accessKey:  cfg.AccessKey,
		secretKey:  cfg.SecretKey,
		logger:     logger,
	}, nil
}

// Download fetches the object identified by key from the bucket.
func (s *S3Storage) Download(ctx context.Context, key string) ([]byte, error) {
	s.logger.Printf("downloading s3://%s/%s", s.bucket, key)

	// Path-style URL: https://endpoint/bucket/key
	url := fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %q: %w", key, err)
	}

	s3Sign(req, s.accessKey, s.secretKey, s.region, "s3")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("S3 GET %q: %w", key, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("S3 GET %q returned %d: %s", key, resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading S3 object %q: %w", key, err)
	}

	s.logger.Printf("downloaded %d bytes for %q", len(data), key)
	return data, nil
}

// Upload stores data under key. The pipeline only ever writes derived text —
// PDF extractions here, transcripts from stt — never the originals.
func (s *S3Storage) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	s.logger.Printf("uploading %d bytes to s3://%s/%s", len(data), s.bucket, key)

	url := fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating PUT request for %q: %w", key, err)
	}
	req.ContentLength = int64(len(data))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	s3Sign(req, s.accessKey, s.secretKey, s.region, "s3")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("S3 PUT %q: %w", key, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("S3 PUT %q returned %d: %s", key, resp.StatusCode, string(body))
	}

	return nil
}

// listBucketResult is the XML response from S3 ListObjectsV2.
type listBucketResult struct {
	XMLName  xml.Name       `xml:"ListBucketResult"`
	Contents []s3ObjectInfo `xml:"Contents"`
}

type s3ObjectInfo struct {
	Key  string `xml:"Key"`
	Size int64  `xml:"Size"`
}

// ListObjects returns all object keys in the bucket.
func (s *S3Storage) ListObjects(ctx context.Context) ([]string, error) {
	s.logger.Printf("listing objects in s3://%s", s.bucket)

	url := fmt.Sprintf("%s/%s?list-type=2", s.endpoint, s.bucket)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating list request: %w", err)
	}

	s3Sign(req, s.accessKey, s.secretKey, s.region, "s3")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("S3 ListObjects: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("S3 ListObjects returned %d: %s", resp.StatusCode, string(body))
	}

	var result listBucketResult
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing ListObjects response: %w", err)
	}

	keys := make([]string, len(result.Contents))
	for i, obj := range result.Contents {
		keys[i] = obj.Key
	}

	s.logger.Printf("found %d objects", len(keys))
	return keys, nil
}
