package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

type stubIAM struct{}

func (s *stubIAM) ListRoles(_ context.Context, _ *iam.ListRolesInput, _ ...func(*iam.Options)) (*iam.ListRolesOutput, error) {
	return &iam.ListRolesOutput{
		Roles: []iamtypes.Role{
			{RoleName: aws.String("lambda-execution-role")},
			{RoleName: aws.String("ecs-task-role")},
		},
	}, nil
}

func (s *stubIAM) GetRole(_ context.Context, _ *iam.GetRoleInput, _ ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	created := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	trustDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	return &iam.GetRoleOutput{
		Role: &iamtypes.Role{
			RoleName:                 aws.String("lambda-execution-role"),
			Arn:                      aws.String("arn:aws:iam::123456789:role/lambda-execution-role"),
			CreateDate:               &created,
			MaxSessionDuration:       aws.Int32(3600),
			Description:              aws.String("Role for Lambda execution"),
			AssumeRolePolicyDocument: aws.String(trustDoc),
		},
	}, nil
}

func (s *stubIAM) ListAttachedRolePolicies(_ context.Context, _ *iam.ListAttachedRolePoliciesInput, _ ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error) {
	return &iam.ListAttachedRolePoliciesOutput{
		AttachedPolicies: []iamtypes.AttachedPolicy{
			{PolicyName: aws.String("AWSLambdaBasicExecutionRole"), PolicyArn: aws.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole")},
		},
	}, nil
}

func (s *stubIAM) ListRolePolicies(_ context.Context, _ *iam.ListRolePoliciesInput, _ ...func(*iam.Options)) (*iam.ListRolePoliciesOutput, error) {
	return &iam.ListRolePoliciesOutput{
		PolicyNames: []string{"inline-s3-read"},
	}, nil
}

func TestIAMProvider_ListItems(t *testing.T) {
	p := awspkg.NewIAMProviderWithClient(&stubIAM{})
	items, err := p.ListItems(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].Name != "lambda-execution-role" {
		t.Errorf("got name %q, want lambda-execution-role", items[0].Name)
	}
	if items[1].Name != "ecs-task-role" {
		t.Errorf("got name %q, want ecs-task-role", items[1].Name)
	}
}

func TestIAMProvider_TabOverview(t *testing.T) {
	p := awspkg.NewIAMProviderWithClient(&stubIAM{})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "lambda-execution-role", Name: "lambda-execution-role"}
	content, err := tabs[0].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "arn:aws:iam::123456789:role/lambda-execution-role") {
		t.Errorf("expected ARN in overview\ngot:\n%s", content)
	}
	if !strings.Contains(content, "1h") {
		t.Errorf("expected max session '1h' in overview\ngot:\n%s", content)
	}
}

func TestIAMProvider_TabPolicies(t *testing.T) {
	p := awspkg.NewIAMProviderWithClient(&stubIAM{})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "lambda-execution-role", Name: "lambda-execution-role"}
	content, err := tabs[1].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "Managed") {
		t.Errorf("expected 'Managed' in policies\ngot:\n%s", content)
	}
	if !strings.Contains(content, "AWSLambdaBasicExecutionRole") {
		t.Errorf("expected managed policy name\ngot:\n%s", content)
	}
	if !strings.Contains(content, "Inline") {
		t.Errorf("expected 'Inline' in policies\ngot:\n%s", content)
	}
	if !strings.Contains(content, "inline-s3-read") {
		t.Errorf("expected inline policy name\ngot:\n%s", content)
	}
}

func TestIAMProvider_TabTrust(t *testing.T) {
	p := awspkg.NewIAMProviderWithClient(&stubIAM{})
	tabs := p.Tabs()
	item := awspkg.Item{ID: "lambda-execution-role", Name: "lambda-execution-role"}
	content, err := tabs[2].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "lambda.amazonaws.com") {
		t.Errorf("expected 'lambda.amazonaws.com' in trust\ngot:\n%s", content)
	}
	if !strings.Contains(content, "sts:AssumeRole") {
		t.Errorf("expected 'sts:AssumeRole' in trust\ngot:\n%s", content)
	}
}

func TestIAMProvider_Tabs_Count(t *testing.T) {
	p := awspkg.NewIAMProviderWithClient(&stubIAM{})
	tabs := p.Tabs()
	if len(tabs) != 3 {
		t.Fatalf("got %d tabs, want 3", len(tabs))
	}
	labels := []string{"Overview", "Policies", "Trust"}
	for i, label := range labels {
		if tabs[i].Label != label {
			t.Errorf("tab %d label = %q, want %q", i, tabs[i].Label, label)
		}
	}
}
