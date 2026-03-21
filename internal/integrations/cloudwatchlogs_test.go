//go:build integration

package integrations_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

// endpoint returns the LocalStack endpoint from the environment, or skips the test.
func localstackEndpoint(t *testing.T) string {
	t.Helper()
	ep := os.Getenv("LOCALSTACK_ENDPOINT")
	if ep == "" {
		t.Skip("LOCALSTACK_ENDPOINT not set — skipping integration test")
	}
	return ep
}

// buildCWLClient returns a real cloudwatchlogs.Client pointed at the given endpoint.
func buildCWLClient(t *testing.T, endpoint string) *cloudwatchlogs.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	return cloudwatchlogs.NewFromConfig(cfg, func(o *cloudwatchlogs.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// TestIntegration_MultiGroupTailSeesRecentEvents verifies that StartTailGroups
// delivers log events that were written to a group before tailing began.
//
// This test is the regression test for the bug where StartTailGroups used
// StartLiveTail (real-time only, no history) causing pre-existing events
// (e.g. Lambda START/END logs) to be silently dropped.
func TestIntegration_MultiGroupTailSeesRecentEvents(t *testing.T) {
	endpoint := localstackEndpoint(t)
	ctx := context.Background()

	rawClient := buildCWLClient(t, endpoint)

	// Use a unique group name so parallel/repeated runs don't collide.
	groupName := fmt.Sprintf("/test/lazyaws-multigroup-%d", time.Now().UnixNano())
	streamName := "test-stream-0"

	// Cleanup on exit.
	t.Cleanup(func() {
		_, _ = rawClient.DeleteLogGroup(context.Background(), &cloudwatchlogs.DeleteLogGroupInput{
			LogGroupName: aws.String(groupName),
		})
	})

	// Create log group + stream.
	if _, err := rawClient.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(groupName),
	}); err != nil {
		t.Fatalf("create log group: %v", err)
	}
	if _, err := rawClient.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(groupName),
		LogStreamName: aws.String(streamName),
	}); err != nil {
		t.Fatalf("create log stream: %v", err)
	}

	// Write events simulating a Lambda invocation — these are written BEFORE
	// tailing starts, which is exactly the scenario that was broken.
	now := time.Now().UnixMilli()
	_, err := rawClient.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(groupName),
		LogStreamName: aws.String(streamName),
		LogEvents: []cwltypes.InputLogEvent{
			{Timestamp: aws.Int64(now), Message: aws.String("START RequestId: abc123 Version: $LATEST")},
			{Timestamp: aws.Int64(now + 1), Message: aws.String("END RequestId: abc123")},
			{Timestamp: aws.Int64(now + 2), Message: aws.String("REPORT RequestId: abc123 Duration: 42.00 ms")},
		},
	})
	if err != nil {
		t.Fatalf("put log events: %v", err)
	}

	// Build provider using the real client via NewCloudWatchLogsProvider.
	p := awspkg.NewCloudWatchLogsProvider(mustLoadConfig(t, endpoint), endpoint)

	// Tail with a timeout — if the bug is present, no events arrive and the
	// context expires; after the fix, FilterLogEvents backfills immediately.
	tailCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var received []string
	_ = p.StartTailGroups(tailCtx, []string{groupName}, func(_ int64, _, _, msg string) {
		received = append(received, msg)
		// Cancel as soon as we see START — no need to wait for the full timeout.
		if strings.Contains(msg, "START RequestId") {
			cancel()
		}
	})

	if len(received) == 0 {
		t.Fatal("StartTailGroups delivered no events — expected START/END logs from pre-tail invocation")
	}
	hasStart := false
	for _, m := range received {
		if strings.Contains(m, "START RequestId") {
			hasStart = true
			break
		}
	}
	if !hasStart {
		t.Errorf("expected a 'START RequestId' event, got: %v", received)
	}
}

// mustLoadConfig builds an awssdk.Config for LocalStack.
func mustLoadConfig(t *testing.T, endpoint string) aws.Config {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	return cfg
}
