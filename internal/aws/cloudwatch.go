package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// CloudWatchAPI is the subset of the CloudWatch client methods used by CloudWatchProvider.
type CloudWatchAPI interface {
	DescribeAlarms(ctx context.Context, in *cloudwatch.DescribeAlarmsInput, opts ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error)
	DescribeAlarmHistory(ctx context.Context, in *cloudwatch.DescribeAlarmHistoryInput, opts ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmHistoryOutput, error)
}

// CloudWatchProvider implements Provider for Amazon CloudWatch Alarms.
type CloudWatchProvider struct {
	client CloudWatchAPI
}

func NewCloudWatchProvider(cfg awssdk.Config, endpointURL string) *CloudWatchProvider {
	var opts []func(*cloudwatch.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *cloudwatch.Options) {
			o.BaseEndpoint = awssdk.String(endpointURL)
		})
	}
	return &CloudWatchProvider{client: cloudwatch.NewFromConfig(cfg, opts...)}
}

func NewCloudWatchProviderWithClient(client CloudWatchAPI) *CloudWatchProvider {
	return &CloudWatchProvider{client: client}
}

func (p *CloudWatchProvider) Name() string { return "CloudWatch" }

func (p *CloudWatchProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	var items []Item
	var nextToken *string
	for {
		out, err := p.client.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe alarms: %w", err)
		}
		for _, a := range out.MetricAlarms {
			name := awssdk.ToString(a.AlarmName)
			items = append(items, Item{
				ID:   name,
				Name: name,
				Meta: map[string]string{"state": string(a.StateValue)},
			})
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return filterItems(items, query), nil
}

func (p *CloudWatchProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *CloudWatchProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "History", Fetch: p.tabHistory},
	}
}

func (p *CloudWatchProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{
		AlarmNames: []string{item.ID},
	})
	if err != nil {
		return "", err
	}
	if len(out.MetricAlarms) == 0 {
		return "  (alarm not found)\n", nil
	}
	a := out.MetricAlarms[0]

	var dims []string
	for _, d := range a.Dimensions {
		dims = append(dims, awssdk.ToString(d.Name)+" = "+awssdk.ToString(d.Value))
	}

	var alarmActions []string
	for _, arn := range a.AlarmActions {
		alarmActions = append(alarmActions, Link(arnLastSegment(arn), "SNS", arn))
	}
	actionsStr := strings.Join(alarmActions, ", ")
	if actionsStr == "" {
		actionsStr = "-"
	}

	return KV([][2]string{
		{"Name", awssdk.ToString(a.AlarmName)},
		{"State", string(a.StateValue)},
		{"State Reason", awssdk.ToString(a.StateReason)},
		{"Namespace", awssdk.ToString(a.Namespace)},
		{"Metric", awssdk.ToString(a.MetricName)},
		{"Dimensions", strings.Join(dims, ", ")},
		{"Threshold", fmt.Sprintf("%.0f", awssdk.ToFloat64(a.Threshold))},
		{"Comparison", string(a.ComparisonOperator)},
		{"Period", fmt.Sprintf("%ds", awssdk.ToInt32(a.Period))},
		{"Eval Periods", fmt.Sprintf("%d", awssdk.ToInt32(a.EvaluationPeriods))},
		{"Actions (ALARM)", actionsStr},
	}), nil
}

func (p *CloudWatchProvider) tabHistory(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeAlarmHistory(ctx, &cloudwatch.DescribeAlarmHistoryInput{
		AlarmName:  awssdk.String(item.ID),
		MaxRecords: awssdk.Int32(20),
		ScanBy:     cwtypes.ScanByTimestampDescending,
	})
	if err != nil {
		return "", err
	}
	if len(out.AlarmHistoryItems) == 0 {
		return "  (no history found)\n", nil
	}
	rows := make([][]string, len(out.AlarmHistoryItems))
	for i, h := range out.AlarmHistoryItems {
		ts := ""
		if h.Timestamp != nil {
			ts = h.Timestamp.Format(time.DateTime)
		}
		rows[i] = []string{ts, string(h.HistoryItemType), awssdk.ToString(h.HistorySummary)}
	}
	return Table([]string{"Time", "Type", "Summary"}, rows), nil
}
