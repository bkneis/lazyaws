package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoDBItemRow holds one Scan result row for display and expansion.
type DynamoDBItemRow struct {
	Cells    []string // truncated display values aligned to current headers
	FullJSON string   // pretty-printed JSON of all attributes
}

// DynamoDBAPI is the subset of the DynamoDB client methods used by DynamoDBProvider.
type DynamoDBAPI interface {
	ListTables(ctx context.Context, in *dynamodb.ListTablesInput, opts ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error)
	DescribeTable(ctx context.Context, in *dynamodb.DescribeTableInput, opts ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
	ListBackups(ctx context.Context, in *dynamodb.ListBackupsInput, opts ...func(*dynamodb.Options)) (*dynamodb.ListBackupsOutput, error)
	Scan(ctx context.Context, in *dynamodb.ScanInput, opts ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
}

// DynamoDBProvider implements Provider for Amazon DynamoDB.
type DynamoDBProvider struct {
	client DynamoDBAPI

	itemsMu         sync.RWMutex
	scanHeaders     []string
	scanItems       []DynamoDBItemRow
	pageStack       []map[string]dbtypes.AttributeValue // ExclusiveStartKey per back-step
	currentStartKey map[string]dbtypes.AttributeValue   // key used to fetch the current page
	nextPageKey     map[string]dbtypes.AttributeValue   // nil = no more pages
	scanTable       string                              // reset detection
	scanPage        int                                 // 1-based page number
}

func NewDynamoDBProvider(cfg awssdk.Config, endpointURL string) *DynamoDBProvider {
	var opts []func(*dynamodb.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *dynamodb.Options) {
			o.BaseEndpoint = awssdk.String(endpointURL)
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
		{Label: "Items", Fetch: p.tabItems},
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

const scanPageSize = 20

func (p *DynamoDBProvider) tabItems(ctx context.Context, item Item) (string, error) {
	p.itemsMu.Lock()
	sameTable := p.scanTable == item.ID && p.scanTable != ""
	p.itemsMu.Unlock()

	if sameTable {
		// Already loaded — re-render from cached state.
		p.itemsMu.RLock()
		defer p.itemsMu.RUnlock()
		return p.renderItemsLocked(), nil
	}

	// New table: reset state and fetch page 1.
	rows, headers, nextKey, err := p.fetchPage(ctx, item.ID, nil)
	if err != nil {
		return "", err
	}

	p.itemsMu.Lock()
	p.scanTable = item.ID
	p.scanHeaders = headers
	p.scanItems = rows
	p.pageStack = nil
	p.currentStartKey = nil
	p.nextPageKey = nextKey
	p.scanPage = 1
	result := p.renderItemsLocked()
	p.itemsMu.Unlock()

	return result, nil
}

// fetchPage runs a single Scan call and returns parsed rows, headers, and the
// LastEvaluatedKey (nil when there are no more pages).
func (p *DynamoDBProvider) fetchPage(ctx context.Context, tableName string, startKey map[string]dbtypes.AttributeValue) ([]DynamoDBItemRow, []string, map[string]dbtypes.AttributeValue, error) {
	in := &dynamodb.ScanInput{
		TableName: awssdk.String(tableName),
		Limit:     awssdk.Int32(scanPageSize),
	}
	if len(startKey) > 0 {
		in.ExclusiveStartKey = startKey
	}

	out, err := p.client.Scan(ctx, in)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("scan: %w", err)
	}

	// Collect all attribute names from this page, sorted for stable columns.
	nameSet := map[string]struct{}{}
	for _, dbItem := range out.Items {
		for k := range dbItem {
			nameSet[k] = struct{}{}
		}
	}
	headers := make([]string, 0, len(nameSet))
	for k := range nameSet {
		headers = append(headers, k)
	}
	sort.Strings(headers)

	rows := make([]DynamoDBItemRow, len(out.Items))
	for i, dbItem := range out.Items {
		cells := make([]string, len(headers))
		for j, h := range headers {
			av, ok := dbItem[h]
			if !ok {
				cells[j] = ""
				continue
			}
			v := formatAttrValue(av)
			if len(v) > 40 {
				v = v[:37] + "..."
			}
			cells[j] = v
		}
		fullJSON := attrMapToJSON(dbItem)
		rows[i] = DynamoDBItemRow{Cells: cells, FullJSON: fullJSON}
	}

	return rows, headers, out.LastEvaluatedKey, nil
}

// renderItemsLocked builds the display string from current scan state.
// Must be called with itemsMu held (read or write).
func (p *DynamoDBProvider) renderItemsLocked() string {
	if len(p.scanItems) == 0 {
		return fmt.Sprintf("  Items  (page %d, 0 items)\n\n  (no items)\n", p.scanPage)
	}

	rows := make([][]string, len(p.scanItems))
	for i, r := range p.scanItems {
		rows[i] = r.Cells
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "  Items  (page %d, %d items)\n\n", p.scanPage, len(p.scanItems))
	sb.WriteString(Table(p.scanHeaders, rows))

	prevHint := "p: prev page"
	if len(p.pageStack) == 0 {
		prevHint = "[::d]p: prev page[::-]"
	}
	nextHint := "n: next page"
	if len(p.nextPageKey) == 0 {
		nextHint = "[::d]n: next page[::-]"
	}
	fmt.Fprintf(&sb, "\n  [%s]  [%s]  [Enter: expand item]\n", prevHint, nextHint)
	return sb.String()
}

// ScanPage returns the current 1-based page number.
func (p *DynamoDBProvider) ScanPage() int {
	p.itemsMu.RLock()
	defer p.itemsMu.RUnlock()
	return p.scanPage
}

// GetCurrentItems returns a copy of the current scan page's rows and headers.
func (p *DynamoDBProvider) GetCurrentItems() ([]DynamoDBItemRow, []string) {
	p.itemsMu.RLock()
	defer p.itemsMu.RUnlock()
	rows := make([]DynamoDBItemRow, len(p.scanItems))
	copy(rows, p.scanItems)
	headers := make([]string, len(p.scanHeaders))
	copy(headers, p.scanHeaders)
	return rows, headers
}

// HasNextPage reports whether a next page is available.
func (p *DynamoDBProvider) HasNextPage() bool {
	p.itemsMu.RLock()
	defer p.itemsMu.RUnlock()
	return len(p.nextPageKey) > 0
}

// HasPrevPage reports whether a previous page is available.
func (p *DynamoDBProvider) HasPrevPage() bool {
	p.itemsMu.RLock()
	defer p.itemsMu.RUnlock()
	return len(p.pageStack) > 0
}

// NextPage fetches the next page of scan results and returns the new rows and headers.
func (p *DynamoDBProvider) NextPage(ctx context.Context, tableName string) ([]DynamoDBItemRow, []string, error) {
	p.itemsMu.RLock()
	startKey := p.nextPageKey
	currentStart := p.currentStartKey
	p.itemsMu.RUnlock()

	if len(startKey) == 0 {
		return nil, nil, fmt.Errorf("no next page")
	}

	rows, headers, nextKey, err := p.fetchPage(ctx, tableName, startKey)
	if err != nil {
		return nil, nil, err
	}

	p.itemsMu.Lock()
	p.pageStack = append(p.pageStack, currentStart)
	p.currentStartKey = startKey
	p.scanItems = rows
	p.scanHeaders = headers
	p.nextPageKey = nextKey
	p.scanPage++
	p.itemsMu.Unlock()

	return rows, headers, nil
}

// PrevPage fetches the previous page of scan results and returns the new rows and headers.
func (p *DynamoDBProvider) PrevPage(ctx context.Context, tableName string) ([]DynamoDBItemRow, []string, error) {
	p.itemsMu.RLock()
	stackLen := len(p.pageStack)
	p.itemsMu.RUnlock()

	if stackLen == 0 {
		return nil, nil, fmt.Errorf("no previous page")
	}

	p.itemsMu.Lock()
	prevStart := p.pageStack[len(p.pageStack)-1]
	p.pageStack = p.pageStack[:len(p.pageStack)-1]
	p.itemsMu.Unlock()

	rows, headers, nextKey, err := p.fetchPage(ctx, tableName, prevStart)
	if err != nil {
		return nil, nil, err
	}

	p.itemsMu.Lock()
	p.currentStartKey = prevStart
	p.scanItems = rows
	p.scanHeaders = headers
	p.nextPageKey = nextKey
	p.scanPage--
	p.itemsMu.Unlock()

	return rows, headers, nil
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
			proj := ""
		if gsi.Projection != nil {
			proj = string(gsi.Projection.ProjectionType)
		}
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
			proj := ""
		if lsi.Projection != nil {
			proj = string(lsi.Projection.ProjectionType)
		}
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

// formatAttrValue returns a short human-readable string for a DynamoDB attribute value.
func formatAttrValue(av dbtypes.AttributeValue) string {
	switch v := av.(type) {
	case *dbtypes.AttributeValueMemberS:
		return v.Value
	case *dbtypes.AttributeValueMemberN:
		return v.Value
	case *dbtypes.AttributeValueMemberBOOL:
		if v.Value {
			return "true"
		}
		return "false"
	case *dbtypes.AttributeValueMemberNULL:
		return "(null)"
	case *dbtypes.AttributeValueMemberB:
		return fmt.Sprintf("(binary %dB)", len(v.Value))
	case *dbtypes.AttributeValueMemberL:
		return fmt.Sprintf("[%d items]", len(v.Value))
	case *dbtypes.AttributeValueMemberM:
		return fmt.Sprintf("{%d keys}", len(v.Value))
	case *dbtypes.AttributeValueMemberSS:
		return joinStrings(v.Value)
	case *dbtypes.AttributeValueMemberNS:
		return joinStrings(v.Value)
	case *dbtypes.AttributeValueMemberBS:
		return fmt.Sprintf("(binary set, %d items)", len(v.Value))
	default:
		return ""
	}
}

func joinStrings(vals []string) string {
	if len(vals) <= 3 {
		return strings.Join(vals, ", ")
	}
	return strings.Join(vals[:3], ", ") + ", ..."
}

// attrMapToJSON converts a DynamoDB item map to a pretty-printed JSON string.
func attrMapToJSON(item map[string]dbtypes.AttributeValue) string {
	m := make(map[string]interface{}, len(item))
	for k, av := range item {
		m[k] = attrToInterface(av)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Sprintf("(marshal error: %v)", err)
	}
	return string(b)
}

// attrToInterface converts a DynamoDB AttributeValue to a plain Go value for JSON marshalling.
func attrToInterface(av dbtypes.AttributeValue) interface{} {
	switch v := av.(type) {
	case *dbtypes.AttributeValueMemberS:
		return v.Value
	case *dbtypes.AttributeValueMemberN:
		return json.Number(v.Value)
	case *dbtypes.AttributeValueMemberBOOL:
		return v.Value
	case *dbtypes.AttributeValueMemberNULL:
		return nil
	case *dbtypes.AttributeValueMemberB:
		return v.Value // marshals as base64
	case *dbtypes.AttributeValueMemberL:
		result := make([]interface{}, len(v.Value))
		for i, elem := range v.Value {
			result[i] = attrToInterface(elem)
		}
		return result
	case *dbtypes.AttributeValueMemberM:
		result := make(map[string]interface{}, len(v.Value))
		for k, elem := range v.Value {
			result[k] = attrToInterface(elem)
		}
		return result
	case *dbtypes.AttributeValueMemberSS:
		return v.Value
	case *dbtypes.AttributeValueMemberNS:
		nums := make([]json.Number, len(v.Value))
		for i, n := range v.Value {
			nums[i] = json.Number(n)
		}
		return nums
	case *dbtypes.AttributeValueMemberBS:
		result := make([][]byte, len(v.Value))
		copy(result, v.Value)
		return result
	default:
		return nil
	}
}
