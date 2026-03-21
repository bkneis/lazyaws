package aws_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	awspkg "github.com/bkneis/lazyaws/internal/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// stubIAMPoliciesAPI implements IAMPoliciesAPI for testing.
type stubIAMPoliciesAPI struct {
	policies []iamtypes.Policy // NOTE: ListPolicies returns types.Policy, not ManagedPolicy
	document string            // raw policy JSON to URL-encode as version document
}

func (s *stubIAMPoliciesAPI) ListPolicies(_ context.Context, _ *iam.ListPoliciesInput, _ ...func(*iam.Options)) (*iam.ListPoliciesOutput, error) {
	return &iam.ListPoliciesOutput{Policies: s.policies}, nil
}

func (s *stubIAMPoliciesAPI) GetPolicy(_ context.Context, in *iam.GetPolicyInput, _ ...func(*iam.Options)) (*iam.GetPolicyOutput, error) {
	desc := "Test policy description"
	return &iam.GetPolicyOutput{Policy: &iamtypes.Policy{
		Arn:             in.PolicyArn,
		Description:     awssdk.String(desc),
		AttachmentCount: awssdk.Int32(3),
	}}, nil
}

func (s *stubIAMPoliciesAPI) GetPolicyVersion(_ context.Context, in *iam.GetPolicyVersionInput, _ ...func(*iam.Options)) (*iam.GetPolicyVersionOutput, error) {
	encoded := url.QueryEscape(s.document)
	return &iam.GetPolicyVersionOutput{
		PolicyVersion: &iamtypes.PolicyVersion{
			Document:         awssdk.String(encoded),
			IsDefaultVersion: true,
		},
	}, nil
}

func TestIAMPoliciesProvider_ListItems(t *testing.T) {
	now := time.Now()
	stub := &stubIAMPoliciesAPI{
		policies: []iamtypes.Policy{
			{
				Arn:              awssdk.String("arn:aws:iam::123:policy/MyPolicy"),
				PolicyName:       awssdk.String("MyPolicy"),
				DefaultVersionId: awssdk.String("v1"),
				AttachmentCount:  awssdk.Int32(3),
				CreateDate:       &now,
				// Note: Description is NOT populated by ListPolicies; fetched via GetPolicy
			},
		},
	}
	p := awspkg.NewIAMPoliciesProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	item := items[0]
	if item.ID != "arn:aws:iam::123:policy/MyPolicy" {
		t.Errorf("want ID=arn:aws:iam::123:policy/MyPolicy, got %s", item.ID)
	}
	if item.Name != "MyPolicy" {
		t.Errorf("want Name=MyPolicy, got %s", item.Name)
	}
	if item.Meta["defaultVersionId"] != "v1" {
		t.Errorf("want meta defaultVersionId=v1, got %s", item.Meta["defaultVersionId"])
	}
}

func TestIAMPoliciesProvider_ListItems_Filter(t *testing.T) {
	stub := &stubIAMPoliciesAPI{
		policies: []iamtypes.Policy{
			{Arn: awssdk.String("arn:aws:iam::123:policy/Alpha"), PolicyName: awssdk.String("Alpha"), DefaultVersionId: awssdk.String("v1")},
			{Arn: awssdk.String("arn:aws:iam::123:policy/Beta"), PolicyName: awssdk.String("Beta"), DefaultVersionId: awssdk.String("v1")},
		},
	}
	p := awspkg.NewIAMPoliciesProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "alp")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "Alpha" {
		t.Errorf("want [Alpha], got %v", items)
	}
}

func TestIAMPoliciesProvider_DocumentTab(t *testing.T) {
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`
	stub := &stubIAMPoliciesAPI{document: doc}
	p := awspkg.NewIAMPoliciesProviderWithClient(stub)
	item := awspkg.Item{ID: "arn:aws:iam::123:policy/MyPolicy", Meta: map[string]string{"defaultVersionId": "v1"}}

	tabs := p.Tabs()
	var docTab awspkg.TabDef
	for _, tab := range tabs {
		if tab.Label == "Document" {
			docTab = tab
			break
		}
	}
	if docTab.Fetch == nil {
		t.Fatal("Document tab not found")
	}

	content, err := docTab.Fetch(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, `"Version": "2012-10-17"`) {
		t.Errorf("want pretty-printed JSON with Version field, got %q", content)
	}
	if !strings.Contains(content, "\n") {
		t.Errorf("want multi-line JSON, got %q", content)
	}
}
