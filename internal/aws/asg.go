package aws

import (
	"context"
	"encoding/json"
	"log"
	"fmt"
	"strconv"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
)

// ASGAPI is the subset of Auto Scaling client methods used by ASGProvider.
type ASGAPI interface {
	DescribeAutoScalingGroups(ctx context.Context, in *autoscaling.DescribeAutoScalingGroupsInput, opts ...func(*autoscaling.Options)) (*autoscaling.DescribeAutoScalingGroupsOutput, error)
	DescribePolicies(ctx context.Context, in *autoscaling.DescribePoliciesInput, opts ...func(*autoscaling.Options)) (*autoscaling.DescribePoliciesOutput, error)
	DescribeScalingActivities(ctx context.Context, in *autoscaling.DescribeScalingActivitiesInput, opts ...func(*autoscaling.Options)) (*autoscaling.DescribeScalingActivitiesOutput, error)
}

// ASGProvider implements Provider for Auto Scaling Groups.
type ASGProvider struct {
	client ASGAPI
}

func NewASGProvider(cfg awssdk.Config, endpointURL string) *ASGProvider {
	var opts []func(*autoscaling.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *autoscaling.Options) {
			o.BaseEndpoint = awssdk.String(endpointURL)
		})
	}
	return &ASGProvider{client: autoscaling.NewFromConfig(cfg, opts...)}
}

func NewASGProviderWithClient(client ASGAPI) *ASGProvider {
	return &ASGProvider{client: client}
}

func (p *ASGProvider) Name() string { return "Auto Scaling" }

func (p *ASGProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	var items []Item
	var nextToken *string
	for {
		out, err := p.client.DescribeAutoScalingGroups(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe auto scaling groups: %w", err)
		}
		for _, asg := range out.AutoScalingGroups {
			name := awssdk.ToString(asg.AutoScalingGroupName)
			asgJSON, jsonErr := json.Marshal(asg)
			if jsonErr != nil {
				log.Printf("warn: marshal ASG %s: %v", name, jsonErr)
			}
			items = append(items, Item{
				ID:   name,
				Name: name,
				Meta: map[string]string{
					"desired":  strconv.Itoa(int(awssdk.ToInt32(asg.DesiredCapacity))),
					"min":      strconv.Itoa(int(awssdk.ToInt32(asg.MinSize))),
					"max":      strconv.Itoa(int(awssdk.ToInt32(asg.MaxSize))),
					"status":   awssdk.ToString(asg.Status),
					"asg_json": string(asgJSON),
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

func (p *ASGProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *ASGProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Instances", Fetch: p.tabInstances},
		{Label: "Scaling Policies", Fetch: p.tabScalingPolicies},
		{Label: "Activities", Fetch: p.tabActivities},
	}
}

func (p *ASGProvider) asgFromMeta(item Item) (*asgtypes.AutoScalingGroup, error) {
	raw, ok := item.Meta["asg_json"]
	if !ok || raw == "" {
		return nil, fmt.Errorf("asg data not available")
	}
	var asg asgtypes.AutoScalingGroup
	if err := json.Unmarshal([]byte(raw), &asg); err != nil {
		return nil, fmt.Errorf("parse asg: %w", err)
	}
	return &asg, nil
}

func (p *ASGProvider) tabOverview(_ context.Context, item Item) (string, error) {
	asg, err := p.asgFromMeta(item)
	if err != nil {
		return "", err
	}

	created := ""
	if asg.CreatedTime != nil {
		created = asg.CreatedTime.Format(time.DateTime)
	}

	launchRef := "-"
	if asg.LaunchTemplate != nil {
		launchRef = awssdk.ToString(asg.LaunchTemplate.LaunchTemplateName)
	} else if asg.LaunchConfigurationName != nil {
		launchRef = awssdk.ToString(asg.LaunchConfigurationName)
	}

	return KV([][2]string{
		{"Name", awssdk.ToString(asg.AutoScalingGroupName)},
		{"Desired", strconv.Itoa(int(awssdk.ToInt32(asg.DesiredCapacity)))},
		{"Min", strconv.Itoa(int(awssdk.ToInt32(asg.MinSize)))},
		{"Max", strconv.Itoa(int(awssdk.ToInt32(asg.MaxSize)))},
		{"Health Check Type", awssdk.ToString(asg.HealthCheckType)},
		{"Health Check Grace Period", strconv.Itoa(int(awssdk.ToInt32(asg.HealthCheckGracePeriod))) + "s"},
		{"Launch Template/Config", launchRef},
		{"Created", created},
		{"AZs", strings.Join(asg.AvailabilityZones, ", ")},
	}), nil
}

func (p *ASGProvider) tabInstances(_ context.Context, item Item) (string, error) {
	asg, err := p.asgFromMeta(item)
	if err != nil {
		return "", err
	}
	if len(asg.Instances) == 0 {
		return "  (no instances)\n", nil
	}
	rows := make([][]string, len(asg.Instances))
	for i, inst := range asg.Instances {
		ltName := "-"
		if inst.LaunchTemplate != nil {
			ltName = awssdk.ToString(inst.LaunchTemplate.LaunchTemplateName)
		}
		rows[i] = []string{
			awssdk.ToString(inst.InstanceId),
			awssdk.ToString(inst.AvailabilityZone),
			string(inst.LifecycleState),
			awssdk.ToString(inst.HealthStatus),
			ltName,
		}
	}
	return Table([]string{"Instance ID", "AZ", "State", "Health", "Launch Template"}, rows), nil
}

func (p *ASGProvider) tabScalingPolicies(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribePolicies(ctx, &autoscaling.DescribePoliciesInput{
		AutoScalingGroupName: awssdk.String(item.ID),
	})
	if err != nil {
		return "", err
	}
	if len(out.ScalingPolicies) == 0 {
		return "  (no scaling policies)\n", nil
	}
	rows := make([][]string, len(out.ScalingPolicies))
	for i, pol := range out.ScalingPolicies {
		adjustment := "-"
		if pol.ScalingAdjustment != nil {
			adjustment = strconv.Itoa(int(awssdk.ToInt32(pol.ScalingAdjustment)))
		}
		cooldown := "-"
		if pol.Cooldown != nil {
			cooldown = strconv.Itoa(int(awssdk.ToInt32(pol.Cooldown))) + "s"
		}
		rows[i] = []string{
			awssdk.ToString(pol.PolicyName),
			awssdk.ToString(pol.PolicyType),
			adjustment,
			cooldown,
		}
	}
	return Table([]string{"Name", "Type", "Adjustment", "Cooldown"}, rows), nil
}

func (p *ASGProvider) tabActivities(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeScalingActivities(ctx, &autoscaling.DescribeScalingActivitiesInput{
		AutoScalingGroupName: awssdk.String(item.ID),
		MaxRecords:           awssdk.Int32(20),
	})
	if err != nil {
		return "", err
	}
	if len(out.Activities) == 0 {
		return "  (no activities)\n", nil
	}
	rows := make([][]string, len(out.Activities))
	for i, act := range out.Activities {
		startTime := ""
		if act.StartTime != nil {
			startTime = act.StartTime.Format(time.DateTime)
		}
		rows[i] = []string{
			startTime,
			string(act.StatusCode),
			awssdk.ToString(act.Description),
		}
	}
	return Table([]string{"Start Time", "Status", "Description"}, rows), nil
}
