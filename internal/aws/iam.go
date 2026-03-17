package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

type IAMAPI interface {
	ListRoles(ctx context.Context, in *iam.ListRolesInput, opts ...func(*iam.Options)) (*iam.ListRolesOutput, error)
	GetRole(ctx context.Context, in *iam.GetRoleInput, opts ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	ListAttachedRolePolicies(ctx context.Context, in *iam.ListAttachedRolePoliciesInput, opts ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error)
	ListRolePolicies(ctx context.Context, in *iam.ListRolePoliciesInput, opts ...func(*iam.Options)) (*iam.ListRolePoliciesOutput, error)
}

type IAMProvider struct{ client IAMAPI }

func NewIAMProvider(cfg awssdk.Config, endpointURL string) *IAMProvider {
	var opts []func(*iam.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *iam.Options) { o.BaseEndpoint = awssdk.String(endpointURL) })
	}
	return &IAMProvider{client: iam.NewFromConfig(cfg, opts...)}
}

func NewIAMProviderWithClient(client IAMAPI) *IAMProvider { return &IAMProvider{client: client} }

func (p *IAMProvider) Name() string { return "IAM Roles" }

func (p *IAMProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.ListRoles(ctx, &iam.ListRolesInput{})
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	items := make([]Item, len(out.Roles))
	for i, r := range out.Roles {
		name := awssdk.ToString(r.RoleName)
		items[i] = Item{ID: name, Name: name}
	}
	return filterItems(items, query), nil
}

func (p *IAMProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *IAMProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Policies", Fetch: p.tabPolicies},
		{Label: "Trust", Fetch: p.tabTrust},
	}
}

func (p *IAMProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetRole(ctx, &iam.GetRoleInput{RoleName: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	r := out.Role
	created := ""
	if r.CreateDate != nil {
		created = r.CreateDate.Format(time.DateOnly)
	}
	maxSession := formatDuration(int64(awssdk.ToInt32(r.MaxSessionDuration)))
	return KV([][2]string{
		{"Name", awssdk.ToString(r.RoleName)},
		{"ARN", awssdk.ToString(r.Arn)},
		{"Created", created},
		{"Max Session", maxSession},
		{"Description", awssdk.ToString(r.Description)},
	}), nil
}

func (p *IAMProvider) tabPolicies(ctx context.Context, item Item) (string, error) {
	managed, err := p.client.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	inline, err := p.client.ListRolePolicies(ctx, &iam.ListRolePoliciesInput{RoleName: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	var rows [][]string
	for _, mp := range managed.AttachedPolicies {
		rows = append(rows, []string{"Managed", awssdk.ToString(mp.PolicyName)})
	}
	for _, name := range inline.PolicyNames {
		rows = append(rows, []string{"Inline", name})
	}
	if len(rows) == 0 {
		return "  (no policies attached)\n", nil
	}
	return Table([]string{"Type", "Name"}, rows), nil
}

func (p *IAMProvider) tabTrust(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetRole(ctx, &iam.GetRoleInput{RoleName: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	// AssumeRolePolicyDocument is URL-encoded JSON
	docStr, err := url.QueryUnescape(awssdk.ToString(out.Role.AssumeRolePolicyDocument))
	if err != nil {
		docStr = awssdk.ToString(out.Role.AssumeRolePolicyDocument)
	}

	var doc struct {
		Statement []struct {
			Principal json.RawMessage `json:"Principal"`
			Action    json.RawMessage `json:"Action"`
			Condition json.RawMessage `json:"Condition"`
		} `json:"Statement"`
	}
	if err := json.Unmarshal([]byte(docStr), &doc); err != nil || len(doc.Statement) == 0 {
		return "  " + docStr + "\n", nil
	}

	stmt := doc.Statement[0]
	principal := parsePrincipal(stmt.Principal)
	action := parseJSONStringOrArray(stmt.Action)
	condition := "(none)"
	if len(stmt.Condition) > 0 && string(stmt.Condition) != "null" {
		condition = string(stmt.Condition)
	}
	return KV([][2]string{
		{"Principal", principal},
		{"Action", action},
		{"Condition", condition},
	}), nil
}

func parsePrincipal(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try {"Service": "lambda.amazonaws.com"}
	var svcMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &svcMap); err == nil {
		if svc, ok := svcMap["Service"]; ok {
			return parseJSONStringOrArray(svc)
		}
		if fed, ok := svcMap["Federated"]; ok {
			return parseJSONStringOrArray(fed)
		}
		if aws, ok := svcMap["AWS"]; ok {
			return parseJSONStringOrArray(aws)
		}
	}
	// Try plain string or array
	return parseJSONStringOrArray(raw)
}

func parseJSONStringOrArray(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		if len(arr) == 1 {
			return arr[0]
		}
		return strings.Join(arr, ", ")
	}
	return string(raw)
}

func formatDuration(seconds int64) string {
	switch {
	case seconds >= 86400:
		return fmt.Sprintf("%dd", seconds/86400)
	case seconds >= 3600:
		return fmt.Sprintf("%dh", seconds/3600)
	case seconds >= 60:
		return fmt.Sprintf("%dm", seconds/60)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}
