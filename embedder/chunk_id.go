package main

import (
	"fmt"
	"strconv"
	"strings"
)

// EncodeChunkID produces a deterministic, reversible chunk identifier.
// Format: "{start}-{end}" where start and end are decimal integers.
//
// start/end are always inclusive BYTE offsets into the text object named by
// file_key in the payload, so a chunk ID translates straight into an HTTP
// "Range: bytes=start-end" request against the bucket.
func EncodeChunkID(start, end int64) string {
	return fmt.Sprintf("%d-%d", start, end)
}

// DecodeChunkID parses a chunk ID back into its start and end components.
// Returns an error if the format is invalid or values are negative.
func DecodeChunkID(id string) (start, end int64, err error) {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid chunk_id format %q: expected {start}-{end}", id)
	}

	start, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid chunk_id start %q: %w", parts[0], err)
	}

	end, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid chunk_id end %q: %w", parts[1], err)
	}

	if start < 0 || end < 0 {
		return 0, 0, fmt.Errorf("chunk_id values must be non-negative, got start=%d end=%d", start, end)
	}

	if start > end {
		return 0, 0, fmt.Errorf("chunk_id start (%d) must be <= end (%d)", start, end)
	}

	return start, end, nil
}
