package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	apigwv1 "github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwv2 "github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
)

type APIGatewayV2API interface {
	GetApis(ctx context.Context, in *apigwv2.GetApisInput, opts ...func(*apigwv2.Options)) (*apigwv2.GetApisOutput, error)
	GetApi(ctx context.Context, in *apigwv2.GetApiInput, opts ...func(*apigwv2.Options)) (*apigwv2.GetApiOutput, error)
	GetRoutes(ctx context.Context, in *apigwv2.GetRoutesInput, opts ...func(*apigwv2.Options)) (*apigwv2.GetRoutesOutput, error)
	GetStages(ctx context.Context, in *apigwv2.GetStagesInput, opts ...func(*apigwv2.Options)) (*apigwv2.GetStagesOutput, error)
}

type APIGatewayV1API interface {
	GetRestApis(ctx context.Context, in *apigwv1.GetRestApisInput, opts ...func(*apigwv1.Options)) (*apigwv1.GetRestApisOutput, error)
	GetRestApi(ctx context.Context, in *apigwv1.GetRestApiInput, opts ...func(*apigwv1.Options)) (*apigwv1.GetRestApiOutput, error)
	GetResources(ctx context.Context, in *apigwv1.GetResourcesInput, opts ...func(*apigwv1.Options)) (*apigwv1.GetResourcesOutput, error)
	GetStages(ctx context.Context, in *apigwv1.GetStagesInput, opts ...func(*apigwv1.Options)) (*apigwv1.GetStagesOutput, error)
}

type APIGatewayProvider struct {
	v2 APIGatewayV2API
	v1 APIGatewayV1API
}

func NewAPIGatewayProvider(cfg awssdk.Config, local bool) *APIGatewayProvider {
	var optsV2 []func(*apigwv2.Options)
	var optsV1 []func(*apigwv1.Options)
	if local {
		optsV2 = append(optsV2, func(o *apigwv2.Options) { o.BaseEndpoint = awssdk.String("http://localhost:4566") })
		optsV1 = append(optsV1, func(o *apigwv1.Options) { o.BaseEndpoint = awssdk.String("http://localhost:4566") })
	}
	return &APIGatewayProvider{
		v2: apigwv2.NewFromConfig(cfg, optsV2...),
		v1: apigwv1.NewFromConfig(cfg, optsV1...),
	}
}

func NewAPIGatewayProviderWithClients(v2 APIGatewayV2API, v1 APIGatewayV1API) *APIGatewayProvider {
	return &APIGatewayProvider{v2: v2, v1: v1}
}

func (p *APIGatewayProvider) Name() string { return "API Gateway" }

func (p *APIGatewayProvider) ListItems(ctx context.Context) ([]Item, error) {
	var items []Item

	v2out, err := p.v2.GetApis(ctx, &apigwv2.GetApisInput{})
	if err != nil {
		return nil, fmt.Errorf("get apis: %w", err)
	}
	for _, api := range v2out.Items {
		apiType := strings.ToUpper(string(api.ProtocolType))
		items = append(items, Item{
			ID:   awssdk.ToString(api.ApiId),
			Name: fmt.Sprintf("%s (%s)", awssdk.ToString(api.Name), apiType),
			Meta: map[string]string{"type": apiType},
		})
	}

	v1out, err := p.v1.GetRestApis(ctx, &apigwv1.GetRestApisInput{})
	if err != nil {
		return nil, fmt.Errorf("get rest apis: %w", err)
	}
	for _, api := range v1out.Items {
		items = append(items, Item{
			ID:   awssdk.ToString(api.Id),
			Name: fmt.Sprintf("%s (REST)", awssdk.ToString(api.Name)),
			Meta: map[string]string{"type": "REST"},
		})
	}

	return items, nil
}

func (p *APIGatewayProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *APIGatewayProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Routes", Fetch: p.tabRoutes},
		{Label: "Stages", Fetch: p.tabStages},
	}
}

func (p *APIGatewayProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	if item.Meta["type"] != "REST" {
		out, err := p.v2.GetApi(ctx, &apigwv2.GetApiInput{ApiId: awssdk.String(item.ID)})
		if err != nil {
			return "", err
		}
		created := ""
		if out.CreatedDate != nil {
			created = out.CreatedDate.Format(time.DateOnly)
		}
		return KV([][2]string{
			{"API ID", awssdk.ToString(out.ApiId)},
			{"Type", string(out.ProtocolType)},
			{"Endpoint", awssdk.ToString(out.ApiEndpoint)},
			{"Created", created},
		}), nil
	}
	out, err := p.v1.GetRestApi(ctx, &apigwv1.GetRestApiInput{RestApiId: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	created := ""
	if out.CreatedDate != nil {
		created = out.CreatedDate.Format(time.DateOnly)
	}
	return KV([][2]string{
		{"API ID", awssdk.ToString(out.Id)},
		{"Type", "REST"},
		{"Endpoint", fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com", awssdk.ToString(out.Id), "us-east-1")},
		{"Created", created},
	}), nil
}

func (p *APIGatewayProvider) tabRoutes(ctx context.Context, item Item) (string, error) {
	if item.Meta["type"] != "REST" {
		out, err := p.v2.GetRoutes(ctx, &apigwv2.GetRoutesInput{ApiId: awssdk.String(item.ID)})
		if err != nil {
			return "", err
		}
		if len(out.Items) == 0 {
			return "  (no routes)\n", nil
		}
		rows := make([][]string, len(out.Items))
		for i, r := range out.Items {
			rows[i] = []string{awssdk.ToString(r.RouteKey), awssdk.ToString(r.Target)}
		}
		return Table([]string{"Route Key", "Target"}, rows), nil
	}
	out, err := p.v1.GetResources(ctx, &apigwv1.GetResourcesInput{RestApiId: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	var rows [][]string
	for _, res := range out.Items {
		for method := range res.ResourceMethods {
			rows = append(rows, []string{method, awssdk.ToString(res.Path), "(REST)"})
		}
	}
	if len(rows) == 0 {
		return "  (no resources/methods)\n", nil
	}
	return Table([]string{"Method", "Path", "Integration"}, rows), nil
}

func (p *APIGatewayProvider) tabStages(ctx context.Context, item Item) (string, error) {
	if item.Meta["type"] != "REST" {
		out, err := p.v2.GetStages(ctx, &apigwv2.GetStagesInput{ApiId: awssdk.String(item.ID)})
		if err != nil {
			return "", err
		}
		if len(out.Items) == 0 {
			return "  (no stages)\n", nil
		}
		rows := make([][]string, len(out.Items))
		for i, s := range out.Items {
			autoDeploy := "No"
			if awssdk.ToBool(s.AutoDeploy) {
				autoDeploy = "Yes"
			}
			lastDeployed := ""
			if s.LastUpdatedDate != nil {
				lastDeployed = s.LastUpdatedDate.Format(time.DateOnly)
			}
			rows[i] = []string{awssdk.ToString(s.StageName), awssdk.ToString(s.DeploymentId), autoDeploy, lastDeployed}
		}
		return Table([]string{"Stage", "Deployment ID", "Auto-Deploy", "Last Deployed"}, rows), nil
	}
	out, err := p.v1.GetStages(ctx, &apigwv1.GetStagesInput{RestApiId: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	if len(out.Item) == 0 {
		return "  (no stages)\n", nil
	}
	rows := make([][]string, len(out.Item))
	for i, s := range out.Item {
		created := ""
		if s.CreatedDate != nil {
			created = s.CreatedDate.Format(time.DateOnly)
		}
		rows[i] = []string{awssdk.ToString(s.StageName), awssdk.ToString(s.DeploymentId), "No", created}
	}
	return Table([]string{"Stage", "Deployment ID", "Auto-Deploy", "Created"}, rows), nil
}
