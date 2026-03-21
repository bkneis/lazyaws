package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	apigwv1 "github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwv1types "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	apigwv2 "github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apigwv2types "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

// stubAPIGatewayV2 implements awspkg.APIGatewayV2API.
type stubAPIGatewayV2 struct {
	apis   []apigwv2types.Api
	routes []apigwv2types.Route
	stages []apigwv2types.Stage
}

func (s *stubAPIGatewayV2) GetApis(_ context.Context, _ *apigwv2.GetApisInput, _ ...func(*apigwv2.Options)) (*apigwv2.GetApisOutput, error) {
	return &apigwv2.GetApisOutput{Items: s.apis}, nil
}

func (s *stubAPIGatewayV2) GetApi(_ context.Context, in *apigwv2.GetApiInput, _ ...func(*apigwv2.Options)) (*apigwv2.GetApiOutput, error) {
	for _, a := range s.apis {
		if aws.ToString(a.ApiId) == aws.ToString(in.ApiId) {
			created := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
			return &apigwv2.GetApiOutput{
				ApiId:        a.ApiId,
				Name:         a.Name,
				ProtocolType: a.ProtocolType,
				ApiEndpoint:  aws.String("https://abc123.execute-api.us-east-1.amazonaws.com"),
				CreatedDate:  &created,
			}, nil
		}
	}
	return &apigwv2.GetApiOutput{}, nil
}

func (s *stubAPIGatewayV2) GetRoutes(_ context.Context, _ *apigwv2.GetRoutesInput, _ ...func(*apigwv2.Options)) (*apigwv2.GetRoutesOutput, error) {
	return &apigwv2.GetRoutesOutput{Items: s.routes}, nil
}

func (s *stubAPIGatewayV2) GetStages(_ context.Context, _ *apigwv2.GetStagesInput, _ ...func(*apigwv2.Options)) (*apigwv2.GetStagesOutput, error) {
	return &apigwv2.GetStagesOutput{Items: s.stages}, nil
}

func (s *stubAPIGatewayV2) GetIntegration(_ context.Context, _ *apigwv2.GetIntegrationInput, _ ...func(*apigwv2.Options)) (*apigwv2.GetIntegrationOutput, error) {
	return &apigwv2.GetIntegrationOutput{}, nil
}

// stubAPIGatewayV1 implements awspkg.APIGatewayV1API.
type stubAPIGatewayV1 struct {
	apis      []apigwv1types.RestApi
	resources []apigwv1types.Resource
	stages    []apigwv1types.Stage
}

func (s *stubAPIGatewayV1) GetRestApis(_ context.Context, _ *apigwv1.GetRestApisInput, _ ...func(*apigwv1.Options)) (*apigwv1.GetRestApisOutput, error) {
	return &apigwv1.GetRestApisOutput{Items: s.apis}, nil
}

func (s *stubAPIGatewayV1) GetRestApi(_ context.Context, in *apigwv1.GetRestApiInput, _ ...func(*apigwv1.Options)) (*apigwv1.GetRestApiOutput, error) {
	for _, a := range s.apis {
		if aws.ToString(a.Id) == aws.ToString(in.RestApiId) {
			created := time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)
			return &apigwv1.GetRestApiOutput{
				Id:          a.Id,
				Name:        a.Name,
				CreatedDate: &created,
			}, nil
		}
	}
	return &apigwv1.GetRestApiOutput{}, nil
}

func (s *stubAPIGatewayV1) GetResources(_ context.Context, _ *apigwv1.GetResourcesInput, _ ...func(*apigwv1.Options)) (*apigwv1.GetResourcesOutput, error) {
	return &apigwv1.GetResourcesOutput{Items: s.resources}, nil
}

func (s *stubAPIGatewayV1) GetStages(_ context.Context, _ *apigwv1.GetStagesInput, _ ...func(*apigwv1.Options)) (*apigwv1.GetStagesOutput, error) {
	return &apigwv1.GetStagesOutput{Item: s.stages}, nil
}

func (s *stubAPIGatewayV1) GetMethod(_ context.Context, _ *apigwv1.GetMethodInput, _ ...func(*apigwv1.Options)) (*apigwv1.GetMethodOutput, error) {
	return &apigwv1.GetMethodOutput{}, nil
}

func newTestAPIGatewayProvider() *awspkg.APIGatewayProvider {
	v2stub := &stubAPIGatewayV2{
		apis: []apigwv2types.Api{
			{ApiId: aws.String("api-http-1"), Name: aws.String("my-http-api"), ProtocolType: apigwv2types.ProtocolTypeHttp},
			{ApiId: aws.String("api-ws-1"), Name: aws.String("my-ws-api"), ProtocolType: apigwv2types.ProtocolTypeWebsocket},
		},
		routes: []apigwv2types.Route{
			{RouteKey: aws.String("GET /users"), Target: aws.String("integrations/abc")},
		},
		stages: []apigwv2types.Stage{
			{StageName: aws.String("$default"), DeploymentId: aws.String("dep-1"), AutoDeploy: aws.Bool(true), LastUpdatedDate: func() *time.Time { t := time.Date(2024, 11, 3, 0, 0, 0, 0, time.UTC); return &t }()},
		},
	}
	v1stub := &stubAPIGatewayV1{
		apis: []apigwv1types.RestApi{
			{Id: aws.String("rest-api-1"), Name: aws.String("my-rest-api")},
		},
		resources: []apigwv1types.Resource{
			{Path: aws.String("/pets"), ResourceMethods: map[string]apigwv1types.Method{"GET": {}, "POST": {}}},
		},
		stages: []apigwv1types.Stage{
			{StageName: aws.String("prod"), DeploymentId: aws.String("dep-rest-1")},
		},
	}
	return awspkg.NewAPIGatewayProviderWithClients(v2stub, v1stub)
}

func TestAPIGatewayProvider_ListItems_MergesV2AndV1(t *testing.T) {
	p := newTestAPIGatewayProvider()
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}

	metaTypes := map[string]string{}
	for _, item := range items {
		metaTypes[item.ID] = item.Meta["type"]
	}

	cases := []struct {
		id       string
		wantType string
	}{
		{"api-http-1", "HTTP"},
		{"api-ws-1", "WEBSOCKET"},
		{"rest-api-1", "REST"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			got, ok := metaTypes[tc.id]
			if !ok {
				t.Fatalf("item %q not found in results", tc.id)
			}
			if got != tc.wantType {
				t.Errorf("Meta[\"type\"] = %q, want %q", got, tc.wantType)
			}
		})
	}
}

func TestAPIGatewayProvider_ListItems_Filter(t *testing.T) {
	v2stub := &stubAPIGatewayV2{
		apis: []apigwv2types.Api{
			{ApiId: aws.String("api-1"), Name: aws.String("my-api"), ProtocolType: apigwv2types.ProtocolTypeHttp},
			{ApiId: aws.String("api-2"), Name: aws.String("other-api"), ProtocolType: apigwv2types.ProtocolTypeHttp},
		},
	}
	v1stub := &stubAPIGatewayV1{}
	p := awspkg.NewAPIGatewayProviderWithClients(v2stub, v1stub)
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

func TestAPIGatewayProvider_TabOverview_HTTP(t *testing.T) {
	p := newTestAPIGatewayProvider()
	tabs := p.Tabs()

	item := awspkg.Item{ID: "api-http-1", Name: "my-http-api (HTTP)", Meta: map[string]string{"type": "HTTP"}}
	content, err := tabs[0].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []string{"api-http-1", "HTTP", "execute-api"}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("overview missing %q\ngot:\n%s", want, content)
		}
	}
}

func TestAPIGatewayProvider_TabOverview_REST(t *testing.T) {
	p := newTestAPIGatewayProvider()
	tabs := p.Tabs()

	item := awspkg.Item{ID: "rest-api-1", Name: "my-rest-api (REST)", Meta: map[string]string{"type": "REST"}}
	content, err := tabs[0].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []string{"rest-api-1", "REST", "execute-api"}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("overview missing %q\ngot:\n%s", want, content)
		}
	}
}

func TestAPIGatewayProvider_TabRoutes_HTTP(t *testing.T) {
	p := newTestAPIGatewayProvider()
	tabs := p.Tabs()

	item := awspkg.Item{ID: "api-http-1", Name: "my-http-api (HTTP)", Meta: map[string]string{"type": "HTTP"}}
	content, err := tabs[1].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "GET /users") {
		t.Errorf("routes missing GET /users\ngot:\n%s", content)
	}
}

func TestAPIGatewayProvider_TabRoutes_REST(t *testing.T) {
	p := newTestAPIGatewayProvider()
	tabs := p.Tabs()

	item := awspkg.Item{ID: "rest-api-1", Name: "my-rest-api (REST)", Meta: map[string]string{"type": "REST"}}
	content, err := tabs[1].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "/pets") {
		t.Errorf("routes missing /pets\ngot:\n%s", content)
	}
}

func TestAPIGatewayProvider_TabStages_HTTP(t *testing.T) {
	p := newTestAPIGatewayProvider()
	tabs := p.Tabs()

	item := awspkg.Item{ID: "api-http-1", Name: "my-http-api (HTTP)", Meta: map[string]string{"type": "HTTP"}}
	content, err := tabs[2].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checks := []string{"$default", "dep-1", "Yes", "2024-11-03"}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("stages missing %q\ngot:\n%s", want, content)
		}
	}
}

func TestAPIGatewayProvider_FetchItem(t *testing.T) {
	p := newTestAPIGatewayProvider()

	cases := []struct {
		name     string
		id       string
		wantErr  bool
		wantName string
		wantType string
	}{
		{
			name:     "V1 REST id",
			id:       "rest-api-1",
			wantName: "my-rest-api (REST)",
			wantType: "REST",
		},
		{
			name:     "V2 HTTP id",
			id:       "api-http-1",
			wantName: "my-http-api (HTTP)",
			wantType: "HTTP",
		},
		{
			name:    "both fail",
			id:      "unknown-id",
			wantErr: true,
		},
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
			if item.Name != tc.wantName {
				t.Errorf("got Name=%q, want %q", item.Name, tc.wantName)
			}
			if item.Meta["type"] != tc.wantType {
				t.Errorf("got Meta[type]=%q, want %q", item.Meta["type"], tc.wantType)
			}
			if item.ID != tc.id {
				t.Errorf("got ID=%q, want %q", item.ID, tc.id)
			}
		})
	}
}

func TestAPIGatewayProvider_TabStages_REST(t *testing.T) {
	p := newTestAPIGatewayProvider()
	tabs := p.Tabs()

	item := awspkg.Item{ID: "rest-api-1", Name: "my-rest-api (REST)", Meta: map[string]string{"type": "REST"}}
	content, err := tabs[2].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checks := []string{"prod", "dep-rest-1"}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("stages missing %q\ngot:\n%s", want, content)
		}
	}
}
