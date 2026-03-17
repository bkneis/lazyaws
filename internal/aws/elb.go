package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

// ELBV2API is the subset of ELBv2 client methods used by ELBProvider.
type ELBV2API interface {
	DescribeLoadBalancers(ctx context.Context, in *elbv2.DescribeLoadBalancersInput, opts ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error)
	DescribeListeners(ctx context.Context, in *elbv2.DescribeListenersInput, opts ...func(*elbv2.Options)) (*elbv2.DescribeListenersOutput, error)
	DescribeTargetGroups(ctx context.Context, in *elbv2.DescribeTargetGroupsInput, opts ...func(*elbv2.Options)) (*elbv2.DescribeTargetGroupsOutput, error)
}

// ELBProvider implements Provider for Elastic Load Balancers (v2).
type ELBProvider struct {
	client ELBV2API
}

func NewELBProvider(cfg awssdk.Config, endpointURL string) *ELBProvider {
	var opts []func(*elbv2.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *elbv2.Options) {
			o.BaseEndpoint = awssdk.String(endpointURL)
		})
	}
	return &ELBProvider{client: elbv2.NewFromConfig(cfg, opts...)}
}

func NewELBProviderWithClient(client ELBV2API) *ELBProvider {
	return &ELBProvider{client: client}
}

func (p *ELBProvider) Name() string { return "Load Balancers" }

func (p *ELBProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	var items []Item
	var marker *string
	for {
		out, err := p.client.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
			Marker: marker,
		})
		if err != nil {
			return nil, fmt.Errorf("describe load balancers: %w", err)
		}
		for _, lb := range out.LoadBalancers {
			arn := awssdk.ToString(lb.LoadBalancerArn)
			name := awssdk.ToString(lb.LoadBalancerName)
			azNames := make([]string, len(lb.AvailabilityZones))
			for i, az := range lb.AvailabilityZones {
				azNames[i] = awssdk.ToString(az.ZoneName)
			}
			state := "-"
			if lb.State != nil {
				state = string(lb.State.Code)
			}
			created := ""
			if lb.CreatedTime != nil {
				created = lb.CreatedTime.Format(time.DateTime)
			}
			items = append(items, Item{
				ID:   arn,
				Name: name,
				Meta: map[string]string{
					"type":     string(lb.Type),
					"scheme":   string(lb.Scheme),
					"state":    state,
					"dns_name": awssdk.ToString(lb.DNSName),
					"vpc_id":   awssdk.ToString(lb.VpcId),
					"created":  created,
					"azs":      strings.Join(azNames, ", "),
				},
			})
		}
		if out.NextMarker == nil {
			break
		}
		marker = out.NextMarker
	}
	return filterItems(items, query), nil
}

func (p *ELBProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *ELBProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Listeners", Fetch: p.tabListeners},
		{Label: "Target Groups", Fetch: p.tabTargetGroups},
	}
}

func (p *ELBProvider) tabOverview(_ context.Context, item Item) (string, error) {
	return KV([][2]string{
		{"Name", item.Name},
		{"Type", item.Meta["type"]},
		{"Scheme", item.Meta["scheme"]},
		{"State", item.Meta["state"]},
		{"DNS Name", item.Meta["dns_name"]},
		{"VPC", item.Meta["vpc_id"]},
		{"Created", item.Meta["created"]},
		{"AZs", item.Meta["azs"]},
	}), nil
}

func (p *ELBProvider) tabListeners(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeListeners(ctx, &elbv2.DescribeListenersInput{
		LoadBalancerArn: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	if len(out.Listeners) == 0 {
		return "  (no listeners)\n", nil
	}
	rows := make([][]string, len(out.Listeners))
	for i, l := range out.Listeners {
		port := "-"
		if l.Port != nil {
			port = fmt.Sprintf("%d", awssdk.ToInt32(l.Port))
		}
		defaultAction := "-"
		if len(l.DefaultActions) > 0 {
			defaultAction = string(l.DefaultActions[0].Type)
		}
		rows[i] = []string{port, string(l.Protocol), defaultAction}
	}
	return Table([]string{"Port", "Protocol", "Default Action"}, rows), nil
}

func (p *ELBProvider) tabTargetGroups(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeTargetGroups(ctx, &elbv2.DescribeTargetGroupsInput{
		LoadBalancerArn: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	if len(out.TargetGroups) == 0 {
		return "  (no target groups)\n", nil
	}
	rows := make([][]string, len(out.TargetGroups))
	for i, tg := range out.TargetGroups {
		port := "-"
		if tg.Port != nil {
			port = fmt.Sprintf("%d", awssdk.ToInt32(tg.Port))
		}
		hcPath := "-"
		if tg.HealthCheckPath != nil {
			hcPath = awssdk.ToString(tg.HealthCheckPath)
		}
		rows[i] = []string{
			awssdk.ToString(tg.TargetGroupName),
			string(tg.Protocol),
			port,
			string(tg.TargetType),
			hcPath,
		}
	}
	return Table([]string{"Name", "Protocol", "Port", "Target Type", "Health Check Path"}, rows), nil
}
