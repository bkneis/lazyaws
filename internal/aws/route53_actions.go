package aws

import (
	"context"
	"fmt"
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
	DeleteHostedZone(ctx context.Context, in *route53.DeleteHostedZoneInput, opts ...func(*route53.Options)) (*route53.DeleteHostedZoneOutput, error)
	ListResourceRecordSets(ctx context.Context, in *route53.ListResourceRecordSetsInput, opts ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
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
		actions = append(actions,
			ActionDef{
				Label: "Delete hosted zone",
				Key:   'D',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.ConfirmDelete(item.Name, func() {
						go func() {
							_, err := wc.DeleteHostedZone(context.Background(), &route53.DeleteHostedZoneInput{
								Id: awssdk.String(item.ID),
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
				Label: "Delete record",
				Key:   'r',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.PromptInput("Record name", "", func(name string) {
						ac.PromptInput("Type (A/CNAME/MX/...)", "A", func(rtype string) {
							go func() {
								out, err := wc.ListResourceRecordSets(context.Background(), &route53.ListResourceRecordSetsInput{
									HostedZoneId:    awssdk.String(item.ID),
									StartRecordName: awssdk.String(name),
									StartRecordType: r53types.RRType(rtype),
									MaxItems:        awssdk.Int32(1),
								})
								if err != nil {
									ac.ShowError(err)
									return
								}
								var found *r53types.ResourceRecordSet
								for i := range out.ResourceRecordSets {
									rrs := &out.ResourceRecordSets[i]
									if awssdk.ToString(rrs.Name) == name && string(rrs.Type) == rtype {
										found = rrs
										break
									}
								}
								if found == nil {
									ac.ShowError(fmt.Errorf("record not found"))
									return
								}
								_, err = wc.ChangeResourceRecordSets(context.Background(), &route53.ChangeResourceRecordSetsInput{
									HostedZoneId: awssdk.String(item.ID),
									ChangeBatch: &r53types.ChangeBatch{
										Changes: []r53types.Change{{
											Action:            r53types.ChangeActionDelete,
											ResourceRecordSet: found,
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
					return nil
				},
			},
		)
		actions = append(actions, ActionDef{
			Label: "Add/update record",
			Key:   'a',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.PromptInput("Record name", "", func(name string) {
					ac.PromptInput("Type", "A", func(rtype string) {
						ac.PromptInput("TTL (seconds)", "300", func(ttlStr string) {
							ac.PromptInput("Value", "", func(value string) {
								go func() {
									ttl, err := strconv.ParseInt(ttlStr, 10, 64)
									if err != nil || ttl <= 0 {
										ac.ShowError(fmt.Errorf("invalid TTL %q: must be a positive integer", ttlStr))
										return
									}
									_, err = wc.ChangeResourceRecordSets(context.Background(), &route53.ChangeResourceRecordSetsInput{
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
