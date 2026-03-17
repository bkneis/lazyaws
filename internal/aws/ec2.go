package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// EC2API is the subset of the EC2 client methods used by EC2Provider.
type EC2API interface {
	DescribeInstances(ctx context.Context, in *ec2.DescribeInstancesInput, opts ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

// EC2Provider implements Provider for Amazon EC2 Instances.
type EC2Provider struct {
	client EC2API
}

func NewEC2Provider(cfg awssdk.Config, endpointURL string) *EC2Provider {
	var opts []func(*ec2.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *ec2.Options) {
			o.BaseEndpoint = awssdk.String(endpointURL)
		})
	}
	return &EC2Provider{client: ec2.NewFromConfig(cfg, opts...)}
}

func NewEC2ProviderWithClient(client EC2API) *EC2Provider {
	return &EC2Provider{client: client}
}

func (p *EC2Provider) Name() string { return "EC2" }

func (p *EC2Provider) ListItems(ctx context.Context, query string) ([]Item, error) {
	var items []Item
	var nextToken *string
	for {
		out, err := p.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe instances: %w", err)
		}
		for _, r := range out.Reservations {
			for _, inst := range r.Instances {
				id := awssdk.ToString(inst.InstanceId)
				name := ec2NameTag(inst.Tags)
				if name == "" {
					name = id
				}
				// Marshal instance JSON for zero-cost tab rendering
				instJSON, _ := json.Marshal(inst)
				items = append(items, Item{
					ID:   id,
					Name: name,
					Meta: map[string]string{
						"state":         string(inst.State.Name),
						"type":          string(inst.InstanceType),
						"instance_json": string(instJSON),
					},
				})
			}
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return filterItems(items, query), nil
}

func (p *EC2Provider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *EC2Provider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Network", Fetch: p.tabNetwork},
		{Label: "Storage", Fetch: p.tabStorage},
		{Label: "Tags", Fetch: p.tabTags},
	}
}

func (p *EC2Provider) instanceFromMeta(item Item) (*ec2types.Instance, error) {
	raw, ok := item.Meta["instance_json"]
	if !ok || raw == "" {
		return nil, fmt.Errorf("instance data not available")
	}
	var inst ec2types.Instance
	if err := json.Unmarshal([]byte(raw), &inst); err != nil {
		return nil, fmt.Errorf("parse instance: %w", err)
	}
	return &inst, nil
}

func (p *EC2Provider) tabOverview(_ context.Context, item Item) (string, error) {
	inst, err := p.instanceFromMeta(item)
	if err != nil {
		return "", err
	}

	launched := ""
	if inst.LaunchTime != nil {
		launched = inst.LaunchTime.Format(time.DateTime)
	}

	az := ""
	if inst.Placement != nil {
		az = awssdk.ToString(inst.Placement.AvailabilityZone)
	}

	platform := "Linux/UNIX"
	if inst.Platform != "" {
		platform = string(inst.Platform)
	}

	iamProfile := "-"
	if inst.IamInstanceProfile != nil {
		arn := awssdk.ToString(inst.IamInstanceProfile.Arn)
		iamProfile = Link(arnLastSegment(arn), "IAM Roles", arn)
	}

	return KV([][2]string{
		{"Instance ID", awssdk.ToString(inst.InstanceId)},
		{"State", string(inst.State.Name)},
		{"Type", string(inst.InstanceType)},
		{"AMI", awssdk.ToString(inst.ImageId)},
		{"Platform", platform},
		{"Architecture", string(inst.Architecture)},
		{"Launch Time", launched},
		{"AZ", az},
		{"Key Pair", awssdk.ToString(inst.KeyName)},
		{"IAM Profile", iamProfile},
	}), nil
}

func (p *EC2Provider) tabNetwork(_ context.Context, item Item) (string, error) {
	inst, err := p.instanceFromMeta(item)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(KV([][2]string{
		{"VPC", awssdk.ToString(inst.VpcId)},
		{"Subnet", awssdk.ToString(inst.SubnetId)},
		{"Private IP", awssdk.ToString(inst.PrivateIpAddress)},
		{"Public IP", awssdk.ToString(inst.PublicIpAddress)},
		{"Public DNS", awssdk.ToString(inst.PublicDnsName)},
	}))

	if len(inst.SecurityGroups) > 0 {
		sb.WriteString("\n  Security Groups\n")
		rows := make([][]string, len(inst.SecurityGroups))
		for i, sg := range inst.SecurityGroups {
			rows[i] = []string{awssdk.ToString(sg.GroupId), awssdk.ToString(sg.GroupName)}
		}
		sb.WriteString(Table([]string{"ID", "Name"}, rows))
	}

	return sb.String(), nil
}

func (p *EC2Provider) tabStorage(_ context.Context, item Item) (string, error) {
	inst, err := p.instanceFromMeta(item)
	if err != nil {
		return "", err
	}
	if len(inst.BlockDeviceMappings) == 0 {
		return "  (no block devices)\n", nil
	}
	rows := make([][]string, len(inst.BlockDeviceMappings))
	for i, bd := range inst.BlockDeviceMappings {
		volID := ""
		state := ""
		if bd.Ebs != nil {
			volID = awssdk.ToString(bd.Ebs.VolumeId)
			state = string(bd.Ebs.Status)
		}
		rows[i] = []string{awssdk.ToString(bd.DeviceName), volID, state}
	}
	return Table([]string{"Device", "Volume ID", "State"}, rows), nil
}

func (p *EC2Provider) tabTags(_ context.Context, item Item) (string, error) {
	inst, err := p.instanceFromMeta(item)
	if err != nil {
		return "", err
	}
	if len(inst.Tags) == 0 {
		return "  (no tags)\n", nil
	}
	rows := make([][]string, len(inst.Tags))
	for i, t := range inst.Tags {
		rows[i] = []string{awssdk.ToString(t.Key), awssdk.ToString(t.Value)}
	}
	return Table([]string{"Key", "Value"}, rows), nil
}

// ec2NameTag extracts the "Name" tag value from an EC2 tag list.
func ec2NameTag(tags []ec2types.Tag) string {
	for _, t := range tags {
		if awssdk.ToString(t.Key) == "Name" {
			return awssdk.ToString(t.Value)
		}
	}
	return ""
}
