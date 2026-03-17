package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
)

// KinesisAPI is the subset of the Kinesis client methods used by KinesisProvider.
type KinesisAPI interface {
	ListStreams(ctx context.Context, in *kinesis.ListStreamsInput, opts ...func(*kinesis.Options)) (*kinesis.ListStreamsOutput, error)
	DescribeStreamSummary(ctx context.Context, in *kinesis.DescribeStreamSummaryInput, opts ...func(*kinesis.Options)) (*kinesis.DescribeStreamSummaryOutput, error)
	ListShards(ctx context.Context, in *kinesis.ListShardsInput, opts ...func(*kinesis.Options)) (*kinesis.ListShardsOutput, error)
	ListStreamConsumers(ctx context.Context, in *kinesis.ListStreamConsumersInput, opts ...func(*kinesis.Options)) (*kinesis.ListStreamConsumersOutput, error)
}

// KinesisProvider implements Provider for Amazon Kinesis Data Streams.
type KinesisProvider struct {
	client KinesisAPI
}

func NewKinesisProvider(cfg awssdk.Config, endpointURL string) *KinesisProvider {
	var opts []func(*kinesis.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *kinesis.Options) {
			o.BaseEndpoint = awssdk.String(endpointURL)
		})
	}
	return &KinesisProvider{client: kinesis.NewFromConfig(cfg, opts...)}
}

func NewKinesisProviderWithClient(client KinesisAPI) *KinesisProvider {
	return &KinesisProvider{client: client}
}

func (p *KinesisProvider) Name() string { return "Kinesis" }

func (p *KinesisProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	var items []Item
	var nextToken *string
	for {
		out, err := p.client.ListStreams(ctx, &kinesis.ListStreamsInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list streams: %w", err)
		}
		for _, s := range out.StreamSummaries {
			name := awssdk.ToString(s.StreamName)
			items = append(items, Item{ID: name, Name: name})
		}
		if !awssdk.ToBool(out.HasMoreStreams) || out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return filterItems(items, query), nil
}

func (p *KinesisProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *KinesisProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Shards", Fetch: p.tabShards},
		{Label: "Consumers", Fetch: p.tabConsumers},
	}
}

func (p *KinesisProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeStreamSummary(ctx, &kinesis.DescribeStreamSummaryInput{
		StreamName: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	s := out.StreamDescriptionSummary

	created := ""
	if s.StreamCreationTimestamp != nil {
		created = s.StreamCreationTimestamp.Format(time.DateTime)
	}

	retention := fmt.Sprintf("%dh", awssdk.ToInt32(s.RetentionPeriodHours))
	shardCount := fmt.Sprintf("%d", awssdk.ToInt32(s.OpenShardCount))

	encryption := string(s.EncryptionType)
	encVal := encryption
	if s.KeyId != nil && awssdk.ToString(s.KeyId) != "" {
		encVal = encryption + "  " + Link(awssdk.ToString(s.KeyId), "KMS", awssdk.ToString(s.KeyId))
	}

	var monMetrics []string
	for _, m := range s.EnhancedMonitoring {
		for _, sh := range m.ShardLevelMetrics {
			monMetrics = append(monMetrics, string(sh))
		}
	}
	monitoring := strings.Join(monMetrics, ", ")
	if monitoring == "" {
		monitoring = "None"
	}

	return KV([][2]string{
		{"Name", awssdk.ToString(s.StreamName)},
		{"ARN", awssdk.ToString(s.StreamARN)},
		{"Status", string(s.StreamStatus)},
		{"Shard Count", shardCount},
		{"Retention", retention},
		{"Created", created},
		{"Encryption", encVal},
		{"Enhanced Mon.", monitoring},
	}), nil
}

func (p *KinesisProvider) tabShards(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListShards(ctx, &kinesis.ListShardsInput{
		StreamName: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	if len(out.Shards) == 0 {
		return "  (no shards found)\n", nil
	}
	rows := make([][]string, len(out.Shards))
	for i, s := range out.Shards {
		startHash := ""
		endHash := ""
		if s.HashKeyRange != nil {
			startHash = awssdk.ToString(s.HashKeyRange.StartingHashKey)
			endHash = awssdk.ToString(s.HashKeyRange.EndingHashKey)
		}
		rows[i] = []string{awssdk.ToString(s.ShardId), startHash, endHash}
	}
	return Table([]string{"Shard ID", "Start Hash", "End Hash"}, rows), nil
}

func (p *KinesisProvider) tabConsumers(ctx context.Context, item Item) (string, error) {
	// ListStreamConsumers requires the stream ARN, so fetch it first
	desc, err := p.client.DescribeStreamSummary(ctx, &kinesis.DescribeStreamSummaryInput{
		StreamName: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	streamARN := desc.StreamDescriptionSummary.StreamARN

	out, err := p.client.ListStreamConsumers(ctx, &kinesis.ListStreamConsumersInput{
		StreamARN: streamARN,
	})
	if err != nil {
		return "", err
	}
	if len(out.Consumers) == 0 {
		return "  (no consumers registered)\n", nil
	}
	rows := make([][]string, len(out.Consumers))
	for i, c := range out.Consumers {
		rows[i] = []string{
			awssdk.ToString(c.ConsumerName),
			awssdk.ToString(c.ConsumerARN),
			string(c.ConsumerStatus),
		}
	}
	return Table([]string{"Name", "ARN", "Status"}, rows), nil
}
