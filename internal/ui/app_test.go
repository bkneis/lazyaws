package ui

import (
	"fmt"
	"strings"
	"testing"

	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

func TestRenderTabBar(t *testing.T) {
	tabs := []awspkg.TabDef{
		{Label: "Overview"},
		{Label: "Objects"},
		{Label: "Policy"},
	}
	cases := []struct {
		active int
		expect string
	}{
		{0, "[cyan][[]Overview][-]  [gray]Objects[-]  [gray]Policy[-]"},
		{1, "[gray]Overview[-]  [cyan][[]Objects][-]  [gray]Policy[-]"},
		{2, "[gray]Overview[-]  [gray]Objects[-]  [cyan][[]Policy][-]"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("active=%d", tc.active), func(t *testing.T) {
			got := renderTabBar(tabs, tc.active)
			if !strings.Contains(got, tc.expect) {
				t.Errorf("active=%d: got %q, want to contain %q", tc.active, got, tc.expect)
			}
		})
	}
}
