package aws

import (
	"context"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// stubCW implements CloudWatchMetricsAPI with canned results.
type stubCW struct {
	results []cwtypes.MetricDataResult
	err     error
}

func (s *stubCW) GetMetricData(_ context.Context, _ *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &cloudwatch.GetMetricDataOutput{MetricDataResults: s.results}, nil
}

// makeResult builds a MetricDataResult with values in newest-first order (as CloudWatch returns them).
func makeResult(id string, newestFirst []float64) cwtypes.MetricDataResult {
	n := len(newestFirst)
	vals := make([]float64, n)
	ts := make([]time.Time, n)
	now := time.Now()
	for i, v := range newestFirst {
		vals[i] = v
		ts[i] = now.Add(-time.Duration(i) * time.Minute)
	}
	return cwtypes.MetricDataResult{
		Id:         awssdk.String(id),
		Values:     vals,
		Timestamps: ts,
	}
}

// ---------------------------------------------------------------------------
// fetchSparklines
// ---------------------------------------------------------------------------

func TestFetchSparklines_NilClient(t *testing.T) {
	specs := []metricSpec{{id: "cpu", label: "CPU"}}
	data, err := fetchSparklines(context.Background(), nil, specs, 1, 60)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil map for nil client, got %v", data)
	}
}

func TestFetchSparklines_ReversesOrder(t *testing.T) {
	// CloudWatch returns newest-first; fetchSparklines must reverse to oldest-first.
	stub := &stubCW{
		results: []cwtypes.MetricDataResult{
			makeResult("inv", []float64{30, 20, 10}), // newest→oldest from CW
		},
	}
	specs := []metricSpec{{id: "inv", label: "Invocations", ns: "AWS/Lambda", name: "Invocations", stat: "Sum", dimKey: "FunctionName", dimVal: "my-fn"}}
	data, err := fetchSparklines(context.Background(), stub, specs, 1, 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vals := data["inv"]
	if len(vals) != 3 {
		t.Fatalf("expected 3 values, got %d", len(vals))
	}
	// After reversal: oldest-first → [10, 20, 30]
	want := []float64{10, 20, 30}
	for i, w := range want {
		if vals[i] != w {
			t.Errorf("vals[%d] = %v, want %v", i, vals[i], w)
		}
	}
}

func TestFetchSparklines_MultipleSpecs(t *testing.T) {
	stub := &stubCW{
		results: []cwtypes.MetricDataResult{
			makeResult("cpu", []float64{50, 60}),
			makeResult("mem", []float64{80, 70}),
		},
	}
	specs := []metricSpec{
		{id: "cpu", label: "CPU", dimKey: "ClusterName", dimVal: "c1"},
		{id: "mem", label: "Mem", dimKey: "ClusterName", dimVal: "c1"},
	}
	data, err := fetchSparklines(context.Background(), stub, specs, 1, 60)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := data["cpu"]; !ok {
		t.Error("missing cpu key")
	}
	if _, ok := data["mem"]; !ok {
		t.Error("missing mem key")
	}
}

func TestFetchSparklines_SecondDimension(t *testing.T) {
	// Verify that a non-empty dim2Key is included without error.
	stub := &stubCW{
		results: []cwtypes.MetricDataResult{
			makeResult("size", []float64{1024}),
		},
	}
	specs := []metricSpec{
		{id: "size", label: "Size", ns: "AWS/S3", name: "BucketSizeBytes", stat: "Average",
			dimKey: "BucketName", dimVal: "my-bucket", dim2Key: "StorageType", dim2Val: "StandardStorage"},
	}
	data, err := fetchSparklines(context.Background(), stub, specs, 24, 86400)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data["size"]) != 1 {
		t.Errorf("expected 1 value, got %d", len(data["size"]))
	}
}

// ---------------------------------------------------------------------------
// renderMetricsTab
// ---------------------------------------------------------------------------

func TestRenderMetricsTab_NilData(t *testing.T) {
	specs := []metricSpec{{id: "x", label: "X"}}
	out := renderMetricsTab(specs, nil, 1, 300)
	if !strings.Contains(out, "not available") {
		t.Errorf("expected 'not available' message, got: %q", out)
	}
}

func TestRenderMetricsTab_ContainsLabels(t *testing.T) {
	specs := []metricSpec{
		{id: "cpu", label: "CPU Utilization"},
		{id: "mem", label: "Memory Utilization"},
	}
	data := map[string][]float64{
		"cpu": {10, 20, 30},
		"mem": {40, 50, 60},
	}
	out := renderMetricsTab(specs, data, 1, 60)
	if !strings.Contains(out, "CPU Utilization") {
		t.Errorf("missing CPU label in output: %q", out)
	}
	if !strings.Contains(out, "Memory Utilization") {
		t.Errorf("missing Memory label in output: %q", out)
	}
}

func TestRenderMetricsTab_ErrorMetricColored(t *testing.T) {
	specs := []metricSpec{
		{id: "errs", label: "Errors", stat: "Sum", isError: true},
	}
	data := map[string][]float64{
		"errs": {0, 0, 5, 0}, // has non-zero → should be red
	}
	out := renderMetricsTab(specs, data, 1, 300)
	if !strings.Contains(out, "[red]") {
		t.Errorf("expected [red] color tag for error metric with data, got: %q", out)
	}
}

func TestRenderMetricsTab_ErrorMetricAllZeroNoColor(t *testing.T) {
	specs := []metricSpec{
		{id: "errs", label: "Errors", stat: "Sum", isError: true},
	}
	data := map[string][]float64{
		"errs": {0, 0, 0},
	}
	out := renderMetricsTab(specs, data, 1, 300)
	if strings.Contains(out, "[red]") {
		t.Errorf("expected no [red] tag for all-zero error metric, got: %q", out)
	}
}

func TestRenderMetricsTab_SumStatShowsTotal(t *testing.T) {
	specs := []metricSpec{{id: "inv", label: "Invocations", stat: "Sum"}}
	data := map[string][]float64{"inv": {10, 20, 30}}
	out := renderMetricsTab(specs, data, 1, 300)
	if !strings.Contains(out, "total=") {
		t.Errorf("Sum stat should show total=, got: %q", out)
	}
}

func TestRenderMetricsTab_AverageStatNoTotal(t *testing.T) {
	specs := []metricSpec{{id: "cpu", label: "CPU", stat: "Average"}}
	data := map[string][]float64{"cpu": {10, 20, 30}}
	out := renderMetricsTab(specs, data, 1, 300)
	if strings.Contains(out, "total=") {
		t.Errorf("Average stat should not show total=, got: %q", out)
	}
	if !strings.Contains(out, "avg=") {
		t.Errorf("Average stat should show avg=, got: %q", out)
	}
}

func TestRenderMetricsTab_MissingSpecKeyShowsFlat(t *testing.T) {
	// A spec whose ID has no entry in data → nil values → renders without panic.
	specs := []metricSpec{{id: "missing", label: "Missing", stat: "Sum"}}
	data := map[string][]float64{} // empty
	out := renderMetricsTab(specs, data, 1, 300)
	if !strings.Contains(out, "Missing") {
		t.Errorf("should still render label for missing data: %q", out)
	}
}

// ---------------------------------------------------------------------------
// formatMetricVal
// ---------------------------------------------------------------------------

func TestFormatMetricVal(t *testing.T) {
	tests := []struct {
		v    float64
		unit string
		want string
	}{
		{1_073_741_824, "B", "1.0 GB"},
		{1_048_576, "B", "1.0 MB"},
		{1024, "B", "1 KB"},
		{512, "B", "512 B"},
		{1500, "ms", "1.5s"},
		{250, "ms", "250ms"},
		{75.5, "%", "75.5%"},
		{1_500_000, "", "1.5M"},
		{2500, "", "2.5k"},
		{42, "", "42"},
		{3.14, "", "3.14"},
	}
	for _, tc := range tests {
		got := formatMetricVal(tc.v, tc.unit)
		if got != tc.want {
			t.Errorf("formatMetricVal(%v, %q) = %q, want %q", tc.v, tc.unit, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatSize
// ---------------------------------------------------------------------------

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1 KB"},
		{1_048_576, "1.0 MB"},
		{1_073_741_824, "1.0 GB"},
	}
	for _, tc := range tests {
		got := FormatSize(tc.bytes)
		if got != tc.want {
			t.Errorf("FormatSize(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}
