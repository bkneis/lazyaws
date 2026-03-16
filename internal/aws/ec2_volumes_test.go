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

type stubVolumes struct {
	volumes   []ec2types.Volume
	snapshots []ec2types.Snapshot
}

func (s *stubVolumes) DescribeVolumes(_ context.Context, _ *ec2.DescribeVolumesInput, _ ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	return &ec2.DescribeVolumesOutput{Volumes: s.volumes}, nil
}

func (s *stubVolumes) DescribeSnapshots(_ context.Context, _ *ec2.DescribeSnapshotsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSnapshotsOutput, error) {
	return &ec2.DescribeSnapshotsOutput{Snapshots: s.snapshots}, nil
}

func TestEC2VolumesProvider_ListItems(t *testing.T) {
	now := time.Now()
	stub := &stubVolumes{
		volumes: []ec2types.Volume{
			{
				VolumeId:         aws.String("vol-111"),
				Size:             aws.Int32(100),
				State:            ec2types.VolumeStateInUse,
				VolumeType:       ec2types.VolumeTypeGp3,
				AvailabilityZone: aws.String("us-east-1a"),
				Encrypted:        aws.Bool(true),
				Tags: []ec2types.Tag{
					{Key: aws.String("Name"), Value: aws.String("my-vol")},
				},
				Attachments: []ec2types.VolumeAttachment{
					{InstanceId: aws.String("i-abc"), Device: aws.String("/dev/xvda"), State: ec2types.VolumeAttachmentStateAttached, AttachTime: &now},
				},
			},
		},
	}
	p := awspkg.NewEC2VolumesProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].ID != "vol-111" {
		t.Errorf("want ID=vol-111, got %s", items[0].ID)
	}
	if items[0].Name != "my-vol" {
		t.Errorf("want Name=my-vol, got %s", items[0].Name)
	}
}

func TestEC2VolumesProvider_ListItems_Filter(t *testing.T) {
	stub := &stubVolumes{
		volumes: []ec2types.Volume{
			{VolumeId: aws.String("vol-111"), Size: aws.Int32(10), State: ec2types.VolumeStateAvailable, VolumeType: ec2types.VolumeTypeGp2, AvailabilityZone: aws.String("us-east-1a"),
				Tags: []ec2types.Tag{{Key: aws.String("Name"), Value: aws.String("data-vol")}}},
			{VolumeId: aws.String("vol-222"), Size: aws.Int32(20), State: ec2types.VolumeStateAvailable, VolumeType: ec2types.VolumeTypeGp2, AvailabilityZone: aws.String("us-east-1b"),
				Tags: []ec2types.Tag{{Key: aws.String("Name"), Value: aws.String("root-vol")}}},
		},
	}
	p := awspkg.NewEC2VolumesProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "data")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "data-vol" {
		t.Errorf("filter expected [data-vol], got %v", items)
	}
}

func TestEC2VolumesProvider_Tabs(t *testing.T) {
	now := time.Now()
	stub := &stubVolumes{
		volumes: []ec2types.Volume{
			{
				VolumeId:           aws.String("vol-111"),
				Size:               aws.Int32(100),
				State:              ec2types.VolumeStateInUse,
				VolumeType:         ec2types.VolumeTypeGp3,
				AvailabilityZone:   aws.String("us-east-1a"),
				Encrypted:          aws.Bool(true),
				Iops:               aws.Int32(3000),
				Throughput:         aws.Int32(125),
				MultiAttachEnabled: aws.Bool(false),
				Attachments: []ec2types.VolumeAttachment{
					{InstanceId: aws.String("i-abc"), Device: aws.String("/dev/xvda"), State: ec2types.VolumeAttachmentStateAttached, AttachTime: &now},
				},
			},
		},
		snapshots: []ec2types.Snapshot{
			{SnapshotId: aws.String("snap-abc"), State: ec2types.SnapshotStateCompleted, Description: aws.String("backup"), StartTime: &now},
		},
	}
	p := awspkg.NewEC2VolumesProviderWithClient(stub)

	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	item := items[0]

	cases := []struct {
		label string
		want  string
	}{
		{"Overview", "gp3"},
		{"Attachments", "i-abc"},
		{"Snapshots", "snap-abc"},
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

func TestEC2VolumesProvider_Tab_NoAttachments(t *testing.T) {
	stub := &stubVolumes{
		volumes: []ec2types.Volume{
			{VolumeId: aws.String("vol-222"), Size: aws.Int32(50), State: ec2types.VolumeStateAvailable, VolumeType: ec2types.VolumeTypeGp2, AvailabilityZone: aws.String("us-east-1a")},
		},
	}
	p := awspkg.NewEC2VolumesProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.Tabs()[1].Fetch(context.Background(), items[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not attached") {
		t.Errorf("expected 'not attached' message, got: %s", out)
	}
}
