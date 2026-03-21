package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// SQSActionsAPI defines write operations needed by the SQS actions menu.
type SQSActionsAPI interface {
	CreateQueue(ctx context.Context, in *sqs.CreateQueueInput, opts ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error)
	DeleteQueue(ctx context.Context, in *sqs.DeleteQueueInput, opts ...func(*sqs.Options)) (*sqs.DeleteQueueOutput, error)
	SendMessage(ctx context.Context, in *sqs.SendMessageInput, opts ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	PurgeQueue(ctx context.Context, in *sqs.PurgeQueueInput, opts ...func(*sqs.Options)) (*sqs.PurgeQueueOutput, error)
}

// Actions implements Actionable for SQSProvider.
func (p *SQSProvider) Actions(item Item) []ActionDef {
	wc, ok := p.client.(SQSActionsAPI)
	if !ok {
		return nil
	}

	actions := []ActionDef{
		{
			Label: "Create queue",
			Key:   'c',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.PromptInput("Queue name", "", func(name string) {
					go func() {
						_, err := wc.CreateQueue(context.Background(), &sqs.CreateQueueInput{
							QueueName: awssdk.String(name),
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
	}

	if item.ID != "" {
		actions = append(actions,
			ActionDef{
				Label: "Delete queue",
				Key:   'd',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.ConfirmDelete(item.Name, func() {
						go func() {
							_, err := wc.DeleteQueue(context.Background(), &sqs.DeleteQueueInput{
								QueueUrl: awssdk.String(item.ID),
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
				Label: "Send message",
				Key:   's',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.PromptInput("Message body", "", func(body string) {
						go func() {
							_, err := wc.SendMessage(context.Background(), &sqs.SendMessageInput{
								QueueUrl:    awssdk.String(item.ID),
								MessageBody: awssdk.String(body),
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
				Label: "Purge queue",
				Key:   'p',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.Confirm(fmt.Sprintf("Purge queue %q? All messages will be deleted.", item.Name), func() {
						go func() {
							_, err := wc.PurgeQueue(context.Background(), &sqs.PurgeQueueInput{
								QueueUrl: awssdk.String(item.ID),
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
