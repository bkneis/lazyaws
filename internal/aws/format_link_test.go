package aws

import (
	"strings"
	"testing"
)

func TestLink(t *testing.T) {
	got := Link("my-function", "Lambda", "my-function")
	for _, want := range []string{
		"\x02Lambda:my-function\x03", // control-char marker encodes provider + targetID
		"[aqua::u]my-function",       // label styled aqua underline
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Link() = %q, want to contain %q", got, want)
		}
	}
}

func TestArnLastSegment(t *testing.T) {
	tests := []struct{ arn, want string }{
		{"arn:aws:iam::123456789012:role/MyRole", "MyRole"},      // slash stripped → role name only
		{"arn:aws:lambda:us-east-1:123:function:my-fn", "my-fn"}, // no slash
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
