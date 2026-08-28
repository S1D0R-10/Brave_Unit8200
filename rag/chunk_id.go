package main

import (
	"fmt"
	"strconv"
	"strings"
)

// DecodeChunkID parses a deterministic chunk ID ("{start}-{end}") into start and end integers.
// If the chunk ID is not in the correct format, it returns an error.
func DecodeChunkID(id string) (start int64, end int64, err error) {
	parts := strings.Split(id, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid chunk ID format, expected {start}-{end}, got %q", id)
	}

	start, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start index: %w", err)
	}

	end, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end index: %w", err)
	}

	if start > end {
		return 0, 0, fmt.Errorf("start index %d > end index %d", start, end)
	}

	return start, end, nil
}
