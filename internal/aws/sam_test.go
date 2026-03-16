package aws_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

type stubSAM struct {
	stacks    []cfntypes.Stack
	transforms []string
	resources  []cfntypes.StackResourceSummary
	changeSets []cfntypes.ChangeSetSummary
}

func (s *stubSAM) DescribeStacks(_ context.Context, in *cloudformation.DescribeStacksInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error) {
	return &cloudformation.DescribeStacksOutput{Stacks: s.stacks}, nil
}

func (s *stubSAM) GetTemplateSummary(_ context.Context, _ *cloudformation.GetTemplateSummaryInput, _ ...func(*cloudformation.Options)) (*cloudformation.GetTemplateSummaryOutput, error) {
	return &cloudformation.GetTemplateSummaryOutput{DeclaredTransforms: s.transforms}, nil
}

func (s *stubSAM) ListStackResources(_ context.Context, _ *cloudformation.ListStackResourcesInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListStackResourcesOutput, error) {
	return &cloudformation.ListStackResourcesOutput{StackResourceSummaries: s.resources}, nil
}

func (s *stubSAM) ListChangeSets(_ context.Context, _ *cloudformation.ListChangeSetsInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListChangeSetsOutput, error) {
	return &cloudformation.ListChangeSetsOutput{Summaries: s.changeSets}, nil
}

func TestSAMProvider_ListItems(t *testing.T) {
	stub := &stubSAM{
		stacks: []cfntypes.Stack{
			{StackName: aws.String("my-sam-app")},
			{StackName: aws.String("plain-cfn-stack")},
		},
		transforms: []string{"AWS::Serverless-2016-10-31"},
	}
	p := awspkg.NewSAMProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	// Both stacks match because the stub always returns the same transforms.
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].ID != "my-sam-app" {
		t.Errorf("want ID=my-sam-app, got %s", items[0].ID)
	}
}

func TestSAMProvider_ListItems_NonSAMFiltered(t *testing.T) {
	stub := &stubSAM{
		stacks: []cfntypes.Stack{
			{StackName: aws.String("plain-cfn-stack")},
		},
		transforms: []string{}, // no SAM transform
	}
	p := awspkg.NewSAMProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("want 0 SAM items (non-SAM stacks filtered), got %d", len(items))
	}
}

func TestSAMProvider_ListItems_Filter(t *testing.T) {
	stub := &stubSAM{
		stacks: []cfntypes.Stack{
			{StackName: aws.String("api-service")},
			{StackName: aws.String("worker-service")},
		},
		transforms: []string{"AWS::Serverless-2016-10-31"},
	}
	p := awspkg.NewSAMProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "api-service" {
		t.Errorf("filter expected [api-service], got %v", items)
	}
}

func TestSAMProvider_Tabs(t *testing.T) {
	stub := &stubSAM{
		stacks: []cfntypes.Stack{{StackName: aws.String("my-app")}},
		transforms: []string{"AWS::Serverless-2016-10-31"},
		resources: []cfntypes.StackResourceSummary{
			{
				LogicalResourceId:  aws.String("MyFunction"),
				PhysicalResourceId: aws.String("my-app-MyFunction-ABC"),
				ResourceType:       aws.String("AWS::Lambda::Function"),
				ResourceStatus:     cfntypes.ResourceStatusCreateComplete,
			},
		},
		changeSets: []cfntypes.ChangeSetSummary{
			{
				ChangeSetName: aws.String("deploy-v1"),
				Status:        cfntypes.ChangeSetStatusCreateComplete,
			},
		},
	}
	p := awspkg.NewSAMProviderWithClient(stub)
	item := awspkg.Item{ID: "my-app", Name: "my-app"}

	cases := []struct {
		label string
		want  string
	}{
		{"Resources", "MyFunction"},
		{"Deployments", "deploy-v1"},
	}
	tabs := p.Tabs()
	if len(tabs) != len(cases) {
		t.Fatalf("want %d tabs, got %d", len(cases), len(tabs))
	}
	for i, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			out, err := tabs[i].Fetch(context.Background(), item)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("tab %q: want %q in output, got:\n%s", tc.label, tc.want, out)
			}
		})
	}
}

func TestSAMProvider_Tabs_Empty(t *testing.T) {
	stub := &stubSAM{}
	p := awspkg.NewSAMProviderWithClient(stub)
	item := awspkg.Item{ID: "empty-app", Name: "empty-app"}
	tabs := p.Tabs()
	for _, tab := range tabs {
		out, err := tab.Fetch(context.Background(), item)
		if err != nil {
			t.Errorf("tab %q returned error: %v", tab.Label, err)
		}
		if out == "" {
			t.Errorf("tab %q returned empty string", tab.Label)
		}
	}
}
