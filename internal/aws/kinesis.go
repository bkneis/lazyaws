package aws

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	kinesistypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
)

// KinesisAPI is the subset of the Kinesis client methods used by KinesisProvider.
type KinesisAPI interface {
	ListStreams(ctx context.Context, in *kinesis.ListStreamsInput, opts ...func(*kinesis.Options)) (*kinesis.ListStreamsOutput, error)
	DescribeStreamSummary(ctx context.Context, in *kinesis.DescribeStreamSummaryInput, opts ...func(*kinesis.Options)) (*kinesis.DescribeStreamSummaryOutput, error)
	ListShards(ctx context.Context, in *kinesis.ListShardsInput, opts ...func(*kinesis.Options)) (*kinesis.ListShardsOutput, error)
	ListStreamConsumers(ctx context.Context, in *kinesis.ListStreamConsumersInput, opts ...func(*kinesis.Options)) (*kinesis.ListStreamConsumersOutput, error)
	GetShardIterator(ctx context.Context, in *kinesis.GetShardIteratorInput, opts ...func(*kinesis.Options)) (*kinesis.GetShardIteratorOutput, error)
	GetRecords(ctx context.Context, in *kinesis.GetRecordsInput, opts ...func(*kinesis.Options)) (*kinesis.GetRecordsOutput, error)
}

// KinesisShardItem holds display data for a single Kinesis shard row.
type KinesisShardItem struct {
	ShardID   string
	StartHash string
	EndHash   string
}

// KinesisProvider implements Provider for Amazon Kinesis Data Streams.
type KinesisProvider struct {
	client          KinesisAPI
	mu              sync.RWMutex
	lastShards      []KinesisShardItem
	selectedShardID string
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
		{Label: "Records", Fetch: p.tabRecords},
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
	shards := make([]KinesisShardItem, len(out.Shards))
	rows := make([][]string, len(out.Shards))
	for i, s := range out.Shards {
		startHash := ""
		endHash := ""
		if s.HashKeyRange != nil {
			startHash = awssdk.ToString(s.HashKeyRange.StartingHashKey)
			endHash = awssdk.ToString(s.HashKeyRange.EndingHashKey)
		}
		shardID := awssdk.ToString(s.ShardId)
		shards[i] = KinesisShardItem{ShardID: shardID, StartHash: startHash, EndHash: endHash}
		rows[i] = []string{shardID, startHash, endHash}
	}
	p.mu.Lock()
	p.lastShards = shards
	p.mu.Unlock()
	return Table([]string{"Shard ID", "Start Hash", "End Hash"}, rows), nil
}

// GetLastShards returns the shards cached by the most recent tabShards call.
func (p *KinesisProvider) GetLastShards() []KinesisShardItem {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]KinesisShardItem, len(p.lastShards))
	copy(out, p.lastShards)
	return out
}

// SetSelectedShard records the currently selected shard ID for the Records tab.
func (p *KinesisProvider) SetSelectedShard(id string) {
	p.mu.Lock()
	p.selectedShardID = id
	p.mu.Unlock()
}

func (p *KinesisProvider) tabRecords(ctx context.Context, item Item) (string, error) {
	p.mu.RLock()
	shardID := p.selectedShardID
	p.mu.RUnlock()
	if shardID == "" {
		return "  Select a shard in the Shards tab (j/k) to view records.\n", nil
	}
	itOut, err := p.client.GetShardIterator(ctx, &kinesis.GetShardIteratorInput{
		StreamName:        awssdk.String(item.ID),
		ShardId:           awssdk.String(shardID),
		ShardIteratorType: kinesistypes.ShardIteratorTypeLatest,
	})
	if err != nil {
		return "", fmt.Errorf("get shard iterator: %w", err)
	}
	recOut, err := p.client.GetRecords(ctx, &kinesis.GetRecordsInput{
		ShardIterator: itOut.ShardIterator,
		Limit:         awssdk.Int32(50),
	})
	if err != nil {
		return "", fmt.Errorf("get records: %w", err)
	}
	if len(recOut.Records) == 0 {
		return fmt.Sprintf("  (no records in shard %s — iterator is at LATEST)\n", shardID), nil
	}
	rows := make([][]string, len(recOut.Records))
	for i, r := range recOut.Records {
		ts := ""
		if r.ApproximateArrivalTimestamp != nil {
			ts = r.ApproximateArrivalTimestamp.Format(time.DateTime)
		}
		rows[i] = []string{
			awssdk.ToString(r.SequenceNumber),
			awssdk.ToString(r.PartitionKey),
			formatRecordData(r.Data),
			ts,
		}
	}
	return Table([]string{"Seq #", "Partition Key", "Data", "Timestamp"}, rows), nil
}

// formatRecordData returns a human-readable representation of Kinesis record data.
// Valid UTF-8 is displayed directly (truncated to 80 chars); binary data shows a hex prefix.
func formatRecordData(data []byte) string {
	if len(data) == 0 {
		return "(empty)"
	}
	if utf8.Valid(data) {
		s := strings.ReplaceAll(string(data), "\n", " ")
		if len(s) > 80 {
			return s[:80] + "…"
		}
		return s
	}
	n := 8
	if len(data) < n {
		n = len(data)
	}
	return "0x" + hex.EncodeToString(data[:n]) + "…"
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
