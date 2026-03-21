package aws

import (
	"context"
	"encoding/json"
	"log"
	"fmt"
	"strconv"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// ImagesAPI is the subset of EC2 client methods used by EC2ImagesProvider.
type ImagesAPI interface {
	DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, opts ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
	DescribeImageAttribute(ctx context.Context, in *ec2.DescribeImageAttributeInput, opts ...func(*ec2.Options)) (*ec2.DescribeImageAttributeOutput, error)
}

// EC2ImagesProvider implements Provider for Amazon Machine Images (AMIs).
type EC2ImagesProvider struct {
	client ImagesAPI
}

func NewEC2ImagesProvider(cfg awssdk.Config, endpointURL string) *EC2ImagesProvider {
	var opts []func(*ec2.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *ec2.Options) {
			o.BaseEndpoint = awssdk.String(endpointURL)
		})
	}
	return &EC2ImagesProvider{client: ec2.NewFromConfig(cfg, opts...)}
}

func NewEC2ImagesProviderWithClient(client ImagesAPI) *EC2ImagesProvider {
	return &EC2ImagesProvider{client: client}
}

func (p *EC2ImagesProvider) Name() string { return "EC2 Images" }

func (p *EC2ImagesProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{"self"},
	})
	if err != nil {
		return nil, fmt.Errorf("describe images: %w", err)
	}
	items := make([]Item, len(out.Images))
	for i, img := range out.Images {
		id := awssdk.ToString(img.ImageId)
		name := awssdk.ToString(img.Name)
		if name == "" {
			name = id
		}
		imgJSON, jsonErr := json.Marshal(img)
		if jsonErr != nil {
			log.Printf("warn: marshal image %s: %v", id, jsonErr)
		}
		items[i] = Item{
			ID:   id,
			Name: name,
			Meta: map[string]string{
				"state":      string(img.State),
				"arch":       string(img.Architecture),
				"image_json": string(imgJSON),
			},
		}
	}
	return filterItems(items, query), nil
}

func (p *EC2ImagesProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *EC2ImagesProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Block Devices", Fetch: p.tabBlockDevices},
		{Label: "Launch Permissions", Fetch: p.tabLaunchPermissions},
	}
}

func (p *EC2ImagesProvider) imageFromMeta(item Item) (*ec2types.Image, error) {
	raw, ok := item.Meta["image_json"]
	if !ok || raw == "" {
		return nil, fmt.Errorf("image data not available")
	}
	var img ec2types.Image
	if err := json.Unmarshal([]byte(raw), &img); err != nil {
		return nil, fmt.Errorf("parse image: %w", err)
	}
	return &img, nil
}

func (p *EC2ImagesProvider) tabOverview(_ context.Context, item Item) (string, error) {
	img, err := p.imageFromMeta(item)
	if err != nil {
		return "", err
	}
	platform := awssdk.ToString(img.PlatformDetails)
	if platform == "" {
		platform = "-"
	}
	return KV([][2]string{
		{"Image ID", awssdk.ToString(img.ImageId)},
		{"Name", awssdk.ToString(img.Name)},
		{"State", string(img.State)},
		{"Architecture", string(img.Architecture)},
		{"Virtualization Type", string(img.VirtualizationType)},
		{"Root Device Type", string(img.RootDeviceType)},
		{"Platform", platform},
		{"Public", strconv.FormatBool(awssdk.ToBool(img.Public))},
		{"Creation Date", awssdk.ToString(img.CreationDate)},
	}), nil
}

func (p *EC2ImagesProvider) tabBlockDevices(_ context.Context, item Item) (string, error) {
	img, err := p.imageFromMeta(item)
	if err != nil {
		return "", err
	}
	if len(img.BlockDeviceMappings) == 0 {
		return "  (no block devices)\n", nil
	}
	rows := make([][]string, len(img.BlockDeviceMappings))
	for i, bd := range img.BlockDeviceMappings {
		snapshotID := "-"
		sizeGiB := "-"
		volType := "-"
		deleteOnTerm := "-"
		if bd.Ebs != nil {
			snapshotID = awssdk.ToString(bd.Ebs.SnapshotId)
			if bd.Ebs.VolumeSize != nil {
				sizeGiB = strconv.Itoa(int(awssdk.ToInt32(bd.Ebs.VolumeSize)))
			}
			volType = string(bd.Ebs.VolumeType)
			if bd.Ebs.DeleteOnTermination != nil {
				deleteOnTerm = strconv.FormatBool(awssdk.ToBool(bd.Ebs.DeleteOnTermination))
			}
		}
		rows[i] = []string{
			awssdk.ToString(bd.DeviceName),
			snapshotID,
			sizeGiB,
			volType,
			deleteOnTerm,
		}
	}
	return Table([]string{"Device", "Snapshot ID", "Size GiB", "Type", "Delete on Term"}, rows), nil
}

func (p *EC2ImagesProvider) tabLaunchPermissions(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeImageAttribute(ctx, &ec2.DescribeImageAttributeInput{
		ImageId:   awssdk.String(item.ID),
		Attribute: ec2types.ImageAttributeNameLaunchPermission,
	})
	if err != nil {
		return "", err
	}
	if len(out.LaunchPermissions) == 0 {
		return "  (private — no launch permissions)\n", nil
	}
	rows := make([][]string, len(out.LaunchPermissions))
	for i, lp := range out.LaunchPermissions {
		userID := awssdk.ToString(lp.UserId)
		group := string(lp.Group)
		entity := userID
		if entity == "" {
			entity = group
		}
		rows[i] = []string{entity}
	}
	return Table([]string{"User / Group"}, rows), nil
}
