package aws_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

type stubRDS struct {
	instances []rdstypes.DBInstance
	snapshots []rdstypes.DBSnapshot
}

func (s *stubRDS) DescribeDBInstances(_ context.Context, _ *rds.DescribeDBInstancesInput, _ ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	return &rds.DescribeDBInstancesOutput{DBInstances: s.instances}, nil
}

func (s *stubRDS) DescribeDBSnapshots(_ context.Context, _ *rds.DescribeDBSnapshotsInput, _ ...func(*rds.Options)) (*rds.DescribeDBSnapshotsOutput, error) {
	return &rds.DescribeDBSnapshotsOutput{DBSnapshots: s.snapshots}, nil
}

func baseInstance() rdstypes.DBInstance {
	iops := int32(3000)
	port := int32(3306)
	return rdstypes.DBInstance{
		DBInstanceIdentifier:       aws.String("my-db"),
		DBInstanceClass:            aws.String("db.t3.medium"),
		Engine:                     aws.String("mysql"),
		EngineVersion:              aws.String("8.0.36"),
		DBInstanceStatus:           aws.String("available"),
		MultiAZ:                    aws.Bool(true),
		StorageType:                aws.String("gp3"),
		AllocatedStorage:           aws.Int32(100),
		Iops:                       &iops,
		AvailabilityZone:           aws.String("us-east-1a"),
		AutoMinorVersionUpgrade:    aws.Bool(true),
		PubliclyAccessible:         aws.Bool(false),
		MasterUsername:             aws.String("admin"),
		DBName:                     aws.String("mydb"),
		BackupRetentionPeriod:      aws.Int32(7),
		PreferredBackupWindow:      aws.String("03:00-04:00"),
		PreferredMaintenanceWindow: aws.String("sun:05:00-sun:06:00"),
		DeletionProtection:         aws.Bool(true),
		CopyTagsToSnapshot:         aws.Bool(true),
		CACertificateIdentifier:    aws.String("rds-ca-2019"),
		Endpoint: &rdstypes.Endpoint{
			Address: aws.String("my-db.abc123.us-east-1.rds.amazonaws.com"),
			Port:    &port,
		},
		DBSubnetGroup: &rdstypes.DBSubnetGroup{
			DBSubnetGroupName: aws.String("default-vpc"),
			VpcId:             aws.String("vpc-12345678"),
		},
		VpcSecurityGroups: []rdstypes.VpcSecurityGroupMembership{
			{VpcSecurityGroupId: aws.String("sg-aabbccdd")},
			{VpcSecurityGroupId: aws.String("sg-11223344")},
		},
		DBParameterGroups: []rdstypes.DBParameterGroupStatus{
			{DBParameterGroupName: aws.String("default.mysql8.0")},
		},
		OptionGroupMemberships: []rdstypes.OptionGroupMembership{
			{OptionGroupName: aws.String("default:mysql-8-0")},
		},
	}
}

func TestRDSProvider_ListItems(t *testing.T) {
	stub := &stubRDS{instances: []rdstypes.DBInstance{baseInstance()}}
	p := awspkg.NewRDSProviderWithClient(stub)

	cases := []struct {
		query string
		want  int
	}{
		{"", 1},
		{"my", 1},
		{"xyz", 0},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			items, err := p.ListItems(context.Background(), tc.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(items) != tc.want {
				t.Errorf("got %d items, want %d", len(items), tc.want)
			}
		})
	}
}

func TestRDSProvider_ListItems_Empty(t *testing.T) {
	p := awspkg.NewRDSProviderWithClient(&stubRDS{})
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("got %d items, want 0", len(items))
	}
}

func TestRDSProvider_TabOverview(t *testing.T) {
	stub := &stubRDS{instances: []rdstypes.DBInstance{baseInstance()}}
	p := awspkg.NewRDSProviderWithClient(stub)
	item := awspkg.Item{ID: "my-db", Name: "my-db"}

	cases := []struct {
		want string
	}{
		{"my-db"},
		{"db.t3.medium"},
		{"mysql 8.0.36"},
		{"available"},
		{"Multi-AZ"},
		{"Yes"},
		{"gp3"},
		{"100"},
		{"3000"},
		{"us-east-1a"},
	}
	content, err := p.Tabs()[0].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, tc := range cases {
		if !strings.Contains(content, tc.want) {
			t.Errorf("overview tab missing %q\ngot:\n%s", tc.want, content)
		}
	}
}

func TestRDSProvider_TabOverview_NoIOPS(t *testing.T) {
	db := baseInstance()
	db.Iops = nil
	stub := &stubRDS{instances: []rdstypes.DBInstance{db}}
	p := awspkg.NewRDSProviderWithClient(stub)
	item := awspkg.Item{ID: "my-db", Name: "my-db"}

	content, err := p.Tabs()[0].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "—") {
		t.Errorf("expected — for missing IOPS\ngot:\n%s", content)
	}
}

func TestRDSProvider_TabConnectivity(t *testing.T) {
	stub := &stubRDS{instances: []rdstypes.DBInstance{baseInstance()}}
	p := awspkg.NewRDSProviderWithClient(stub)
	item := awspkg.Item{ID: "my-db", Name: "my-db"}

	cases := []struct {
		want string
	}{
		{"my-db.abc123.us-east-1.rds.amazonaws.com"},
		{"3306"},
		{"vpc-12345678"},
		{"default-vpc"},
		{"sg-aabbccdd"},
		{"sg-11223344"},
		{"rds-ca-2019"},
	}
	content, err := p.Tabs()[1].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, tc := range cases {
		if !strings.Contains(content, tc.want) {
			t.Errorf("connectivity tab missing %q\ngot:\n%s", tc.want, content)
		}
	}
}

func TestRDSProvider_TabConnectivity_NoVPC(t *testing.T) {
	db := baseInstance()
	db.DBSubnetGroup = nil
	db.Endpoint = nil
	db.VpcSecurityGroups = nil
	stub := &stubRDS{instances: []rdstypes.DBInstance{db}}
	p := awspkg.NewRDSProviderWithClient(stub)
	item := awspkg.Item{ID: "my-db", Name: "my-db"}

	content, err := p.Tabs()[1].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// all optional fields should show —
	if strings.Count(content, "—") < 3 {
		t.Errorf("expected multiple — for missing fields\ngot:\n%s", content)
	}
}

func TestRDSProvider_TabConfig(t *testing.T) {
	stub := &stubRDS{instances: []rdstypes.DBInstance{baseInstance()}}
	p := awspkg.NewRDSProviderWithClient(stub)
	item := awspkg.Item{ID: "my-db", Name: "my-db"}

	cases := []struct {
		want string
	}{
		{"admin"},
		{"mydb"},
		{"default.mysql8.0"},
		{"default:mysql-8-0"},
		{"7 days"},
		{"03:00-04:00"},
		{"sun:05:00-sun:06:00"},
		{"Deletion Protection"},
		{"Copy Tags"},
	}
	content, err := p.Tabs()[2].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, tc := range cases {
		if !strings.Contains(content, tc.want) {
			t.Errorf("config tab missing %q\ngot:\n%s", tc.want, content)
		}
	}
}

func TestRDSProvider_TabSnapshots(t *testing.T) {
	stub := &stubRDS{
		instances: []rdstypes.DBInstance{baseInstance()},
		snapshots: []rdstypes.DBSnapshot{
			{
				DBSnapshotIdentifier: aws.String("my-db-snap-1"),
				SnapshotType:         aws.String("automated"),
				Status:               aws.String("available"),
				AllocatedStorage:     aws.Int32(100),
			},
		},
	}
	p := awspkg.NewRDSProviderWithClient(stub)
	item := awspkg.Item{ID: "my-db", Name: "my-db"}

	content, err := p.Tabs()[3].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"my-db-snap-1", "automated", "available", "100"} {
		if !strings.Contains(content, want) {
			t.Errorf("snapshots tab missing %q\ngot:\n%s", want, content)
		}
	}
}

func TestRDSProvider_TabSnapshots_Empty(t *testing.T) {
	p := awspkg.NewRDSProviderWithClient(&stubRDS{instances: []rdstypes.DBInstance{baseInstance()}})
	item := awspkg.Item{ID: "my-db", Name: "my-db"}

	content, err := p.Tabs()[3].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "(no snapshots)") {
		t.Errorf("expected '(no snapshots)'\ngot:\n%s", content)
	}
}
