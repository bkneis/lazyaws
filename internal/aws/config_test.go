package aws_test

import (
	"context"
	"testing"

	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

func TestLoadConfig_returnsValidConfig(t *testing.T) {
	cfg, err := awspkg.LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Region == "" {
		t.Skip("no AWS region configured; skipping config test")
	}
}
