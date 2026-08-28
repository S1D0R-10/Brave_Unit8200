package main

import (
	"math"
	"testing"
)

func TestEncodeChunkID(t *testing.T) {
	tests := []struct {
		start, end int64
		want       string
	}{
		{0, 499, "0-499"},
		{500, 999, "500-999"},
		{0, 30000, "0-30000"},
		{0, 0, "0-0"},
		{1000000, 2000000, "1000000-2000000"},
	}
	for _, tt := range tests {
		got := EncodeChunkID(tt.start, tt.end)
		if got != tt.want {
			t.Errorf("EncodeChunkID(%d, %d) = %q, want %q", tt.start, tt.end, got, tt.want)
		}
	}
}

func TestDecodeChunkID(t *testing.T) {
	tests := []struct {
		id        string
		wantStart int64
		wantEnd   int64
		wantErr   bool
	}{
		{"0-499", 0, 499, false},
		{"500-999", 500, 999, false},
		{"0-30000", 0, 30000, false},
		{"0-0", 0, 0, false},
		{"1000000-2000000", 1000000, 2000000, false},
		// errors
		{"", 0, 0, true},
		{"abc", 0, 0, true},
		{"1-abc", 0, 0, true},
		{"abc-1", 0, 0, true},
		{"-1-5", 0, 0, true},   // negative start
		{"5-3", 0, 0, true},    // start > end
	}
	for _, tt := range tests {
		start, end, err := DecodeChunkID(tt.id)
		if tt.wantErr {
			if err == nil {
				t.Errorf("DecodeChunkID(%q) expected error, got (%d, %d, nil)", tt.id, start, end)
			}
			continue
		}
		if err != nil {
			t.Errorf("DecodeChunkID(%q) unexpected error: %v", tt.id, err)
			continue
		}
		if start != tt.wantStart || end != tt.wantEnd {
			t.Errorf("DecodeChunkID(%q) = (%d, %d), want (%d, %d)", tt.id, start, end, tt.wantStart, tt.wantEnd)
		}
	}
}

func TestRoundtrip(t *testing.T) {
	pairs := [][2]int64{
		{0, 499},
		{30000, 60000},
		{0, 0},
		{0, math.MaxInt32},
	}
	for _, p := range pairs {
		id := EncodeChunkID(p[0], p[1])
		s, e, err := DecodeChunkID(id)
		if err != nil {
			t.Fatalf("roundtrip failed for (%d, %d): encode=%q, decode err=%v", p[0], p[1], id, err)
		}
		if s != p[0] || e != p[1] {
			t.Errorf("roundtrip mismatch: (%d,%d) → %q → (%d,%d)", p[0], p[1], id, s, e)
		}
	}
}
