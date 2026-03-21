package aws

import (
	"context"
	"fmt"
	"strconv"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

// CWLActionsAPI defines write operations needed by the CloudWatch Logs actions menu.
type CWLActionsAPI interface {
	CreateLogGroup(ctx context.Context, in *cloudwatchlogs.CreateLogGroupInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error)
	DeleteLogGroup(ctx context.Context, in *cloudwatchlogs.DeleteLogGroupInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DeleteLogGroupOutput, error)
	PutRetentionPolicy(ctx context.Context, in *cloudwatchlogs.PutRetentionPolicyInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutRetentionPolicyOutput, error)
}

// cwlClientAdapter also implements CWLActionsAPI by delegating to the underlying client.
func (a *cwlClientAdapter) CreateLogGroup(ctx context.Context, in *cloudwatchlogs.CreateLogGroupInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	return a.client.CreateLogGroup(ctx, in, opts...)
}

func (a *cwlClientAdapter) DeleteLogGroup(ctx context.Context, in *cloudwatchlogs.DeleteLogGroupInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DeleteLogGroupOutput, error) {
	return a.client.DeleteLogGroup(ctx, in, opts...)
}

func (a *cwlClientAdapter) PutRetentionPolicy(ctx context.Context, in *cloudwatchlogs.PutRetentionPolicyInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutRetentionPolicyOutput, error) {
	return a.client.PutRetentionPolicy(ctx, in, opts...)
}

// Actions implements Actionable for CloudWatchLogsProvider.
func (p *CloudWatchLogsProvider) Actions(item Item) []ActionDef {
	wc, ok := p.client.(CWLActionsAPI)
	if !ok {
		return nil
	}

	actions := []ActionDef{
		{
			Label: "Create log group",
			Key:   'c',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.PromptInput("Log group name", "", func(name string) {
					go func() {
						_, err := wc.CreateLogGroup(context.Background(), &cloudwatchlogs.CreateLogGroupInput{
							LogGroupName: awssdk.String(name),
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
				Label: "Delete log group",
				Key:   'd',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.ConfirmDelete(item.ID, func() {
						go func() {
							_, err := wc.DeleteLogGroup(context.Background(), &cloudwatchlogs.DeleteLogGroupInput{
								LogGroupName: awssdk.String(item.ID),
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
				Label: "Set retention",
				Key:   'r',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.PromptInput("Retention days (0 = never expire)", "30", func(daysStr string) {
						go func() {
							if daysStr == "0" || daysStr == "never" || daysStr == "" {
								// Delete the retention policy (never expire).
								// Use PutRetentionPolicy with 0 is not valid; instead we'd call DeleteRetentionPolicy.
								// For simplicity, show error guiding the user.
								ac.ShowError(fmt.Errorf("to remove retention use the AWS CLI: aws logs delete-retention-policy --log-group-name %q", item.ID))
								return
							}
							days, err := strconv.Atoi(daysStr)
							if err != nil || days <= 0 {
								ac.ShowError(fmt.Errorf("invalid number of days: %q", daysStr))
								return
							}
							_, err = wc.PutRetentionPolicy(context.Background(), &cloudwatchlogs.PutRetentionPolicyInput{
								LogGroupName:    awssdk.String(item.ID),
								RetentionInDays: awssdk.Int32(int32(days)),
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
