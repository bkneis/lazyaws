package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// IAMPoliciesAPI is the subset of IAM client methods used by IAMPoliciesProvider.
// Note: Description is NOT returned by ListPolicies — GetPolicy is used in tabOverview.
type IAMPoliciesAPI interface {
	ListPolicies(ctx context.Context, in *iam.ListPoliciesInput, opts ...func(*iam.Options)) (*iam.ListPoliciesOutput, error)
	GetPolicy(ctx context.Context, in *iam.GetPolicyInput, opts ...func(*iam.Options)) (*iam.GetPolicyOutput, error)
	GetPolicyVersion(ctx context.Context, in *iam.GetPolicyVersionInput, opts ...func(*iam.Options)) (*iam.GetPolicyVersionOutput, error)
}

// IAMPoliciesProvider implements Provider for customer-managed IAM Policies.
type IAMPoliciesProvider struct{ client IAMPoliciesAPI }

func NewIAMPoliciesProvider(cfg awssdk.Config, endpointURL string) *IAMPoliciesProvider {
	var opts []func(*iam.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *iam.Options) { o.BaseEndpoint = awssdk.String(endpointURL) })
	}
	return &IAMPoliciesProvider{client: iam.NewFromConfig(cfg, opts...)}
}

func NewIAMPoliciesProviderWithClient(client IAMPoliciesAPI) *IAMPoliciesProvider {
	return &IAMPoliciesProvider{client: client}
}

func (p *IAMPoliciesProvider) Name() string { return "IAM Policies" }

func (p *IAMPoliciesProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.ListPolicies(ctx, &iam.ListPoliciesInput{
		Scope: iamtypes.PolicyScopeTypeLocal,
	})
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	items := make([]Item, 0, len(out.Policies))
	for _, pol := range out.Policies {
		created := ""
		if pol.CreateDate != nil {
			created = pol.CreateDate.Format(time.DateOnly)
		}
		items = append(items, Item{
			ID:   awssdk.ToString(pol.Arn),
			Name: awssdk.ToString(pol.PolicyName),
			Meta: map[string]string{
				"defaultVersionId": awssdk.ToString(pol.DefaultVersionId),
				"attachmentCount":  fmt.Sprintf("%d", awssdk.ToInt32(pol.AttachmentCount)),
				"createDate":       created,
				// Description is not returned by ListPolicies; fetched in tabOverview via GetPolicy
			},
		})
	}
	return filterItems(items, query), nil
}

func (p *IAMPoliciesProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *IAMPoliciesProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Document", Fetch: p.tabDocument},
	}
}

func (p *IAMPoliciesProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	// GetPolicy is needed for Description (not returned by ListPolicies).
	description := ""
	if out, err := p.client.GetPolicy(ctx, &iam.GetPolicyInput{PolicyArn: awssdk.String(item.ID)}); err == nil {
		description = awssdk.ToString(out.Policy.Description)
	}
	return KV([][2]string{
		{"ARN", item.ID},
		{"Attachments", item.Meta["attachmentCount"]},
		{"Created", item.Meta["createDate"]},
		{"Description", description},
	}), nil
}

func (p *IAMPoliciesProvider) tabDocument(ctx context.Context, item Item) (string, error) {
	versionID := item.Meta["defaultVersionId"]
	out, err := p.client.GetPolicyVersion(ctx, &iam.GetPolicyVersionInput{
		PolicyArn: awssdk.String(item.ID),
		VersionId: awssdk.String(versionID),
	})
	if err != nil {
		return "", fmt.Errorf("get policy version: %w", err)
	}
	docStr, err := url.QueryUnescape(awssdk.ToString(out.PolicyVersion.Document))
	if err != nil {
		docStr = awssdk.ToString(out.PolicyVersion.Document)
	}
	var raw any
	if err := json.Unmarshal([]byte(docStr), &raw); err != nil {
		return "  " + docStr + "\n", nil
	}
	b, _ := json.MarshalIndent(raw, "  ", "  ")
	return "  " + string(b) + "\n", nil
}
