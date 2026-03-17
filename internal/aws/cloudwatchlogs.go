package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// CloudWatchLogsAPI is the subset of the CloudWatch Logs client methods used by CloudWatchLogsProvider.
type CloudWatchLogsAPI interface {
	DescribeLogGroups(ctx context.Context, in *cloudwatchlogs.DescribeLogGroupsInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error)
	DescribeLogStreams(ctx context.Context, in *cloudwatchlogs.DescribeLogStreamsInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error)
	GetLogEvents(ctx context.Context, in *cloudwatchlogs.GetLogEventsInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.GetLogEventsOutput, error)
	// OpenLiveTailStream starts a live tail session and returns the event stream reader.
	// It abstracts StartLiveTailOutput.GetStream() so the interface is directly testable.
	OpenLiveTailStream(ctx context.Context, group string) (cloudwatchlogs.StartLiveTailResponseStreamReader, error)
}

// CWLogStreamRow holds the displayable columns for a single log stream.
// StoredBytes is intentionally omitted — AWS deprecated it for streams in 2019 (always 0).
type CWLogStreamRow struct {
	Name      string // full stream name (used for tail filter)
	LastEvent string
}

// cwlClientAdapter wraps the real cloudwatchlogs.Client to satisfy CloudWatchLogsAPI.
type cwlClientAdapter struct {
	client *cloudwatchlogs.Client
}

func (a *cwlClientAdapter) DescribeLogGroups(ctx context.Context, in *cloudwatchlogs.DescribeLogGroupsInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	return a.client.DescribeLogGroups(ctx, in, opts...)
}

func (a *cwlClientAdapter) DescribeLogStreams(ctx context.Context, in *cloudwatchlogs.DescribeLogStreamsInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	return a.client.DescribeLogStreams(ctx, in, opts...)
}

func (a *cwlClientAdapter) GetLogEvents(ctx context.Context, in *cloudwatchlogs.GetLogEventsInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.GetLogEventsOutput, error) {
	return a.client.GetLogEvents(ctx, in, opts...)
}

func (a *cwlClientAdapter) OpenLiveTailStream(ctx context.Context, group string) (cloudwatchlogs.StartLiveTailResponseStreamReader, error) {
	out, err := a.client.StartLiveTail(ctx, &cloudwatchlogs.StartLiveTailInput{
		LogGroupIdentifiers: []string{group},
	})
	if err != nil {
		return nil, fmt.Errorf("start live tail: %w", err)
	}
	return out.GetStream(), nil
}

// CloudWatchLogsProvider implements Provider for Amazon CloudWatch Logs log groups.
type CloudWatchLogsProvider struct {
	client     CloudWatchLogsAPI
	lastStreams []CWLogStreamRow // cached by tabStreams for row-selection in the App
}

func NewCloudWatchLogsProvider(cfg awssdk.Config, endpointURL string) *CloudWatchLogsProvider {
	var opts []func(*cloudwatchlogs.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *cloudwatchlogs.Options) {
			o.BaseEndpoint = awssdk.String(endpointURL)
		})
	}
	return &CloudWatchLogsProvider{client: &cwlClientAdapter{client: cloudwatchlogs.NewFromConfig(cfg, opts...)}}
}

func NewCloudWatchLogsProviderWithClient(client CloudWatchLogsAPI) *CloudWatchLogsProvider {
	return &CloudWatchLogsProvider{client: client}
}

func (p *CloudWatchLogsProvider) Name() string { return "CW Logs" }

func (p *CloudWatchLogsProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	var items []Item
	var nextToken *string
	for {
		out, err := p.client.DescribeLogGroups(ctx, &cloudwatchlogs.DescribeLogGroupsInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe log groups: %w", err)
		}
		for _, g := range out.LogGroups {
			name := awssdk.ToString(g.LogGroupName)
			retention := "Never expire"
			if g.RetentionInDays != nil {
				retention = fmt.Sprintf("%d days", *g.RetentionInDays)
			}
			bytes := "-"
			if g.StoredBytes != nil {
				bytes = FormatSize(*g.StoredBytes)
			}
			items = append(items, Item{
				ID:   name,
				Name: name,
				Meta: map[string]string{
					"retention": retention,
					"bytes":     bytes,
				},
			})
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return filterItems(items, query), nil
}

func (p *CloudWatchLogsProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *CloudWatchLogsProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Streams", Fetch: p.tabStreams},
		{Label: "Tail", Fetch: p.tabTail},
	}
}

func (p *CloudWatchLogsProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeLogGroups(ctx, &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	// Find exact match first, fall back to first result.
	var g *cwltypes.LogGroup
	for i := range out.LogGroups {
		if awssdk.ToString(out.LogGroups[i].LogGroupName) == item.ID {
			g = &out.LogGroups[i]
			break
		}
	}
	if g == nil && len(out.LogGroups) > 0 {
		g = &out.LogGroups[0]
	}
	if g == nil {
		return "  (log group not found)\n", nil
	}

	retention := "Never expire"
	if g.RetentionInDays != nil {
		retention = fmt.Sprintf("%d days", *g.RetentionInDays)
	}
	bytes := "-"
	if g.StoredBytes != nil {
		bytes = FormatSize(*g.StoredBytes)
	}
	created := "-"
	if g.CreationTime != nil {
		created = time.UnixMilli(*g.CreationTime).UTC().Format(time.RFC3339)
	}
	kmsKey := "-"
	if g.KmsKeyId != nil {
		kmsKey = awssdk.ToString(g.KmsKeyId)
	}
	logClass := "-"
	if g.LogGroupClass != "" {
		logClass = string(g.LogGroupClass)
	}
	return KV([][2]string{
		{"Name", awssdk.ToString(g.LogGroupName)},
		{"Retention", retention},
		{"Stored Bytes", bytes},
		{"Created", created},
		{"KMS Key", kmsKey},
		{"Log Class", logClass},
	}), nil
}

// tabStreams fetches log streams and caches them in lastStreams for row selection.
func (p *CloudWatchLogsProvider) tabStreams(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: awssdk.String(item.ID),
		OrderBy:      cwltypes.OrderByLastEventTime,
		Descending:   awssdk.Bool(true),
		Limit:        awssdk.Int32(25),
	})
	if err != nil {
		return "", err
	}
	if len(out.LogStreams) == 0 {
		p.lastStreams = nil
		return "  (no streams found)\n", nil
	}

	p.lastStreams = make([]CWLogStreamRow, len(out.LogStreams))
	rows := make([][]string, len(out.LogStreams))
	for i, s := range out.LogStreams {
		fullName := awssdk.ToString(s.LogStreamName)
		displayName := fullName
		if len(displayName) > 60 {
			displayName = displayName[:57] + "..."
		}
		lastEvent := "-"
		if s.LastEventTimestamp != nil {
			lastEvent = time.UnixMilli(*s.LastEventTimestamp).UTC().Format(time.DateTime)
		}
		p.lastStreams[i] = CWLogStreamRow{Name: fullName, LastEvent: lastEvent}
		rows[i] = []string{displayName, lastEvent}
	}
	return Table([]string{"Stream Name", "Last Event"}, rows), nil
}

// GetLastStreams returns the streams cached by the most recent tabStreams call.
func (p *CloudWatchLogsProvider) GetLastStreams() []CWLogStreamRow { return p.lastStreams }

// tabTail is the Fetch func for the Tail tab.
// The App overrides rendering via renderCWLogTail(), so this is a no-op placeholder.
func (p *CloudWatchLogsProvider) tabTail(_ context.Context, _ Item) (string, error) {
	return "", nil
}

// StartTail tails the given log group, dispatching on streamName:
//   - streamName == "": uses StartLiveTail (real-time, group-wide, no history)
//   - streamName != "": uses GetLogEvents to fetch recent events then polls for new ones
func (p *CloudWatchLogsProvider) StartTail(ctx context.Context, group, streamName string, onEvent func(ts int64, group, stream, msg string)) error {
	if streamName == "" {
		return p.tailGroup(ctx, group, onEvent)
	}
	return p.tailStream(ctx, group, streamName, onEvent)
}

// tailGroup uses StartLiveTail for real-time group-wide events.
func (p *CloudWatchLogsProvider) tailGroup(ctx context.Context, group string, onEvent func(ts int64, group, stream, msg string)) error {
	reader, err := p.client.OpenLiveTailStream(ctx, group)
	if err != nil {
		return err
	}
	defer reader.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-reader.Events():
			if !ok {
				return reader.Err()
			}
			if ev, ok := event.(*cwltypes.StartLiveTailResponseStreamMemberSessionUpdate); ok {
				for _, le := range ev.Value.SessionResults {
					ts := int64(0)
					if le.Timestamp != nil {
						ts = *le.Timestamp
					}
					onEvent(ts,
						awssdk.ToString(le.LogGroupIdentifier),
						awssdk.ToString(le.LogStreamName),
						strings.TrimRight(awssdk.ToString(le.Message), "\n"),
					)
				}
			}
		}
	}
}

// tailStream uses GetLogEvents to fetch recent events then polls every 5 s for new ones.
func (p *CloudWatchLogsProvider) tailStream(ctx context.Context, group, streamName string, onEvent func(ts int64, group, stream, msg string)) error {
	// Initial fetch: last 100 events (startFromHead=false gives the most recent).
	out, err := p.client.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  awssdk.String(group),
		LogStreamName: awssdk.String(streamName),
		StartFromHead: awssdk.Bool(false),
		Limit:         awssdk.Int32(100),
	})
	if err != nil {
		return fmt.Errorf("get log events: %w", err)
	}
	for _, e := range out.Events {
		ts := int64(0)
		if e.Timestamp != nil {
			ts = *e.Timestamp
		}
		onEvent(ts, group, streamName, strings.TrimRight(awssdk.ToString(e.Message), "\n"))
	}
	nextToken := out.NextForwardToken

	// Poll for new events every 5 seconds using NextForwardToken.
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			out, err := p.client.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
				LogGroupName:  awssdk.String(group),
				LogStreamName: awssdk.String(streamName),
				NextToken:     nextToken,
			})
			if err != nil {
				continue // skip transient errors; keep the old token
			}
			for _, e := range out.Events {
				ts := int64(0)
				if e.Timestamp != nil {
					ts = *e.Timestamp
				}
				onEvent(ts, group, streamName, strings.TrimRight(awssdk.ToString(e.Message), "\n"))
			}
			nextToken = out.NextForwardToken
		}
	}
}
