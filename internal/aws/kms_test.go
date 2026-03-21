package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

type stubKMS struct {
	keys    []kmstypes.KeyListEntry
	keyMeta *kmstypes.KeyMetadata
	policy  string
	aliases []kmstypes.AliasListEntry
}

func (s *stubKMS) ListKeys(_ context.Context, _ *kms.ListKeysInput, _ ...func(*kms.Options)) (*kms.ListKeysOutput, error) {
	return &kms.ListKeysOutput{Keys: s.keys}, nil
}

func (s *stubKMS) DescribeKey(_ context.Context, _ *kms.DescribeKeyInput, _ ...func(*kms.Options)) (*kms.DescribeKeyOutput, error) {
	return &kms.DescribeKeyOutput{KeyMetadata: s.keyMeta}, nil
}

func (s *stubKMS) GetKeyRotationStatus(_ context.Context, _ *kms.GetKeyRotationStatusInput, _ ...func(*kms.Options)) (*kms.GetKeyRotationStatusOutput, error) {
	period := int32(365)
	return &kms.GetKeyRotationStatusOutput{KeyRotationEnabled: true, RotationPeriodInDays: &period}, nil
}

func (s *stubKMS) GetKeyPolicy(_ context.Context, _ *kms.GetKeyPolicyInput, _ ...func(*kms.Options)) (*kms.GetKeyPolicyOutput, error) {
	return &kms.GetKeyPolicyOutput{Policy: aws.String(s.policy)}, nil
}

func (s *stubKMS) ListAliases(_ context.Context, _ *kms.ListAliasesInput, _ ...func(*kms.Options)) (*kms.ListAliasesOutput, error) {
	return &kms.ListAliasesOutput{Aliases: s.aliases}, nil
}

func TestKMSProvider_ListItems(t *testing.T) {
	cases := []struct {
		name  string
		keys  []kmstypes.KeyListEntry
		query string
		want  int
	}{
		{"all keys", []kmstypes.KeyListEntry{{KeyId: aws.String("abc-123")}, {KeyId: aws.String("def-456")}}, "", 2},
		{"filter match", []kmstypes.KeyListEntry{{KeyId: aws.String("abc-123")}}, "abc", 1},
		{"filter no match", []kmstypes.KeyListEntry{{KeyId: aws.String("abc-123")}}, "xyz", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := awspkg.NewKMSProviderWithClient(&stubKMS{keys: tc.keys})
			items, err := p.ListItems(context.Background(), tc.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(items) != tc.want {
				t.Errorf("got %d items, want %d", len(items), tc.want)
			}
		})
	}
}

func TestKMSProvider_Tabs(t *testing.T) {
	created := time.Date(2023, 6, 1, 12, 0, 0, 0, time.UTC)
	aliasCreated := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	stub := &stubKMS{
		keys: []kmstypes.KeyListEntry{{KeyId: aws.String("abc-123")}},
		keyMeta: &kmstypes.KeyMetadata{
			KeyId:        aws.String("abc-123"),
			Arn:          aws.String("arn:aws:kms:us-east-1:123:key/abc-123"),
			Description:  aws.String("RDS encryption key"),
			KeyState:     kmstypes.KeyStateEnabled,
			KeyUsage:     kmstypes.KeyUsageTypeEncryptDecrypt,
			KeySpec:      kmstypes.KeySpecSymmetricDefault,
			Origin:       kmstypes.OriginTypeAwsKms,
			CreationDate: &created,
		},
		policy: `{"Version":"2012-10-17","Statement":[]}`,
		aliases: []kmstypes.AliasListEntry{
			{AliasName: aws.String("alias/rds-prod"), CreationDate: &aliasCreated},
		},
	}
	p := awspkg.NewKMSProviderWithClient(stub)
	item := awspkg.Item{ID: "abc-123", Name: "abc-123"}
	tabs := p.Tabs()

	cases := []struct {
		tabIdx int
		label  string
		want   string
	}{
		{0, "Overview", "RDS encryption key"},
		{1, "Policy", "2012-10-17"},
		{2, "Aliases", "alias/rds-prod"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if tabs[tc.tabIdx].Label != tc.label {
				t.Errorf("tab %d label = %q, want %q", tc.tabIdx, tabs[tc.tabIdx].Label, tc.label)
			}
			content, err := tabs[tc.tabIdx].Fetch(context.Background(), item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(content, tc.want) {
				t.Errorf("tab %q missing %q\ngot:\n%s", tc.label, tc.want, content)
			}
		})
	}
}
