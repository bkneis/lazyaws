package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
)

// LambdaActionsAPI defines write operations needed by the Lambda actions menu.
// The real lambda.Client satisfies this interface structurally.
type LambdaActionsAPI interface {
	Invoke(ctx context.Context, in *lambda.InvokeInput, opts ...func(*lambda.Options)) (*lambda.InvokeOutput, error)
	DeleteFunction(ctx context.Context, in *lambda.DeleteFunctionInput, opts ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error)
}

// Actions implements Actionable for LambdaProvider.
func (p *LambdaProvider) Actions(item Item) []ActionDef {
	if item.ID == "" {
		return nil
	}
	wc, ok := p.client.(LambdaActionsAPI)
	if !ok {
		return nil
	}
	return []ActionDef{
		{
			Label: "Invoke function",
			Key:   'i',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.PromptInput("Invoke payload (JSON)", "{}", func(payload string) {
					go func() {
						out, err := wc.Invoke(context.Background(), &lambda.InvokeInput{
							FunctionName: awssdk.String(item.ID),
							Payload:      []byte(payload),
						})
						if err != nil {
							ac.ShowError(err)
							return
						}
						if out.FunctionError != nil {
							ac.ShowError(fmt.Errorf("%s: %s", *out.FunctionError, string(out.Payload)))
							return
						}
						resp := string(out.Payload)
						if resp == "" || resp == "null" {
							resp = "(no response body)"
						}
						ac.ShowInfo(fmt.Sprintf("Invoked %s (status %d)\n\n%s", item.ID, out.StatusCode, resp))
					}()
				})
				return nil
			},
		},
		{
			Label: "Delete function",
			Key:   'd',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.ConfirmDelete(item.ID, func() {
					go func() {
						_, err := wc.DeleteFunction(context.Background(), &lambda.DeleteFunctionInput{
							FunctionName: awssdk.String(item.ID),
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
