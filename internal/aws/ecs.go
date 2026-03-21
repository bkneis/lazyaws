package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

// ECSAPI is the subset of the ECS client used by ECSProvider.
type ECSAPI interface {
	ListClusters(ctx context.Context, in *ecs.ListClustersInput, opts ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
	DescribeClusters(ctx context.Context, in *ecs.DescribeClustersInput, opts ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error)
	ListServices(ctx context.Context, in *ecs.ListServicesInput, opts ...func(*ecs.Options)) (*ecs.ListServicesOutput, error)
	DescribeServices(ctx context.Context, in *ecs.DescribeServicesInput, opts ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	ListTasks(ctx context.Context, in *ecs.ListTasksInput, opts ...func(*ecs.Options)) (*ecs.ListTasksOutput, error)
	DescribeTasks(ctx context.Context, in *ecs.DescribeTasksInput, opts ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
}

// ECSProvider implements Provider for Amazon ECS Clusters.
type ECSProvider struct {
	client  ECSAPI
	metrics CloudWatchMetricsAPI
}

func NewECSProvider(cfg awssdk.Config, endpointURL string) *ECSProvider {
	var opts []func(*ecs.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *ecs.Options) { o.BaseEndpoint = awssdk.String(endpointURL) })
	}
	return &ECSProvider{client: ecs.NewFromConfig(cfg, opts...), metrics: cloudwatch.NewFromConfig(cfg)}
}

func NewECSProviderWithClient(client ECSAPI) *ECSProvider { return &ECSProvider{client: client} }

func (p *ECSProvider) Name() string { return "ECS" }

func (p *ECSProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	arns, err := p.client.ListClusters(ctx, &ecs.ListClustersInput{})
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	if len(arns.ClusterArns) == 0 {
		return nil, nil
	}

	out, err := p.client.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: arns.ClusterArns,
	})
	if err != nil {
		return nil, fmt.Errorf("describe clusters: %w", err)
	}

	items := make([]Item, 0, len(out.Clusters))
	for _, cl := range out.Clusters {
		clusterJSON, jsonErr := json.Marshal(cl)
		if jsonErr != nil {
			log.Printf("warn: marshal cluster %s: %v", awssdk.ToString(cl.ClusterArn), jsonErr)
		}
		arn := awssdk.ToString(cl.ClusterArn)
		name := awssdk.ToString(cl.ClusterName)
		items = append(items, Item{
			ID:   arn,
			Name: name,
			Meta: map[string]string{
				"status":        awssdk.ToString(cl.Status),
				"running_tasks": fmt.Sprintf("%d", cl.RunningTasksCount),
				"cluster_json":  string(clusterJSON),
			},
		})
	}
	return filterItems(items, query), nil
}

func (p *ECSProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *ECSProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Services", Fetch: p.tabServices},
		{Label: "Tasks", Fetch: p.tabTasks},
		{Label: "Metrics", Fetch: p.tabMetrics},
	}
}

func (p *ECSProvider) tabOverview(_ context.Context, item Item) (string, error) {
	raw := item.Meta["cluster_json"]
	if raw == "" {
		return KV([][2]string{
			{"ARN", item.ID},
			{"Status", item.Meta["status"]},
			{"Running Tasks", item.Meta["running_tasks"]},
		}), nil
	}
	var cl struct {
		ClusterName              *string `json:"ClusterName"`
		Status                   *string `json:"Status"`
		RunningTasksCount        int32   `json:"RunningTasksCount"`
		PendingTasksCount        int32   `json:"PendingTasksCount"`
		ActiveServicesCount      int32   `json:"ActiveServicesCount"`
		RegisteredContainerCount int32   `json:"RegisteredContainerInstancesCount"`
	}
	if err := json.Unmarshal([]byte(raw), &cl); err != nil {
		return "", err
	}
	return KV([][2]string{
		{"Name", awssdk.ToString(cl.ClusterName)},
		{"ARN", item.ID},
		{"Status", awssdk.ToString(cl.Status)},
		{"Running Tasks", fmt.Sprintf("%d", cl.RunningTasksCount)},
		{"Pending Tasks", fmt.Sprintf("%d", cl.PendingTasksCount)},
		{"Active Services", fmt.Sprintf("%d", cl.ActiveServicesCount)},
		{"Container Instances", fmt.Sprintf("%d", cl.RegisteredContainerCount)},
	}), nil
}

func (p *ECSProvider) tabServices(ctx context.Context, item Item) (string, error) {
	listOut, err := p.client.ListServices(ctx, &ecs.ListServicesInput{
		Cluster: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	if len(listOut.ServiceArns) == 0 {
		return "  (no services)\n", nil
	}
	descOut, err := p.client.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  awssdk.String(item.ID),
		Services: listOut.ServiceArns,
	})
	if err != nil {
		return "", err
	}
	rows := make([][]string, len(descOut.Services))
	for i, svc := range descOut.Services {
		rows[i] = []string{
			awssdk.ToString(svc.ServiceName),
			awssdk.ToString(svc.Status),
			fmt.Sprintf("%d", svc.DesiredCount),
			fmt.Sprintf("%d", svc.RunningCount),
			fmt.Sprintf("%d", svc.PendingCount),
		}
	}
	return Table([]string{"Name", "Status", "Desired", "Running", "Pending"}, rows), nil
}

func (p *ECSProvider) tabTasks(ctx context.Context, item Item) (string, error) {
	listOut, err := p.client.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	if len(listOut.TaskArns) == 0 {
		return "  (no tasks)\n", nil
	}
	descOut, err := p.client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: awssdk.String(item.ID),
		Tasks:   listOut.TaskArns,
	})
	if err != nil {
		return "", err
	}
	rows := make([][]string, len(descOut.Tasks))
	for i, t := range descOut.Tasks {
		taskID := arnLastSegment(awssdk.ToString(t.TaskArn))
		containerNames := make([]string, len(t.Containers))
		for j, c := range t.Containers {
			containerNames[j] = awssdk.ToString(c.Name)
		}
		rows[i] = []string{
			taskID,
			awssdk.ToString(t.LastStatus),
			awssdk.ToString(t.TaskDefinitionArn),
			strings.Join(containerNames, ", "),
		}
	}
	return Table([]string{"Task ID", "Status", "Definition", "Containers"}, rows), nil
}

func (p *ECSProvider) tabMetrics(ctx context.Context, item Item) (string, error) {
	specs := []metricSpec{
		{id: "cpu", label: "CPU Utilization", ns: "AWS/ECS", name: "CPUUtilization", stat: "Average", dimKey: "ClusterName", dimVal: item.Name, unit: "%"},
		{id: "mem", label: "Memory Utilization", ns: "AWS/ECS", name: "MemoryUtilization", stat: "Average", dimKey: "ClusterName", dimVal: item.Name, unit: "%"},
	}
	data, err := fetchSparklines(ctx, p.metrics, specs, 1, 60)
	if err != nil {
		return "", err
	}
	return renderMetricsTab(specs, data, 1, 60), nil
}
