package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
)

// EventBridgeAPI is the subset of the EventBridge client methods used by EventBridgeProvider.
type EventBridgeAPI interface {
	ListRules(ctx context.Context, in *eventbridge.ListRulesInput, opts ...func(*eventbridge.Options)) (*eventbridge.ListRulesOutput, error)
	DescribeRule(ctx context.Context, in *eventbridge.DescribeRuleInput, opts ...func(*eventbridge.Options)) (*eventbridge.DescribeRuleOutput, error)
	ListTargetsByRule(ctx context.Context, in *eventbridge.ListTargetsByRuleInput, opts ...func(*eventbridge.Options)) (*eventbridge.ListTargetsByRuleOutput, error)
}

// EventBridgeProvider implements Provider for Amazon EventBridge.
type EventBridgeProvider struct {
	client EventBridgeAPI
}

func NewEventBridgeProvider(cfg awssdk.Config, local bool) *EventBridgeProvider {
	var opts []func(*eventbridge.Options)
	if local {
		opts = append(opts, func(o *eventbridge.Options) {
			o.BaseEndpoint = awssdk.String("http://localhost:4566")
		})
	}
	return &EventBridgeProvider{client: eventbridge.NewFromConfig(cfg, opts...)}
}

func NewEventBridgeProviderWithClient(client EventBridgeAPI) *EventBridgeProvider {
	return &EventBridgeProvider{client: client}
}

func (p *EventBridgeProvider) Name() string { return "EventBridge" }

func (p *EventBridgeProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	var items []Item
	var nextToken *string
	for {
		out, err := p.client.ListRules(ctx, &eventbridge.ListRulesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list rules: %w", err)
		}
		for _, r := range out.Rules {
			name := awssdk.ToString(r.Name)
			bus := awssdk.ToString(r.EventBusName)
			items = append(items, Item{
				ID:   name,
				Name: name,
				Meta: map[string]string{"bus": bus},
			})
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return filterItems(items, query), nil
}

func (p *EventBridgeProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *EventBridgeProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Targets", Fetch: p.tabTargets},
	}
}

func (p *EventBridgeProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	bus := item.Meta["bus"]
	in := &eventbridge.DescribeRuleInput{Name: awssdk.String(item.ID)}
	if bus != "" {
		in.EventBusName = awssdk.String(bus)
	}
	out, err := p.client.DescribeRule(ctx, in)
	if err != nil {
		return "", err
	}

	schedule := awssdk.ToString(out.ScheduleExpression)
	if schedule == "" {
		schedule = "-"
	}
	pattern := awssdk.ToString(out.EventPattern)
	if pattern == "" {
		pattern = "(none)"
	}

	roleARN := awssdk.ToString(out.RoleArn)
	roleDisplay := "-"
	if roleARN != "" {
		roleDisplay = Link(arnLastSegment(roleARN), "IAM Roles", roleARN)
	}

	return KV([][2]string{
		{"Name", awssdk.ToString(out.Name)},
		{"Bus", awssdk.ToString(out.EventBusName)},
		{"State", string(out.State)},
		{"Description", awssdk.ToString(out.Description)},
		{"Schedule", schedule},
		{"Event Pattern", pattern},
		{"Role", roleDisplay},
	}), nil
}

func (p *EventBridgeProvider) tabTargets(ctx context.Context, item Item) (string, error) {
	bus := item.Meta["bus"]
	in := &eventbridge.ListTargetsByRuleInput{Rule: awssdk.String(item.ID)}
	if bus != "" {
		in.EventBusName = awssdk.String(bus)
	}
	out, err := p.client.ListTargetsByRule(ctx, in)
	if err != nil {
		return "", err
	}
	if len(out.Targets) == 0 {
		return "  (no targets)\n", nil
	}
	rows := make([][]string, len(out.Targets))
	for i, t := range out.Targets {
		input := awssdk.ToString(t.Input)
		if input == "" {
			input = "(matched)"
		}
		rows[i] = []string{awssdk.ToString(t.Id), awssdk.ToString(t.Arn), input}
	}
	return Table([]string{"ID", "ARN", "Input"}, rows), nil
}
