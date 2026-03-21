package aws_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

type stubImages struct {
	images           []ec2types.Image
	launchPermissions []ec2types.LaunchPermission
}

func (s *stubImages) DescribeImages(_ context.Context, _ *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return &ec2.DescribeImagesOutput{Images: s.images}, nil
}

func (s *stubImages) DescribeImageAttribute(_ context.Context, _ *ec2.DescribeImageAttributeInput, _ ...func(*ec2.Options)) (*ec2.DescribeImageAttributeOutput, error) {
	return &ec2.DescribeImageAttributeOutput{LaunchPermissions: s.launchPermissions}, nil
}

func TestEC2ImagesProvider_ListItems(t *testing.T) {
	stub := &stubImages{
		images: []ec2types.Image{
			{
				ImageId:      aws.String("ami-111"),
				Name:         aws.String("my-golden-ami"),
				State:        ec2types.ImageStateAvailable,
				Architecture: ec2types.ArchitectureValuesX8664,
			},
		},
	}
	p := awspkg.NewEC2ImagesProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].ID != "ami-111" {
		t.Errorf("want ID=ami-111, got %s", items[0].ID)
	}
	if items[0].Name != "my-golden-ami" {
		t.Errorf("want Name=my-golden-ami, got %s", items[0].Name)
	}
}

func TestEC2ImagesProvider_ListItems_Filter(t *testing.T) {
	stub := &stubImages{
		images: []ec2types.Image{
			{ImageId: aws.String("ami-111"), Name: aws.String("prod-ami"), State: ec2types.ImageStateAvailable, Architecture: ec2types.ArchitectureValuesX8664},
			{ImageId: aws.String("ami-222"), Name: aws.String("dev-ami"), State: ec2types.ImageStateAvailable, Architecture: ec2types.ArchitectureValuesArm64},
		},
	}
	p := awspkg.NewEC2ImagesProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "dev-ami" {
		t.Errorf("filter expected [dev-ami], got %v", items)
	}
}

func TestEC2ImagesProvider_Tabs(t *testing.T) {
	stub := &stubImages{
		images: []ec2types.Image{
			{
				ImageId:            aws.String("ami-111"),
				Name:               aws.String("my-ami"),
				State:              ec2types.ImageStateAvailable,
				Architecture:       ec2types.ArchitectureValuesX8664,
				VirtualizationType: ec2types.VirtualizationTypeHvm,
				RootDeviceType:     ec2types.DeviceTypeEbs,
				Public:             aws.Bool(false),
				CreationDate:       aws.String("2025-01-15T12:00:00.000Z"),
				BlockDeviceMappings: []ec2types.BlockDeviceMapping{
					{
						DeviceName: aws.String("/dev/xvda"),
						Ebs: &ec2types.EbsBlockDevice{
							SnapshotId:          aws.String("snap-xyz"),
							VolumeSize:          aws.Int32(20),
							VolumeType:          ec2types.VolumeTypeGp3,
							DeleteOnTermination: aws.Bool(true),
						},
					},
				},
			},
		},
		launchPermissions: []ec2types.LaunchPermission{
			{UserId: aws.String("987654321098")},
		},
	}
	p := awspkg.NewEC2ImagesProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	item := items[0]

	cases := []struct {
		label string
		want  string
	}{
		{"Overview", "x86_64"},
		{"Block Devices", "snap-xyz"},
		{"Launch Permissions", "987654321098"},
	}
	tabs := p.Tabs()
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

func TestEC2ImagesProvider_Tab_NoBlockDevices(t *testing.T) {
	stub := &stubImages{
		images: []ec2types.Image{
			{ImageId: aws.String("ami-222"), Name: aws.String("empty-ami"), State: ec2types.ImageStateAvailable, Architecture: ec2types.ArchitectureValuesX8664},
		},
	}
	p := awspkg.NewEC2ImagesProviderWithClient(stub)
	items, _ := p.ListItems(context.Background(), "")
	out, err := p.Tabs()[1].Fetch(context.Background(), items[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no block devices") {
		t.Errorf("expected 'no block devices' message, got: %s", out)
	}
}

func TestEC2ImagesProvider_Tab_PrivateImage(t *testing.T) {
	stub := &stubImages{
		images: []ec2types.Image{
			{ImageId: aws.String("ami-333"), Name: aws.String("private-ami"), State: ec2types.ImageStateAvailable, Architecture: ec2types.ArchitectureValuesX8664},
		},
		launchPermissions: []ec2types.LaunchPermission{},
	}
	p := awspkg.NewEC2ImagesProviderWithClient(stub)
	items, _ := p.ListItems(context.Background(), "")
	out, err := p.Tabs()[2].Fetch(context.Background(), items[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "private") {
		t.Errorf("expected 'private' message for no launch permissions, got: %s", out)
	}
}
