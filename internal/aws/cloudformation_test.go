package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

func (s *stubCF) DescribeStackEvents(_ context.Context, _ *cloudformation.DescribeStackEventsInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeStackEventsOutput, error) {
	ts := time.Date(2026, 3, 15, 14, 22, 5, 0, time.UTC)
	return &cloudformation.DescribeStackEventsOutput{
		StackEvents: []cftypes.StackEvent{
			{
				Timestamp:            &ts,
				LogicalResourceId:    aws.String("my-app-stack"),
				ResourceType:         aws.String("AWS::CloudFormation::Stack"),
				ResourceStatus:       cftypes.ResourceStatusCreateComplete,
				ResourceStatusReason: aws.String(""),
			},
			{
				Timestamp:            &ts,
				LogicalResourceId:    aws.String("MyFunction"),
				ResourceType:         aws.String("AWS::Lambda::Function"),
				ResourceStatus:       cftypes.ResourceStatusCreateComplete,
				ResourceStatusReason: aws.String(""),
			},
		},
	}, nil
}

func (s *stubCFFilter) DescribeStackEvents(_ context.Context, _ *cloudformation.DescribeStackEventsInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeStackEventsOutput, error) {
	return &cloudformation.DescribeStackEventsOutput{}, nil
}

type stubCF struct{ withOutputs bool }

func (s *stubCF) DescribeStacks(_ context.Context, in *cloudformation.DescribeStacksInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error) {
	created := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	outputs := []cftypes.Output{}
	params := []cftypes.Parameter{
		{ParameterKey: aws.String("Env"), ParameterValue: aws.String("production")},
		{ParameterKey: aws.String("Region"), ParameterValue: aws.String("us-east-1")},
	}
	if s.withOutputs {
		outputs = []cftypes.Output{
			{OutputKey: aws.String("ApiEndpoint"), OutputValue: aws.String("https://api.example.com")},
			{OutputKey: aws.String("BucketName"), OutputValue: aws.String("my-app-bucket")},
		}
	}
	return &cloudformation.DescribeStacksOutput{
		Stacks: []cftypes.Stack{
			{
				StackName:    aws.String("my-app-stack"),
				StackStatus:  cftypes.StackStatusCreateComplete,
				CreationTime: &created,
				Description:  aws.String("My application stack"),
				Outputs:      outputs,
				Parameters:   params,
			},
		},
	}, nil
}

func (s *stubCF) ListStackResources(_ context.Context, _ *cloudformation.ListStackResourcesInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListStackResourcesOutput, error) {
	return &cloudformation.ListStackResourcesOutput{
		StackResourceSummaries: []cftypes.StackResourceSummary{
			{
				LogicalResourceId: aws.String("MyBucket"),
				ResourceType:      aws.String("AWS::S3::Bucket"),
				ResourceStatus:    cftypes.ResourceStatusCreateComplete,
			},
			{
				LogicalResourceId: aws.String("MyFunction"),
				ResourceType:      aws.String("AWS::Lambda::Function"),
				ResourceStatus:    cftypes.ResourceStatusCreateComplete,
			},
		},
	}, nil
}

func TestCFProvider_ListItems(t *testing.T) {
	p := awspkg.NewCFProviderWithClient(&stubCF{})
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Name != "my-app-stack" {
		t.Errorf("got name %q, want my-app-stack", items[0].Name)
	}
	if items[0].ID != "my-app-stack" {
		t.Errorf("got ID %q, want my-app-stack", items[0].ID)
	}
}

func TestCFProvider_ListItems_Filter(t *testing.T) {
	stub := &stubCFFilter{}
	p := awspkg.NewCFProviderWithClient(stub)
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

type stubCFFilter struct{}

func (s *stubCFFilter) DescribeStacks(_ context.Context, _ *cloudformation.DescribeStacksInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error) {
	return &cloudformation.DescribeStacksOutput{
		Stacks: []cftypes.Stack{
			{StackName: aws.String("my-stack"), StackStatus: cftypes.StackStatusCreateComplete},
			{StackName: aws.String("other-stack"), StackStatus: cftypes.StackStatusCreateComplete},
		},
	}, nil
}

func (s *stubCFFilter) ListStackResources(_ context.Context, _ *cloudformation.ListStackResourcesInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListStackResourcesOutput, error) {
	return &cloudformation.ListStackResourcesOutput{}, nil
}

func TestCFProvider_Tabs(t *testing.T) {
	p := awspkg.NewCFProviderWithClient(&stubCF{withOutputs: true})
	tabs := p.Tabs()
	if len(tabs) != 5 {
		t.Fatalf("got %d tabs, want 5", len(tabs))
	}
	item := awspkg.Item{ID: "my-app-stack", Name: "my-app-stack"}

	cases := []struct {
		idx   int
		label string
		want  string
	}{
		{0, "Overview", "my-app-stack"},
		{0, "Overview", "CREATE_COMPLETE"},
		{1, "Resources", "AWS::S3::Bucket"},
		{2, "Outputs", "ApiEndpoint"},
		{3, "Parameters", "production"},
		{4, "Events", "AWS::CloudFormation::Stack"},
		{4, "Events", "MyFunction"},
	}
	for _, tc := range cases {
		t.Run(tc.label+"/"+tc.want, func(t *testing.T) {
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

func TestCFProvider_TabOutputs_NoOutputs(t *testing.T) {
	p := awspkg.NewCFProviderWithClient(&stubCF{withOutputs: false})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "my-app-stack", Name: "my-app-stack"}
	content, err := tabs[2].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "(no outputs)") {
		t.Errorf("expected '(no outputs)'\ngot:\n%s", content)
	}
}

func TestCFProvider_TabResources_LogicalID(t *testing.T) {
	p := awspkg.NewCFProviderWithClient(&stubCF{})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "my-app-stack", Name: "my-app-stack"}
	content, err := tabs[1].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "MyFunction") {
		t.Errorf("expected MyFunction in resources\ngot:\n%s", content)
	}
}

func TestCFProvider_TabEvents(t *testing.T) {
	p := awspkg.NewCFProviderWithClient(&stubCF{})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "my-app-stack", Name: "my-app-stack"}

	cases := []struct {
		want string
	}{
		{"AWS::CloudFormation::Stack"},
		{"MyFunction"},
		{"AWS::Lambda::Function"},
		{"CREATE_COMPLETE"},
		{"2026-03-15"},
	}
	content, err := tabs[4].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, tc := range cases {
		if !strings.Contains(content, tc.want) {
			t.Errorf("events tab missing %q\ngot:\n%s", tc.want, content)
		}
	}
}

func TestCFProvider_TabEvents_Empty(t *testing.T) {
	p := awspkg.NewCFProviderWithClient(&stubCFFilter{})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "my-stack", Name: "my-stack"}
	content, err := tabs[4].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "(no events)") {
		t.Errorf("expected '(no events)'\ngot:\n%s", content)
	}
}
