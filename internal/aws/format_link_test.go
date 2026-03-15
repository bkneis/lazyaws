package aws

import (
	"strings"
	"testing"
)

func TestLink(t *testing.T) {
	tests := []struct {
		label, provider, targetID, wantContains string
	}{
		{
			label:        "my-function",
			provider:     "Lambda",
			targetID:     "my-function",
			wantContains: `["link:Lambda:my-function"]`,
		},
		{
			label:        "my-function",
			provider:     "Lambda",
			targetID:     "my-function",
			wantContains: "[aqua::u]my-function",
		},
	}
	for _, tc := range tests {
		got := Link(tc.label, tc.provider, tc.targetID)
		if !strings.Contains(got, tc.wantContains) {
			t.Errorf("Link(%q,%q,%q) = %q, want to contain %q", tc.label, tc.provider, tc.targetID, got, tc.wantContains)
		}
	}
}

func TestArnLastSegment(t *testing.T) {
	tests := []struct{ arn, want string }{
		{"arn:aws:iam::123456789012:role/MyRole", "role/MyRole"},
		{"arn:aws:sqs:us-east-1:123:my-queue", "my-queue"},
		{"simple", "simple"},
	}
	for _, tc := range tests {
		if got := arnLastSegment(tc.arn); got != tc.want {
			t.Errorf("arnLastSegment(%q) = %q, want %q", tc.arn, got, tc.want)
		}
	}
}

func TestArnToSQSURL(t *testing.T) {
	tests := []struct{ arn, want string }{
		{
			"arn:aws:sqs:us-east-1:123456789012:my-queue",
			"https://sqs.us-east-1.amazonaws.com/123456789012/my-queue",
		},
		{
			"not-an-arn",
			"not-an-arn", // pass-through on bad input
		},
	}
	for _, tc := range tests {
		if got := arnToSQSURL(tc.arn); got != tc.want {
			t.Errorf("arnToSQSURL(%q) = %q, want %q", tc.arn, got, tc.want)
		}
	}
}

func TestParseLambdaFromIntegrationURI(t *testing.T) {
	tests := []struct{ uri, want string }{
		{
			"arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/arn:aws:lambda:us-east-1:123:function:my-fn/invocations",
			"my-fn",
		},
		{
			"arn:aws:lambda:us-east-1:123456789012:function:my-fn",
			"my-fn",
		},
	}
	for _, tc := range tests {
		if got := parseLambdaFromIntegrationURI(tc.uri); got != tc.want {
			t.Errorf("parseLambdaFromIntegrationURI(%q) = %q, want %q", tc.uri, got, tc.want)
		}
	}
}
