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
		wantOffsets []int
	}{
		{active: 0, wantOffsets: []int{0, 10, 19}},
		{active: 1, wantOffsets: []int{0, 10, 19}},
		{active: 2, wantOffsets: []int{0, 10, 19}},
	}
	theme := DetectTheme()
	for _, tc := range cases {
		t.Run(fmt.Sprintf("active=%d", tc.active), func(t *testing.T) {
			a := &App{panels: newPanels(theme), theme: theme, activeTab: tc.active}
			a.renderTabBar(tabs)
			got := a.panels.tabBar.GetText(false)
			wantActive := theme.ActiveTabTag + "[ " + tabs[tc.active].Label + " ][-]"
			if !strings.Contains(got, wantActive) {
				t.Errorf("active=%d: got %q, want to contain %q", tc.active, got, wantActive)
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
