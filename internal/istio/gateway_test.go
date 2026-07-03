package istio

import (
	"reflect"
	"testing"
)

func TestAppendSortedCompact(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		val      string
		expected []string
	}{
		{
			name:     "adds to empty slice",
			slice:    []string{},
			val:      "vg.no",
			expected: []string{"vg.no"},
		},
		{
			name:     "inserts in alphabetical order",
			slice:    []string{"example.com", "vg.no"},
			val:      "apple.com",
			expected: []string{"apple.com", "example.com", "vg.no"},
		},
		{
			name:     "appends at end when lexicographically last",
			slice:    []string{"apple.com", "example.com"},
			val:      "vg.no",
			expected: []string{"apple.com", "example.com", "vg.no"},
		},
		{
			name:     "deduplicates an already-present value",
			slice:    []string{"apple.com", "vg.no"},
			val:      "vg.no",
			expected: []string{"apple.com", "vg.no"},
		},
		{
			name:     "deduplicates the only element",
			slice:    []string{"vg.no"},
			val:      "vg.no",
			expected: []string{"vg.no"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendSortedCompact(tt.slice, tt.val)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}
