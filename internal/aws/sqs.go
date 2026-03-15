package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type SQSAPI interface {
	ListQueues(ctx context.Context, in *sqs.ListQueuesInput, opts ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error)
	GetQueueAttributes(ctx context.Context, in *sqs.GetQueueAttributesInput, opts ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
}

type SQSProvider struct{ client SQSAPI }

func NewSQSProvider(cfg awssdk.Config, local bool) *SQSProvider {
	var opts []func(*sqs.Options)
	if local {
		opts = append(opts, func(o *sqs.Options) { o.BaseEndpoint = awssdk.String("http://localhost:4566") })
	}
	return &SQSProvider{client: sqs.NewFromConfig(cfg, opts...)}
}

func NewSQSProviderWithClient(client SQSAPI) *SQSProvider { return &SQSProvider{client: client} }

func (p *SQSProvider) Name() string { return "SQS" }

func (p *SQSProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.ListQueues(ctx, &sqs.ListQueuesInput{})
	if err != nil {
		return nil, fmt.Errorf("list queues: %w", err)
	}
	items := make([]Item, len(out.QueueUrls))
	for i, url := range out.QueueUrls {
		name := url
		if parts := strings.Split(url, "/"); len(parts) > 0 {
			name = parts[len(parts)-1]
		}
		items[i] = Item{ID: url, Name: name}
	}
	return filterItems(items, query), nil
}

func (p *SQSProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *SQSProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Config", Fetch: p.tabConfig},
		{Label: "DLQ", Fetch: p.tabDLQ},
	}
}

func (p *SQSProvider) getAttrs(ctx context.Context, queueURL string) (map[string]string, error) {
	out, err := p.client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       awssdk.String(queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameAll},
	})
	if err != nil {
		return nil, err
	}
	return out.Attributes, nil
}

func (p *SQSProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	attrs, err := p.getAttrs(ctx, item.ID)
	if err != nil {
		return "", err
	}
	queueType := "Standard"
	if strings.HasSuffix(item.Name, ".fifo") {
		queueType = "FIFO"
	}
	available := attrs["ApproximateNumberOfMessages"]
	inflight := attrs["ApproximateNumberOfMessagesNotVisible"]
	delayed := attrs["ApproximateNumberOfMessagesDelayed"]
	arn := attrs["QueueArn"]
	return KV([][2]string{
		{"Type", queueType},
		{"Available", available + " messages"},
		{"In-flight", inflight + " messages"},
		{"Delayed", delayed + " messages"},
		{"ARN", arn},
	}), nil
}

func (p *SQSProvider) tabConfig(ctx context.Context, item Item) (string, error) {
	attrs, err := p.getAttrs(ctx, item.ID)
	if err != nil {
		return "", err
	}
	encryption := "None"
	if attrs["SqsManagedSseEnabled"] == "true" {
		encryption = "SSE-SQS"
	} else if attrs["KmsMasterKeyId"] != "" {
		encryption = "SSE-KMS"
	}
	return KV([][2]string{
		{"Visibility Timeout", formatSeconds(attrs["VisibilityTimeout"])},
		{"Message Retention", formatSeconds(attrs["MessageRetentionPeriod"])},
		{"Max Message Size", formatBytes(attrs["MaximumMessageSize"])},
		{"Delivery Delay", formatSeconds(attrs["DelaySeconds"])},
		{"Receive Wait Time", formatSeconds(attrs["ReceiveMessageWaitTimeSeconds"])},
		{"Encryption", encryption},
	}), nil
}

func (p *SQSProvider) tabDLQ(ctx context.Context, item Item) (string, error) {
	attrs, err := p.getAttrs(ctx, item.ID)
	if err != nil {
		return "", err
	}
	policy := attrs["RedrivePolicy"]
	if policy == "" {
		return "  (no dead-letter queue configured)\n", nil
	}
	var redrive struct {
		DeadLetterTargetArn string `json:"deadLetterTargetArn"`
		MaxReceiveCount     int    `json:"maxReceiveCount"`
	}
	if err := json.Unmarshal([]byte(policy), &redrive); err != nil {
		return "  (could not parse redrive policy)\n", nil
	}
	dlqURL := arnToSQSURL(redrive.DeadLetterTargetArn)
	return KV([][2]string{
		{"DLQ", Link(redrive.DeadLetterTargetArn, "SQS", dlqURL)},
		{"Max Receives", fmt.Sprintf("%d", redrive.MaxReceiveCount)},
	}), nil
}

func formatSeconds(s string) string {
	if s == "" {
		return "0s"
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return s
	}
	switch {
	case n >= 86400:
		return fmt.Sprintf("%d days", n/86400)
	case n >= 3600:
		return fmt.Sprintf("%dh", n/3600)
	case n >= 60:
		return fmt.Sprintf("%dm", n/60)
	default:
		return fmt.Sprintf("%ds", n)
	}
}

func formatBytes(s string) string {
	if s == "" {
		return "0 B"
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return s
	}
	if n >= 1024 {
		return fmt.Sprintf("%d KB", n/1024)
	}
	return fmt.Sprintf("%d B", n)
}
