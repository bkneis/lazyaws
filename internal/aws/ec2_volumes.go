package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// VolumesAPI is the subset of EC2 client methods used by EC2VolumesProvider.
type VolumesAPI interface {
	DescribeVolumes(ctx context.Context, in *ec2.DescribeVolumesInput, opts ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
	DescribeSnapshots(ctx context.Context, in *ec2.DescribeSnapshotsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSnapshotsOutput, error)
}

// EC2VolumesProvider implements Provider for Amazon EBS Volumes.
type EC2VolumesProvider struct {
	client VolumesAPI
}

func NewEC2VolumesProvider(cfg awssdk.Config, endpointURL string) *EC2VolumesProvider {
	var opts []func(*ec2.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *ec2.Options) {
			o.BaseEndpoint = awssdk.String(endpointURL)
		})
	}
	return &EC2VolumesProvider{client: ec2.NewFromConfig(cfg, opts...)}
}

func NewEC2VolumesProviderWithClient(client VolumesAPI) *EC2VolumesProvider {
	return &EC2VolumesProvider{client: client}
}

func (p *EC2VolumesProvider) Name() string { return "EC2 Volumes" }

func (p *EC2VolumesProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	var items []Item
	var nextToken *string
	for {
		out, err := p.client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe volumes: %w", err)
		}
		for _, vol := range out.Volumes {
			id := awssdk.ToString(vol.VolumeId)
			name := ec2NameTag(vol.Tags)
			if name == "" {
				name = id
			}
			volJSON, _ := json.Marshal(vol)
			items = append(items, Item{
				ID:   id,
				Name: name,
				Meta: map[string]string{
					"size_gb":    strconv.Itoa(int(awssdk.ToInt32(vol.Size))),
					"state":      string(vol.State),
					"type":       string(vol.VolumeType),
					"az":         awssdk.ToString(vol.AvailabilityZone),
					"volume_json": string(volJSON),
				},
			})
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return filterItems(items, query), nil
}

func (p *EC2VolumesProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *EC2VolumesProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Attachments", Fetch: p.tabAttachments},
		{Label: "Snapshots", Fetch: p.tabSnapshots},
	}
}

func (p *EC2VolumesProvider) volumeFromMeta(item Item) (*ec2types.Volume, error) {
	raw, ok := item.Meta["volume_json"]
	if !ok || raw == "" {
		return nil, fmt.Errorf("volume data not available")
	}
	var vol ec2types.Volume
	if err := json.Unmarshal([]byte(raw), &vol); err != nil {
		return nil, fmt.Errorf("parse volume: %w", err)
	}
	return &vol, nil
}

func (p *EC2VolumesProvider) tabOverview(_ context.Context, item Item) (string, error) {
	vol, err := p.volumeFromMeta(item)
	if err != nil {
		return "", err
	}
	iops := "-"
	if vol.Iops != nil {
		iops = strconv.Itoa(int(awssdk.ToInt32(vol.Iops)))
	}
	throughput := "-"
	if vol.Throughput != nil {
		throughput = strconv.Itoa(int(awssdk.ToInt32(vol.Throughput))) + " MiB/s"
	}
	return KV([][2]string{
		{"Volume ID", awssdk.ToString(vol.VolumeId)},
		{"Size (GiB)", strconv.Itoa(int(awssdk.ToInt32(vol.Size)))},
		{"Type", string(vol.VolumeType)},
		{"IOPS", iops},
		{"Throughput", throughput},
		{"State", string(vol.State)},
		{"AZ", awssdk.ToString(vol.AvailabilityZone)},
		{"Encrypted", strconv.FormatBool(awssdk.ToBool(vol.Encrypted))},
		{"Multi-Attach", strconv.FormatBool(awssdk.ToBool(vol.MultiAttachEnabled))},
	}), nil
}

func (p *EC2VolumesProvider) tabAttachments(_ context.Context, item Item) (string, error) {
	vol, err := p.volumeFromMeta(item)
	if err != nil {
		return "", err
	}
	if len(vol.Attachments) == 0 {
		return "  (not attached)\n", nil
	}
	rows := make([][]string, len(vol.Attachments))
	for i, a := range vol.Attachments {
		attachTime := ""
		if a.AttachTime != nil {
			attachTime = a.AttachTime.Format(time.DateTime)
		}
		rows[i] = []string{
			awssdk.ToString(a.InstanceId),
			awssdk.ToString(a.Device),
			string(a.State),
			attachTime,
		}
	}
	return Table([]string{"Instance ID", "Device", "State", "Attach Time"}, rows), nil
}

func (p *EC2VolumesProvider) tabSnapshots(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
		Filters: []ec2types.Filter{
			{Name: awssdk.String("volume-id"), Values: []string{item.ID}},
		},
	})
	if err != nil {
		return "", err
	}
	if len(out.Snapshots) == 0 {
		return "  (no snapshots)\n", nil
	}
	rows := make([][]string, len(out.Snapshots))
	for i, s := range out.Snapshots {
		startTime := ""
		if s.StartTime != nil {
			startTime = s.StartTime.Format(time.DateTime)
		}
		rows[i] = []string{
			awssdk.ToString(s.SnapshotId),
			string(s.State),
			startTime,
			awssdk.ToString(s.Description),
		}
	}
	return Table([]string{"Snapshot ID", "State", "Start Time", "Description"}, rows), nil
}
