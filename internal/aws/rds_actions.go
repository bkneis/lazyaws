package aws

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
)

// rdsInferConnect builds a DB shell command from instance meta.
// Maps engine type to the appropriate CLI tool (psql/mysql).
func rdsInferConnect(item Item) string {
	engine := item.Meta["engine_type"]
	endpoint := item.Meta["endpoint"]
	port := item.Meta["port"]
	user := item.Meta["master_username"]

	if endpoint == "" {
		return ""
	}

	switch {
	case strings.HasPrefix(engine, "postgres"), strings.HasPrefix(engine, "aurora-postgresql"):
		return fmt.Sprintf("psql -h %s -p %s -U %s", endpoint, port, user)
	case strings.HasPrefix(engine, "mysql"), strings.HasPrefix(engine, "aurora-mysql"), engine == "mariadb":
		return fmt.Sprintf("mysql -h %s -P %s -u %s -p", endpoint, port, user)
	default:
		return ""
	}
}

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

	// Enter shell is always available — pure shell-out, no SDK write ops needed.
	actions := []ActionDef{
		{
			Label: "Enter DB shell",
			Key:   'e',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				cmd := rdsInferConnect(item)
				if cmd == "" {
					cmd = "psql -h <endpoint> -p 5432 -U <user>"
				}
				ac.PromptInput("DB connect command", cmd, func(command string) {
					ac.SuspendAndRun(func() {
						parts := strings.Fields(command)
						if len(parts) == 0 {
							return
						}
						c := exec.Command(parts[0], parts[1:]...)
						c.Stdin = os.Stdin
						c.Stdout = os.Stdout
						c.Stderr = os.Stderr
						_ = c.Run()
					})
				})
				return nil
			},
		},
	}

	wc, ok := p.client.(RDSActionsAPI)
	if !ok {
		return actions
	}
	return append(actions, []ActionDef{
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
	}...)
}
