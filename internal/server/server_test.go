package server

import (
	"fmt"
	"testing"
)

func TestParseSegmentSeconds(t *testing.T) {
	if got := parseSegmentSeconds("5"); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
	if got := parseSegmentSeconds("invalid"); got != 0 {
		t.Fatalf("expected 0 for invalid input, got %d", got)
	}
	if got := parseSegmentSeconds(""); got != 0 {
		t.Fatalf("expected 0 for empty input, got %d", got)
	}
}

func TestParseVariantConfig(t *testing.T) {
	tests := []struct {
		input    string
		expected []int
	}{
		{"64,128,192", []int{64, 128, 192}},
		{"128,128,invalid", []int{128}},
		{"", nil},
	}

	for _, tc := range tests {
		variants := parseVariantConfig(tc.input)
		if len(variants) != len(tc.expected) {
			t.Fatalf("input %q: expected %d variants, got %d", tc.input, len(tc.expected), len(variants))
		}
		for i, v := range variants {
			if v.BitrateKbps != tc.expected[i] {
				t.Fatalf("input %q: expected bitrate %d, got %d", tc.input, tc.expected[i], v.BitrateKbps)
			}
			expectedName := fmt.Sprintf("%dk", tc.expected[i])
			if v.Name != expectedName {
				t.Fatalf("expected name %s, got %s", expectedName, v.Name)
			}
		}
	}

	// ensure duplicates removed
	variants := parseVariantConfig("64,64,64")
	if len(variants) != 1 || variants[0].BitrateKbps != 64 {
		t.Fatalf("expected single variant 64k, got %#v", variants)
	}
}
