package aws_test

import (
	"strings"
	"testing"

	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

func TestKV(t *testing.T) {
	cases := []struct {
		name   string
		pairs  [][2]string
		expect []string
	}{
		{
			name:   "aligns values",
			pairs:  [][2]string{{"Region", "us-east-1"}, {"Versioning", "Enabled"}},
			expect: []string{"Region:", "us-east-1", "Versioning:", "Enabled"},
		},
		{
			name:   "empty",
			pairs:  nil,
			expect: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := awspkg.KV(tc.pairs)
			for _, want := range tc.expect {
				if !strings.Contains(out, want) {
					t.Errorf("KV output missing %q\ngot:\n%s", want, out)
				}
			}
		})
	}
}

func TestTable(t *testing.T) {
	headers := []string{"Name", "Size", "Modified"}
	rows := [][]string{
		{"images/hero.png", "2.3 MB", "2024-11-01"},
		{"data/export.csv", "892 KB", "2024-11-05"},
	}
	out := awspkg.Table(headers, rows)
	for _, want := range []string{"Name", "Size", "images/hero.png", "892 KB", "─"} {
		if !strings.Contains(out, want) {
			t.Errorf("Table output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestIsSensitiveKey(t *testing.T) {
	cases := []struct {
		key       string
		sensitive bool
	}{
		{"db_password", true},
		{"api_secret", true},
		{"access_token", true},
		{"api_key", true},
		{"DB_PASSWORD", true},
		{"db_host", false},
		{"db_port", false},
		{"log_level", false},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			got := awspkg.IsSensitiveKey(tc.key)
			if got != tc.sensitive {
				t.Errorf("IsSensitiveKey(%q) = %v, want %v", tc.key, got, tc.sensitive)
			}
		})
	}
}
