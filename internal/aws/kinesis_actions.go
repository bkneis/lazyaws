package aws

import (
	"context"
	"fmt"
	"strconv"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
)

// KinesisActionsAPI defines write operations needed by the Kinesis actions menu.
type KinesisActionsAPI interface {
	CreateStream(ctx context.Context, in *kinesis.CreateStreamInput, opts ...func(*kinesis.Options)) (*kinesis.CreateStreamOutput, error)
	DeleteStream(ctx context.Context, in *kinesis.DeleteStreamInput, opts ...func(*kinesis.Options)) (*kinesis.DeleteStreamOutput, error)
	PutRecord(ctx context.Context, in *kinesis.PutRecordInput, opts ...func(*kinesis.Options)) (*kinesis.PutRecordOutput, error)
}

// Actions implements Actionable for KinesisProvider.
func (p *KinesisProvider) Actions(item Item) []ActionDef {
	wc, ok := p.client.(KinesisActionsAPI)
	if !ok {
		return nil
	}

	actions := []ActionDef{
		{
			Label: "Create stream",
			Key:   'c',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.PromptInput("Stream name", "", func(name string) {
					ac.PromptInput("Shard count", "1", func(shardStr string) {
						go func() {
							shards, err := strconv.ParseInt(shardStr, 10, 32)
							if err != nil || shards <= 0 {
								ac.ShowError(fmt.Errorf("invalid shard count %q: must be a positive integer", shardStr))
								return
							}
							n := int32(shards)
							_, err = wc.CreateStream(context.Background(), &kinesis.CreateStreamInput{
								StreamName: awssdk.String(name),
								ShardCount: &n,
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
				Label: "Delete stream",
				Key:   'd',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.ConfirmDelete(item.ID, func() {
						go func() {
							_, err := wc.DeleteStream(context.Background(), &kinesis.DeleteStreamInput{
								StreamName: awssdk.String(item.ID),
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
				Label: "Put record",
				Key:   'p',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.PromptInput("Partition key", "", func(partKey string) {
						ac.PromptInput("Data", "", func(data string) {
							go func() {
								_, err := wc.PutRecord(context.Background(), &kinesis.PutRecordInput{
									StreamName:   awssdk.String(item.ID),
									PartitionKey: awssdk.String(partKey),
									Data:         []byte(data),
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
		)
	}

	return actions
}
