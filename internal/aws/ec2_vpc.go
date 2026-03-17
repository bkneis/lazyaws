package aws

import (
	"context"
	"fmt"
	"strconv"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// VpcAPI is the subset of the EC2 client methods used by EC2VPCProvider.
type VpcAPI interface {
	DescribeVpcs(ctx context.Context, in *ec2.DescribeVpcsInput, opts ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	DescribeSubnets(ctx context.Context, in *ec2.DescribeSubnetsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeRouteTables(ctx context.Context, in *ec2.DescribeRouteTablesInput, opts ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
}

// EC2VPCProvider implements Provider for Amazon VPCs.
type EC2VPCProvider struct {
	client VpcAPI
}

func NewEC2VPCProvider(cfg awssdk.Config, endpointURL string) *EC2VPCProvider {
	var opts []func(*ec2.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *ec2.Options) {
			o.BaseEndpoint = awssdk.String(endpointURL)
		})
	}
	return &EC2VPCProvider{client: ec2.NewFromConfig(cfg, opts...)}
}

func NewEC2VPCProviderWithClient(client VpcAPI) *EC2VPCProvider {
	return &EC2VPCProvider{client: client}
}

func (p *EC2VPCProvider) Name() string { return "EC2 VPC" }

func (p *EC2VPCProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, fmt.Errorf("describe vpcs: %w", err)
	}
	items := make([]Item, len(out.Vpcs))
	for i, vpc := range out.Vpcs {
		id := awssdk.ToString(vpc.VpcId)
		name := ec2NameTag(vpc.Tags)
		if name == "" {
			name = id
		}
		items[i] = Item{
			ID:   id,
			Name: name,
			Meta: map[string]string{
				"cidr":       awssdk.ToString(vpc.CidrBlock),
				"state":      string(vpc.State),
				"is_default": strconv.FormatBool(awssdk.ToBool(vpc.IsDefault)),
				"owner_id":   awssdk.ToString(vpc.OwnerId),
				"dhcp_opts":  awssdk.ToString(vpc.DhcpOptionsId),
			},
		}
	}
	return filterItems(items, query), nil
}

func (p *EC2VPCProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *EC2VPCProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Subnets", Fetch: p.tabSubnets},
		{Label: "Route Tables", Fetch: p.tabRouteTables},
	}
}

func (p *EC2VPCProvider) tabOverview(_ context.Context, item Item) (string, error) {
	return KV([][2]string{
		{"VPC ID", item.ID},
		{"CIDR", item.Meta["cidr"]},
		{"State", item.Meta["state"]},
		{"Is Default", item.Meta["is_default"]},
		{"Owner ID", item.Meta["owner_id"]},
		{"DHCP Options ID", item.Meta["dhcp_opts"]},
	}), nil
}

func (p *EC2VPCProvider) tabSubnets(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{Name: awssdk.String("vpc-id"), Values: []string{item.ID}},
		},
	})
	if err != nil {
		return "", err
	}
	if len(out.Subnets) == 0 {
		return "  (no subnets)\n", nil
	}
	rows := make([][]string, len(out.Subnets))
	for i, s := range out.Subnets {
		rows[i] = []string{
			awssdk.ToString(s.SubnetId),
			awssdk.ToString(s.AvailabilityZone),
			awssdk.ToString(s.CidrBlock),
			strconv.Itoa(int(awssdk.ToInt32(s.AvailableIpAddressCount))),
			strconv.FormatBool(awssdk.ToBool(s.DefaultForAz)),
		}
	}
	return Table([]string{"Subnet ID", "AZ", "CIDR", "Available IPs", "Default"}, rows), nil
}

func (p *EC2VPCProvider) tabRouteTables(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []ec2types.Filter{
			{Name: awssdk.String("vpc-id"), Values: []string{item.ID}},
		},
	})
	if err != nil {
		return "", err
	}
	if len(out.RouteTables) == 0 {
		return "  (no route tables)\n", nil
	}
	rows := make([][]string, len(out.RouteTables))
	for i, rt := range out.RouteTables {
		assocCount := strconv.Itoa(len(rt.Associations))
		rows[i] = []string{
			awssdk.ToString(rt.RouteTableId),
			strconv.Itoa(len(rt.Routes)),
			assocCount,
		}
	}
	return Table([]string{"Route Table ID", "Routes", "Associations"}, rows), nil
}


