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

type stubSG struct {
	groups []ec2types.SecurityGroup
}

func (s *stubSG) DescribeSecurityGroups(_ context.Context, _ *ec2.DescribeSecurityGroupsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: s.groups}, nil
}

func TestEC2SGProvider_ListItems(t *testing.T) {
	stub := &stubSG{
		groups: []ec2types.SecurityGroup{
			{
				GroupId:     aws.String("sg-111"),
				GroupName:   aws.String("web-sg"),
				Description: aws.String("Web tier"),
				VpcId:       aws.String("vpc-abc"),
				OwnerId:     aws.String("123456789012"),
			},
		},
	}
	p := awspkg.NewEC2SGProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].ID != "sg-111" {
		t.Errorf("want ID=sg-111, got %s", items[0].ID)
	}
	if items[0].Name != "web-sg" {
		t.Errorf("want Name=web-sg, got %s", items[0].Name)
	}
}

func TestEC2SGProvider_ListItems_Filter(t *testing.T) {
	stub := &stubSG{
		groups: []ec2types.SecurityGroup{
			{GroupId: aws.String("sg-111"), GroupName: aws.String("web-sg"), Description: aws.String("Web"), VpcId: aws.String("vpc-abc"), OwnerId: aws.String("123456789012")},
			{GroupId: aws.String("sg-222"), GroupName: aws.String("db-sg"), Description: aws.String("DB"), VpcId: aws.String("vpc-abc"), OwnerId: aws.String("123456789012")},
		},
	}
	p := awspkg.NewEC2SGProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "db")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "db-sg" {
		t.Errorf("filter expected [db-sg], got %v", items)
	}
}

func TestEC2SGProvider_Tabs(t *testing.T) {
	stub := &stubSG{
		groups: []ec2types.SecurityGroup{
			{
				GroupId:     aws.String("sg-111"),
				GroupName:   aws.String("web-sg"),
				Description: aws.String("Web tier"),
				VpcId:       aws.String("vpc-abc"),
				OwnerId:     aws.String("123456789012"),
				IpPermissions: []ec2types.IpPermission{
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int32(443),
						ToPort:     aws.Int32(443),
						IpRanges: []ec2types.IpRange{
							{CidrIp: aws.String("0.0.0.0/0"), Description: aws.String("public HTTPS")},
						},
					},
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int32(8080),
						ToPort:     aws.Int32(8090),
						IpRanges: []ec2types.IpRange{
							{CidrIp: aws.String("10.0.0.0/8"), Description: aws.String("internal range")},
						},
					},
				},
				IpPermissionsEgress: []ec2types.IpPermission{
					{
						IpProtocol: aws.String("-1"),
						IpRanges: []ec2types.IpRange{
							{CidrIp: aws.String("0.0.0.0/0")},
						},
					},
				},
			},
		},
	}
	p := awspkg.NewEC2SGProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	item := items[0]

	cases := []struct {
		label string
		wants []string
	}{
		{"Overview", []string{"sg-111", "web-sg", "vpc-abc"}},
		{"Inbound Rules", []string{"443", "0.0.0.0/0", "8080-8090", "10.0.0.0/8"}},
		{"Outbound Rules", []string{"All", "0.0.0.0/0"}},
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
			for _, want := range tc.wants {
				if !strings.Contains(out, want) {
					t.Errorf("tab %q: want %q in output, got:\n%s", tc.label, want, out)
				}
			}
		})
	}
}

func TestEC2SGProvider_Tab_SGReference(t *testing.T) {
	stub := &stubSG{
		groups: []ec2types.SecurityGroup{
			{
				GroupId:     aws.String("sg-222"),
				GroupName:   aws.String("app-sg"),
				Description: aws.String("App tier"),
				VpcId:       aws.String("vpc-abc"),
				OwnerId:     aws.String("123456789012"),
				IpPermissions: []ec2types.IpPermission{
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int32(8080),
						ToPort:     aws.Int32(8080),
						UserIdGroupPairs: []ec2types.UserIdGroupPair{
							{GroupId: aws.String("sg-111"), GroupName: aws.String("web-sg"), Description: aws.String("from web tier")},
						},
					},
				},
			},
		},
	}
	p := awspkg.NewEC2SGProviderWithClient(stub)
	items, _ := p.ListItems(context.Background(), "")
	out, err := p.Tabs()[1].Fetch(context.Background(), items[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sg-111") {
		t.Errorf("expected referenced SG ID in output, got: %s", out)
	}
}

func TestEC2SGProvider_Tab_NoRules(t *testing.T) {
	stub := &stubSG{
		groups: []ec2types.SecurityGroup{
			{GroupId: aws.String("sg-333"), GroupName: aws.String("empty-sg"), Description: aws.String("Empty"), VpcId: aws.String("vpc-abc"), OwnerId: aws.String("123456789012")},
		},
	}
	p := awspkg.NewEC2SGProviderWithClient(stub)
	items, _ := p.ListItems(context.Background(), "")

	inbound, _ := p.Tabs()[1].Fetch(context.Background(), items[0])
	if !strings.Contains(inbound, "no inbound rules") {
		t.Errorf("expected 'no inbound rules', got: %s", inbound)
	}

	outbound, _ := p.Tabs()[2].Fetch(context.Background(), items[0])
	if !strings.Contains(outbound, "no outbound rules") {
		t.Errorf("expected 'no outbound rules', got: %s", outbound)
	}
}
