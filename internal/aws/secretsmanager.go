package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type SMAPI interface {
	ListSecrets(ctx context.Context, in *secretsmanager.ListSecretsInput, opts ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
	DescribeSecret(ctx context.Context, in *secretsmanager.DescribeSecretInput, opts ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error)
	GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput, opts ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	ListSecretVersionIds(ctx context.Context, in *secretsmanager.ListSecretVersionIdsInput, opts ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

type SMProvider struct{ client SMAPI }

func NewSecretsManagerProvider(cfg awssdk.Config, endpointURL string) *SMProvider {
	var opts []func(*secretsmanager.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *secretsmanager.Options) { o.BaseEndpoint = awssdk.String(endpointURL) })
	}
	return &SMProvider{client: secretsmanager.NewFromConfig(cfg, opts...)}
}

func NewSMProviderWithClient(client SMAPI) *SMProvider { return &SMProvider{client: client} }

func (p *SMProvider) Name() string { return "Secrets Manager" }

func (p *SMProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.ListSecrets(ctx, &secretsmanager.ListSecretsInput{})
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	items := make([]Item, len(out.SecretList))
	for i, s := range out.SecretList {
		name := awssdk.ToString(s.Name)
		items[i] = Item{ID: awssdk.ToString(s.ARN), Name: name}
	}
	return filterItems(items, query), nil
}

func (p *SMProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *SMProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Value", Fetch: p.tabValue},
		{Label: "Versions", Fetch: p.tabVersions},
	}
}

func (p *SMProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	rotation := "Disabled"
	if awssdk.ToBool(out.RotationEnabled) {
		freq := ""
		if out.RotationRules != nil && out.RotationRules.AutomaticallyAfterDays != nil {
			freq = fmt.Sprintf(" (every %d days)", awssdk.ToInt64(out.RotationRules.AutomaticallyAfterDays))
		}
		rotation = "Enabled" + freq
	}
	kms := awssdk.ToString(out.KmsKeyId)
	if kms == "" {
		kms = "aws/secretsmanager"
	}
	formatTime := func(t *time.Time) string {
		if t == nil {
			return "(never)"
		}
		return t.Format(time.DateOnly)
	}
	return KV([][2]string{
		{"ARN", awssdk.ToString(out.ARN)},
		{"Rotation", rotation},
		{"Last Rotated", formatTime(out.LastRotatedDate)},
		{"Last Accessed", formatTime(out.LastAccessedDate)},
		{"Last Changed", formatTime(out.LastChangedDate)},
		{"KMS Key", kms},
	}), nil
}

func (p *SMProvider) tabValue(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	secretStr := awssdk.ToString(out.SecretString)

	// Try to parse as JSON object
	var kvMap map[string]any
	if err := json.Unmarshal([]byte(secretStr), &kvMap); err == nil {
		keys := make([]string, 0, len(kvMap))
		for k := range kvMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([][2]string, len(keys))
		for i, k := range keys {
			v := kvMap[k]
			var valStr string
			switch tv := v.(type) {
			case string:
				if IsSensitiveKey(k) {
					valStr = "••••••••••••••••"
				} else {
					valStr = tv
				}
			case map[string]any:
				valStr = "[object]"
			default:
				valStr = fmt.Sprintf("%v", v)
			}
			pairs[i] = [2]string{k, valStr}
		}
		return KV(pairs), nil
	}

	// Plain string
	return "  " + secretStr + "\n", nil
}

func (p *SMProvider) tabVersions(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{SecretId: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	if len(out.Versions) == 0 {
		return "  (no versions)\n", nil
	}
	rows := make([][]string, len(out.Versions))
	for i, v := range out.Versions {
		labels := strings.Join(v.VersionStages, ", ")
		created := ""
		if v.CreatedDate != nil {
			created = v.CreatedDate.Format(time.DateOnly)
		}
		rows[i] = []string{awssdk.ToString(v.VersionId), labels, created}
	}
	return Table([]string{"Version ID", "Staging Labels", "Created"}, rows), nil
}
