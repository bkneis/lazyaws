package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

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
	items, err := p.ListItems(context.Background())
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

func TestCFProvider_Tabs(t *testing.T) {
	p := awspkg.NewCFProviderWithClient(&stubCF{withOutputs: true})
	tabs := p.Tabs()
	if len(tabs) != 4 {
		t.Fatalf("got %d tabs, want 4", len(tabs))
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
