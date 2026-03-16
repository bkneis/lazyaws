package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoDBAPI is the subset of the DynamoDB client methods used by DynamoDBProvider.
type DynamoDBAPI interface {
	ListTables(ctx context.Context, in *dynamodb.ListTablesInput, opts ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error)
	DescribeTable(ctx context.Context, in *dynamodb.DescribeTableInput, opts ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
	ListBackups(ctx context.Context, in *dynamodb.ListBackupsInput, opts ...func(*dynamodb.Options)) (*dynamodb.ListBackupsOutput, error)
}

// DynamoDBProvider implements Provider for Amazon DynamoDB.
type DynamoDBProvider struct {
	client DynamoDBAPI
}

func NewDynamoDBProvider(cfg awssdk.Config, local bool) *DynamoDBProvider {
	var opts []func(*dynamodb.Options)
	if local {
		opts = append(opts, func(o *dynamodb.Options) {
			o.BaseEndpoint = awssdk.String("http://localhost:4566")
		})
	}
	return &DynamoDBProvider{client: dynamodb.NewFromConfig(cfg, opts...)}
}

func NewDynamoDBProviderWithClient(client DynamoDBAPI) *DynamoDBProvider {
	return &DynamoDBProvider{client: client}
}

func (p *DynamoDBProvider) Name() string { return "DynamoDB" }

func (p *DynamoDBProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	var items []Item
	var lastKey *string
	for {
		out, err := p.client.ListTables(ctx, &dynamodb.ListTablesInput{
			ExclusiveStartTableName: lastKey,
		})
		if err != nil {
			return nil, fmt.Errorf("list tables: %w", err)
		}
		for _, name := range out.TableNames {
			items = append(items, Item{ID: name, Name: name})
		}
		if out.LastEvaluatedTableName == nil {
			break
		}
		lastKey = out.LastEvaluatedTableName
	}
	return filterItems(items, query), nil
}

func (p *DynamoDBProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *DynamoDBProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Indexes", Fetch: p.tabIndexes},
		{Label: "Backups", Fetch: p.tabBackups},
	}
}

func (p *DynamoDBProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	t := out.Table

	status := string(t.TableStatus)
	billingMode := "PROVISIONED"
	if t.BillingModeSummary != nil {
		billingMode = string(t.BillingModeSummary.BillingMode)
	}

	created := ""
	if t.CreationDateTime != nil {
		created = t.CreationDateTime.Format(time.DateTime)
	}

	itemCount := fmt.Sprintf("%d", awssdk.ToInt64(t.ItemCount))
	tableSize := FormatSize(awssdk.ToInt64(t.TableSizeBytes))
	arn := awssdk.ToString(t.TableArn)

	streams := "Disabled"
	if t.StreamSpecification != nil && awssdk.ToBool(t.StreamSpecification.StreamEnabled) {
		streams = fmt.Sprintf("Enabled (%s)", string(t.StreamSpecification.StreamViewType))
	}

	encryption := "DEFAULT"
	if t.SSEDescription != nil {
		encryption = string(t.SSEDescription.SSEType)
	}

	return KV([][2]string{
		{"Name", awssdk.ToString(t.TableName)},
		{"Status", status},
		{"Item Count", itemCount},
		{"Table Size", tableSize},
		{"Billing Mode", billingMode},
		{"Created", created},
		{"ARN", arn},
		{"Streams", streams},
		{"Encryption", encryption},
	}), nil
}

func (p *DynamoDBProvider) tabIndexes(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	t := out.Table

	var sb strings.Builder

	// Primary key
	var hashKey, rangeKey string
	for _, ks := range t.KeySchema {
		switch ks.KeyType {
		case dbtypes.KeyTypeHash:
			hashKey = awssdk.ToString(ks.AttributeName)
		case dbtypes.KeyTypeRange:
			rangeKey = awssdk.ToString(ks.AttributeName)
		}
	}
	pkStr := hashKey + " (HASH)"
	if rangeKey != "" {
		pkStr += " + " + rangeKey + " (RANGE)"
	}
	sb.WriteString(KV([][2]string{{"Primary Key", pkStr}}))

	// GSIs
	if len(t.GlobalSecondaryIndexes) > 0 {
		sb.WriteString("\n  GSIs\n")
		rows := make([][]string, len(t.GlobalSecondaryIndexes))
		for i, gsi := range t.GlobalSecondaryIndexes {
			keyStr := gsiKeyStr(gsi.KeySchema)
			proj := string(gsi.Projection.ProjectionType)
			rows[i] = []string{awssdk.ToString(gsi.IndexName), keyStr, proj, string(gsi.IndexStatus)}
		}
		sb.WriteString(Table([]string{"Name", "Key (Hash+Range)", "Projection", "Status"}, rows))
	}

	// LSIs
	if len(t.LocalSecondaryIndexes) > 0 {
		sb.WriteString("\n  LSIs\n")
		rows := make([][]string, len(t.LocalSecondaryIndexes))
		for i, lsi := range t.LocalSecondaryIndexes {
			var rangeAttr string
			for _, ks := range lsi.KeySchema {
				if ks.KeyType == dbtypes.KeyTypeRange {
					rangeAttr = awssdk.ToString(ks.AttributeName)
				}
			}
			proj := string(lsi.Projection.ProjectionType)
			rows[i] = []string{awssdk.ToString(lsi.IndexName), rangeAttr, proj}
		}
		sb.WriteString(Table([]string{"Name", "Range Key", "Projection"}, rows))
	}

	return sb.String(), nil
}

func gsiKeyStr(ks []dbtypes.KeySchemaElement) string {
	var hash, rng string
	for _, k := range ks {
		switch k.KeyType {
		case dbtypes.KeyTypeHash:
			hash = awssdk.ToString(k.AttributeName)
		case dbtypes.KeyTypeRange:
			rng = awssdk.ToString(k.AttributeName)
		}
	}
	if rng != "" {
		return hash + "+" + rng
	}
	return hash
}

func (p *DynamoDBProvider) tabBackups(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListBackups(ctx, &dynamodb.ListBackupsInput{
		TableName: awssdk.String(item.ID),
		Limit:     awssdk.Int32(10),
	})
	if err != nil {
		return "", err
	}
	if len(out.BackupSummaries) == 0 {
		return "  (no backups found)\n", nil
	}
	rows := make([][]string, len(out.BackupSummaries))
	for i, b := range out.BackupSummaries {
		created := ""
		if b.BackupCreationDateTime != nil {
			created = b.BackupCreationDateTime.Format(time.DateOnly)
		}
		rows[i] = []string{
			awssdk.ToString(b.BackupName),
			string(b.BackupStatus),
			created,
			string(b.BackupType),
		}
	}
	return Table([]string{"Name", "Status", "Created", "Type"}, rows), nil
}
