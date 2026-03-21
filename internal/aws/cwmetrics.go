package aws

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// CloudWatchMetricsAPI is the subset of the CloudWatch client used for metric sparklines.
// Satisfied structurally by *cloudwatch.Client.
type CloudWatchMetricsAPI interface {
	GetMetricData(ctx context.Context, in *cloudwatch.GetMetricDataInput, opts ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)
}

// metricSpec describes one CloudWatch metric to fetch and render.
type metricSpec struct {
	id      string // unique ID used to match GetMetricData results
	label   string // display label
	ns      string // CloudWatch namespace, e.g. "AWS/Lambda"
	name    string // metric name, e.g. "Invocations"
	stat    string // "Sum", "Average", "Maximum"
	dimKey  string // primary dimension key
	dimVal  string // primary dimension value (from the selected item)
	dim2Key string // optional second dimension key (e.g. "StorageType" for S3)
	dim2Val string // optional second dimension value
	unit    string // display suffix, e.g. "ms", "%", "B", ""
	isError bool   // true → non-zero bars render in [red]
}

const sparkChars = "▁▂▃▄▅▆▇█"

// sparkline converts a slice of float64 values into a single-line bar string.
// All-zero input → flat "▁▁▁..." baseline.
func sparkline(values []float64) string {
	if len(values) == 0 {
		return ""
	}
	maxVal := 0.0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	runes := []rune(sparkChars)
	bars := make([]rune, len(values))
	for i, v := range values {
		if maxVal == 0 {
			bars[i] = runes[0]
		} else {
			idx := int(math.Round(v/maxVal*float64(len(runes)-1)))
			if idx >= len(runes) {
				idx = len(runes) - 1
			}
			bars[i] = runes[idx]
		}
	}
	return string(bars)
}

// renderSparkline renders one metric block: header line + bar line + summary.
func renderSparkline(spec metricSpec, values []float64, windowHours, periodSecs int) string {
	periodLabel := fmt.Sprintf("%dm", periodSecs/60)
	if periodSecs >= 3600 {
		periodLabel = fmt.Sprintf("%dh", periodSecs/3600)
	}
	windowLabel := fmt.Sprintf("last %dh", windowHours)
	if windowHours >= 24 {
		windowLabel = fmt.Sprintf("last %dd", windowHours/24)
	}

	header := fmt.Sprintf("  %s%s[-]   %s · %s · %s\n", ActiveTags.Header, spec.label, spec.stat, windowLabel, periodLabel)

	// Compute summary stats
	sum, maxV, count := 0.0, 0.0, 0
	for _, v := range values {
		sum += v
		if v > maxV {
			maxV = v
		}
		if v > 0 {
			count++
		}
	}
	avg := 0.0
	if len(values) > 0 {
		avg = sum / float64(len(values))
	}

	bars := sparkline(values)
	hasNonZero := maxV > 0

	// Color the bar line red for error metrics that have non-zero data
	var barLine string
	if spec.isError && hasNonZero {
		barLine = fmt.Sprintf("  [red]%s[-]\n", bars)
	} else {
		barLine = fmt.Sprintf("  %s\n", bars)
	}

	// Build summary: format numbers compactly
	var summary string
	switch spec.stat {
	case "Sum":
		summary = fmt.Sprintf("  total=%-8s  avg=%-8s  max=%s", formatMetricVal(sum, spec.unit), formatMetricVal(avg, spec.unit), formatMetricVal(maxV, spec.unit))
	default:
		summary = fmt.Sprintf("  avg=%-8s  max=%s", formatMetricVal(avg, spec.unit), formatMetricVal(maxV, spec.unit))
	}
	_ = count

	return header + barLine + summary + "\n\n"
}

// formatMetricVal formats a metric value with its unit suffix.
func formatMetricVal(v float64, unit string) string {
	switch unit {
	case "B":
		return FormatSize(int64(v))
	case "ms":
		if v >= 1000 {
			return fmt.Sprintf("%.1fs", v/1000)
		}
		return fmt.Sprintf("%.0fms", v)
	case "%":
		return fmt.Sprintf("%.1f%%", v)
	default:
		if v >= 1_000_000 {
			return fmt.Sprintf("%.1fM", v/1_000_000)
		}
		if v >= 1_000 {
			return fmt.Sprintf("%.1fk", v/1_000)
		}
		if v == math.Trunc(v) {
			return fmt.Sprintf("%.0f", v)
		}
		return fmt.Sprintf("%.2f", v)
	}
}

// fetchSparklines fetches all specs in a single GetMetricData call.
// Returns map of spec.id → time-ordered []float64 (oldest first).
func fetchSparklines(ctx context.Context, client CloudWatchMetricsAPI, specs []metricSpec, windowHours int, periodSecs int32) (map[string][]float64, error) {
	if client == nil {
		return nil, nil
	}
	now := time.Now().UTC()
	startTime := now.Add(-time.Duration(windowHours) * time.Hour)

	queries := make([]cwtypes.MetricDataQuery, len(specs))
	for i, s := range specs {
		dims := []cwtypes.Dimension{
			{Name: awssdk.String(s.dimKey), Value: awssdk.String(s.dimVal)},
		}
		if s.dim2Key != "" {
			dims = append(dims, cwtypes.Dimension{Name: awssdk.String(s.dim2Key), Value: awssdk.String(s.dim2Val)})
		}
		queries[i] = cwtypes.MetricDataQuery{
			Id:    awssdk.String(s.id),
			Label: awssdk.String(s.label),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  awssdk.String(s.ns),
					MetricName: awssdk.String(s.name),
					Dimensions: dims,
				},
				Period: awssdk.Int32(periodSecs),
				Stat:   awssdk.String(s.stat),
			},
		}
	}

	out, err := client.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
		StartTime:         awssdk.Time(startTime),
		EndTime:           awssdk.Time(now),
		MetricDataQueries: queries,
	})
	if err != nil {
		return nil, err
	}

	result := make(map[string][]float64, len(specs))
	for _, r := range out.MetricDataResults {
		id := awssdk.ToString(r.Id)
		// CloudWatch returns values newest-first; reverse to oldest-first for sparkline.
		vals := make([]float64, len(r.Values))
		for i, v := range r.Values {
			vals[len(r.Values)-1-i] = v
		}
		result[id] = vals
	}
	return result, nil
}

// renderMetricsTab renders a full Metrics tab from fetched sparkline data.
// If data is nil (client unavailable) or all specs have no data, shows a notice.
func renderMetricsTab(specs []metricSpec, data map[string][]float64, windowHours, periodSecs int) string {
	if data == nil {
		return "  (metrics not available in this environment)\n"
	}

	var sb strings.Builder
	sb.WriteString("\n")
	for _, s := range specs {
		values := data[s.id] // nil → empty slice → flat baseline
		sb.WriteString(renderSparkline(s, values, windowHours, periodSecs))
	}
	return sb.String()
}
