package aws_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

type stubSNS struct{}

func (s *stubSNS) ListTopics(_ context.Context, _ *sns.ListTopicsInput, _ ...func(*sns.Options)) (*sns.ListTopicsOutput, error) {
	return &sns.ListTopicsOutput{
		Topics: []snstypes.Topic{
			{TopicArn: aws.String("arn:aws:sns:us-east-1:123456789:order-events")},
		},
	}, nil
}

func (s *stubSNS) GetTopicAttributes(_ context.Context, _ *sns.GetTopicAttributesInput, _ ...func(*sns.Options)) (*sns.GetTopicAttributesOutput, error) {
	return &sns.GetTopicAttributesOutput{
		Attributes: map[string]string{
			"TopicArn":               "arn:aws:sns:us-east-1:123456789:order-events",
			"SubscriptionsConfirmed": "3",
			"SubscriptionsPending":   "1",
			"SubscriptionsDeleted":   "0",
			"FifoTopic":              "false",
		},
	}, nil
}

func (s *stubSNS) ListSubscriptionsByTopic(_ context.Context, _ *sns.ListSubscriptionsByTopicInput, _ ...func(*sns.Options)) (*sns.ListSubscriptionsByTopicOutput, error) {
	return &sns.ListSubscriptionsByTopicOutput{
		Subscriptions: []snstypes.Subscription{
			{
				Protocol:        aws.String("sqs"),
				Endpoint:        aws.String("arn:aws:sqs:us-east-1:123456789:order-queue"),
				SubscriptionArn: aws.String("arn:aws:sns:us-east-1:123456789:order-events:abc123"),
			},
			{
				Protocol:        aws.String("email"),
				Endpoint:        aws.String("ops@example.com"),
				SubscriptionArn: aws.String("PendingConfirmation"),
			},
		},
	}, nil
}

// stubSNSGetTopicErr returns an error from GetTopicAttributes and stubs others empty.
type stubSNSGetTopicErr struct{ err error }

func (s *stubSNSGetTopicErr) ListTopics(_ context.Context, _ *sns.ListTopicsInput, _ ...func(*sns.Options)) (*sns.ListTopicsOutput, error) {
	return &sns.ListTopicsOutput{}, nil
}

func (s *stubSNSGetTopicErr) GetTopicAttributes(_ context.Context, _ *sns.GetTopicAttributesInput, _ ...func(*sns.Options)) (*sns.GetTopicAttributesOutput, error) {
	return nil, s.err
}

func (s *stubSNSGetTopicErr) ListSubscriptionsByTopic(_ context.Context, _ *sns.ListSubscriptionsByTopicInput, _ ...func(*sns.Options)) (*sns.ListSubscriptionsByTopicOutput, error) {
	return &sns.ListSubscriptionsByTopicOutput{}, nil
}

func TestSNSProvider_FetchItem(t *testing.T) {
	cases := []struct {
		name     string
		id       string
		stub     awspkg.SNSAPI
		wantErr  bool
		wantID   string
		wantName string
	}{
		{
			name:     "found",
			id:       "arn:aws:sns:us-east-1:123456789:order-events",
			stub:     &stubSNS{},
			wantID:   "arn:aws:sns:us-east-1:123456789:order-events",
			wantName: "order-events",
		},
		{
			name:    "not found",
			id:      "arn:aws:sns:us-east-1:123456789:missing-topic",
			stub:    &stubSNSGetTopicErr{err: fmt.Errorf("topic not found")},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := awspkg.NewSNSProviderWithClient(tc.stub)
			item, err := p.FetchItem(context.Background(), tc.id)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if item.ID != tc.wantID {
				t.Errorf("got ID=%q, want %q", item.ID, tc.wantID)
			}
			if item.Name != tc.wantName {
				t.Errorf("got Name=%q, want %q", item.Name, tc.wantName)
			}
		})
	}
}

func TestSNSProvider_ListItems(t *testing.T) {
	p := awspkg.NewSNSProviderWithClient(&stubSNS{})
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Name != "order-events" {
		t.Errorf("got name %q, want order-events", items[0].Name)
	}
	if items[0].ID != "arn:aws:sns:us-east-1:123456789:order-events" {
		t.Errorf("got ID %q, want full ARN", items[0].ID)
	}
}

func TestSNSProvider_ListItems_Filter(t *testing.T) {
	stub := &stubSNSFilter{}
	p := awspkg.NewSNSProviderWithClient(stub)
	cases := []struct {
		query string
		want  int
	}{
		{"", 2},
		{"my", 1},
		{"MY", 1},
		{"xyz", 0},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
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

type stubSNSFilter struct{}

func (s *stubSNSFilter) ListTopics(_ context.Context, _ *sns.ListTopicsInput, _ ...func(*sns.Options)) (*sns.ListTopicsOutput, error) {
	return &sns.ListTopicsOutput{
		Topics: []snstypes.Topic{
			{TopicArn: aws.String("arn:aws:sns:us-east-1:123456789:my-topic")},
			{TopicArn: aws.String("arn:aws:sns:us-east-1:123456789:other-topic")},
		},
	}, nil
}

func (s *stubSNSFilter) GetTopicAttributes(_ context.Context, _ *sns.GetTopicAttributesInput, _ ...func(*sns.Options)) (*sns.GetTopicAttributesOutput, error) {
	return &sns.GetTopicAttributesOutput{Attributes: map[string]string{}}, nil
}

func (s *stubSNSFilter) ListSubscriptionsByTopic(_ context.Context, _ *sns.ListSubscriptionsByTopicInput, _ ...func(*sns.Options)) (*sns.ListSubscriptionsByTopicOutput, error) {
	return &sns.ListSubscriptionsByTopicOutput{}, nil
}

func TestSNSProvider_Tabs(t *testing.T) {
	p := awspkg.NewSNSProviderWithClient(&stubSNS{})
	tabs := p.Tabs()
	if len(tabs) != 2 {
		t.Fatalf("got %d tabs, want 2", len(tabs))
	}
	item := awspkg.Item{ID: "arn:aws:sns:us-east-1:123456789:order-events", Name: "order-events"}

	cases := []struct {
		idx   int
		label string
		want  string
	}{
		{0, "Overview", "order-events"},
		{1, "Subscriptions", "order-queue"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if tabs[tc.idx].Label != tc.label {
				t.Errorf("tab %d label = %q, want %q", tc.idx, tabs[tc.idx].Label, tc.label)
			}
			content, err := tabs[tc.idx].Fetch(context.Background(), item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(content, tc.want) {
				t.Errorf("tab %d missing %q\ngot:\n%s", tc.idx, tc.want, content)
			}
		})
	}
}

func TestSNSProvider_TabSubscriptions_Status(t *testing.T) {
	cases := []struct {
		subArn string
		want   string
	}{
		{"arn:aws:sns:us-east-1:123456789:order-events:abc123", "Confirmed"},
		{"PendingConfirmation", "Pending"},
		{"Deleted", "Deleted"},
	}
	for _, tc := range cases {
		t.Run(tc.subArn, func(t *testing.T) {
			stub := &stubSNSStatus{subArn: tc.subArn}
			p := awspkg.NewSNSProviderWithClient(stub)
			tabs := p.Tabs()
			item := awspkg.Item{ID: "arn:aws:sns:us-east-1:123:topic", Name: "topic"}
			content, err := tabs[1].Fetch(context.Background(), item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(content, tc.want) {
				t.Errorf("subArn=%q: expected status %q in output\ngot:\n%s", tc.subArn, tc.want, content)
			}
		})
	}
}

type stubSNSStatus struct {
	subArn string
}

func (s *stubSNSStatus) ListTopics(_ context.Context, _ *sns.ListTopicsInput, _ ...func(*sns.Options)) (*sns.ListTopicsOutput, error) {
	return &sns.ListTopicsOutput{}, nil
}

func (s *stubSNSStatus) GetTopicAttributes(_ context.Context, _ *sns.GetTopicAttributesInput, _ ...func(*sns.Options)) (*sns.GetTopicAttributesOutput, error) {
	return &sns.GetTopicAttributesOutput{Attributes: map[string]string{}}, nil
}

func (s *stubSNSStatus) ListSubscriptionsByTopic(_ context.Context, _ *sns.ListSubscriptionsByTopicInput, _ ...func(*sns.Options)) (*sns.ListSubscriptionsByTopicOutput, error) {
	return &sns.ListSubscriptionsByTopicOutput{
		Subscriptions: []snstypes.Subscription{
			{Protocol: aws.String("sqs"), Endpoint: aws.String("endpoint"), SubscriptionArn: aws.String(s.subArn)},
		},
	}, nil
}
