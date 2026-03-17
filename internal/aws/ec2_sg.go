package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// SecurityGroupAPI is the subset of EC2 client methods used by EC2SGProvider.
type SecurityGroupAPI interface {
	DescribeSecurityGroups(ctx context.Context, in *ec2.DescribeSecurityGroupsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
}

// EC2SGProvider implements Provider for EC2 Security Groups.
type EC2SGProvider struct {
	client SecurityGroupAPI
}

func NewEC2SGProvider(cfg awssdk.Config, endpointURL string) *EC2SGProvider {
	var opts []func(*ec2.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *ec2.Options) {
			o.BaseEndpoint = awssdk.String(endpointURL)
		})
	}
	return &EC2SGProvider{client: ec2.NewFromConfig(cfg, opts...)}
}

func NewEC2SGProviderWithClient(client SecurityGroupAPI) *EC2SGProvider {
	return &EC2SGProvider{client: client}
}

func (p *EC2SGProvider) Name() string { return "Security Groups" }

func (p *EC2SGProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{})
	if err != nil {
		return nil, fmt.Errorf("describe security groups: %w", err)
	}
	items := make([]Item, len(out.SecurityGroups))
	for i, sg := range out.SecurityGroups {
		sgJSON, _ := json.Marshal(sg)
		items[i] = Item{
			ID:   awssdk.ToString(sg.GroupId),
			Name: awssdk.ToString(sg.GroupName),
			Meta: map[string]string{
				"description": awssdk.ToString(sg.Description),
				"vpc_id":      awssdk.ToString(sg.VpcId),
				"owner_id":    awssdk.ToString(sg.OwnerId),
				"sg_json":     string(sgJSON),
			},
		}
	}
	return filterItems(items, query), nil
}

func (p *EC2SGProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *EC2SGProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Inbound Rules", Fetch: p.tabInbound},
		{Label: "Outbound Rules", Fetch: p.tabOutbound},
	}
}

func (p *EC2SGProvider) sgFromMeta(item Item) (*ec2types.SecurityGroup, error) {
	raw, ok := item.Meta["sg_json"]
	if !ok || raw == "" {
		return nil, fmt.Errorf("security group data not available")
	}
	var sg ec2types.SecurityGroup
	if err := json.Unmarshal([]byte(raw), &sg); err != nil {
		return nil, fmt.Errorf("parse security group: %w", err)
	}
	return &sg, nil
}

func (p *EC2SGProvider) tabOverview(_ context.Context, item Item) (string, error) {
	return KV([][2]string{
		{"Group ID", item.ID},
		{"Name", item.Name},
		{"Description", item.Meta["description"]},
		{"VPC ID", item.Meta["vpc_id"]},
		{"Owner ID", item.Meta["owner_id"]},
	}), nil
}

func (p *EC2SGProvider) tabInbound(_ context.Context, item Item) (string, error) {
	sg, err := p.sgFromMeta(item)
	if err != nil {
		return "", err
	}
	if len(sg.IpPermissions) == 0 {
		return "  (no inbound rules)\n", nil
	}
	return formatRules(sg.IpPermissions, "Source"), nil
}

func (p *EC2SGProvider) tabOutbound(_ context.Context, item Item) (string, error) {
	sg, err := p.sgFromMeta(item)
	if err != nil {
		return "", err
	}
	if len(sg.IpPermissionsEgress) == 0 {
		return "  (no outbound rules)\n", nil
	}
	return formatRules(sg.IpPermissionsEgress, "Destination"), nil
}

// formatRules renders a slice of IpPermission as a table, expanding one row
// per source/destination so CIDRs and SG references are individually scannable.
func formatRules(perms []ec2types.IpPermission, sourceHeader string) string {
	var rows [][]string
	for _, perm := range perms {
		proto := formatProtocol(awssdk.ToString(perm.IpProtocol))
		portRange := formatPortRange(perm)

		sources := collectSources(perm)
		if len(sources) == 0 {
			rows = append(rows, []string{proto, portRange, "-", "-"})
			continue
		}
		for _, src := range sources {
			rows = append(rows, []string{proto, portRange, src[0], src[1]})
		}
	}
	return Table([]string{"Protocol", "Port Range", sourceHeader, "Description"}, rows)
}

// collectSources returns [source, description] pairs from all source types in a permission.
func collectSources(perm ec2types.IpPermission) [][2]string {
	var out [][2]string
	for _, r := range perm.IpRanges {
		out = append(out, [2]string{awssdk.ToString(r.CidrIp), awssdk.ToString(r.Description)})
	}
	for _, r := range perm.Ipv6Ranges {
		out = append(out, [2]string{awssdk.ToString(r.CidrIpv6), awssdk.ToString(r.Description)})
	}
	for _, r := range perm.UserIdGroupPairs {
		label := awssdk.ToString(r.GroupId)
		if n := awssdk.ToString(r.GroupName); n != "" {
			label = n + " (" + label + ")"
		}
		out = append(out, [2]string{label, awssdk.ToString(r.Description)})
	}
	for _, r := range perm.PrefixListIds {
		out = append(out, [2]string{awssdk.ToString(r.PrefixListId), awssdk.ToString(r.Description)})
	}
	return out
}

func formatProtocol(proto string) string {
	if proto == "-1" {
		return "All"
	}
	return strings.ToUpper(proto)
}

func formatPortRange(perm ec2types.IpPermission) string {
	if awssdk.ToString(perm.IpProtocol) == "-1" {
		return "All"
	}
	if perm.FromPort == nil {
		return "-"
	}
	from := awssdk.ToInt32(perm.FromPort)
	to := awssdk.ToInt32(perm.ToPort)
	if from == to {
		return fmt.Sprintf("%d", from)
	}
	return fmt.Sprintf("%d-%d", from, to)
}
