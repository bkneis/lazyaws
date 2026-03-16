package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

type stubEC2 struct {
	reservations []ec2types.Reservation
}

func (s *stubEC2) DescribeInstances(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return &ec2.DescribeInstancesOutput{Reservations: s.reservations}, nil
}

func testInstance() ec2types.Instance {
	launched := time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC)
	return ec2types.Instance{
		InstanceId:       aws.String("i-0abc123def456789"),
		InstanceType:     ec2types.InstanceTypeT3Medium,
		ImageId:          aws.String("ami-0abcdef1234567890"),
		Architecture:     ec2types.ArchitectureValuesX8664,
		PrivateIpAddress: aws.String("10.0.1.50"),
		PublicIpAddress:  aws.String("54.123.45.67"),
		VpcId:            aws.String("vpc-12345678"),
		SubnetId:         aws.String("subnet-87654321"),
		LaunchTime:       &launched,
		State:            &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
		Placement:        &ec2types.Placement{AvailabilityZone: aws.String("us-east-1a")},
		Tags: []ec2types.Tag{
			{Key: aws.String("Name"), Value: aws.String("my-web-server")},
			{Key: aws.String("Environment"), Value: aws.String("production")},
		},
		SecurityGroups: []ec2types.GroupIdentifier{
			{GroupId: aws.String("sg-0abc12345"), GroupName: aws.String("web-servers")},
		},
		BlockDeviceMappings: []ec2types.InstanceBlockDeviceMapping{
			{DeviceName: aws.String("/dev/xvda"), Ebs: &ec2types.EbsInstanceBlockDevice{
				VolumeId: aws.String("vol-0abc123def456789"),
				Status:   ec2types.AttachmentStatusAttached,
			}},
		},
	}
}

func TestEC2Provider_ListItems(t *testing.T) {
	inst := testInstance()
	cases := []struct {
		name         string
		reservations []ec2types.Reservation
		query        string
		wantCount    int
		wantName     string
	}{
		{"by tag name", []ec2types.Reservation{{Instances: []ec2types.Instance{inst}}}, "", 1, "my-web-server"},
		{"filter match", []ec2types.Reservation{{Instances: []ec2types.Instance{inst}}}, "web", 1, "my-web-server"},
		{"filter no match", []ec2types.Reservation{{Instances: []ec2types.Instance{inst}}}, "xyz", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := awspkg.NewEC2ProviderWithClient(&stubEC2{reservations: tc.reservations})
			items, err := p.ListItems(context.Background(), tc.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(items) != tc.wantCount {
				t.Errorf("got %d items, want %d", len(items), tc.wantCount)
			}
			if tc.wantName != "" && len(items) > 0 && items[0].Name != tc.wantName {
				t.Errorf("got name %q, want %q", items[0].Name, tc.wantName)
			}
		})
	}
}

func TestEC2Provider_Tabs(t *testing.T) {
	inst := testInstance()
	p := awspkg.NewEC2ProviderWithClient(&stubEC2{reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{inst}}}})
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	item := items[0]
	tabs := p.Tabs()

	cases := []struct {
		tabIdx int
		label  string
		want   string
	}{
		{0, "Overview", "t3.medium"},
		{1, "Network", "10.0.1.50"},
		{2, "Storage", "/dev/xvda"},
		{3, "Tags", "production"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if tabs[tc.tabIdx].Label != tc.label {
				t.Errorf("tab %d label = %q, want %q", tc.tabIdx, tabs[tc.tabIdx].Label, tc.label)
			}
			content, err := tabs[tc.tabIdx].Fetch(context.Background(), item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(content, tc.want) {
				t.Errorf("tab %q missing %q\ngot:\n%s", tc.label, tc.want, content)
			}
		})
	}
}
