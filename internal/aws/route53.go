package aws

import (
	"context"
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
)

type Route53API interface {
	ListHostedZones(ctx context.Context, in *route53.ListHostedZonesInput, opts ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error)
	GetHostedZone(ctx context.Context, in *route53.GetHostedZoneInput, opts ...func(*route53.Options)) (*route53.GetHostedZoneOutput, error)
	ListResourceRecordSets(ctx context.Context, in *route53.ListResourceRecordSetsInput, opts ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
}

type Route53Provider struct{ client Route53API }

func NewRoute53Provider(cfg awssdk.Config, endpointURL string) *Route53Provider {
	var opts []func(*route53.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *route53.Options) { o.BaseEndpoint = awssdk.String(endpointURL) })
	}
	return &Route53Provider{client: route53.NewFromConfig(cfg, opts...)}
}

func NewRoute53ProviderWithClient(client Route53API) *Route53Provider {
	return &Route53Provider{client: client}
}

func (p *Route53Provider) Name() string { return "Route53" }

func (p *Route53Provider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.ListHostedZones(ctx, &route53.ListHostedZonesInput{})
	if err != nil {
		return nil, fmt.Errorf("list hosted zones: %w", err)
	}
	items := make([]Item, len(out.HostedZones))
	for i, z := range out.HostedZones {
		rawID := awssdk.ToString(z.Id)
		id := strings.TrimPrefix(rawID, "/hostedzone/")
		name := awssdk.ToString(z.Name)
		items[i] = Item{ID: id, Name: name}
	}
	return filterItems(items, query), nil
}

func (p *Route53Provider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *Route53Provider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Records", Fetch: p.tabRecords},
	}
}

func (p *Route53Provider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetHostedZone(ctx, &route53.GetHostedZoneInput{Id: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	z := out.HostedZone
	zoneType := "Public"
	if z.Config != nil && z.Config.PrivateZone {
		zoneType = "Private"
	}
	comment := ""
	if z.Config != nil {
		comment = awssdk.ToString(z.Config.Comment)
	}
	recordCount := fmt.Sprintf("%d", awssdk.ToInt64(z.ResourceRecordSetCount))
	return KV([][2]string{
		{"Zone", awssdk.ToString(z.Name)},
		{"Zone ID", item.ID},
		{"Type", zoneType},
		{"Record Count", recordCount},
		{"Comment", comment},
	}), nil
}

func (p *Route53Provider) tabRecords(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	if len(out.ResourceRecordSets) == 0 {
		return "  (no records)\n", nil
	}
	rows := make([][]string, 0, len(out.ResourceRecordSets))
	for _, rr := range out.ResourceRecordSets {
		rows = append(rows, []string{
			awssdk.ToString(rr.Name),
			string(rr.Type),
			formatTTL(rr.TTL),
			truncate(recordValue(rr), 45),
		})
	}
	return Table([]string{"Name", "Type", "TTL", "Value"}, rows), nil
}

func formatTTL(ttl *int64) string {
	if ttl == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *ttl)
}

func recordValue(rr r53types.ResourceRecordSet) string {
	if rr.AliasTarget != nil {
		return "ALIAS " + awssdk.ToString(rr.AliasTarget.DNSName)
	}
	if len(rr.ResourceRecords) > 0 {
		return awssdk.ToString(rr.ResourceRecords[0].Value)
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
