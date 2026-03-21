package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// EC2ActionsAPI defines write operations needed by the EC2 instance actions menu.
type EC2ActionsAPI interface {
	StartInstances(ctx context.Context, in *ec2.StartInstancesInput, opts ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error)
	StopInstances(ctx context.Context, in *ec2.StopInstancesInput, opts ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error)
	RebootInstances(ctx context.Context, in *ec2.RebootInstancesInput, opts ...func(*ec2.Options)) (*ec2.RebootInstancesOutput, error)
	TerminateInstances(ctx context.Context, in *ec2.TerminateInstancesInput, opts ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}

// SGActionsAPI defines write operations needed by the security group actions menu.
type SGActionsAPI interface {
	DeleteSecurityGroup(ctx context.Context, in *ec2.DeleteSecurityGroupInput, opts ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error)
}

// Actions implements Actionable for EC2Provider (instances).
func (p *EC2Provider) Actions(item Item) []ActionDef {
	if item.ID == "" {
		return nil
	}
	wc, ok := p.client.(EC2ActionsAPI)
	if !ok {
		return nil
	}
	return []ActionDef{
		{
			Label: "Start instance",
			Key:   's',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.Confirm(fmt.Sprintf("Start instance %q?", item.ID), func() {
					go func() {
						_, err := wc.StartInstances(context.Background(), &ec2.StartInstancesInput{
							InstanceIds: []string{item.ID},
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
		{
			Label: "Stop instance",
			Key:   'o',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.Confirm(fmt.Sprintf("Stop instance %q?", item.ID), func() {
					go func() {
						_, err := wc.StopInstances(context.Background(), &ec2.StopInstancesInput{
							InstanceIds: []string{item.ID},
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
		{
			Label: "Reboot instance",
			Key:   'r',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.Confirm(fmt.Sprintf("Reboot instance %q?", item.ID), func() {
					go func() {
						_, err := wc.RebootInstances(context.Background(), &ec2.RebootInstancesInput{
							InstanceIds: []string{item.ID},
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
		{
			Label: "Terminate instance",
			Key:   't',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.ConfirmDelete(item.ID, func() {
					go func() {
						_, err := wc.TerminateInstances(context.Background(), &ec2.TerminateInstancesInput{
							InstanceIds: []string{item.ID},
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
}

// Actions implements Actionable for EC2SGProvider (security groups).
func (p *EC2SGProvider) Actions(item Item) []ActionDef {
	if item.ID == "" {
		return nil
	}
	wc, ok := p.client.(SGActionsAPI)
	if !ok {
		return nil
	}
	return []ActionDef{
		{
			Label: "Delete security group",
			Key:   'd',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.ConfirmDelete(item.ID, func() {
					go func() {
						_, err := wc.DeleteSecurityGroup(context.Background(), &ec2.DeleteSecurityGroupInput{
							GroupId: awssdk.String(item.ID),
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
}
