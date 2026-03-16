package aws

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

// cfnTypeToProvider maps CloudFormation resource types to lazyaws provider names.
// PhysicalResourceId matches Item.ID directly for each provider.
var cfnTypeToProvider = map[string]string{
	"AWS::Lambda::Function":       "Lambda",
	"AWS::SQS::Queue":             "SQS",
	"AWS::SNS::Topic":             "SNS",
	"AWS::S3::Bucket":             "S3",
	"AWS::ApiGateway::RestApi":    "API Gateway",
	"AWS::ApiGatewayV2::Api":      "API Gateway",
	"AWS::IAM::Role":              "IAM Roles",
	"AWS::SecretsManager::Secret": "Secrets Manager",
}

type CFAPI interface {
	DescribeStacks(ctx context.Context, in *cloudformation.DescribeStacksInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error)
	ListStackResources(ctx context.Context, in *cloudformation.ListStackResourcesInput, opts ...func(*cloudformation.Options)) (*cloudformation.ListStackResourcesOutput, error)
}

type CFProvider struct{ client CFAPI }

func NewCloudFormationProvider(cfg awssdk.Config, local bool) *CFProvider {
	var opts []func(*cloudformation.Options)
	if local {
		opts = append(opts, func(o *cloudformation.Options) { o.BaseEndpoint = awssdk.String("http://localhost:4566") })
	}
	return &CFProvider{client: cloudformation.NewFromConfig(cfg, opts...)}
}

func NewCFProviderWithClient(client CFAPI) *CFProvider { return &CFProvider{client: client} }

func (p *CFProvider) Name() string { return "CloudFormation" }

func (p *CFProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{})
	if err != nil {
		return nil, fmt.Errorf("describe stacks: %w", err)
	}
	items := make([]Item, len(out.Stacks))
	for i, s := range out.Stacks {
		name := awssdk.ToString(s.StackName)
		items[i] = Item{ID: name, Name: name}
	}
	return filterItems(items, query), nil
}

func (p *CFProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *CFProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Resources", Fetch: p.tabResources},
		{Label: "Outputs", Fetch: p.tabOutputs},
		{Label: "Parameters", Fetch: p.tabParameters},
	}
}

func (p *CFProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	if len(out.Stacks) == 0 {
		return "  (stack not found)\n", nil
	}
	s := out.Stacks[0]
	created := ""
	if s.CreationTime != nil {
		created = s.CreationTime.Format(time.DateTime)
	}
	updated := ""
	if s.LastUpdatedTime != nil {
		updated = s.LastUpdatedTime.Format(time.DateTime)
	}
	return KV([][2]string{
		{"Name", awssdk.ToString(s.StackName)},
		{"Status", string(s.StackStatus)},
		{"Created", created},
		{"Last Updated", updated},
		{"Description", awssdk.ToString(s.Description)},
	}), nil
}

func (p *CFProvider) tabResources(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListStackResources(ctx, &cloudformation.ListStackResourcesInput{StackName: awssdk.String(item.ID)})
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
			targetID := physicalID
			displayID = Link(logicalID, provider, targetID)
		}
		rows[i] = []string{displayID, resourceType, string(r.ResourceStatus)}
	}
	return Table([]string{"Logical ID", "Type", "Status"}, rows), nil
}

func (p *CFProvider) tabOutputs(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	if len(out.Stacks) == 0 || len(out.Stacks[0].Outputs) == 0 {
		return "  (no outputs)\n", nil
	}
	pairs := make([][2]string, len(out.Stacks[0].Outputs))
	for i, o := range out.Stacks[0].Outputs {
		pairs[i] = [2]string{awssdk.ToString(o.OutputKey), awssdk.ToString(o.OutputValue)}
	}
	return KV(pairs), nil
}

func (p *CFProvider) tabParameters(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	if len(out.Stacks) == 0 || len(out.Stacks[0].Parameters) == 0 {
		return "  (no parameters)\n", nil
	}
	pairs := make([][2]string, len(out.Stacks[0].Parameters))
	for i, param := range out.Stacks[0].Parameters {
		pairs[i] = [2]string{awssdk.ToString(param.ParameterKey), awssdk.ToString(param.ParameterValue)}
	}
	return KV(pairs), nil
}
