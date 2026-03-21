package aws

import (
	"context"
	"encoding/json"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// SMActionsAPI defines write operations needed by the Secrets Manager actions menu.
type SMActionsAPI interface {
	CreateSecret(ctx context.Context, in *secretsmanager.CreateSecretInput, opts ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	DeleteSecret(ctx context.Context, in *secretsmanager.DeleteSecretInput, opts ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
	PutSecretValue(ctx context.Context, in *secretsmanager.PutSecretValueInput, opts ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
	GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput, opts ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// Actions implements Actionable for SMProvider.
func (p *SMProvider) Actions(item Item) []ActionDef {
	wc, ok := p.client.(SMActionsAPI)
	if !ok {
		return nil
	}

	actions := []ActionDef{
		{
			Label: "Create secret",
			Key:   'c',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.PromptInput("Secret name", "", func(name string) {
					ac.PromptInput("Secret value", "", func(value string) {
						go func() {
							_, err := wc.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
								Name:         awssdk.String(name),
								SecretString: awssdk.String(value),
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
				Label: "Delete secret",
				Key:   'd',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.ConfirmDelete(item.Name, func() {
						go func() {
							_, err := wc.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
								SecretId:                   awssdk.String(item.ID),
								ForceDeleteWithoutRecovery: awssdk.Bool(false),
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
				Label: "Delete key",
				Key:   'k',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					go func() {
						out, err := wc.GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{
							SecretId: awssdk.String(item.ID),
						})
						if err != nil {
							ac.ShowError(err)
							return
						}
						var m map[string]any
						if json.Unmarshal([]byte(awssdk.ToString(out.SecretString)), &m) != nil {
							ac.ShowError(fmt.Errorf("secret is not JSON"))
							return
						}
						ac.PromptInput("Key to delete", "", func(key string) {
							go func() {
								delete(m, key)
								b, _ := json.Marshal(m)
								_, err := wc.PutSecretValue(context.Background(), &secretsmanager.PutSecretValueInput{
									SecretId:     awssdk.String(item.ID),
									SecretString: awssdk.String(string(b)),
								})
								if err != nil {
									ac.ShowError(err)
									return
								}
								ac.Refresh()
							}()
						})
					}()
					return nil
				},
			},
			ActionDef{
				Label: "Update value",
				Key:   'u',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					go func() {
						out, err := wc.GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{
							SecretId: awssdk.String(item.ID),
						})
						if err != nil {
							ac.ShowError(err)
							return
						}
						current := awssdk.ToString(out.SecretString)
						var m map[string]any
						if json.Unmarshal([]byte(current), &m) == nil {
							// JSON secret: update a single key
							ac.PromptInput("Key to update/add", "", func(key string) {
								ac.PromptInput("New value for "+key, "", func(val string) {
									go func() {
										m[key] = val
										b, _ := json.Marshal(m)
										_, err := wc.PutSecretValue(context.Background(), &secretsmanager.PutSecretValueInput{
											SecretId:     awssdk.String(item.ID),
											SecretString: awssdk.String(string(b)),
										})
										if err != nil {
											ac.ShowError(err)
											return
										}
										ac.Refresh()
									}()
								})
							})
						} else {
							// Plain string secret
							ac.PromptInput("New secret value", "", func(value string) {
								go func() {
									_, err := wc.PutSecretValue(context.Background(), &secretsmanager.PutSecretValueInput{
										SecretId:     awssdk.String(item.ID),
										SecretString: awssdk.String(value),
									})
									if err != nil {
										ac.ShowError(err)
										return
									}
									ac.Refresh()
								}()
							})
						}
					}()
					return nil
				},
			},
		)
	}

	return actions
}
