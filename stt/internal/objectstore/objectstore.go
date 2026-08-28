// Package objectstore is a minimal S3-compatible client for the one job stt
// has in the bucket: pull down a recording, push back its transcript.
package objectstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
)

// Config holds connection settings for the S3-compatible store.
type Config struct {
	Endpoint  string // e.g. "https://t3.storageapi.dev"
	Bucket    string
	Region    string // e.g. "auto"
	AccessKey string
	SecretKey string
}

// Client talks to the bucket with AWS Signature V4 over plain net/http.
type Client struct {
	config     Config
	httpClient *http.Client
}

// New returns a Client, or an error when the store is not configured.
func New(config Config) (*Client, error) {
	if config.Endpoint == "" || config.Bucket == "" {
		return nil, fmt.Errorf("object store endpoint and bucket are required")
	}
	if config.AccessKey == "" || config.SecretKey == "" {
		return nil, fmt.Errorf("object store credentials are required")
	}
	config.Endpoint = strings.TrimRight(config.Endpoint, "/")
	return &Client{config: config, httpClient: &http.Client{}}, nil
}

// Bucket returns the configured bucket name.
func (c *Client) Bucket() string { return c.config.Bucket }

// objectURL builds the path-style URL for key with the key percent-encoded.
// Keys come straight from user file names, so they can contain "#", spaces or
// non-ASCII bytes — sent raw, "#" would truncate the path into a fragment.
func (c *Client) objectURL(key string) string {
	return fmt.Sprintf("%s/%s/%s", c.config.Endpoint, c.config.Bucket, encodeKey(key))
}

// encodeKey percent-encodes an object key for a URL path following the AWS
// SigV4 canonical-URI rules: every byte except unreserved characters is
// encoded, and "/" keeps separating segments.
func encodeKey(key string) string {
	var b strings.Builder
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '.', c == '_', c == '~', c == '/':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// DownloadTo streams the object at key into a local file. Recordings run to
// gigabytes, so nothing is buffered in memory.
func (c *Client) DownloadTo(ctx context.Context, key, destination string) (int64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.objectURL(key), nil)
	if err != nil {
		return 0, fmt.Errorf("creating GET request for %q: %w", key, err)
	}
	sign(request, c.config.AccessKey, c.config.SecretKey, c.config.Region, "s3")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return 0, fmt.Errorf("GET %q: %w", key, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return 0, fmt.Errorf("GET %q returned %d: %s", key, response.StatusCode, string(body))
	}

	file, err := os.Create(destination)
	if err != nil {
		return 0, fmt.Errorf("creating %q: %w", destination, err)
	}
	written, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return 0, fmt.Errorf("writing %q: %w", destination, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return 0, fmt.Errorf("closing %q: %w", destination, closeErr)
	}

	return written, nil
}

// UploadFile pushes a local file to the bucket under key.
func (c *Client) UploadFile(ctx context.Context, key, source, contentType string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("reading %q: %w", source, err)
	}
	return c.Upload(ctx, key, data, contentType)
}

// Upload stores data under key.
func (c *Client) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.objectURL(key), strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("creating PUT request for %q: %w", key, err)
	}
	request.ContentLength = int64(len(data))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	sign(request, c.config.AccessKey, c.config.SecretKey, c.config.Region, "s3")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("PUT %q: %w", key, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("PUT %q returned %d: %s", key, response.StatusCode, string(body))
	}

	return nil
}

// SiblingKey returns key with its extension replaced by suffix, keeping the
// original prefix: "kurs/film.mp4" + "-transcription.txt" becomes
// "kurs/film-transcription.txt".
func SiblingKey(key, suffix string) string {
	return strings.TrimSuffix(key, path.Ext(key)) + suffix
}
