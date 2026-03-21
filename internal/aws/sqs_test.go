package aws_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

type stubSQS struct{ withDLQ bool }

func (s *stubSQS) ListQueues(_ context.Context, _ *sqs.ListQueuesInput, _ ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error) {
	return &sqs.ListQueuesOutput{
		QueueUrls: []string{"https://sqs.us-east-1.amazonaws.com/123456789/order-queue"},
	}, nil
}

func (s *stubSQS) GetQueueAttributes(_ context.Context, _ *sqs.GetQueueAttributesInput, _ ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	attrs := map[string]string{
		string(sqstypes.QueueAttributeNameQueueArn):                              "arn:aws:sqs:us-east-1:123456789:order-queue",
		string(sqstypes.QueueAttributeNameApproximateNumberOfMessages):           "42",
		string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible): "3",
		string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesDelayed):    "0",
		string(sqstypes.QueueAttributeNameVisibilityTimeout):                     "30",
		string(sqstypes.QueueAttributeNameMessageRetentionPeriod):                "345600",
		string(sqstypes.QueueAttributeNameMaximumMessageSize):                    "262144",
		string(sqstypes.QueueAttributeNameDelaySeconds):                          "0",
		string(sqstypes.QueueAttributeNameReceiveMessageWaitTimeSeconds):         "0",
		"SqsManagedSseEnabled": "true",
	}
	if s.withDLQ {
		attrs[string(sqstypes.QueueAttributeNameRedrivePolicy)] = `{"deadLetterTargetArn":"arn:aws:sqs:us-east-1:123:order-dlq","maxReceiveCount":3}`
	}
	return &sqs.GetQueueAttributesOutput{Attributes: attrs}, nil
}

func (s *stubSQS) ReceiveMessage(_ context.Context, _ *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	return &sqs.ReceiveMessageOutput{}, nil
}

// stubSQSGetAttrErr returns an error from GetQueueAttributes.
type stubSQSGetAttrErr struct{ err error }

func (s *stubSQSGetAttrErr) ListQueues(_ context.Context, _ *sqs.ListQueuesInput, _ ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error) {
	return &sqs.ListQueuesOutput{}, nil
}

func (s *stubSQSGetAttrErr) GetQueueAttributes(_ context.Context, _ *sqs.GetQueueAttributesInput, _ ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	return nil, s.err
}

func (s *stubSQSGetAttrErr) ReceiveMessage(_ context.Context, _ *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	return &sqs.ReceiveMessageOutput{}, nil
}

func TestSQSProvider_FetchItem(t *testing.T) {
	cases := []struct {
		name     string
		id       string
		stub     awspkg.SQSAPI
		wantErr  bool
		wantID   string
		wantName string
	}{
		{
			name:     "found",
			id:       "https://sqs.us-east-1.amazonaws.com/123456789/order-queue",
			stub:     &stubSQS{},
			wantID:   "https://sqs.us-east-1.amazonaws.com/123456789/order-queue",
			wantName: "order-queue",
		},
		{
			name:    "not found",
			id:      "https://sqs.us-east-1.amazonaws.com/123456789/missing-queue",
			stub:    &stubSQSGetAttrErr{err: fmt.Errorf("queue not found")},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := awspkg.NewSQSProviderWithClient(tc.stub)
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

func TestSQSProvider_ListItems(t *testing.T) {
	p := awspkg.NewSQSProviderWithClient(&stubSQS{})
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Name != "order-queue" {
		t.Errorf("got name %q, want order-queue", items[0].Name)
	}
}

func TestSQSProvider_ListItems_Filter(t *testing.T) {
	stub := &stubSQSFilter{}
	p := awspkg.NewSQSProviderWithClient(stub)
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

type stubSQSFilter struct{}

func (s *stubSQSFilter) ListQueues(_ context.Context, _ *sqs.ListQueuesInput, _ ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error) {
	return &sqs.ListQueuesOutput{
		QueueUrls: []string{
			"https://sqs.us-east-1.amazonaws.com/123456789/my-queue",
			"https://sqs.us-east-1.amazonaws.com/123456789/other-queue",
		},
	}, nil
}

func (s *stubSQSFilter) GetQueueAttributes(_ context.Context, _ *sqs.GetQueueAttributesInput, _ ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	return &sqs.GetQueueAttributesOutput{Attributes: map[string]string{}}, nil
}

func (s *stubSQSFilter) ReceiveMessage(_ context.Context, _ *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	return &sqs.ReceiveMessageOutput{}, nil
}

func TestSQSProvider_Tabs(t *testing.T) {
	p := awspkg.NewSQSProviderWithClient(&stubSQS{})
	tabs := p.Tabs()
	if len(tabs) != 4 {
		t.Fatalf("got %d tabs, want 4", len(tabs))
	}
	item := awspkg.Item{ID: "https://sqs.us-east-1.amazonaws.com/123/order-queue", Name: "order-queue"}

	cases := []struct {
		idx   int
		label string
		want  string
	}{
		{0, "Overview", "42"},
		{1, "Config", "30s"},
		{2, "DLQ", "(no dead-letter queue configured)"},
		{3, "Messages", "(no messages)"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
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

func TestSQSProvider_TabDLQ_WithDLQ(t *testing.T) {
	p := awspkg.NewSQSProviderWithClient(&stubSQS{withDLQ: true})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "https://sqs.us-east-1.amazonaws.com/123/order-queue", Name: "order-queue"}
	content, err := tabs[2].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "order-dlq") {
		t.Errorf("DLQ tab missing order-dlq ARN\ngot:\n%s", content)
	}
}

func TestSQSProvider_FormatSeconds(t *testing.T) {
	p := awspkg.NewSQSProviderWithClient(&stubSQS{})
	tabs := p.Tabs()
	// MessageRetentionPeriod = 345600 = 4 days
	item := awspkg.Item{ID: "https://sqs.us-east-1.amazonaws.com/123/order-queue", Name: "order-queue"}
	content, err := tabs[1].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "4 days") {
		t.Errorf("expected '4 days' for 345600s retention\ngot:\n%s", content)
	}
}
