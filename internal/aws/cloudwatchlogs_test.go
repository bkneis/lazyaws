package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

// stubCWLogs implements CloudWatchLogsAPI for testing.
type stubCWLogs struct {
	logGroups   []cwltypes.LogGroup
	logStreams   []cwltypes.LogStream
	logEvents   []cwltypes.OutputLogEvent
	tailUpdates []cwltypes.LiveTailSessionUpdate
}

func (s *stubCWLogs) DescribeLogGroups(_ context.Context, _ *cloudwatchlogs.DescribeLogGroupsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	return &cloudwatchlogs.DescribeLogGroupsOutput{LogGroups: s.logGroups}, nil
}

func (s *stubCWLogs) DescribeLogStreams(_ context.Context, _ *cloudwatchlogs.DescribeLogStreamsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	return &cloudwatchlogs.DescribeLogStreamsOutput{LogStreams: s.logStreams}, nil
}

func (s *stubCWLogs) GetLogEvents(_ context.Context, _ *cloudwatchlogs.GetLogEventsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.GetLogEventsOutput, error) {
	return &cloudwatchlogs.GetLogEventsOutput{Events: s.logEvents}, nil
}

func (s *stubCWLogs) OpenLiveTailStream(_ context.Context, _ []string) (cloudwatchlogs.StartLiveTailResponseStreamReader, error) {
	return newStubStreamReader(s.tailUpdates), nil
}

// stubStreamReader implements cloudwatchlogs.StartLiveTailResponseStreamReader.
type stubStreamReader struct {
	ch chan cwltypes.StartLiveTailResponseStream
}

func newStubStreamReader(updates []cwltypes.LiveTailSessionUpdate) *stubStreamReader {
	ch := make(chan cwltypes.StartLiveTailResponseStream, len(updates))
	for _, u := range updates {
		ch <- &cwltypes.StartLiveTailResponseStreamMemberSessionUpdate{Value: u}
	}
	close(ch)
	return &stubStreamReader{ch: ch}
}

func (r *stubStreamReader) Events() <-chan cwltypes.StartLiveTailResponseStream { return r.ch }
func (r *stubStreamReader) Close() error                                        { return nil }
func (r *stubStreamReader) Err() error                                          { return nil }

// ── helpers ──────────────────────────────────────────────────────────────────

func makeLogGroup(name string, retentionDays *int32, storedBytes *int64, creationTimeMs *int64) cwltypes.LogGroup {
	return cwltypes.LogGroup{
		LogGroupName:    aws.String(name),
		RetentionInDays: retentionDays,
		StoredBytes:     storedBytes,
		CreationTime:    creationTimeMs,
		LogGroupClass:   cwltypes.LogGroupClassStandard,
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestCWLogsProvider_ListItems(t *testing.T) {
	retention := int32(30)
	bytes := int64(1024 * 1024)
	groups := []cwltypes.LogGroup{
		makeLogGroup("/aws/lambda/func-a", &retention, &bytes, nil),
		makeLogGroup("/aws/lambda/func-b", nil, nil, nil),
	}
	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"all", "", 2},
		{"filter match", "func-a", 1},
		{"no match", "xyz", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := awspkg.NewCloudWatchLogsProviderWithClient(&stubCWLogs{logGroups: groups})
			items, err := p.ListItems(context.Background(), tc.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(items) != tc.want {
				t.Errorf("got %d items, want %d", len(items), tc.want)
			}
		})
	}
}

func TestCWLogsProvider_Tabs(t *testing.T) {
	retention := int32(30)
	bytes := int64(1258291)
	createdMs := int64(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli())
	lastEvent := int64(time.Date(2024, 2, 1, 9, 0, 0, 0, time.UTC).UnixMilli())

	stub := &stubCWLogs{
		logGroups: []cwltypes.LogGroup{
			makeLogGroup("/aws/lambda/my-func", &retention, &bytes, &createdMs),
		},
		logStreams: []cwltypes.LogStream{
			{
				LogStreamName:      aws.String("2024/01/01/[$LATEST]abc123"),
				LastEventTimestamp: &lastEvent,
			},
			{
				LogStreamName: aws.String("2024/01/02/[$LATEST]def456"),
			},
		},
	}
	p := awspkg.NewCloudWatchLogsProviderWithClient(stub)
	item := awspkg.Item{ID: "/aws/lambda/my-func", Name: "/aws/lambda/my-func"}
	tabs := p.Tabs()

	cases := []struct {
		tabIdx int
		label  string
		want   string
	}{
		{0, "Overview", "30 days"},
		{0, "Overview", "2024-01-01T00:00:00Z"},
		{1, "Streams", "2024/01/01"},
		{2, "Tail", ""},
	}
	for _, tc := range cases {
		t.Run(tc.label+"/"+tc.want, func(t *testing.T) {
			if tabs[tc.tabIdx].Label != tc.label {
				t.Errorf("tab %d label = %q, want %q", tc.tabIdx, tabs[tc.tabIdx].Label, tc.label)
			}
			content, err := tabs[tc.tabIdx].Fetch(context.Background(), item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.want != "" && !strings.Contains(content, tc.want) {
				t.Errorf("tab %q content missing %q\ngot:\n%s", tc.label, tc.want, content)
			}
		})
	}
}

func TestCWLogsProvider_GetLastStreams(t *testing.T) {
	lastEvent := int64(time.Date(2024, 2, 1, 9, 0, 0, 0, time.UTC).UnixMilli())
	stub := &stubCWLogs{
		logStreams: []cwltypes.LogStream{
			{LogStreamName: aws.String("stream-a"), LastEventTimestamp: &lastEvent},
			{LogStreamName: aws.String("stream-b")},
		},
	}
	p := awspkg.NewCloudWatchLogsProviderWithClient(stub)
	item := awspkg.Item{ID: "/aws/lambda/my-func"}
	tabs := p.Tabs()

	// Fetch the Streams tab to populate cache.
	_, err := tabs[1].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := p.GetLastStreams()
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Name != "stream-a" {
		t.Errorf("row[0].Name = %q, want stream-a", rows[0].Name)
	}
}

func TestCWLogsProvider_StartTail(t *testing.T) {
	ts := int64(1700000000000)

	type event struct {
		ts     int64
		group  string
		stream string
		msg    string
	}

	t.Run("stream tail via GetLogEvents", func(t *testing.T) {
		stub := &stubCWLogs{
			logEvents: []cwltypes.OutputLogEvent{
				{
					Timestamp: &ts,
					Message:   aws.String("hello world\n"),
				},
			},
		}
		p := awspkg.NewCloudWatchLogsProviderWithClient(stub)

		// Cancel immediately after initial fetch to avoid blocking on the poll ticker.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		var got []event
		err := p.StartTail(ctx, "/aws/lambda/my-func", "stream-1", func(ts int64, group, stream, msg string) {
			got = append(got, event{ts, group, stream, msg})
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d events, want 1", len(got))
		}
		if got[0].ts != ts {
			t.Errorf("ts = %d, want %d", got[0].ts, ts)
		}
		if got[0].group != "/aws/lambda/my-func" {
			t.Errorf("group = %q, want /aws/lambda/my-func", got[0].group)
		}
		if got[0].stream != "stream-1" {
			t.Errorf("stream = %q, want stream-1", got[0].stream)
		}
		if got[0].msg != "hello world" {
			t.Errorf("msg = %q, want \"hello world\"", got[0].msg)
		}
	})

	t.Run("group tail via StartLiveTail", func(t *testing.T) {
		stub := &stubCWLogs{
			tailUpdates: []cwltypes.LiveTailSessionUpdate{
				{
					SessionResults: []cwltypes.LiveTailSessionLogEvent{
						{
							Timestamp:          &ts,
							LogGroupIdentifier: aws.String("/aws/lambda/my-func"),
							LogStreamName:      aws.String("stream-1"),
							Message:            aws.String("live event\n"),
						},
					},
				},
			},
		}
		p := awspkg.NewCloudWatchLogsProviderWithClient(stub)

		var got []event
		// Empty stream name → tailGroup; stub channel closes after sending updates.
		err := p.StartTail(context.Background(), "/aws/lambda/my-func", "", func(ts int64, group, stream, msg string) {
			got = append(got, event{ts, group, stream, msg})
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d events, want 1", len(got))
		}
		if got[0].msg != "live event" {
			t.Errorf("msg = %q, want \"live event\"", got[0].msg)
		}
	})
}
