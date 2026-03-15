package aws

import (
	"fmt"
	"strings"
)

// KV renders key-value pairs as left-aligned columns with keys right-padded.
func KV(pairs [][2]string) string {
	maxKey := 0
	for _, p := range pairs {
		if len(p[0]) > maxKey {
			maxKey = len(p[0])
		}
	}
	var sb strings.Builder
	for _, p := range pairs {
		fmt.Fprintf(&sb, "  [cyan]%-*s[-]  %s\n", maxKey+1, p[0]+":", p[1])
	}
	return sb.String()
}

// Table renders a header row, a separator line, and data rows.
func Table(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	padded := make([]string, len(headers))
	for i, h := range headers {
		padded[i] = fmt.Sprintf("%-*s", widths[i]+2, h)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "  [cyan]%s[-]\n  ", strings.Join(padded, ""))
	for _, w := range widths {
		sb.WriteString(strings.Repeat("─", w) + "  ")
	}
	sb.WriteString("\n")
	for _, row := range rows {
		sb.WriteString("  ")
		for i, cell := range row {
			if i < len(widths) {
				fmt.Fprintf(&sb, "%-*s", widths[i]+2, cell)
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// IsSensitiveKey returns true if the key name suggests a sensitive value
// (contains password, secret, token, or key — case-insensitive).
func IsSensitiveKey(k string) bool {
	lower := strings.ToLower(k)
	for _, s := range []string{"password", "secret", "token", "key"} {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
