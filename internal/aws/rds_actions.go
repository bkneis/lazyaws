package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
)

// RDSActionsAPI defines write operations needed by the RDS actions menu.
type RDSActionsAPI interface {
	StartDBInstance(ctx context.Context, in *rds.StartDBInstanceInput, opts ...func(*rds.Options)) (*rds.StartDBInstanceOutput, error)
	StopDBInstance(ctx context.Context, in *rds.StopDBInstanceInput, opts ...func(*rds.Options)) (*rds.StopDBInstanceOutput, error)
	RebootDBInstance(ctx context.Context, in *rds.RebootDBInstanceInput, opts ...func(*rds.Options)) (*rds.RebootDBInstanceOutput, error)
	CreateDBSnapshot(ctx context.Context, in *rds.CreateDBSnapshotInput, opts ...func(*rds.Options)) (*rds.CreateDBSnapshotOutput, error)
	DeleteDBInstance(ctx context.Context, in *rds.DeleteDBInstanceInput, opts ...func(*rds.Options)) (*rds.DeleteDBInstanceOutput, error)
}

// Actions implements Actionable for RDSProvider.
func (p *RDSProvider) Actions(item Item) []ActionDef {
	if item.ID == "" {
		return nil
	}
	wc, ok := p.client.(RDSActionsAPI)
	if !ok {
		return nil
	}
	return []ActionDef{
		{
			Label: "Start DB instance",
			Key:   's',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.Confirm(fmt.Sprintf("Start DB instance %q?", item.ID), func() {
					go func() {
						_, err := wc.StartDBInstance(context.Background(), &rds.StartDBInstanceInput{
							DBInstanceIdentifier: awssdk.String(item.ID),
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
			Label: "Stop DB instance",
			Key:   'o',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.Confirm(fmt.Sprintf("Stop DB instance %q?", item.ID), func() {
					go func() {
						_, err := wc.StopDBInstance(context.Background(), &rds.StopDBInstanceInput{
							DBInstanceIdentifier: awssdk.String(item.ID),
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
			Label: "Reboot DB instance",
			Key:   'r',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.Confirm(fmt.Sprintf("Reboot DB instance %q?", item.ID), func() {
					go func() {
						_, err := wc.RebootDBInstance(context.Background(), &rds.RebootDBInstanceInput{
							DBInstanceIdentifier: awssdk.String(item.ID),
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
			Label: "Create snapshot",
			Key:   'c',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.PromptInput("Snapshot ID", item.ID+"-snapshot", func(snapshotID string) {
					go func() {
						_, err := wc.CreateDBSnapshot(context.Background(), &rds.CreateDBSnapshotInput{
							DBInstanceIdentifier: awssdk.String(item.ID),
							DBSnapshotIdentifier: awssdk.String(snapshotID),
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
			Label: "Delete DB instance",
			Key:   'd',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.ConfirmDelete(item.ID, func() {
					go func() {
						_, err := wc.DeleteDBInstance(context.Background(), &rds.DeleteDBInstanceInput{
							DBInstanceIdentifier: awssdk.String(item.ID),
							SkipFinalSnapshot:    awssdk.Bool(true),
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
