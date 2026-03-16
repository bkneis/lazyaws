package ui

import (
	"testing"
)

func TestPanels_next(t *testing.T) {
	cases := []struct {
		start    int
		expected int
	}{
		{start: 0, expected: 1},
		{start: 1, expected: 2},
		{start: 2, expected: 0}, // wraps around
	}
	for _, tc := range cases {
		p := newPanels(DetectTheme())
		p.focused = tc.start
		p.next()
		if p.focused != tc.expected {
			t.Errorf("next from %d: got %d, want %d", tc.start, p.focused, tc.expected)
		}
	}
}

func TestPanels_prev(t *testing.T) {
	cases := []struct {
		start    int
		expected int
	}{
		{start: 2, expected: 1},
		{start: 1, expected: 0},
		{start: 0, expected: 2}, // wraps around
	}
	for _, tc := range cases {
		p := newPanels(DetectTheme())
		p.focused = tc.start
		p.prev()
		if p.focused != tc.expected {
			t.Errorf("prev from %d: got %d, want %d", tc.start, p.focused, tc.expected)
		}
	}
}
