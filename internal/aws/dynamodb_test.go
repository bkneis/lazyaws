package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

import "strconv"

type stubDynamoDB struct {
	tables    []string
	table     *dbtypes.TableDescription
	backups   []dbtypes.BackupSummary
	scanPages [][]map[string]dbtypes.AttributeValue // each element is one page of items
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

// Scan returns pages by decoding a "_pageIdx" key from ExclusiveStartKey.
// This lets PrevPage re-fetch earlier pages correctly (unlike a plain counter).
func (s *stubDynamoDB) Scan(_ context.Context, in *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	if len(s.scanPages) == 0 {
		return &dynamodb.ScanOutput{}, nil
	}
	idx := 0
	if pageAV, ok := in.ExclusiveStartKey["_pageIdx"]; ok {
		if pv, ok2 := pageAV.(*dbtypes.AttributeValueMemberN); ok2 {
			if n, err := strconv.Atoi(pv.Value); err == nil {
				idx = n
			}
		}
	}
	if idx >= len(s.scanPages) {
		idx = len(s.scanPages) - 1
	}
	items := s.scanPages[idx]
	var lastKey map[string]dbtypes.AttributeValue
	if idx+1 < len(s.scanPages) {
		lastKey = map[string]dbtypes.AttributeValue{
			"_pageIdx": &dbtypes.AttributeValueMemberN{Value: strconv.Itoa(idx + 1)},
		}
	}
	return &dynamodb.ScanOutput{Items: items, LastEvaluatedKey: lastKey}, nil
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
		{2, "Indexes", "orderId (HASH)"},
		{3, "Backups", "Orders-backup"},
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

func TestDynamoDBProvider_TabItems(t *testing.T) {
	page1 := []map[string]dbtypes.AttributeValue{
		{
			"orderId":    &dbtypes.AttributeValueMemberS{Value: "order-abc-1234"},
			"customerId": &dbtypes.AttributeValueMemberS{Value: "cust-9999"},
			"status":     &dbtypes.AttributeValueMemberS{Value: "SHIPPED"},
			"total":      &dbtypes.AttributeValueMemberN{Value: "99.99"},
		},
	}

	cases := []struct {
		name      string
		scanPages [][]map[string]dbtypes.AttributeValue
		wantItems int
		wantText  string
		wantPage  int
	}{
		{
			name:      "first page with items",
			scanPages: [][]map[string]dbtypes.AttributeValue{page1},
			wantItems: 1,
			wantText:  "order-abc-1234",
			wantPage:  1,
		},
		{
			name:      "empty table",
			scanPages: nil,
			wantItems: 0,
			wantText:  "no items",
			wantPage:  1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubDynamoDB{scanPages: tc.scanPages}
			p := awspkg.NewDynamoDBProviderWithClient(stub)
			item := awspkg.Item{ID: "Orders", Name: "Orders"}

			tabs := p.Tabs()
			var itemsTab awspkg.TabDef
			for _, tab := range tabs {
				if tab.Label == "Items" {
					itemsTab = tab
					break
				}
			}

			content, err := itemsTab.Fetch(context.Background(), item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(content, tc.wantText) {
				t.Errorf("missing %q in:\n%s", tc.wantText, content)
			}
			rows, _ := p.GetCurrentItems()
			if len(rows) != tc.wantItems {
				t.Errorf("got %d items, want %d", len(rows), tc.wantItems)
			}
			if p.ScanPage() != tc.wantPage {
				t.Errorf("page = %d, want %d", p.ScanPage(), tc.wantPage)
			}
		})
	}
}

func TestDynamoDBProvider_TabItems_TableSwitch(t *testing.T) {
	page := []map[string]dbtypes.AttributeValue{
		{"id": &dbtypes.AttributeValueMemberS{Value: "row1"}},
	}
	stub := &stubDynamoDB{scanPages: [][]map[string]dbtypes.AttributeValue{page, page}}
	p := awspkg.NewDynamoDBProviderWithClient(stub)

	tabs := p.Tabs()
	var itemsTab awspkg.TabDef
	for _, tab := range tabs {
		if tab.Label == "Items" {
			itemsTab = tab
			break
		}
	}

	// Fetch for table A.
	_, err := itemsTab.Fetch(context.Background(), awspkg.Item{ID: "TableA"})
	if err != nil {
		t.Fatalf("fetch A: %v", err)
	}
	if p.ScanPage() != 1 {
		t.Errorf("after TableA page = %d, want 1", p.ScanPage())
	}

	// Fetch for table B — should reset and start from page 1.
	_, err = itemsTab.Fetch(context.Background(), awspkg.Item{ID: "TableB"})
	if err != nil {
		t.Fatalf("fetch B: %v", err)
	}
	if p.ScanPage() != 1 {
		t.Errorf("after TableB page = %d, want 1", p.ScanPage())
	}
	if p.HasPrevPage() {
		t.Error("expected no prev page after table switch")
	}
}

func TestDynamoDBProvider_NextPage_PrevPage(t *testing.T) {
	makeItem := func(id string) map[string]dbtypes.AttributeValue {
		return map[string]dbtypes.AttributeValue{
			"id": &dbtypes.AttributeValueMemberS{Value: id},
		}
	}
	pages := [][]map[string]dbtypes.AttributeValue{
		{makeItem("p1-r1")},
		{makeItem("p2-r1")},
		{makeItem("p3-r1")},
	}
	stub := &stubDynamoDB{scanPages: pages}
	p := awspkg.NewDynamoDBProviderWithClient(stub)

	tabs := p.Tabs()
	var itemsTab awspkg.TabDef
	for _, tab := range tabs {
		if tab.Label == "Items" {
			itemsTab = tab
			break
		}
	}

	// Load page 1.
	_, err := itemsTab.Fetch(context.Background(), awspkg.Item{ID: "T"})
	if err != nil {
		t.Fatalf("fetch page 1: %v", err)
	}
	if p.ScanPage() != 1 {
		t.Errorf("page = %d, want 1", p.ScanPage())
	}
	if !p.HasNextPage() {
		t.Error("expected HasNextPage after page 1")
	}
	if p.HasPrevPage() {
		t.Error("expected no HasPrevPage on page 1")
	}

	// Advance to page 2.
	rows, _, err := p.NextPage(context.Background(), "T")
	if err != nil {
		t.Fatalf("NextPage: %v", err)
	}
	if p.ScanPage() != 2 {
		t.Errorf("page = %d, want 2", p.ScanPage())
	}
	if len(rows) == 0 || rows[0].Cells[0] != "p2-r1" {
		t.Errorf("unexpected row on page 2: %v", rows)
	}
	if !p.HasPrevPage() {
		t.Error("expected HasPrevPage on page 2")
	}

	// Go back to page 1.
	rows, _, err = p.PrevPage(context.Background(), "T")
	if err != nil {
		t.Fatalf("PrevPage: %v", err)
	}
	if p.ScanPage() != 1 {
		t.Errorf("page = %d, want 1", p.ScanPage())
	}
	if len(rows) == 0 || rows[0].Cells[0] != "p1-r1" {
		t.Errorf("unexpected row on page 1 after prev: %v", rows)
	}
	if p.HasPrevPage() {
		t.Error("expected no HasPrevPage after going back to page 1")
	}
}
