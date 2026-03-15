package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
)

// LambdaAPI is the subset of the Lambda client methods used by LambdaProvider.
type LambdaAPI interface {
	ListFunctions(ctx context.Context, in *lambda.ListFunctionsInput, opts ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error)
	GetFunction(ctx context.Context, in *lambda.GetFunctionInput, opts ...func(*lambda.Options)) (*lambda.GetFunctionOutput, error)
	GetFunctionConfiguration(ctx context.Context, in *lambda.GetFunctionConfigurationInput, opts ...func(*lambda.Options)) (*lambda.GetFunctionConfigurationOutput, error)
	ListEventSourceMappings(ctx context.Context, in *lambda.ListEventSourceMappingsInput, opts ...func(*lambda.Options)) (*lambda.ListEventSourceMappingsOutput, error)
}

// LambdaProvider implements Provider for AWS Lambda.
type LambdaProvider struct{ client LambdaAPI }

func NewLambdaProvider(cfg awssdk.Config, local bool) *LambdaProvider {
	var opts []func(*lambda.Options)
	if local {
		opts = append(opts, func(o *lambda.Options) {
			o.BaseEndpoint = awssdk.String("http://localhost:4566")
		})
	}
	return &LambdaProvider{client: lambda.NewFromConfig(cfg, opts...)}
}

func NewLambdaProviderWithClient(client LambdaAPI) *LambdaProvider {
	return &LambdaProvider{client: client}
}

func (p *LambdaProvider) Name() string { return "Lambda" }

func (p *LambdaProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.ListFunctions(ctx, &lambda.ListFunctionsInput{})
	if err != nil {
		return nil, fmt.Errorf("list functions: %w", err)
	}
	items := make([]Item, len(out.Functions))
	for i, f := range out.Functions {
		name := awssdk.ToString(f.FunctionName)
		items[i] = Item{ID: name, Name: name}
	}
	return filterItems(items, query), nil
}

func (p *LambdaProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *LambdaProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Env", Fetch: p.tabEnv},
		{Label: "Triggers", Fetch: p.tabTriggers},
	}
}

func (p *LambdaProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: awssdk.String(item.ID),
	})
	if err != nil {
		return "", fmt.Errorf("get function: %w", err)
	}
	if out.Configuration == nil {
		return "", fmt.Errorf("configuration not available for %s", item.ID)
	}
	cfg := out.Configuration

	memory := fmt.Sprintf("%d MB", awssdk.ToInt32(cfg.MemorySize))
	timeout := fmt.Sprintf("%ds", awssdk.ToInt32(cfg.Timeout))
	codeSize := formatSize(cfg.CodeSize)
	lastMod := awssdk.ToString(cfg.LastModified)
	if t, err := time.Parse(time.RFC3339, lastMod); err == nil {
		lastMod = t.Format("2006-01-02 15:04")
	}

	pairs := [][2]string{
		{"Runtime", string(cfg.Runtime)},
		{"Memory", memory},
		{"Timeout", timeout},
		{"Handler", awssdk.ToString(cfg.Handler)},
		{"Code Size", codeSize},
		{"Last Mod", lastMod},
		{"Role", awssdk.ToString(cfg.Role)},
	}
	if cfg.Description != nil && awssdk.ToString(cfg.Description) != "" {
		pairs = append(pairs, [2]string{"Description", awssdk.ToString(cfg.Description)})
	}
	return KV(pairs), nil
}

func (p *LambdaProvider) tabEnv(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetFunctionConfiguration(ctx, &lambda.GetFunctionConfigurationInput{
		FunctionName: awssdk.String(item.ID),
	})
	if err != nil {
		return "", fmt.Errorf("get function configuration: %w", err)
	}
	if out.Environment == nil || len(out.Environment.Variables) == 0 {
		return "  (no environment variables)\n", nil
	}
	pairs := make([][2]string, 0, len(out.Environment.Variables))
	for k, v := range out.Environment.Variables {
		pairs = append(pairs, [2]string{k, v})
	}
	return KV(pairs), nil
}

func (p *LambdaProvider) tabTriggers(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListEventSourceMappings(ctx, &lambda.ListEventSourceMappingsInput{
		FunctionName: awssdk.String(item.ID),
	})
	if err != nil {
		return "", fmt.Errorf("list event source mappings: %w", err)
	}
	if len(out.EventSourceMappings) == 0 {
		return "  (no triggers)\n", nil
	}
	rows := make([][]string, len(out.EventSourceMappings))
	for i, m := range out.EventSourceMappings {
		sourceType := triggerType(awssdk.ToString(m.EventSourceArn))
		rows[i] = []string{sourceType, awssdk.ToString(m.EventSourceArn), awssdk.ToString(m.State)}
	}
	return Table([]string{"Type", "Source ARN", "State"}, rows), nil
}

// triggerType derives a short service name from an event source ARN.
func triggerType(arn string) string {
	switch {
	case strings.Contains(arn, ":sqs:"):
		return "SQS"
	case strings.Contains(arn, ":sns:"):
		return "SNS"
	case strings.Contains(arn, ":dynamodb:"):
		return "DynamoDB"
	case strings.Contains(arn, ":kinesis:"):
		return "Kinesis"
	case strings.Contains(arn, ":events:"):
		return "EventBridge"
	case strings.Contains(arn, ":kafka:"):
		return "MSK"
	case strings.Contains(arn, ":s3:"):
		return "S3"
	default:
		return "Unknown"
	}
}
