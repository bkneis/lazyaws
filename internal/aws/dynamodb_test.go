package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

type stubDynamoDB struct {
	tables  []string
	table   *dbtypes.TableDescription
	backups []dbtypes.BackupSummary
}

func (s *stubDynamoDB) ListTables(_ context.Context, _ *dynamodb.ListTablesInput, _ ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error) {
	return &dynamodb.ListTablesOutput{TableNames: s.tables}, nil
}

func (s *stubDynamoDB) DescribeTable(_ context.Context, _ *dynamodb.DescribeTableInput, _ ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	return &dynamodb.DescribeTableOutput{Table: s.table}, nil
}

func (s *stubDynamoDB) ListBackups(_ context.Context, _ *dynamodb.ListBackupsInput, _ ...func(*dynamodb.Options)) (*dynamodb.ListBackupsOutput, error) {
	return &dynamodb.ListBackupsOutput{BackupSummaries: s.backups}, nil
}

func TestDynamoDBProvider_ListItems(t *testing.T) {
	cases := []struct {
		name   string
		tables []string
		query  string
		want   int
	}{
		{"all tables", []string{"Orders", "Users"}, "", 2},
		{"filter match", []string{"Orders", "Users"}, "ord", 1},
		{"filter no match", []string{"Orders", "Users"}, "xyz", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := awspkg.NewDynamoDBProviderWithClient(&stubDynamoDB{tables: tc.tables})
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

func TestDynamoDBProvider_Tabs(t *testing.T) {
	created := time.Date(2023, 3, 15, 9, 0, 0, 0, time.UTC)
	billingMode := dbtypes.BillingModePayPerRequest
	table := &dbtypes.TableDescription{
		TableName:       aws.String("Orders"),
		TableStatus:     dbtypes.TableStatusActive,
		ItemCount:       aws.Int64(1000),
		TableSizeBytes:  aws.Int64(2 * 1024 * 1024 * 1024),
		TableArn:        aws.String("arn:aws:dynamodb:us-east-1:123:table/Orders"),
		CreationDateTime: &created,
		BillingModeSummary: &dbtypes.BillingModeSummary{
			BillingMode: billingMode,
		},
		KeySchema: []dbtypes.KeySchemaElement{
			{AttributeName: aws.String("orderId"), KeyType: dbtypes.KeyTypeHash},
			{AttributeName: aws.String("createdAt"), KeyType: dbtypes.KeyTypeRange},
		},
		GlobalSecondaryIndexes: []dbtypes.GlobalSecondaryIndexDescription{
			{
				IndexName:   aws.String("customerId-idx"),
				IndexStatus: dbtypes.IndexStatusActive,
				KeySchema: []dbtypes.KeySchemaElement{
					{AttributeName: aws.String("customerId"), KeyType: dbtypes.KeyTypeHash},
				},
				Projection: &dbtypes.Projection{ProjectionType: dbtypes.ProjectionTypeAll},
			},
		},
	}
	backup := dbtypes.BackupSummary{
		BackupName:             aws.String("Orders-backup"),
		BackupStatus:           dbtypes.BackupStatusAvailable,
		BackupType:             dbtypes.BackupTypeUser,
		BackupCreationDateTime: &created,
	}
	stub := &stubDynamoDB{
		tables:  []string{"Orders"},
		table:   table,
		backups: []dbtypes.BackupSummary{backup},
	}
	p := awspkg.NewDynamoDBProviderWithClient(stub)
	item := awspkg.Item{ID: "Orders", Name: "Orders"}
	tabs := p.Tabs()

	cases := []struct {
		tabIdx int
		label  string
		want   string
	}{
		{0, "Overview", "PAY_PER_REQUEST"},
		{1, "Indexes", "orderId (HASH)"},
		{2, "Backups", "Orders-backup"},
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
