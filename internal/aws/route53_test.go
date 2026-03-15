package aws_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

type stubRoute53 struct{}

func (s *stubRoute53) ListHostedZones(_ context.Context, _ *route53.ListHostedZonesInput, _ ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error) {
	count := int64(12)
	return &route53.ListHostedZonesOutput{
		HostedZones: []r53types.HostedZone{
			{
				Id:                     aws.String("/hostedzone/Z1234ABCDEFGHIJ"),
				Name:                   aws.String("example.com."),
				ResourceRecordSetCount: &count,
				Config:                 &r53types.HostedZoneConfig{Comment: aws.String("Main zone"), PrivateZone: false},
			},
		},
	}, nil
}

func (s *stubRoute53) GetHostedZone(_ context.Context, _ *route53.GetHostedZoneInput, _ ...func(*route53.Options)) (*route53.GetHostedZoneOutput, error) {
	count := int64(12)
	return &route53.GetHostedZoneOutput{
		HostedZone: &r53types.HostedZone{
			Id:                     aws.String("/hostedzone/Z1234ABCDEFGHIJ"),
			Name:                   aws.String("example.com."),
			ResourceRecordSetCount: &count,
			Config:                 &r53types.HostedZoneConfig{Comment: aws.String("Main zone"), PrivateZone: false},
		},
	}, nil
}

func (s *stubRoute53) ListResourceRecordSets(_ context.Context, _ *route53.ListResourceRecordSetsInput, _ ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	ttl := int64(300)
	return &route53.ListResourceRecordSetsOutput{
		ResourceRecordSets: []r53types.ResourceRecordSet{
			{
				Name: aws.String("example.com"),
				Type: r53types.RRTypeA,
				AliasTarget: &r53types.AliasTarget{
					DNSName: aws.String("d1234.cloudfront.net"),
				},
			},
			{
				Name: aws.String("www.example.com"),
				Type: r53types.RRTypeCname,
				TTL:  &ttl,
				ResourceRecords: []r53types.ResourceRecord{
					{Value: aws.String("example.com")},
				},
			},
		},
	}, nil
}

func TestRoute53Provider_ListItems(t *testing.T) {
	p := awspkg.NewRoute53ProviderWithClient(&stubRoute53{})
	items, err := p.ListItems(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].ID != "Z1234ABCDEFGHIJ" {
		t.Errorf("got ID %q, want Z1234ABCDEFGHIJ (prefix stripped)", items[0].ID)
	}
	if items[0].Name != "example.com." {
		t.Errorf("got name %q, want example.com.", items[0].Name)
	}
}

func TestRoute53Provider_Tabs(t *testing.T) {
	p := awspkg.NewRoute53ProviderWithClient(&stubRoute53{})
	tabs := p.Tabs()
	if len(tabs) != 2 {
		t.Fatalf("got %d tabs, want 2", len(tabs))
	}
	item := awspkg.Item{ID: "Z1234ABCDEFGHIJ", Name: "example.com."}

	cases := []struct {
		idx   int
		label string
		want  string
	}{
		{0, "Overview", "example.com"},
		{1, "Records", "cloudfront.net"},
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

func TestRoute53Provider_TabRecords_AliasTTL(t *testing.T) {
	p := awspkg.NewRoute53ProviderWithClient(&stubRoute53{})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "Z1234ABCDEFGHIJ", Name: "example.com."}
	content, err := tabs[1].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ALIAS record (no TTL) should show "-"
	if !strings.Contains(content, "-") {
		t.Errorf("expected '-' for ALIAS record TTL\ngot:\n%s", content)
	}
	// Non-alias record should show numeric TTL
	if !strings.Contains(content, "300") {
		t.Errorf("expected TTL '300' for CNAME record\ngot:\n%s", content)
	}
}
