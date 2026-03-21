package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// SSMAPI is the subset of the SSM client used by SSMProvider.
type SSMAPI interface {
	DescribeParameters(ctx context.Context, in *ssm.DescribeParametersInput, opts ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error)
	GetParameter(ctx context.Context, in *ssm.GetParameterInput, opts ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	GetParameterHistory(ctx context.Context, in *ssm.GetParameterHistoryInput, opts ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
}

// SSMProvider implements Provider for AWS Systems Manager Parameter Store.
type SSMProvider struct {
	client SSMAPI
}

func NewSSMProvider(cfg awssdk.Config, endpointURL string) *SSMProvider {
	var opts []func(*ssm.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *ssm.Options) { o.BaseEndpoint = awssdk.String(endpointURL) })
	}
	return &SSMProvider{client: ssm.NewFromConfig(cfg, opts...)}
}

func NewSSMProviderWithClient(client SSMAPI) *SSMProvider { return &SSMProvider{client: client} }

func (p *SSMProvider) Name() string { return "SSM Params" }

func (p *SSMProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	var items []Item
	var nextToken *string
	for {
		out, err := p.client.DescribeParameters(ctx, &ssm.DescribeParametersInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe parameters: %w", err)
		}
		for _, pm := range out.Parameters {
			name := awssdk.ToString(pm.Name)
			modified := ""
			if pm.LastModifiedDate != nil {
				modified = pm.LastModifiedDate.Format(time.DateOnly)
			}
			items = append(items, Item{
				ID:   name,
				Name: name,
				Meta: map[string]string{
					"type":        string(pm.Type),
					"description": awssdk.ToString(pm.Description),
					"tier":        string(pm.Tier),
					"key_id":      awssdk.ToString(pm.KeyId),
					"modified":    modified,
					"version":     fmt.Sprintf("%d", pm.Version),
				},
			})
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return filterItems(items, query), nil
}

func (p *SSMProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *SSMProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Value", Fetch: p.tabValue},
		{Label: "History", Fetch: p.tabHistory},
	}
}

func (p *SSMProvider) tabOverview(_ context.Context, item Item) (string, error) {
	return KV([][2]string{
		{"Name", item.ID},
		{"Type", item.Meta["type"]},
		{"Tier", item.Meta["tier"]},
		{"Description", item.Meta["description"]},
		{"KMS Key", item.Meta["key_id"]},
		{"Version", item.Meta["version"]},
		{"Last Modified", item.Meta["modified"]},
	}), nil
}

func (p *SSMProvider) tabValue(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           awssdk.String(item.ID),
		WithDecryption: awssdk.Bool(true),
	})
	if err != nil {
		return "", err
	}
	value := awssdk.ToString(out.Parameter.Value)
	paramType := item.Meta["type"]

	var sb strings.Builder
	if paramType == string(ssmtypes.ParameterTypeSecureString) {
		sb.WriteString("[red]SecureString (decrypted)[-]\n\n")
	}
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		// Multi-line structured data — no leading indent so brackets align
		sb.WriteString(tviewEscape(value))
	} else {
		sb.WriteString("  ")
		sb.WriteString(tviewEscape(value))
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func (p *SSMProvider) tabHistory(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
		Name: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	if len(out.Parameters) == 0 {
		return "  (no history)\n", nil
	}
	rows := make([][]string, len(out.Parameters))
	for i, h := range out.Parameters {
		modified := ""
		if h.LastModifiedDate != nil {
			modified = h.LastModifiedDate.Format(time.DateTime)
		}
		rows[i] = []string{
			fmt.Sprintf("%d", h.Version),
			modified,
			awssdk.ToString(h.LastModifiedUser),
		}
	}
	return Table([]string{"Version", "Modified", "User"}, rows), nil
}
