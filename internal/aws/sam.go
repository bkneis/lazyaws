package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

// SAMAPI is the subset of CloudFormation client methods used by SAMProvider.
type SAMAPI interface {
	DescribeStacks(ctx context.Context, in *cloudformation.DescribeStacksInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error)
	GetTemplateSummary(ctx context.Context, in *cloudformation.GetTemplateSummaryInput, opts ...func(*cloudformation.Options)) (*cloudformation.GetTemplateSummaryOutput, error)
	ListStackResources(ctx context.Context, in *cloudformation.ListStackResourcesInput, opts ...func(*cloudformation.Options)) (*cloudformation.ListStackResourcesOutput, error)
	ListChangeSets(ctx context.Context, in *cloudformation.ListChangeSetsInput, opts ...func(*cloudformation.Options)) (*cloudformation.ListChangeSetsOutput, error)
}

// SAMProvider implements Provider for AWS SAM Applications (Serverless stacks).
type SAMProvider struct {
	client SAMAPI
}

func NewSAMProvider(cfg awssdk.Config, local bool) *SAMProvider {
	var opts []func(*cloudformation.Options)
	if local {
		opts = append(opts, func(o *cloudformation.Options) {
			o.BaseEndpoint = awssdk.String("http://localhost:4566")
		})
	}
	return &SAMProvider{client: cloudformation.NewFromConfig(cfg, opts...)}
}

func NewSAMProviderWithClient(client SAMAPI) *SAMProvider {
	return &SAMProvider{client: client}
}

func (p *SAMProvider) Name() string { return "SAM Applications" }

func (p *SAMProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{})
	if err != nil {
		return nil, fmt.Errorf("describe stacks: %w", err)
	}

	var items []Item
	for _, s := range out.Stacks {
		name := awssdk.ToString(s.StackName)
		summary, err := p.client.GetTemplateSummary(ctx, &cloudformation.GetTemplateSummaryInput{
			StackName: awssdk.String(name),
		})
		if err != nil {
			continue
		}
		if !isSAMStack(summary.DeclaredTransforms) {
			continue
		}
		items = append(items, Item{ID: name, Name: name})
	}
	return filterItems(items, query), nil
}

func (p *SAMProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabResources(ctx, item)
}

func (p *SAMProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Resources", Fetch: p.tabResources},
		{Label: "Deployments", Fetch: p.tabDeployments},
	}
}

func (p *SAMProvider) tabResources(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListStackResources(ctx, &cloudformation.ListStackResourcesInput{
		StackName: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	if len(out.StackResourceSummaries) == 0 {
		return "  (no resources)\n", nil
	}
	rows := make([][]string, len(out.StackResourceSummaries))
	for i, r := range out.StackResourceSummaries {
		logicalID := awssdk.ToString(r.LogicalResourceId)
		physicalID := awssdk.ToString(r.PhysicalResourceId)
		resourceType := awssdk.ToString(r.ResourceType)
		displayID := logicalID
		if provider, ok := cfnTypeToProvider[resourceType]; ok {
			displayID = Link(logicalID, provider, physicalID)
		}
		rows[i] = []string{displayID, resourceType, string(r.ResourceStatus)}
	}
	return Table([]string{"Logical ID", "Type", "Status"}, rows), nil
}

func (p *SAMProvider) tabDeployments(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListChangeSets(ctx, &cloudformation.ListChangeSetsInput{
		StackName: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	if len(out.Summaries) == 0 {
		return "  (no deployments)\n", nil
	}
	rows := make([][]string, len(out.Summaries))
	for i, cs := range out.Summaries {
		created := ""
		if cs.CreationTime != nil {
			created = cs.CreationTime.Format(time.DateTime)
		}
		rows[i] = []string{
			awssdk.ToString(cs.ChangeSetName),
			string(cs.Status),
			created,
		}
	}
	return Table([]string{"Name", "Status", "Created"}, rows), nil
}

// isSAMStack returns true if transforms contains the SAM transform.
func isSAMStack(transforms []string) bool {
	for _, t := range transforms {
		if strings.Contains(t, "AWS::Serverless") {
			return true
		}
	}
	return false
}
