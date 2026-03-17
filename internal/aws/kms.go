package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// KMSAPI is the subset of the KMS client methods used by KMSProvider.
type KMSAPI interface {
	ListKeys(ctx context.Context, in *kms.ListKeysInput, opts ...func(*kms.Options)) (*kms.ListKeysOutput, error)
	DescribeKey(ctx context.Context, in *kms.DescribeKeyInput, opts ...func(*kms.Options)) (*kms.DescribeKeyOutput, error)
	GetKeyRotationStatus(ctx context.Context, in *kms.GetKeyRotationStatusInput, opts ...func(*kms.Options)) (*kms.GetKeyRotationStatusOutput, error)
	GetKeyPolicy(ctx context.Context, in *kms.GetKeyPolicyInput, opts ...func(*kms.Options)) (*kms.GetKeyPolicyOutput, error)
	ListAliases(ctx context.Context, in *kms.ListAliasesInput, opts ...func(*kms.Options)) (*kms.ListAliasesOutput, error)
}

// KMSProvider implements Provider for AWS Key Management Service.
type KMSProvider struct {
	client KMSAPI
}

func NewKMSProvider(cfg awssdk.Config, endpointURL string) *KMSProvider {
	var opts []func(*kms.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *kms.Options) {
			o.BaseEndpoint = awssdk.String(endpointURL)
		})
	}
	return &KMSProvider{client: kms.NewFromConfig(cfg, opts...)}
}

func NewKMSProviderWithClient(client KMSAPI) *KMSProvider {
	return &KMSProvider{client: client}
}

func (p *KMSProvider) Name() string { return "KMS" }

// ListItems shows raw key IDs — description is fetched lazily in the Overview tab
// to avoid N+1 DescribeKey calls on accounts with many keys.
func (p *KMSProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	var items []Item
	var marker *string
	for {
		out, err := p.client.ListKeys(ctx, &kms.ListKeysInput{
			Marker: marker,
		})
		if err != nil {
			return nil, fmt.Errorf("list keys: %w", err)
		}
		for _, k := range out.Keys {
			id := awssdk.ToString(k.KeyId)
			items = append(items, Item{ID: id, Name: id})
		}
		if !out.Truncated || out.NextMarker == nil {
			break
		}
		marker = out.NextMarker
	}
	return filterItems(items, query), nil
}

func (p *KMSProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *KMSProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Policy", Fetch: p.tabPolicy},
		{Label: "Aliases", Fetch: p.tabAliases},
	}
}

func (p *KMSProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	desc, err := p.client.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	k := desc.KeyMetadata

	created := ""
	if k.CreationDate != nil {
		created = k.CreationDate.Format(time.DateTime)
	}

	rotation := "Disabled"
	if rot, err := p.client.GetKeyRotationStatus(ctx, &kms.GetKeyRotationStatusInput{
		KeyId: awssdk.String(item.ID),
	}); err == nil && rot.KeyRotationEnabled {
		rotation = fmt.Sprintf("Enabled (every %d days)", awssdk.ToInt32(rot.RotationPeriodInDays))
	}

	return KV([][2]string{
		{"Key ID", awssdk.ToString(k.KeyId)},
		{"ARN", awssdk.ToString(k.Arn)},
		{"Description", awssdk.ToString(k.Description)},
		{"State", string(k.KeyState)},
		{"Usage", string(k.KeyUsage)},
		{"Spec", string(k.KeySpec)},
		{"Origin", string(k.Origin)},
		{"Created", created},
		{"Rotation", rotation},
	}), nil
}

func (p *KMSProvider) tabPolicy(ctx context.Context, item Item) (string, error) {
	policyName := "default"
	out, err := p.client.GetKeyPolicy(ctx, &kms.GetKeyPolicyInput{
		KeyId:      awssdk.String(item.ID),
		PolicyName: awssdk.String(policyName),
	})
	if err != nil {
		return "", err
	}
	var raw any
	if err := json.Unmarshal([]byte(awssdk.ToString(out.Policy)), &raw); err != nil {
		return awssdk.ToString(out.Policy), nil
	}
	b, _ := json.MarshalIndent(raw, "  ", "  ")
	return "  " + string(b) + "\n", nil
}

func (p *KMSProvider) tabAliases(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListAliases(ctx, &kms.ListAliasesInput{
		KeyId: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	if len(out.Aliases) == 0 {
		return "  (no aliases)\n", nil
	}
	rows := make([][]string, len(out.Aliases))
	for i, a := range out.Aliases {
		created := ""
		if a.CreationDate != nil {
			created = a.CreationDate.Format(time.DateOnly)
		}
		rows[i] = []string{awssdk.ToString(a.AliasName), created}
	}
	return Table([]string{"Alias Name", "Creation Date"}, rows), nil
}
