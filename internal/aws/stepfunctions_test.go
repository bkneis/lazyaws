package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	sfntypes "github.com/aws/aws-sdk-go-v2/service/sfn/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

type stubSFN struct {
	machines   []sfntypes.StateMachineListItem
	describe   *sfn.DescribeStateMachineOutput
	executions []sfntypes.ExecutionListItem
}

func (s *stubSFN) ListStateMachines(_ context.Context, _ *sfn.ListStateMachinesInput, _ ...func(*sfn.Options)) (*sfn.ListStateMachinesOutput, error) {
	return &sfn.ListStateMachinesOutput{StateMachines: s.machines}, nil
}

func (s *stubSFN) DescribeStateMachine(_ context.Context, _ *sfn.DescribeStateMachineInput, _ ...func(*sfn.Options)) (*sfn.DescribeStateMachineOutput, error) {
	return s.describe, nil
}

func (s *stubSFN) ListExecutions(_ context.Context, _ *sfn.ListExecutionsInput, _ ...func(*sfn.Options)) (*sfn.ListExecutionsOutput, error) {
	return &sfn.ListExecutionsOutput{Executions: s.executions}, nil
}

func TestStepFunctionsProvider_ListItems(t *testing.T) {
	cases := []struct {
		name     string
		machines []sfntypes.StateMachineListItem
		query    string
		want     int
	}{
		{"all", []sfntypes.StateMachineListItem{
			{StateMachineArn: aws.String("arn:aws:states:us-east-1:123:stateMachine:Order"), Name: aws.String("Order")},
			{StateMachineArn: aws.String("arn:aws:states:us-east-1:123:stateMachine:Payment"), Name: aws.String("Payment")},
		}, "", 2},
		{"filter", []sfntypes.StateMachineListItem{
			{StateMachineArn: aws.String("arn:aws:states:us-east-1:123:stateMachine:Order"), Name: aws.String("Order")},
		}, "ord", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := awspkg.NewStepFunctionsProviderWithClient(&stubSFN{machines: tc.machines})
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

func TestStepFunctionsProvider_Tabs(t *testing.T) {
	created := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	startTime := time.Date(2024, 2, 1, 14, 1, 0, 0, time.UTC)
	stopTime := time.Date(2024, 2, 1, 14, 1, 5, 0, time.UTC)
	stub := &stubSFN{
		machines: []sfntypes.StateMachineListItem{
			{StateMachineArn: aws.String("arn:aws:states:us-east-1:123:stateMachine:Order"), Name: aws.String("Order")},
		},
		describe: &sfn.DescribeStateMachineOutput{
			StateMachineArn: aws.String("arn:aws:states:us-east-1:123:stateMachine:Order"),
			Name:            aws.String("Order"),
			Type:            sfntypes.StateMachineTypeStandard,
			Status:          sfntypes.StateMachineStatusActive,
			RoleArn:         aws.String("arn:aws:iam::123:role/sfn-role"),
			CreationDate:    &created,
			Definition:      aws.String(`{"Comment":"test","StartAt":"First","States":{"First":{"Type":"Pass","End":true}}}`),
			LoggingConfiguration: &sfntypes.LoggingConfiguration{
				Level: sfntypes.LogLevelError,
			},
		},
		executions: []sfntypes.ExecutionListItem{
			{
				ExecutionArn: aws.String("arn:aws:states:us-east-1:123:execution:Order:abc"),
				Name:         aws.String("exec-abc"),
				Status:       sfntypes.ExecutionStatusSucceeded,
				StartDate:    &startTime,
				StopDate:     &stopTime,
			},
		},
	}
	p := awspkg.NewStepFunctionsProviderWithClient(stub)
	item := awspkg.Item{ID: "arn:aws:states:us-east-1:123:stateMachine:Order", Name: "Order"}
	tabs := p.Tabs()

	cases := []struct {
		tabIdx int
		label  string
		want   string
	}{
		{0, "Overview", "STANDARD"},
		{1, "Definition", "StartAt"},
		{2, "Executions", "exec-abc"},
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
