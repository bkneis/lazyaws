package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

type stubSM struct {
	secretValue string
}

func (s *stubSM) ListSecrets(_ context.Context, _ *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	return &secretsmanager.ListSecretsOutput{
		SecretList: []smtypes.SecretListEntry{
			{
				Name: aws.String("prod/db-credentials"),
				ARN:  aws.String("arn:aws:secretsmanager:us-east-1:123456789:secret:prod/db-credentials-AbCdEf"),
			},
		},
	}, nil
}

func (s *stubSM) DescribeSecret(_ context.Context, _ *secretsmanager.DescribeSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error) {
	lastChanged := time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC)
	return &secretsmanager.DescribeSecretOutput{
		ARN:             aws.String("arn:aws:secretsmanager:us-east-1:123456789:secret:prod/db-credentials-AbCdEf"),
		RotationEnabled: aws.Bool(true),
		RotationRules: &smtypes.RotationRulesType{
			AutomaticallyAfterDays: aws.Int64(30),
		},
		LastChangedDate: &lastChanged,
	}, nil
}

func (s *stubSM) GetSecretValue(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	val := s.secretValue
	if val == "" {
		val = `{"db_host":"localhost","db_password":"s3cr3t","db_port":"5432"}`
	}
	return &secretsmanager.GetSecretValueOutput{
		SecretString: aws.String(val),
	}, nil
}

func (s *stubSM) ListSecretVersionIds(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	created := time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC)
	return &secretsmanager.ListSecretVersionIdsOutput{
		Versions: []smtypes.SecretVersionsListEntry{
			{
				VersionId:     aws.String("abc123def456"),
				VersionStages: []string{"AWSCURRENT"},
				CreatedDate:   &created,
			},
			{
				VersionId:     aws.String("xyz789uvw012"),
				VersionStages: []string{"AWSPREVIOUS"},
				CreatedDate:   &created,
			},
		},
	}, nil
}

func TestSMProvider_ListItems(t *testing.T) {
	p := awspkg.NewSMProviderWithClient(&stubSM{})
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Name != "prod/db-credentials" {
		t.Errorf("got name %q, want prod/db-credentials", items[0].Name)
	}
	if !strings.HasPrefix(items[0].ID, "arn:aws:secretsmanager") {
		t.Errorf("got ID %q, want ARN", items[0].ID)
	}
}

func TestSMProvider_ListItems_Filter(t *testing.T) {
	stub := &stubSMFilter{}
	p := awspkg.NewSMProviderWithClient(stub)
	cases := []struct {
		query string
		want  int
	}{
		{"", 2},
		{"my", 1},
		{"MY", 1},
		{"xyz", 0},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
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

type stubSMFilter struct{}

func (s *stubSMFilter) ListSecrets(_ context.Context, _ *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	return &secretsmanager.ListSecretsOutput{
		SecretList: []smtypes.SecretListEntry{
			{Name: aws.String("my-secret"), ARN: aws.String("arn:aws:secretsmanager:us-east-1:123:secret:my-secret-AbCdEf")},
			{Name: aws.String("other-secret"), ARN: aws.String("arn:aws:secretsmanager:us-east-1:123:secret:other-secret-GhIjKl")},
		},
	}, nil
}

func (s *stubSMFilter) DescribeSecret(_ context.Context, _ *secretsmanager.DescribeSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error) {
	return &secretsmanager.DescribeSecretOutput{}, nil
}

func (s *stubSMFilter) GetSecretValue(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	return &secretsmanager.GetSecretValueOutput{}, nil
}

func (s *stubSMFilter) ListSecretVersionIds(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	return &secretsmanager.ListSecretVersionIdsOutput{}, nil
}

func TestSMProvider_TabOverview(t *testing.T) {
	p := awspkg.NewSMProviderWithClient(&stubSM{})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "arn:aws:secretsmanager:us-east-1:123456789:secret:prod/db-credentials-AbCdEf", Name: "prod/db-credentials"}
	content, err := tabs[0].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "Enabled") {
		t.Errorf("expected rotation 'Enabled'\ngot:\n%s", content)
	}
	if !strings.Contains(content, "30") {
		t.Errorf("expected '30' days rotation frequency\ngot:\n%s", content)
	}
}

func TestSMProvider_TabVersions(t *testing.T) {
	p := awspkg.NewSMProviderWithClient(&stubSM{})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "arn:aws:secretsmanager:us-east-1:123456789:secret:prod/db-credentials-AbCdEf", Name: "prod/db-credentials"}
	content, err := tabs[2].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "AWSCURRENT") {
		t.Errorf("expected AWSCURRENT staging label\ngot:\n%s", content)
	}
	if !strings.Contains(content, "abc123def456") {
		t.Errorf("expected version ID\ngot:\n%s", content)
	}
}

func TestSMProvider_TabValue_Masking(t *testing.T) {
	p := awspkg.NewSMProviderWithClient(&stubSM{})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "arn:aws:secretsmanager:us-east-1:123456789:secret:prod/db-credentials-AbCdEf", Name: "prod/db-credentials"}
	content, err := tabs[1].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// db_password should be masked
	if !strings.Contains(content, "••••") {
		t.Errorf("expected db_password to be masked (contain ••••)\ngot:\n%s", content)
	}
	// db_host should show the value
	if !strings.Contains(content, "localhost") {
		t.Errorf("expected db_host to show 'localhost'\ngot:\n%s", content)
	}
	// raw secret value must not appear
	if strings.Contains(content, "s3cr3t") {
		t.Errorf("raw secret value 's3cr3t' must not appear in output\ngot:\n%s", content)
	}
}

func TestSMProvider_TabValue_PlainString(t *testing.T) {
	p := awspkg.NewSMProviderWithClient(&stubSM{secretValue: "plain-text-secret"})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "arn:aws:secretsmanager:us-east-1:123456789:secret:my-secret", Name: "my-secret"}
	content, err := tabs[1].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "plain-text-secret") {
		t.Errorf("expected plain string value to be shown\ngot:\n%s", content)
	}
}

func TestSMProvider_Tabs_Count(t *testing.T) {
	p := awspkg.NewSMProviderWithClient(&stubSM{})
	tabs := p.Tabs()
	if len(tabs) != 3 {
		t.Fatalf("got %d tabs, want 3", len(tabs))
	}
	labels := []string{"Overview", "Value", "Versions"}
	for i, label := range labels {
		if tabs[i].Label != label {
			t.Errorf("tab %d label = %q, want %q", i, tabs[i].Label, label)
		}
	}
}
