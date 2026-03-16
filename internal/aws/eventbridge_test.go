package aws_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	ebtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

type stubEventBridge struct {
	rules   []ebtypes.Rule
	describe *eventbridge.DescribeRuleOutput
	targets []ebtypes.Target
}

func (s *stubEventBridge) ListRules(_ context.Context, _ *eventbridge.ListRulesInput, _ ...func(*eventbridge.Options)) (*eventbridge.ListRulesOutput, error) {
	return &eventbridge.ListRulesOutput{Rules: s.rules}, nil
}

func (s *stubEventBridge) DescribeRule(_ context.Context, _ *eventbridge.DescribeRuleInput, _ ...func(*eventbridge.Options)) (*eventbridge.DescribeRuleOutput, error) {
	return s.describe, nil
}

func (s *stubEventBridge) ListTargetsByRule(_ context.Context, _ *eventbridge.ListTargetsByRuleInput, _ ...func(*eventbridge.Options)) (*eventbridge.ListTargetsByRuleOutput, error) {
	return &eventbridge.ListTargetsByRuleOutput{Targets: s.targets}, nil
}

func TestEventBridgeProvider_ListItems(t *testing.T) {
	cases := []struct {
		name  string
		rules []ebtypes.Rule
		query string
		want  int
	}{
		{"all", []ebtypes.Rule{
			{Name: aws.String("daily-report"), EventBusName: aws.String("default")},
			{Name: aws.String("hourly-sync"), EventBusName: aws.String("default")},
		}, "", 2},
		{"filter", []ebtypes.Rule{
			{Name: aws.String("daily-report"), EventBusName: aws.String("default")},
		}, "daily", 1},
		{"no match", []ebtypes.Rule{{Name: aws.String("daily-report"), EventBusName: aws.String("default")}}, "xyz", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := awspkg.NewEventBridgeProviderWithClient(&stubEventBridge{rules: tc.rules})
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

func TestEventBridgeProvider_Tabs(t *testing.T) {
	stub := &stubEventBridge{
		rules: []ebtypes.Rule{{Name: aws.String("daily-report"), EventBusName: aws.String("default")}},
		describe: &eventbridge.DescribeRuleOutput{
			Name:               aws.String("daily-report"),
			EventBusName:       aws.String("default"),
			State:              ebtypes.RuleStateEnabled,
			Description:        aws.String("Triggers daily report"),
			ScheduleExpression: aws.String("cron(0 9 * * ? *)"),
		},
		targets: []ebtypes.Target{
			{Id: aws.String("target-1"), Arn: aws.String("arn:aws:lambda::fn/report")},
		},
	}
	p := awspkg.NewEventBridgeProviderWithClient(stub)
	item := awspkg.Item{ID: "daily-report", Name: "daily-report", Meta: map[string]string{"bus": "default"}}
	tabs := p.Tabs()

	cases := []struct {
		tabIdx int
		label  string
		want   string
	}{
		{0, "Overview", "cron(0 9 * * ? *)"},
		{1, "Targets", "target-1"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if tabs[tc.tabIdx].Label != tc.label {
				t.Errorf("tab %d label = %q, want %q", tc.tabIdx, tabs[tc.tabIdx].Label, tc.label)
			}
			content, err := tabs[tc.tabIdx].Fetch(context.Background(), item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(content, tc.want) {
				t.Errorf("tab %q missing %q\ngot:\n%s", tc.label, tc.want, content)
			}
		})
	}
}
