package aws_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	ktypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

type stubKinesis struct {
	streams   []ktypes.StreamSummary
	summary   *ktypes.StreamDescriptionSummary
	shards    []ktypes.Shard
	consumers []ktypes.Consumer
	records   []ktypes.Record
}

func (s *stubKinesis) ListStreams(_ context.Context, _ *kinesis.ListStreamsInput, _ ...func(*kinesis.Options)) (*kinesis.ListStreamsOutput, error) {
	return &kinesis.ListStreamsOutput{StreamSummaries: s.streams}, nil
}

func (s *stubKinesis) DescribeStreamSummary(_ context.Context, _ *kinesis.DescribeStreamSummaryInput, _ ...func(*kinesis.Options)) (*kinesis.DescribeStreamSummaryOutput, error) {
	return &kinesis.DescribeStreamSummaryOutput{StreamDescriptionSummary: s.summary}, nil
}

func (s *stubKinesis) ListShards(_ context.Context, _ *kinesis.ListShardsInput, _ ...func(*kinesis.Options)) (*kinesis.ListShardsOutput, error) {
	return &kinesis.ListShardsOutput{Shards: s.shards}, nil
}

func (s *stubKinesis) ListStreamConsumers(_ context.Context, _ *kinesis.ListStreamConsumersInput, _ ...func(*kinesis.Options)) (*kinesis.ListStreamConsumersOutput, error) {
	return &kinesis.ListStreamConsumersOutput{Consumers: s.consumers}, nil
}

func (s *stubKinesis) GetShardIterator(_ context.Context, _ *kinesis.GetShardIteratorInput, _ ...func(*kinesis.Options)) (*kinesis.GetShardIteratorOutput, error) {
	iter := "stub-iterator"
	return &kinesis.GetShardIteratorOutput{ShardIterator: &iter}, nil
}

func (s *stubKinesis) GetRecords(_ context.Context, _ *kinesis.GetRecordsInput, _ ...func(*kinesis.Options)) (*kinesis.GetRecordsOutput, error) {
	return &kinesis.GetRecordsOutput{Records: s.records}, nil
}

func TestKinesisProvider_ListItems(t *testing.T) {
	cases := []struct {
		name    string
		streams []ktypes.StreamSummary
		query   string
		want    int
	}{
		{"all", []ktypes.StreamSummary{{StreamName: aws.String("events")}, {StreamName: aws.String("logs")}}, "", 2},
		{"filter", []ktypes.StreamSummary{{StreamName: aws.String("events")}, {StreamName: aws.String("logs")}}, "event", 1},
		{"no match", []ktypes.StreamSummary{{StreamName: aws.String("events")}}, "xyz", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := awspkg.NewKinesisProviderWithClient(&stubKinesis{streams: tc.streams})
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

func TestKinesisProvider_Tabs(t *testing.T) {
	retention := int32(24)
	shardCount := int32(2)
	stub := &stubKinesis{
		streams: []ktypes.StreamSummary{{StreamName: aws.String("events")}},
		summary: &ktypes.StreamDescriptionSummary{
			StreamName:           aws.String("events"),
			StreamARN:            aws.String("arn:aws:kinesis:us-east-1:123:stream/events"),
			StreamStatus:         ktypes.StreamStatusActive,
			RetentionPeriodHours: &retention,
			OpenShardCount:       &shardCount,
			EncryptionType:       ktypes.EncryptionTypeNone,
		},
		shards: []ktypes.Shard{
			{
				ShardId: aws.String("shardId-000000000000"),
				HashKeyRange: &ktypes.HashKeyRange{
					StartingHashKey: aws.String("0"),
					EndingHashKey:   aws.String("170141183460469231731687303715884105727"),
				},
			},
		},
		consumers: []ktypes.Consumer{
			{ConsumerName: aws.String("analytics-svc"), ConsumerARN: aws.String("arn:aws:kinesis::consumer/analytics"), ConsumerStatus: ktypes.ConsumerStatusActive},
		},
	}
	p := awspkg.NewKinesisProviderWithClient(stub)
	item := awspkg.Item{ID: "events", Name: "events"}
	tabs := p.Tabs()

	cases := []struct {
		tabIdx int
		label  string
		want   string
	}{
		{0, "Overview", "ACTIVE"},
		{1, "Shards", "shardId-000000000000"},
		{2, "Consumers", "analytics-svc"},
		{3, "Records", "Select a shard"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if tabs[tc.tabIdx].Label != tc.label {
				t.Errorf("tab %d label = %q, want %q", tc.tabIdx, tabs[tc.tabIdx].Label, tc.label)
			}
			content, err := tabs[tc.tabIdx].Fetch(context.Background(), item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(content, tc.want) {
				t.Errorf("tab %q missing %q\ngot:\n%s", tc.label, tc.want, content)
			}
		})
	}
}

func TestKinesisProvider_GetLastShards(t *testing.T) {
	stub := &stubKinesis{
		shards: []ktypes.Shard{
			{
				ShardId: aws.String("shardId-000000000000"),
				HashKeyRange: &ktypes.HashKeyRange{
					StartingHashKey: aws.String("0"),
					EndingHashKey:   aws.String("99"),
				},
			},
		},
	}
	p := awspkg.NewKinesisProviderWithClient(stub)
	item := awspkg.Item{ID: "events"}

	// Initially empty.
	if got := p.GetLastShards(); len(got) != 0 {
		t.Errorf("expected 0 shards before fetch, got %d", len(got))
	}

	// Fetch the Shards tab to populate cache.
	tabs := p.Tabs()
	for _, tab := range tabs {
		if tab.Label == "Shards" {
			if _, err := tab.Fetch(context.Background(), item); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
	}

	shards := p.GetLastShards()
	if len(shards) != 1 {
		t.Fatalf("got %d shards, want 1", len(shards))
	}
	if shards[0].ShardID != "shardId-000000000000" {
		t.Errorf("ShardID = %q, want shardId-000000000000", shards[0].ShardID)
	}

	// SetSelectedShard + Records tab should include the selected shard ID in output.
	p.SetSelectedShard("shardId-000000000000")
	stub.records = []ktypes.Record{
		{
			SequenceNumber: aws.String("seq-1"),
			PartitionKey:   aws.String("pk-1"),
			Data:           []byte("hello world"),
		},
	}
	for _, tab := range p.Tabs() {
		if tab.Label == "Records" {
			content, err := tab.Fetch(context.Background(), item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(content, "hello world") {
				t.Errorf("Records tab missing record data\ngot:\n%s", content)
			}
		}
	}
}
