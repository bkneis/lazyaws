package aws

import (
	"context"
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
)

type SNSAPI interface {
	ListTopics(ctx context.Context, in *sns.ListTopicsInput, opts ...func(*sns.Options)) (*sns.ListTopicsOutput, error)
	GetTopicAttributes(ctx context.Context, in *sns.GetTopicAttributesInput, opts ...func(*sns.Options)) (*sns.GetTopicAttributesOutput, error)
	ListSubscriptionsByTopic(ctx context.Context, in *sns.ListSubscriptionsByTopicInput, opts ...func(*sns.Options)) (*sns.ListSubscriptionsByTopicOutput, error)
}

type SNSProvider struct{ client SNSAPI }

func NewSNSProvider(cfg awssdk.Config, endpointURL string) *SNSProvider {
	var opts []func(*sns.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *sns.Options) { o.BaseEndpoint = awssdk.String(endpointURL) })
	}
	return &SNSProvider{client: sns.NewFromConfig(cfg, opts...)}
}

func NewSNSProviderWithClient(client SNSAPI) *SNSProvider { return &SNSProvider{client: client} }

func (p *SNSProvider) Name() string { return "SNS" }

func (p *SNSProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.ListTopics(ctx, &sns.ListTopicsInput{})
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	items := make([]Item, len(out.Topics))
	for i, t := range out.Topics {
		arn := awssdk.ToString(t.TopicArn)
		name := arn
		if parts := strings.Split(arn, ":"); len(parts) > 0 {
			name = parts[len(parts)-1]
		}
		items[i] = Item{ID: arn, Name: name}
	}
	return filterItems(items, query), nil
}

func (p *SNSProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *SNSProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Subscriptions", Fetch: p.tabSubscriptions},
	}
}

func (p *SNSProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetTopicAttributes(ctx, &sns.GetTopicAttributesInput{TopicArn: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	attrs := out.Attributes
	topicType := "Standard"
	if attrs["FifoTopic"] == "true" {
		topicType = "FIFO"
	}
	kms := attrs["KmsMasterKeyId"]
	if kms == "" {
		kms = "(none)"
	}
	return KV([][2]string{
		{"ARN", item.ID},
		{"Type", topicType},
		{"Confirmed", attrs["SubscriptionsConfirmed"]},
		{"Pending", attrs["SubscriptionsPending"]},
		{"Deleted", attrs["SubscriptionsDeleted"]},
		{"KMS Key", kms},
	}), nil
}

func (p *SNSProvider) tabSubscriptions(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListSubscriptionsByTopic(ctx, &sns.ListSubscriptionsByTopicInput{TopicArn: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	if len(out.Subscriptions) == 0 {
		return "  (no subscriptions)\n", nil
	}
	rows := make([][]string, len(out.Subscriptions))
	for i, s := range out.Subscriptions {
		rows[i] = []string{awssdk.ToString(s.Protocol), awssdk.ToString(s.Endpoint), subscriptionStatus(s)}
	}
	return Table([]string{"Protocol", "Endpoint", "Status"}, rows), nil
}

func subscriptionStatus(s snstypes.Subscription) string {
	arn := awssdk.ToString(s.SubscriptionArn)
	switch arn {
	case "PendingConfirmation":
		return "Pending"
	case "Deleted":
		return "Deleted"
	default:
		if strings.HasPrefix(arn, "arn:") {
			return "Confirmed"
		}
		return arn
	}
}
