package ui

import (
	"fmt"
	"strings"
	"testing"

	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

func TestRenderTabBar(t *testing.T) {
	tabs := []awspkg.TabDef{
		{Label: "Overview"}, // " Overview " = 10 display chars, offset 0
		{Label: "Objects"},  // " Objects "  =  9 display chars, offset 10
		{Label: "Policy"},   // " Policy "   =  8 display chars, offset 19
	}
	cases := []struct {
		active      int
		wantActive  string // substring expected in line1
		wantInLine2 string // distinctive underline substring expected in line2
		wantOffsets []int
	}{
		{
			active:      0,
			wantActive:  "[aqua::bu] Overview [-::-]",
			wantInLine2: "──────────",
			wantOffsets: []int{0, 10, 19},
		},
		{
			active:      1,
			wantActive:  "[aqua::bu] Objects [-::-]",
			wantInLine2: "─────────",
			wantOffsets: []int{0, 10, 19},
		},
		{
			active:      2,
			wantActive:  "[aqua::bu] Policy [-::-]",
			wantInLine2: "────────",
			wantOffsets: []int{0, 10, 19},
		},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("active=%d", tc.active), func(t *testing.T) {
			a := &App{panels: newPanels(), activeTab: tc.active}
			a.renderTabBar(tabs)
			got := a.panels.tabBar.GetText(false)
			if !strings.Contains(got, tc.wantActive) {
				t.Errorf("active=%d: got %q, want to contain %q", tc.active, got, tc.wantActive)
			}
			if !strings.Contains(got, tc.wantInLine2) {
				t.Errorf("active=%d: got %q, want underline %q", tc.active, got, tc.wantInLine2)
			}
			if len(a.tabBarOffsets) != len(tc.wantOffsets) {
				t.Fatalf("active=%d: got %d offsets, want %d", tc.active, len(a.tabBarOffsets), len(tc.wantOffsets))
			}
			for i, want := range tc.wantOffsets {
				if a.tabBarOffsets[i] != want {
					t.Errorf("active=%d: offset[%d] = %d, want %d", tc.active, i, a.tabBarOffsets[i], want)
				}
			}
		})
	}
}
