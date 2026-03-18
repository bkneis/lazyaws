package aws_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

// stubLambda implements awspkg.LambdaAPI using in-memory data.
type stubLambda struct {
	functions []lambdatypes.FunctionConfiguration
}

func newStubLambda() *stubLambda {
	return &stubLambda{
		functions: []lambdatypes.FunctionConfiguration{
			{
				FunctionName: aws.String("my-function"),
				Runtime:      lambdatypes.RuntimePython312,
				MemorySize:   aws.Int32(128),
				Timeout:      aws.Int32(30),
				Handler:      aws.String("handler.main"),
				Role:         aws.String("arn:aws:iam::123456789:role/my-role"),
			},
		},
	}
}

func (s *stubLambda) ListFunctions(_ context.Context, _ *lambda.ListFunctionsInput, _ ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error) {
	return &lambda.ListFunctionsOutput{Functions: s.functions}, nil
}

func (s *stubLambda) GetFunction(_ context.Context, in *lambda.GetFunctionInput, _ ...func(*lambda.Options)) (*lambda.GetFunctionOutput, error) {
	for _, f := range s.functions {
		if aws.ToString(f.FunctionName) == aws.ToString(in.FunctionName) {
			fc := f
			return &lambda.GetFunctionOutput{Configuration: &fc}, nil
		}
	}
	return nil, fmt.Errorf("function not found: %s", aws.ToString(in.FunctionName))
}

func (s *stubLambda) GetFunctionConfiguration(_ context.Context, in *lambda.GetFunctionConfigurationInput, _ ...func(*lambda.Options)) (*lambda.GetFunctionConfigurationOutput, error) {
	for _, f := range s.functions {
		if aws.ToString(f.FunctionName) == aws.ToString(in.FunctionName) {
			return &lambda.GetFunctionConfigurationOutput{
				FunctionName: f.FunctionName,
				Runtime:      f.Runtime,
				MemorySize:   f.MemorySize,
				Timeout:      f.Timeout,
				Handler:      f.Handler,
				Role:         f.Role,
				Environment: &lambdatypes.EnvironmentResponse{
					Variables: map[string]string{
						"DB_HOST":   "localhost",
						"LOG_LEVEL": "INFO",
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("function not found: %s", aws.ToString(in.FunctionName))
}

func (s *stubLambda) ListEventSourceMappings(_ context.Context, _ *lambda.ListEventSourceMappingsInput, _ ...func(*lambda.Options)) (*lambda.ListEventSourceMappingsOutput, error) {
	arn := "arn:aws:sqs:us-east-1:123456789:my-queue"
	state := "Enabled"
	return &lambda.ListEventSourceMappingsOutput{
		EventSourceMappings: []lambdatypes.EventSourceMappingConfiguration{
			{EventSourceArn: &arn, State: &state},
		},
	}, nil
}

func TestLambdaProvider_FetchItem(t *testing.T) {
	p := awspkg.NewLambdaProviderWithClient(newStubLambda())

	cases := []struct {
		name    string
		id      string
		wantErr bool
		wantID  string
	}{
		{"found", "my-function", false, "my-function"},
		{"not found", "unknown-function", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item, err := p.FetchItem(context.Background(), tc.id)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if item.ID != tc.wantID || item.Name != tc.wantID {
				t.Errorf("got ID=%q Name=%q, want both %q", item.ID, item.Name, tc.wantID)
			}
		})
	}
}

func TestLambdaProvider_ListItems(t *testing.T) {
	stub := &stubLambda{
		functions: []lambdatypes.FunctionConfiguration{
			{FunctionName: aws.String("my-function"), Runtime: lambdatypes.RuntimeProvidedal2023},
		},
	}

	p := awspkg.NewLambdaProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Name != "my-function" {
		t.Errorf("got name %q, want my-function", items[0].Name)
	}
}

func TestLambdaProvider_ListItems_Filter(t *testing.T) {
	stub := &stubLambda{
		functions: []lambdatypes.FunctionConfiguration{
			{FunctionName: aws.String("my-function"), Runtime: lambdatypes.RuntimePython312, MemorySize: aws.Int32(128), Timeout: aws.Int32(30), Handler: aws.String("handler.main"), Role: aws.String("arn:aws:iam::123:role/r")},
			{FunctionName: aws.String("other-handler"), Runtime: lambdatypes.RuntimePython312, MemorySize: aws.Int32(256), Timeout: aws.Int32(60), Handler: aws.String("handler.main"), Role: aws.String("arn:aws:iam::123:role/r")},
		},
	}
	p := awspkg.NewLambdaProviderWithClient(stub)
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

type stubLambdaNilConfig struct{}

func (s *stubLambdaNilConfig) ListFunctions(_ context.Context, _ *lambda.ListFunctionsInput, _ ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error) {
	return &lambda.ListFunctionsOutput{}, nil
}

func (s *stubLambdaNilConfig) GetFunction(_ context.Context, _ *lambda.GetFunctionInput, _ ...func(*lambda.Options)) (*lambda.GetFunctionOutput, error) {
	return &lambda.GetFunctionOutput{Configuration: nil}, nil
}

func (s *stubLambdaNilConfig) GetFunctionConfiguration(_ context.Context, _ *lambda.GetFunctionConfigurationInput, _ ...func(*lambda.Options)) (*lambda.GetFunctionConfigurationOutput, error) {
	return &lambda.GetFunctionConfigurationOutput{}, nil
}

func (s *stubLambdaNilConfig) ListEventSourceMappings(_ context.Context, _ *lambda.ListEventSourceMappingsInput, _ ...func(*lambda.Options)) (*lambda.ListEventSourceMappingsOutput, error) {
	return &lambda.ListEventSourceMappingsOutput{}, nil
}

func TestLambdaProvider_GetDetail_nilConfiguration(t *testing.T) {
	p := awspkg.NewLambdaProviderWithClient(&stubLambdaNilConfig{})
	_, err := p.GetDetail(context.Background(), awspkg.Item{ID: "my-function", Name: "my-function"})
	if err == nil {
		t.Error("expected error for nil configuration, got nil")
	}
}

func TestLambdaProvider_GetDetail(t *testing.T) {
	p := awspkg.NewLambdaProviderWithClient(newStubLambda())
	items, _ := p.ListItems(context.Background(), "")

	detail, err := p.GetDetail(context.Background(), items[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(detail, "python3.12") {
		t.Errorf("detail missing runtime\ngot:\n%s", detail)
	}
}

func TestLambdaProvider_Tabs(t *testing.T) {
	p := awspkg.NewLambdaProviderWithClient(newStubLambda())
	tabs := p.Tabs()

	if len(tabs) != 3 {
		t.Fatalf("got %d tabs, want 3", len(tabs))
	}

	item := awspkg.Item{ID: "my-function", Name: "my-function"}

	cases := []struct {
		tabIdx int
		label  string
		want   string
	}{
		{0, "Overview", "python3.12"},
		{1, "Env", "DB_HOST"},
		{2, "Triggers", "my-queue"},
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
				t.Errorf("tab %d content missing %q\ngot:\n%s", tc.tabIdx, tc.want, content)
			}
		})
	}
}
