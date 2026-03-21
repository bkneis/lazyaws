package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

type stubCloudWatch struct {
	alarms  []cwtypes.MetricAlarm
	history []cwtypes.AlarmHistoryItem
}

func (s *stubCloudWatch) DescribeAlarms(_ context.Context, _ *cloudwatch.DescribeAlarmsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error) {
	return &cloudwatch.DescribeAlarmsOutput{MetricAlarms: s.alarms}, nil
}

func (s *stubCloudWatch) DescribeAlarmHistory(_ context.Context, _ *cloudwatch.DescribeAlarmHistoryInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmHistoryOutput, error) {
	return &cloudwatch.DescribeAlarmHistoryOutput{AlarmHistoryItems: s.history}, nil
}

func TestCloudWatchProvider_ListItems(t *testing.T) {
	cases := []struct {
		name   string
		alarms []cwtypes.MetricAlarm
		query  string
		want   int
	}{
		{"all", []cwtypes.MetricAlarm{
			{AlarmName: aws.String("HighCPU"), StateValue: cwtypes.StateValueAlarm},
			{AlarmName: aws.String("LowMem"), StateValue: cwtypes.StateValueOk},
		}, "", 2},
		{"filter", []cwtypes.MetricAlarm{
			{AlarmName: aws.String("HighCPU"), StateValue: cwtypes.StateValueAlarm},
		}, "cpu", 1},
		{"no match", []cwtypes.MetricAlarm{{AlarmName: aws.String("HighCPU"), StateValue: cwtypes.StateValueAlarm}}, "xyz", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := awspkg.NewCloudWatchProviderWithClient(&stubCloudWatch{alarms: tc.alarms})
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

func TestCloudWatchProvider_Tabs(t *testing.T) {
	threshold := float64(80)
	period := int32(300)
	evalPeriods := int32(2)
	ts := time.Date(2024, 2, 1, 9, 0, 0, 0, time.UTC)
	stub := &stubCloudWatch{
		alarms: []cwtypes.MetricAlarm{
			{
				AlarmName:          aws.String("HighCPU"),
				StateValue:         cwtypes.StateValueAlarm,
				StateReason:        aws.String("Threshold Crossed"),
				Namespace:          aws.String("AWS/EC2"),
				MetricName:         aws.String("CPUUtilization"),
				Threshold:          &threshold,
				ComparisonOperator: cwtypes.ComparisonOperatorGreaterThanThreshold,
				Period:             &period,
				EvaluationPeriods:  &evalPeriods,
				AlarmActions:       []string{"arn:aws:sns:us-east-1:123:my-alerts"},
			},
		},
		history: []cwtypes.AlarmHistoryItem{
			{
				Timestamp:       &ts,
				HistoryItemType: cwtypes.HistoryItemTypeStateUpdate,
				HistorySummary:  aws.String("OK -> ALARM"),
			},
		},
	}
	p := awspkg.NewCloudWatchProviderWithClient(stub)
	item := awspkg.Item{ID: "HighCPU", Name: "HighCPU"}
	tabs := p.Tabs()

	cases := []struct {
		tabIdx int
		label  string
		want   string
	}{
		{0, "Overview", "CPUUtilization"},
		{1, "History", "StateUpdate"},
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
