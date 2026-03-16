package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
)

// SFNAPI is the subset of the Step Functions client methods used by StepFunctionsProvider.
type SFNAPI interface {
	ListStateMachines(ctx context.Context, in *sfn.ListStateMachinesInput, opts ...func(*sfn.Options)) (*sfn.ListStateMachinesOutput, error)
	DescribeStateMachine(ctx context.Context, in *sfn.DescribeStateMachineInput, opts ...func(*sfn.Options)) (*sfn.DescribeStateMachineOutput, error)
	ListExecutions(ctx context.Context, in *sfn.ListExecutionsInput, opts ...func(*sfn.Options)) (*sfn.ListExecutionsOutput, error)
}

// StepFunctionsProvider implements Provider for AWS Step Functions.
type StepFunctionsProvider struct {
	client SFNAPI
}

func NewStepFunctionsProvider(cfg awssdk.Config, local bool) *StepFunctionsProvider {
	var opts []func(*sfn.Options)
	if local {
		opts = append(opts, func(o *sfn.Options) {
			o.BaseEndpoint = awssdk.String("http://localhost:4566")
		})
	}
	return &StepFunctionsProvider{client: sfn.NewFromConfig(cfg, opts...)}
}

func NewStepFunctionsProviderWithClient(client SFNAPI) *StepFunctionsProvider {
	return &StepFunctionsProvider{client: client}
}

func (p *StepFunctionsProvider) Name() string { return "Step Functions" }

func (p *StepFunctionsProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	var items []Item
	var nextToken *string
	for {
		out, err := p.client.ListStateMachines(ctx, &sfn.ListStateMachinesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list state machines: %w", err)
		}
		for _, sm := range out.StateMachines {
			arn := awssdk.ToString(sm.StateMachineArn)
			name := awssdk.ToString(sm.Name)
			items = append(items, Item{ID: arn, Name: name})
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return filterItems(items, query), nil
}

func (p *StepFunctionsProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *StepFunctionsProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Definition", Fetch: p.tabDefinition},
		{Label: "Executions", Fetch: p.tabExecutions},
	}
}

func (p *StepFunctionsProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeStateMachine(ctx, &sfn.DescribeStateMachineInput{
		StateMachineArn: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}

	created := ""
	if out.CreationDate != nil {
		created = out.CreationDate.Format(time.DateTime)
	}

	loggingLevel := "OFF"
	loggingDest := "-"
	if out.LoggingConfiguration != nil {
		loggingLevel = string(out.LoggingConfiguration.Level)
		if len(out.LoggingConfiguration.Destinations) > 0 && out.LoggingConfiguration.Destinations[0].CloudWatchLogsLogGroup != nil {
			loggingDest = awssdk.ToString(out.LoggingConfiguration.Destinations[0].CloudWatchLogsLogGroup.LogGroupArn)
		}
	}

	roleARN := awssdk.ToString(out.RoleArn)
	roleDisplay := Link(arnLastSegment(roleARN), "IAM Roles", roleARN)

	return KV([][2]string{
		{"Name", awssdk.ToString(out.Name)},
		{"Type", string(out.Type)},
		{"Status", string(out.Status)},
		{"Role", roleDisplay},
		{"Created", created},
		{"Logging Level", loggingLevel},
		{"Logging Dest", loggingDest},
	}), nil
}

func (p *StepFunctionsProvider) tabDefinition(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeStateMachine(ctx, &sfn.DescribeStateMachineInput{
		StateMachineArn: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	raw := awssdk.ToString(out.Definition)
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return "  " + raw + "\n", nil
	}
	b, _ := json.MarshalIndent(v, "  ", "  ")
	return "  " + string(b) + "\n", nil
}

func (p *StepFunctionsProvider) tabExecutions(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListExecutions(ctx, &sfn.ListExecutionsInput{
		StateMachineArn: awssdk.String(item.ID),
		MaxResults: 20,
	})
	if err != nil {
		return "", err
	}
	if len(out.Executions) == 0 {
		return "  (no executions found)\n", nil
	}
	rows := make([][]string, len(out.Executions))
	for i, e := range out.Executions {
		started := ""
		if e.StartDate != nil {
			started = e.StartDate.Format(time.TimeOnly)
		}
		stopped := "-"
		if e.StopDate != nil {
			stopped = e.StopDate.Format(time.TimeOnly)
		}
		rows[i] = []string{awssdk.ToString(e.Name), string(e.Status), started, stopped}
	}
	return Table([]string{"Name", "Status", "Started", "Stopped"}, rows), nil
}
