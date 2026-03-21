package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

type stubASG struct {
	groups     []asgtypes.AutoScalingGroup
	policies   []asgtypes.ScalingPolicy
	activities []asgtypes.Activity
}

func (s *stubASG) DescribeAutoScalingGroups(_ context.Context, _ *autoscaling.DescribeAutoScalingGroupsInput, _ ...func(*autoscaling.Options)) (*autoscaling.DescribeAutoScalingGroupsOutput, error) {
	return &autoscaling.DescribeAutoScalingGroupsOutput{AutoScalingGroups: s.groups}, nil
}

func (s *stubASG) DescribePolicies(_ context.Context, _ *autoscaling.DescribePoliciesInput, _ ...func(*autoscaling.Options)) (*autoscaling.DescribePoliciesOutput, error) {
	return &autoscaling.DescribePoliciesOutput{ScalingPolicies: s.policies}, nil
}

func (s *stubASG) DescribeScalingActivities(_ context.Context, _ *autoscaling.DescribeScalingActivitiesInput, _ ...func(*autoscaling.Options)) (*autoscaling.DescribeScalingActivitiesOutput, error) {
	return &autoscaling.DescribeScalingActivitiesOutput{Activities: s.activities}, nil
}

func TestASGProvider_ListItems(t *testing.T) {
	now := time.Now()
	stub := &stubASG{
		groups: []asgtypes.AutoScalingGroup{
			{
				AutoScalingGroupName: aws.String("my-asg"),
				DesiredCapacity:      aws.Int32(3),
				MinSize:              aws.Int32(1),
				MaxSize:              aws.Int32(10),
				HealthCheckType:      aws.String("EC2"),
				HealthCheckGracePeriod: aws.Int32(300),
				CreatedTime:          &now,
				AvailabilityZones:    []string{"us-east-1a", "us-east-1b"},
			},
		},
	}
	p := awspkg.NewASGProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].ID != "my-asg" {
		t.Errorf("want ID=my-asg, got %s", items[0].ID)
	}
	if items[0].Meta["desired"] != "3" {
		t.Errorf("want desired=3, got %s", items[0].Meta["desired"])
	}
}

func TestASGProvider_ListItems_Filter(t *testing.T) {
	now := time.Now()
	stub := &stubASG{
		groups: []asgtypes.AutoScalingGroup{
			{AutoScalingGroupName: aws.String("api-asg"), DesiredCapacity: aws.Int32(2), MinSize: aws.Int32(1), MaxSize: aws.Int32(5), HealthCheckType: aws.String("EC2"), HealthCheckGracePeriod: aws.Int32(60), CreatedTime: &now},
			{AutoScalingGroupName: aws.String("worker-asg"), DesiredCapacity: aws.Int32(1), MinSize: aws.Int32(0), MaxSize: aws.Int32(3), HealthCheckType: aws.String("EC2"), HealthCheckGracePeriod: aws.Int32(60), CreatedTime: &now},
		},
	}
	p := awspkg.NewASGProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "api-asg" {
		t.Errorf("filter expected [api-asg], got %v", items)
	}
}

func TestASGProvider_Tabs(t *testing.T) {
	now := time.Now()
	stub := &stubASG{
		groups: []asgtypes.AutoScalingGroup{
			{
				AutoScalingGroupName: aws.String("my-asg"),
				DesiredCapacity:      aws.Int32(2),
				MinSize:              aws.Int32(1),
				MaxSize:              aws.Int32(5),
				HealthCheckType:      aws.String("EC2"),
				HealthCheckGracePeriod: aws.Int32(300),
				CreatedTime:          &now,
				AvailabilityZones:    []string{"us-east-1a"},
				Instances: []asgtypes.Instance{
					{
						InstanceId:       aws.String("i-abc123"),
						AvailabilityZone: aws.String("us-east-1a"),
						LifecycleState:   asgtypes.LifecycleStateInService,
						HealthStatus:     aws.String("Healthy"),
					},
				},
			},
		},
		policies: []asgtypes.ScalingPolicy{
			{
				PolicyName:        aws.String("scale-out"),
				PolicyType:        aws.String("SimpleScaling"),
				ScalingAdjustment: aws.Int32(1),
				Cooldown:          aws.Int32(300),
			},
		},
		activities: []asgtypes.Activity{
			{
				ActivityId:  aws.String("act-1"),
				StartTime:   &now,
				StatusCode:  asgtypes.ScalingActivityStatusCodeSuccessful,
				Description: aws.String("Launching a new EC2 instance"),
			},
		},
	}
	p := awspkg.NewASGProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	item := items[0]

	cases := []struct {
		label string
		want  string
	}{
		{"Overview", "my-asg"},
		{"Instances", "i-abc123"},
		{"Scaling Policies", "scale-out"},
		{"Activities", "Launching"},
	}
	tabs := p.Tabs()
	if len(tabs) != len(cases) {
		t.Fatalf("want %d tabs, got %d", len(cases), len(tabs))
	}
	for i, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			out, err := tabs[i].Fetch(context.Background(), item)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("tab %q: want %q in output, got:\n%s", tc.label, tc.want, out)
			}
		})
	}
}

func TestASGProvider_Tabs_Empty(t *testing.T) {
	now := time.Now()
	stub := &stubASG{
		groups: []asgtypes.AutoScalingGroup{
			{
				AutoScalingGroupName:   aws.String("empty-asg"),
				DesiredCapacity:        aws.Int32(0),
				MinSize:                aws.Int32(0),
				MaxSize:                aws.Int32(0),
				HealthCheckType:        aws.String("EC2"),
				HealthCheckGracePeriod: aws.Int32(0),
				CreatedTime:            &now,
			},
		},
	}
	p := awspkg.NewASGProviderWithClient(stub)
	items, _ := p.ListItems(context.Background(), "")
	for _, tab := range p.Tabs() {
		out, err := tab.Fetch(context.Background(), items[0])
		if err != nil {
			t.Errorf("tab %q returned error: %v", tab.Label, err)
		}
		if out == "" {
			t.Errorf("tab %q returned empty string", tab.Label)
		}
	}
}
