package objectstore

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// sign adds AWS Signature V4 headers to an HTTP request.
func sign(req *http.Request, accessKey, secretKey, region, service string) {
	now := time.Now().UTC()
	datestamp := now.Format("20060102")
	amzdate := now.Format("20060102T150405Z")

	req.Header.Set("x-amz-date", amzdate)
	req.Header.Set("x-amz-content-sha256", payloadHash(req))

	// 1. Canonical request.
	canonicalURI := req.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := req.URL.Query().Encode()

	signedHeaders, canonicalHeaders := canonicalHeaderSet(req)

	canonical := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		req.Header.Get("x-amz-content-sha256"),
	}, "\n")

	// 2. String to sign.
	scope := fmt.Sprintf("%s/%s/%s/aws4_request", datestamp, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzdate,
		scope,
		sha256Hex([]byte(canonical)),
	}, "\n")

	// 3. Signing key.
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(datestamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))

	// 4. Signature.
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	// 5. Authorization header.
	auth := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, scope, signedHeaders, signature)
	req.Header.Set("Authorization", auth)
}

// canonicalHeaderSet builds the canonical headers string and signed headers list.
func canonicalHeaderSet(req *http.Request) (signedHeaders, canonicalHeaders string) {
	// Collect headers to sign: host + all x-amz-* headers.
	headers := make(map[string]string)
	headers["host"] = req.Host
	if headers["host"] == "" {
		headers["host"] = req.URL.Host
	}

	for k, v := range req.Header {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "x-amz-") {
			headers[lower] = strings.TrimSpace(v[0])
		}
	}

	// Sort header names.
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)

	// Build canonical headers (each line: "name:value\n") and signed headers list.
	var chBuf, shBuf strings.Builder
	for i, name := range names {
		chBuf.WriteString(name)
		chBuf.WriteString(":")
		chBuf.WriteString(headers[name])
		chBuf.WriteString("\n")

		if i > 0 {
			shBuf.WriteString(";")
		}
		shBuf.WriteString(name)
	}

	return shBuf.String(), chBuf.String()
}

// payloadHash returns the SHA-256 hex hash of the request body.
// For GET/HEAD requests (no body), returns the hash of empty string.
func payloadHash(req *http.Request) string {
	if req.Body == nil || req.Method == http.MethodGet || req.Method == http.MethodHead {
		return sha256Hex([]byte(""))
	}
	// For requests with a body, read it, hash it, and reset.
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(strings.NewReader(string(body)))
	return sha256Hex(body)
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
