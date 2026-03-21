package aws

import (
	"context"
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/rds"
)

type RDSAPI interface {
	DescribeDBInstances(ctx context.Context, in *rds.DescribeDBInstancesInput, opts ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	DescribeDBSnapshots(ctx context.Context, in *rds.DescribeDBSnapshotsInput, opts ...func(*rds.Options)) (*rds.DescribeDBSnapshotsOutput, error)
}

type RDSProvider struct {
	client  RDSAPI
	metrics CloudWatchMetricsAPI
}

func NewRDSProvider(cfg awssdk.Config, endpointURL string) *RDSProvider {
	var opts []func(*rds.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *rds.Options) { o.BaseEndpoint = awssdk.String(endpointURL) })
	}
	return &RDSProvider{client: rds.NewFromConfig(cfg, opts...), metrics: cloudwatch.NewFromConfig(cfg)}
}

func NewRDSProviderWithClient(client RDSAPI) *RDSProvider { return &RDSProvider{client: client} }

func (p *RDSProvider) Name() string { return "RDS" }

func (p *RDSProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{})
	if err != nil {
		return nil, fmt.Errorf("describe db instances: %w", err)
	}
	items := make([]Item, len(out.DBInstances))
	for i, db := range out.DBInstances {
		id := awssdk.ToString(db.DBInstanceIdentifier)
		engine := awssdk.ToString(db.Engine) + " " + awssdk.ToString(db.EngineVersion)
		endpoint, port := "", ""
		if db.Endpoint != nil {
			endpoint = awssdk.ToString(db.Endpoint.Address)
			port = fmt.Sprintf("%d", awssdk.ToInt32(db.Endpoint.Port))
		}
		items[i] = Item{
			ID:   id,
			Name: id,
			Meta: map[string]string{
				"engine":          engine,
				"engine_type":     awssdk.ToString(db.Engine),
				"status":          awssdk.ToString(db.DBInstanceStatus),
				"endpoint":        endpoint,
				"port":            port,
				"master_username": awssdk.ToString(db.MasterUsername),
			},
		}
	}
	return filterItems(items, query), nil
}

func (p *RDSProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *RDSProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Connectivity", Fetch: p.tabConnectivity},
		{Label: "Config", Fetch: p.tabConfig},
		{Label: "Snapshots", Fetch: p.tabSnapshots},
		{Label: "Metrics", Fetch: p.tabMetrics},
	}
}

func (p *RDSProvider) tabMetrics(ctx context.Context, item Item) (string, error) {
	specs := []metricSpec{
		{id: "cpu", label: "CPU Utilization", ns: "AWS/RDS", name: "CPUUtilization", stat: "Average", dimKey: "DBInstanceIdentifier", dimVal: item.ID, unit: "%"},
		{id: "conns", label: "Connections", ns: "AWS/RDS", name: "DatabaseConnections", stat: "Average", dimKey: "DBInstanceIdentifier", dimVal: item.ID},
		{id: "storage", label: "Free Storage", ns: "AWS/RDS", name: "FreeStorageSpace", stat: "Average", dimKey: "DBInstanceIdentifier", dimVal: item.ID, unit: "B"},
		{id: "readiops", label: "Read IOPS", ns: "AWS/RDS", name: "ReadIOPS", stat: "Average", dimKey: "DBInstanceIdentifier", dimVal: item.ID, unit: "/s"},
		{id: "writeiops", label: "Write IOPS", ns: "AWS/RDS", name: "WriteIOPS", stat: "Average", dimKey: "DBInstanceIdentifier", dimVal: item.ID, unit: "/s"},
	}
	data, err := fetchSparklines(ctx, p.metrics, specs, 6, 300)
	if err != nil {
		return "", err
	}
	return renderMetricsTab(specs, data, 6, 300), nil
}

func (p *RDSProvider) describeInstance(ctx context.Context, id string) (*rds.DescribeDBInstancesOutput, error) {
	return p.client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: awssdk.String(id),
	})
}

func (p *RDSProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.describeInstance(ctx, item.ID)
	if err != nil {
		return "", err
	}
	if len(out.DBInstances) == 0 {
		return "  (instance not found)\n", nil
	}
	db := out.DBInstances[0]

	multiAZ := "No"
	if awssdk.ToBool(db.MultiAZ) {
		multiAZ = "Yes"
	}
	publicAccess := "No"
	if awssdk.ToBool(db.PubliclyAccessible) {
		publicAccess = "Yes"
	}
	autoMinor := "No"
	if awssdk.ToBool(db.AutoMinorVersionUpgrade) {
		autoMinor = "Yes"
	}
	iops := "—"
	if db.Iops != nil {
		iops = fmt.Sprintf("%d", awssdk.ToInt32(db.Iops))
	}

	return KV([][2]string{
		{"Identifier", awssdk.ToString(db.DBInstanceIdentifier)},
		{"Class", awssdk.ToString(db.DBInstanceClass)},
		{"Engine", awssdk.ToString(db.Engine) + " " + awssdk.ToString(db.EngineVersion)},
		{"Status", awssdk.ToString(db.DBInstanceStatus)},
		{"Multi-AZ", multiAZ},
		{"Storage Type", awssdk.ToString(db.StorageType)},
		{"Storage GB", fmt.Sprintf("%d", awssdk.ToInt32(db.AllocatedStorage))},
		{"IOPS", iops},
		{"AZ", awssdk.ToString(db.AvailabilityZone)},
		{"Auto Minor Upgrade", autoMinor},
		{"Public Access", publicAccess},
	}), nil
}

func (p *RDSProvider) tabConnectivity(ctx context.Context, item Item) (string, error) {
	out, err := p.describeInstance(ctx, item.ID)
	if err != nil {
		return "", err
	}
	if len(out.DBInstances) == 0 {
		return "  (instance not found)\n", nil
	}
	db := out.DBInstances[0]

	address, port := "—", "—"
	if db.Endpoint != nil {
		address = awssdk.ToString(db.Endpoint.Address)
		port = fmt.Sprintf("%d", awssdk.ToInt32(db.Endpoint.Port))
	}
	vpcID, subnetGroup := "—", "—"
	if db.DBSubnetGroup != nil {
		subnetGroup = awssdk.ToString(db.DBSubnetGroup.DBSubnetGroupName)
		vpcID = awssdk.ToString(db.DBSubnetGroup.VpcId)
	}
	sgIDs := make([]string, len(db.VpcSecurityGroups))
	for i, sg := range db.VpcSecurityGroups {
		sgIDs[i] = awssdk.ToString(sg.VpcSecurityGroupId)
	}
	sgs := "—"
	if len(sgIDs) > 0 {
		sgs = strings.Join(sgIDs, ", ")
	}

	return KV([][2]string{
		{"Endpoint", address},
		{"Port", port},
		{"VPC ID", vpcID},
		{"Subnet Group", subnetGroup},
		{"Security Groups", sgs},
		{"CA Certificate", awssdk.ToString(db.CACertificateIdentifier)},
	}), nil
}

func (p *RDSProvider) tabConfig(ctx context.Context, item Item) (string, error) {
	out, err := p.describeInstance(ctx, item.ID)
	if err != nil {
		return "", err
	}
	if len(out.DBInstances) == 0 {
		return "  (instance not found)\n", nil
	}
	db := out.DBInstances[0]

	paramGroup := "—"
	if len(db.DBParameterGroups) > 0 {
		paramGroup = awssdk.ToString(db.DBParameterGroups[0].DBParameterGroupName)
	}
	optionGroup := "—"
	if len(db.OptionGroupMemberships) > 0 {
		optionGroup = awssdk.ToString(db.OptionGroupMemberships[0].OptionGroupName)
	}
	deletionProtection := "No"
	if awssdk.ToBool(db.DeletionProtection) {
		deletionProtection = "Yes"
	}
	copyTags := "No"
	if awssdk.ToBool(db.CopyTagsToSnapshot) {
		copyTags = "Yes"
	}

	return KV([][2]string{
		{"Master Username", awssdk.ToString(db.MasterUsername)},
		{"DB Name", awssdk.ToString(db.DBName)},
		{"Parameter Group", paramGroup},
		{"Option Group", optionGroup},
		{"Backup Retention", fmt.Sprintf("%d days", awssdk.ToInt32(db.BackupRetentionPeriod))},
		{"Backup Window", awssdk.ToString(db.PreferredBackupWindow)},
		{"Maintenance Window", awssdk.ToString(db.PreferredMaintenanceWindow)},
		{"Deletion Protection", deletionProtection},
		{"Copy Tags", copyTags},
	}), nil
}

func (p *RDSProvider) tabSnapshots(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeDBSnapshots(ctx, &rds.DescribeDBSnapshotsInput{
		DBInstanceIdentifier: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	if len(out.DBSnapshots) == 0 {
		return "  (no snapshots)\n", nil
	}
	rows := make([][]string, len(out.DBSnapshots))
	for i, snap := range out.DBSnapshots {
		rows[i] = []string{
			awssdk.ToString(snap.DBSnapshotIdentifier),
			awssdk.ToString(snap.SnapshotType),
			awssdk.ToString(snap.Status),
			fmt.Sprintf("%d", awssdk.ToInt32(snap.AllocatedStorage)),
		}
	}
	return Table([]string{"Identifier", "Type", "Status", "Size GB"}, rows), nil
}
