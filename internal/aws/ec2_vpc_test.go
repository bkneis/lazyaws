package aws_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

type stubVpc struct {
	vpcs        []ec2types.Vpc
	subnets     []ec2types.Subnet
	routeTables []ec2types.RouteTable
}

func (s *stubVpc) DescribeVpcs(_ context.Context, _ *ec2.DescribeVpcsInput, _ ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	return &ec2.DescribeVpcsOutput{Vpcs: s.vpcs}, nil
}

func (s *stubVpc) DescribeSubnets(_ context.Context, _ *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return &ec2.DescribeSubnetsOutput{Subnets: s.subnets}, nil
}

func (s *stubVpc) DescribeRouteTables(_ context.Context, _ *ec2.DescribeRouteTablesInput, _ ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	return &ec2.DescribeRouteTablesOutput{RouteTables: s.routeTables}, nil
}

func TestEC2VPCProvider_ListItems(t *testing.T) {
	stub := &stubVpc{
		vpcs: []ec2types.Vpc{
			{
				VpcId:     aws.String("vpc-111"),
				CidrBlock: aws.String("10.0.0.0/16"),
				State:     ec2types.VpcStateAvailable,
				IsDefault: aws.Bool(true),
				OwnerId:   aws.String("123456789012"),
				Tags: []ec2types.Tag{
					{Key: aws.String("Name"), Value: aws.String("main-vpc")},
				},
			},
		},
	}
	p := awspkg.NewEC2VPCProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].ID != "vpc-111" {
		t.Errorf("want ID=vpc-111, got %s", items[0].ID)
	}
	if items[0].Name != "main-vpc" {
		t.Errorf("want Name=main-vpc, got %s", items[0].Name)
	}
}

func TestEC2VPCProvider_ListItems_Filter(t *testing.T) {
	stub := &stubVpc{
		vpcs: []ec2types.Vpc{
			{VpcId: aws.String("vpc-111"), CidrBlock: aws.String("10.0.0.0/16"), State: ec2types.VpcStateAvailable, IsDefault: aws.Bool(false),
				Tags: []ec2types.Tag{{Key: aws.String("Name"), Value: aws.String("prod-vpc")}}},
			{VpcId: aws.String("vpc-222"), CidrBlock: aws.String("172.16.0.0/16"), State: ec2types.VpcStateAvailable, IsDefault: aws.Bool(false),
				Tags: []ec2types.Tag{{Key: aws.String("Name"), Value: aws.String("dev-vpc")}}},
		},
	}
	p := awspkg.NewEC2VPCProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "prod-vpc" {
		t.Errorf("filter expected [prod-vpc], got %v", items)
	}
}

func TestEC2VPCProvider_Tabs(t *testing.T) {
	stub := &stubVpc{
		vpcs: []ec2types.Vpc{
			{VpcId: aws.String("vpc-111"), CidrBlock: aws.String("10.0.0.0/16"), State: ec2types.VpcStateAvailable, IsDefault: aws.Bool(true), OwnerId: aws.String("123456789012"), DhcpOptionsId: aws.String("dopt-abc")},
		},
		subnets: []ec2types.Subnet{
			{SubnetId: aws.String("subnet-aaa"), AvailabilityZone: aws.String("us-east-1a"), CidrBlock: aws.String("10.0.1.0/24"), AvailableIpAddressCount: aws.Int32(250), DefaultForAz: aws.Bool(false)},
		},
		routeTables: []ec2types.RouteTable{
			{RouteTableId: aws.String("rtb-bbb"), Routes: []ec2types.Route{{GatewayId: aws.String("igw-123")}}, Associations: []ec2types.RouteTableAssociation{}},
		},
	}
	p := awspkg.NewEC2VPCProviderWithClient(stub)
	item := awspkg.Item{
		ID:   "vpc-111",
		Name: "vpc-111",
		Meta: map[string]string{
			"cidr":       "10.0.0.0/16",
			"state":      "available",
			"is_default": "true",
			"owner_id":   "123456789012",
			"dhcp_opts":  "dopt-abc",
		},
	}

	cases := []struct {
		label string
		want  string
	}{
		{"Overview", "10.0.0.0/16"},
		{"Subnets", "subnet-aaa"},
		{"Route Tables", "rtb-bbb"},
	}
	tabs := p.Tabs()
	if len(tabs) != len(cases) {
		t.Fatalf("want %d tabs, got %d", len(cases), len(tabs))
	}
	for i, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			out, err := tabs[i].Fetch(context.Background(), item)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("tab %q: want %q in output, got:\n%s", tc.label, tc.want, out)
			}
		})
	}
}

func TestEC2VPCProvider_Tabs_Empty(t *testing.T) {
	stub := &stubVpc{}
	p := awspkg.NewEC2VPCProviderWithClient(stub)
	item := awspkg.Item{
		ID:   "vpc-empty",
		Name: "vpc-empty",
		Meta: map[string]string{"cidr": "", "state": "", "is_default": "false", "owner_id": "", "dhcp_opts": ""},
	}
	for _, tab := range p.Tabs() {
		out, err := tab.Fetch(context.Background(), item)
		if err != nil {
			t.Errorf("tab %q returned error: %v", tab.Label, err)
		}
		if out == "" {
			t.Errorf("tab %q returned empty string", tab.Label)
		}
	}
}
