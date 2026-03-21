package aws

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Actions implements Actionable for ECSProvider.
// The exec action shells out to `aws ecs execute-command` — no SDK write ops needed.
func (p *ECSProvider) Actions(item Item) []ActionDef {
	if item.ID == "" {
		return nil
	}
	return []ActionDef{
		{
			Label: "Exec into container",
			Key:   'e',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				// Step 1: prompt for task ARN (user can paste full ARN or short ID)
				ac.PromptInput("Task ARN or ID", "", func(taskID string) {
					if taskID == "" {
						return
					}
					// Step 2: prompt for container name
					ac.PromptInput("Container name", "app", func(container string) {
						clusterArn := item.ID
						shell := "/bin/sh"
						cmd := fmt.Sprintf(
							"aws ecs execute-command --cluster %s --task %s --container %s --interactive --command %s",
							clusterArn, taskID, container, shell,
						)
						ac.PromptInput("Exec command", cmd, func(command string) {
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
					})
				})
				return nil
			},
		},
	}
}
