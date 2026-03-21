package aws

import (
	"context"
	"encoding/json"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoDBActionsAPI defines write operations needed by the DynamoDB actions menu.
type DynamoDBActionsAPI interface {
	CreateTable(ctx context.Context, in *dynamodb.CreateTableInput, opts ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
	DeleteTable(ctx context.Context, in *dynamodb.DeleteTableInput, opts ...func(*dynamodb.Options)) (*dynamodb.DeleteTableOutput, error)
	CreateBackup(ctx context.Context, in *dynamodb.CreateBackupInput, opts ...func(*dynamodb.Options)) (*dynamodb.CreateBackupOutput, error)
	PutItem(ctx context.Context, in *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateTable(ctx context.Context, in *dynamodb.UpdateTableInput, opts ...func(*dynamodb.Options)) (*dynamodb.UpdateTableOutput, error)
}

// Actions implements Actionable for DynamoDBProvider.
func (p *DynamoDBProvider) Actions(item Item) []ActionDef {
	wc, ok := p.client.(DynamoDBActionsAPI)
	if !ok {
		return nil
	}

	actions := []ActionDef{
		{
			Label: "Create table",
			Key:   'c',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.PromptInput("Table name", "", func(tableName string) {
					ac.PromptInput("Hash key name", "id", func(hashKey string) {
						go func() {
							_, err := wc.CreateTable(context.Background(), &dynamodb.CreateTableInput{
								TableName: awssdk.String(tableName),
								AttributeDefinitions: []dbtypes.AttributeDefinition{
									{
										AttributeName: awssdk.String(hashKey),
										AttributeType: dbtypes.ScalarAttributeTypeS,
									},
								},
								KeySchema: []dbtypes.KeySchemaElement{
									{
										AttributeName: awssdk.String(hashKey),
										KeyType:       dbtypes.KeyTypeHash,
									},
								},
								BillingMode: dbtypes.BillingModePayPerRequest,
							})
							if err != nil {
								ac.ShowError(err)
								return
							}
							ac.Refresh()
						}()
					})
				})
				return nil
			},
		},
	}

	if item.ID != "" {
		actions = append(actions,
			ActionDef{
				Label: "Delete table",
				Key:   'd',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.ConfirmDelete(item.ID, func() {
						go func() {
							_, err := wc.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{
								TableName: awssdk.String(item.ID),
							})
							if err != nil {
								ac.ShowError(err)
								return
							}
							ac.Refresh()
						}()
					})
					return nil
				},
			},
			ActionDef{
				Label: "Add item",
				Key:   'i',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.PromptInput("Item JSON", `{"id": "value"}`, func(raw string) {
						go func() {
							var m map[string]any
							if err := json.Unmarshal([]byte(raw), &m); err != nil {
								ac.ShowError(fmt.Errorf("invalid JSON: %w", err))
								return
							}
							av := make(map[string]dbtypes.AttributeValue, len(m))
							for k, v := range m {
								switch val := v.(type) {
								case string:
									av[k] = &dbtypes.AttributeValueMemberS{Value: val}
								case float64:
									av[k] = &dbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%g", val)}
								case bool:
									av[k] = &dbtypes.AttributeValueMemberBOOL{Value: val}
								}
							}
							_, err := wc.PutItem(context.Background(), &dynamodb.PutItemInput{
								TableName: awssdk.String(item.ID),
								Item:      av,
							})
							if err != nil {
								ac.ShowError(err)
								return
							}
							ac.Refresh()
						}()
					})
					return nil
				},
			},
			ActionDef{
				Label: "Add index (GSI)",
				Key:   'x',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.PromptInput("Index name", "", func(indexName string) {
						ac.PromptInput("Hash key attribute", "", func(hashKey string) {
							ac.PromptInput("Key type (S/N/B)", "S", func(keyType string) {
								go func() {
									_, err := wc.UpdateTable(context.Background(), &dynamodb.UpdateTableInput{
										TableName: awssdk.String(item.ID),
										AttributeDefinitions: []dbtypes.AttributeDefinition{
											{
												AttributeName: awssdk.String(hashKey),
												AttributeType: dbtypes.ScalarAttributeType(keyType),
											},
										},
										GlobalSecondaryIndexUpdates: []dbtypes.GlobalSecondaryIndexUpdate{
											{
												Create: &dbtypes.CreateGlobalSecondaryIndexAction{
													IndexName: awssdk.String(indexName),
													KeySchema: []dbtypes.KeySchemaElement{
														{
															AttributeName: awssdk.String(hashKey),
															KeyType:       dbtypes.KeyTypeHash,
														},
													},
													Projection: &dbtypes.Projection{
														ProjectionType: dbtypes.ProjectionTypeAll,
													},
												},
											},
										},
									})
									if err != nil {
										ac.ShowError(err)
										return
									}
									ac.ShowInfo("GSI creation started")
								}()
							})
						})
					})
					return nil
				},
			},
			ActionDef{
				Label: "Create backup",
				Key:   'b',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.PromptInput("Backup name", item.ID+"-backup", func(backupName string) {
						go func() {
							_, err := wc.CreateBackup(context.Background(), &dynamodb.CreateBackupInput{
								TableName:  awssdk.String(item.ID),
								BackupName: awssdk.String(backupName),
							})
							if err != nil {
								ac.ShowError(err)
								return
							}
							ac.Refresh()
						}()
					})
					return nil
				},
			},
		)
	}

	return actions
}
