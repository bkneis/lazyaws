package aws_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

type stubELB struct {
	lbs          []elbtypes.LoadBalancer
	listeners    []elbtypes.Listener
	targetGroups []elbtypes.TargetGroup
}

func (s *stubELB) DescribeLoadBalancers(_ context.Context, _ *elbv2.DescribeLoadBalancersInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error) {
	return &elbv2.DescribeLoadBalancersOutput{LoadBalancers: s.lbs}, nil
}

func (s *stubELB) DescribeListeners(_ context.Context, _ *elbv2.DescribeListenersInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeListenersOutput, error) {
	return &elbv2.DescribeListenersOutput{Listeners: s.listeners}, nil
}

func (s *stubELB) DescribeTargetGroups(_ context.Context, _ *elbv2.DescribeTargetGroupsInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeTargetGroupsOutput, error) {
	return &elbv2.DescribeTargetGroupsOutput{TargetGroups: s.targetGroups}, nil
}

func TestELBProvider_ListItems(t *testing.T) {
	now := time.Now()
	stub := &stubELB{
		lbs: []elbtypes.LoadBalancer{
			{
				LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-alb/abc"),
				LoadBalancerName: aws.String("my-alb"),
				Type:             elbtypes.LoadBalancerTypeEnumApplication,
				Scheme:           elbtypes.LoadBalancerSchemeEnumInternetFacing,
				DNSName:          aws.String("my-alb.us-east-1.elb.amazonaws.com"),
				VpcId:            aws.String("vpc-111"),
				CreatedTime:      &now,
				State:            &elbtypes.LoadBalancerState{Code: elbtypes.LoadBalancerStateEnumActive},
				AvailabilityZones: []elbtypes.AvailabilityZone{
					{ZoneName: aws.String("us-east-1a")},
				},
			},
		},
	}
	p := awspkg.NewELBProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].Name != "my-alb" {
		t.Errorf("want Name=my-alb, got %s", items[0].Name)
	}
}

func TestELBProvider_ListItems_Filter(t *testing.T) {
	now := time.Now()
	stub := &stubELB{
		lbs: []elbtypes.LoadBalancer{
			{LoadBalancerArn: aws.String("arn:alb/app/api-lb/1"), LoadBalancerName: aws.String("api-lb"), Type: elbtypes.LoadBalancerTypeEnumApplication, Scheme: elbtypes.LoadBalancerSchemeEnumInternetFacing, CreatedTime: &now, State: &elbtypes.LoadBalancerState{Code: elbtypes.LoadBalancerStateEnumActive}},
			{LoadBalancerArn: aws.String("arn:alb/app/worker-lb/2"), LoadBalancerName: aws.String("worker-lb"), Type: elbtypes.LoadBalancerTypeEnumApplication, Scheme: elbtypes.LoadBalancerSchemeEnumInternal, CreatedTime: &now, State: &elbtypes.LoadBalancerState{Code: elbtypes.LoadBalancerStateEnumActive}},
		},
	}
	p := awspkg.NewELBProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "api-lb" {
		t.Errorf("filter expected [api-lb], got %v", items)
	}
}

func TestELBProvider_Tabs(t *testing.T) {
	now := time.Now()
	stub := &stubELB{
		lbs: []elbtypes.LoadBalancer{
			{
				LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-alb/abc"),
				LoadBalancerName: aws.String("my-alb"),
				Type:             elbtypes.LoadBalancerTypeEnumApplication,
				Scheme:           elbtypes.LoadBalancerSchemeEnumInternetFacing,
				DNSName:          aws.String("my-alb.us-east-1.elb.amazonaws.com"),
				VpcId:            aws.String("vpc-111"),
				CreatedTime:      &now,
				State:            &elbtypes.LoadBalancerState{Code: elbtypes.LoadBalancerStateEnumActive},
			},
		},
		listeners: []elbtypes.Listener{
			{
				Port:     aws.Int32(443),
				Protocol: elbtypes.ProtocolEnumHttps,
				DefaultActions: []elbtypes.Action{
					{Type: elbtypes.ActionTypeEnumForward},
				},
			},
		},
		targetGroups: []elbtypes.TargetGroup{
			{
				TargetGroupName:  aws.String("my-tg"),
				Protocol:         elbtypes.ProtocolEnumHttps,
				Port:             aws.Int32(8080),
				TargetType:       elbtypes.TargetTypeEnumInstance,
				HealthCheckPath:  aws.String("/health"),
			},
		},
	}
	p := awspkg.NewELBProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	item := items[0]

	cases := []struct {
		label string
		want  string
	}{
		{"Overview", "my-alb"},
		{"Listeners", "443"},
		{"Target Groups", "my-tg"},
	}
	tabs := p.Tabs()
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

func TestELBProvider_Tabs_Empty(t *testing.T) {
	stub := &stubELB{}
	p := awspkg.NewELBProviderWithClient(stub)
	item := awspkg.Item{
		ID:   "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/empty/abc",
		Name: "empty",
		Meta: map[string]string{"type": "application", "scheme": "internet-facing", "state": "active", "dns_name": "", "vpc_id": "", "created": "", "azs": ""},
	}
	for _, tab := range p.Tabs() {
		out, err := tab.Fetch(context.Background(), item)
		if err != nil {
			t.Errorf("tab %q returned error: %v", tab.Label, err)
		}
		if out == "" {
			t.Errorf("tab %q returned empty string", tab.Label)
		}
	}
}
