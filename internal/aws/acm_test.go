package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

type stubACM struct{}

func (s *stubACM) ListCertificates(_ context.Context, _ *acm.ListCertificatesInput, _ ...func(*acm.Options)) (*acm.ListCertificatesOutput, error) {
	return &acm.ListCertificatesOutput{
		CertificateSummaryList: []acmtypes.CertificateSummary{
			{
				CertificateArn: aws.String("arn:aws:acm:us-east-1:123:certificate/abc123"),
				DomainName:     aws.String("example.com"),
			},
		},
	}, nil
}

func (s *stubACM) DescribeCertificate(_ context.Context, _ *acm.DescribeCertificateInput, _ ...func(*acm.Options)) (*acm.DescribeCertificateOutput, error) {
	expiry := time.Now().Add(357 * 24 * time.Hour)
	return &acm.DescribeCertificateOutput{
		Certificate: &acmtypes.CertificateDetail{
			CertificateArn: aws.String("arn:aws:acm:us-east-1:123:certificate/abc123"),
			DomainName:     aws.String("example.com"),
			Status:         acmtypes.CertificateStatusIssued,
			Type:           acmtypes.CertificateTypeAmazonIssued,
			NotAfter:       &expiry,
			KeyAlgorithm:   acmtypes.KeyAlgorithmRsa2048,
			InUseBy:        []string{"arn:aws:cloudfront::123:distribution/ABC"},
			SubjectAlternativeNames: []string{
				"example.com",
				"www.example.com",
				"api.example.com",
			},
			DomainValidationOptions: []acmtypes.DomainValidation{
				{
					DomainName:       aws.String("example.com"),
					ValidationMethod: acmtypes.ValidationMethodDns,
					ResourceRecord: &acmtypes.ResourceRecord{
						Name:  aws.String("_abc123.example.com."),
						Type:  acmtypes.RecordTypeCname,
						Value: aws.String("_def456.acm-validations.aws."),
					},
				},
			},
		},
	}, nil
}

func TestACMProvider_ListItems(t *testing.T) {
	p := awspkg.NewACMProviderWithClient(&stubACM{})
	items, err := p.ListItems(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Name != "example.com" {
		t.Errorf("got name %q, want example.com", items[0].Name)
	}
}

func TestACMProvider_Tabs(t *testing.T) {
	p := awspkg.NewACMProviderWithClient(&stubACM{})
	tabs := p.Tabs()
	if len(tabs) != 3 {
		t.Fatalf("got %d tabs, want 3", len(tabs))
	}
	item := awspkg.Item{ID: "arn:aws:acm:us-east-1:123:certificate/abc123", Name: "example.com"}

	cases := []struct {
		idx   int
		label string
		want  string
	}{
		{0, "Overview", "example.com"},
		{1, "Domains", "(primary)"},
		{2, "Validation", "DNS"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if tabs[tc.idx].Label != tc.label {
				t.Errorf("tab %d label = %q, want %q", tc.idx, tabs[tc.idx].Label, tc.label)
			}
			content, err := tabs[tc.idx].Fetch(context.Background(), item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(content, tc.want) {
				t.Errorf("tab %d missing %q\ngot:\n%s", tc.idx, tc.want, content)
			}
		})
	}
}

func TestACMProvider_TabOverview_ExpiryDays(t *testing.T) {
	p := awspkg.NewACMProviderWithClient(&stubACM{})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "arn:aws:acm:us-east-1:123:certificate/abc123", Name: "example.com"}
	content, err := tabs[0].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should contain days remaining
	if !strings.Contains(content, "days") {
		t.Errorf("expected 'days' in expiry output\ngot:\n%s", content)
	}
}
