package aws

import (
	"context"
	"strconv"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// Route53ActionsAPI defines write operations needed by the Route53 actions menu.
type Route53ActionsAPI interface {
	CreateHostedZone(ctx context.Context, in *route53.CreateHostedZoneInput, opts ...func(*route53.Options)) (*route53.CreateHostedZoneOutput, error)
	ChangeResourceRecordSets(ctx context.Context, in *route53.ChangeResourceRecordSetsInput, opts ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
}

// Actions implements Actionable for Route53Provider.
func (p *Route53Provider) Actions(item Item) []ActionDef {
	wc, ok := p.client.(Route53ActionsAPI)
	if !ok {
		return nil
	}

	actions := []ActionDef{
		{
			Label: "Create hosted zone",
			Key:   'c',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.PromptInput("Zone name", "example.com.", func(name string) {
					go func() {
						_, err := wc.CreateHostedZone(context.Background(), &route53.CreateHostedZoneInput{
							Name:            awssdk.String(name),
							CallerReference: awssdk.String(strconv.FormatInt(time.Now().UnixNano(), 10)),
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
		actions = append(actions, ActionDef{
			Label: "Add/update record",
			Key:   'a',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.PromptInput("Record name", "", func(name string) {
					ac.PromptInput("Type", "A", func(rtype string) {
						ac.PromptInput("TTL (seconds)", "300", func(ttlStr string) {
							ac.PromptInput("Value", "", func(value string) {
								go func() {
									ttl, _ := strconv.ParseInt(ttlStr, 10, 64)
									if ttl <= 0 {
										ttl = 300
									}
									_, err := wc.ChangeResourceRecordSets(context.Background(), &route53.ChangeResourceRecordSetsInput{
										HostedZoneId: awssdk.String(item.ID),
										ChangeBatch: &r53types.ChangeBatch{
											Changes: []r53types.Change{{
												Action: r53types.ChangeActionUpsert,
												ResourceRecordSet: &r53types.ResourceRecordSet{
													Name: awssdk.String(name),
													Type: r53types.RRType(rtype),
													TTL:  awssdk.Int64(ttl),
													ResourceRecords: []r53types.ResourceRecord{
														{Value: awssdk.String(value)},
													},
												},
											}},
										},
									})
									if err != nil {
										ac.ShowError(err)
										return
									}
									ac.Refresh()
								}()
							})
						})
					})
				})
				return nil
			},
		})
	}

	return actions
}
