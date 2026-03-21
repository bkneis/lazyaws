package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

// SNSActionsAPI defines write operations needed by the SNS actions menu.
type SNSActionsAPI interface {
	CreateTopic(ctx context.Context, in *sns.CreateTopicInput, opts ...func(*sns.Options)) (*sns.CreateTopicOutput, error)
	DeleteTopic(ctx context.Context, in *sns.DeleteTopicInput, opts ...func(*sns.Options)) (*sns.DeleteTopicOutput, error)
	Publish(ctx context.Context, in *sns.PublishInput, opts ...func(*sns.Options)) (*sns.PublishOutput, error)
}

// Actions implements Actionable for SNSProvider.
func (p *SNSProvider) Actions(item Item) []ActionDef {
	wc, ok := p.client.(SNSActionsAPI)
	if !ok {
		return nil
	}

	actions := []ActionDef{
		{
			Label: "Create topic",
			Key:   'c',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.PromptInput("Topic name", "", func(name string) {
					go func() {
						_, err := wc.CreateTopic(context.Background(), &sns.CreateTopicInput{
							Name: awssdk.String(name),
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
				Label: "Delete topic",
				Key:   'd',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.ConfirmDelete(item.Name, func() {
						go func() {
							_, err := wc.DeleteTopic(context.Background(), &sns.DeleteTopicInput{
								TopicArn: awssdk.String(item.ID),
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
				Label: "Publish message",
				Key:   'p',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.PromptInput("Message body", "", func(body string) {
						go func() {
							out, err := wc.Publish(context.Background(), &sns.PublishInput{
								TopicArn: awssdk.String(item.ID),
								Message:  awssdk.String(body),
							})
							if err != nil {
								ac.ShowError(err)
								return
							}
							ac.ShowInfo(fmt.Sprintf("Message published\nID: %s", awssdk.ToString(out.MessageId)))
						}()
					})
					return nil
				},
			},
		)
	}

	return actions
}
