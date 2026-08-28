package main

import (
	"reflect"
	"testing"
)

func TestSortChunksByStart(t *testing.T) {
	chunks := []ScoredPoint{
		{Payload: map[string]interface{}{"chunk_id": "100-199"}},
		{Payload: map[string]interface{}{"chunk_id": "0-99"}},
		{Payload: map[string]interface{}{"chunk_id": "300-399"}},
		{Payload: map[string]interface{}{"chunk_id": "200-299"}},
	}

	sortChunksByStart(chunks)

	expected := []string{"0-99", "100-199", "200-299", "300-399"}
	for i, c := range chunks {
		if c.Payload["chunk_id"] != expected[i] {
			t.Errorf("at index %d: expected %s, got %s", i, expected[i], c.Payload["chunk_id"])
		}
	}
}

func TestFindAdjacency(t *testing.T) {
	chunks := []ScoredPoint{
		{Payload: map[string]interface{}{"chunk_id": "0-99"}},
		{Payload: map[string]interface{}{"chunk_id": "100-199"}},
		{Payload: map[string]interface{}{"chunk_id": "200-299"}},
		{Payload: map[string]interface{}{"chunk_id": "300-399"}},
		{Payload: map[string]interface{}{"chunk_id": "400-499"}},
	}

	tests := []struct {
		name       string
		target     string
		count      int
		expectPrev []string
		expectNext []string
	}{
		{
			name:       "Middle chunk",
			target:     "200-299",
			count:      1,
			expectPrev: []string{"100-199"},
			expectNext: []string{"300-399"},
		},
		{
			name:       "Middle chunk count 2",
			target:     "200-299",
			count:      2,
			expectPrev: []string{"0-99", "100-199"},
			expectNext: []string{"300-399", "400-499"},
		},
		{
			name:       "First chunk",
			target:     "0-99",
			count:      1,
			expectPrev: []string{}, // empty
			expectNext: []string{"100-199"},
		},
		{
			name:       "Last chunk",
			target:     "400-499",
			count:      2,
			expectPrev: []string{"200-299", "300-399"},
			expectNext: []string{},
		},
		{
			name:       "Not found",
			target:     "999-1099",
			count:      1,
			expectPrev: []string{},
			expectNext: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev, next := findAdjacency(chunks, tt.target, tt.count)
			if !reflect.DeepEqual(prev, tt.expectPrev) {
				t.Errorf("prev: expected %v, got %v", tt.expectPrev, prev)
			}
			if !reflect.DeepEqual(next, tt.expectNext) {
				t.Errorf("next: expected %v, got %v", tt.expectNext, next)
			}
		})
	}
}
