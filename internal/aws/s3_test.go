package aws_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

// stubS3 implements awspkg.S3API using in-memory data.
type stubS3 struct {
	buckets  []s3types.Bucket
	location string
}

func (s *stubS3) ListBuckets(_ context.Context, _ *s3.ListBucketsInput, _ ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	return &s3.ListBucketsOutput{Buckets: s.buckets}, nil
}

func (s *stubS3) GetBucketLocation(_ context.Context, in *s3.GetBucketLocationInput, _ ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error) {
	for _, b := range s.buckets {
		if aws.ToString(b.Name) == aws.ToString(in.Bucket) {
			return &s3.GetBucketLocationOutput{
				LocationConstraint: s3types.BucketLocationConstraint(s.location),
			}, nil
		}
	}
	return nil, fmt.Errorf("bucket not found: %s", aws.ToString(in.Bucket))
}

func (s *stubS3) GetBucketVersioning(_ context.Context, _ *s3.GetBucketVersioningInput, _ ...func(*s3.Options)) (*s3.GetBucketVersioningOutput, error) {
	return &s3.GetBucketVersioningOutput{
		Status: s3types.BucketVersioningStatusEnabled,
	}, nil
}

func (s *stubS3) GetPublicAccessBlock(_ context.Context, _ *s3.GetPublicAccessBlockInput, _ ...func(*s3.Options)) (*s3.GetPublicAccessBlockOutput, error) {
	t := true
	return &s3.GetPublicAccessBlockOutput{
		PublicAccessBlockConfiguration: &s3types.PublicAccessBlockConfiguration{
			BlockPublicAcls:   &t,
			BlockPublicPolicy: &t,
		},
	}, nil
}

func (s *stubS3) GetBucketEncryption(_ context.Context, _ *s3.GetBucketEncryptionInput, _ ...func(*s3.Options)) (*s3.GetBucketEncryptionOutput, error) {
	return &s3.GetBucketEncryptionOutput{
		ServerSideEncryptionConfiguration: &s3types.ServerSideEncryptionConfiguration{
			Rules: []s3types.ServerSideEncryptionRule{
				{ApplyServerSideEncryptionByDefault: &s3types.ServerSideEncryptionByDefault{
					SSEAlgorithm: s3types.ServerSideEncryptionAes256,
				}},
			},
		},
	}, nil
}

func (s *stubS3) ListObjectsV2(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	sz := int64(2400000)
	mod := time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC)
	return &s3.ListObjectsV2Output{
		Contents: []s3types.Object{
			{Key: aws.String("images/hero.png"), Size: &sz, LastModified: &mod},
		},
		KeyCount: aws.Int32(1),
	}, nil
}

func (s *stubS3) GetBucketPolicy(_ context.Context, _ *s3.GetBucketPolicyInput, _ ...func(*s3.Options)) (*s3.GetBucketPolicyOutput, error) {
	return &s3.GetBucketPolicyOutput{
		Policy: aws.String(`{"Version":"2012-10-17","Statement":[]}`),
	}, nil
}

func (s *stubS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	content := "hello world"
	return &s3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader(content)),
	}, nil
}

func TestS3Provider_ListItems(t *testing.T) {
	created := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	stub := &stubS3{
		buckets: []s3types.Bucket{
			{Name: aws.String("my-bucket"), CreationDate: &created},
		},
	}

	p := awspkg.NewS3ProviderWithClient(stub)
	items, err := p.ListItems(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Name != "my-bucket" {
		t.Errorf("got name %q, want my-bucket", items[0].Name)
	}
}

func TestS3Provider_GetDetail(t *testing.T) {
	created := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	stub := &stubS3{
		buckets:  []s3types.Bucket{{Name: aws.String("my-bucket"), CreationDate: &created}},
		location: "eu-west-1",
	}

	p := awspkg.NewS3ProviderWithClient(stub)
	items, _ := p.ListItems(context.Background(), "")

	detail, err := p.GetDetail(context.Background(), items[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(detail, "eu-west-1") {
		t.Errorf("detail missing region eu-west-1\nraw: %s", detail)
	}
}

func TestS3Provider_Tabs(t *testing.T) {
	created := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	stub := &stubS3{
		buckets:  []s3types.Bucket{{Name: aws.String("my-bucket"), CreationDate: &created}},
		location: "us-east-1",
	}
	p := awspkg.NewS3ProviderWithClient(stub)
	tabs := p.Tabs()

	if len(tabs) != 5 {
		t.Fatalf("got %d tabs, want 5", len(tabs))
	}

	item := awspkg.Item{ID: "my-bucket", Name: "my-bucket"}

	cases := []struct {
		tabIdx int
		label  string
		want   string
	}{
		{0, "Overview", "us-east-1"},
		{1, "Objects", "images/hero.png"},
		{2, "Policy", "2012-10-17"},
		{3, "Content", "(no object selected"},
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

func TestS3Provider_ListItems_Filter(t *testing.T) {
	created := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	stub := &stubS3{
		buckets: []s3types.Bucket{
			{Name: aws.String("my-bucket"), CreationDate: &created},
			{Name: aws.String("other-store"), CreationDate: &created},
		},
	}
	p := awspkg.NewS3ProviderWithClient(stub)
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

func TestS3Provider_TabOverview_EmptyRegion(t *testing.T) {
	created := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	stub := &stubS3{
		buckets:  []s3types.Bucket{{Name: aws.String("my-bucket"), CreationDate: &created}},
		location: "", // empty = us-east-1 per AWS API
	}
	p := awspkg.NewS3ProviderWithClient(stub)
	tabs := p.Tabs()
	item := awspkg.Item{ID: "my-bucket", Name: "my-bucket"}
	content, err := tabs[0].Fetch(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "us-east-1") {
		t.Errorf("expected us-east-1 for empty LocationConstraint, got:\n%s", content)
	}
}

func TestS3Provider_GetLastObjects(t *testing.T) {
	created := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	stub := &stubS3{
		buckets: []s3types.Bucket{{Name: aws.String("my-bucket"), CreationDate: &created}},
	}
	p := awspkg.NewS3ProviderWithClient(stub)
	item := awspkg.Item{ID: "my-bucket", Name: "my-bucket"}
	// call through the exported Tabs() path
	_, err := p.Tabs()[1].Fetch(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	got := p.GetLastObjects()
	if len(got) != 1 || got[0].Key != "images/hero.png" {
		t.Errorf("want [{images/hero.png ...}], got %v", got)
	}
	if got[0].SizeFormatted == "" {
		t.Error("want non-empty SizeFormatted")
	}
}

func TestS3Provider_DownloadObject(t *testing.T) {
	stub := &stubS3{}
	p := awspkg.NewS3ProviderWithClient(stub)
	var buf strings.Builder
	err := p.DownloadObject(context.Background(), "my-bucket", "readme.txt", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello world" {
		t.Errorf("want 'hello world', got %q", buf.String())
	}
}

func TestS3Provider_FetchItem(t *testing.T) {
	created := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	stub := &stubS3{
		buckets:  []s3types.Bucket{{Name: aws.String("my-bucket"), CreationDate: &created}},
		location: "eu-west-1",
	}
	p := awspkg.NewS3ProviderWithClient(stub)

	cases := []struct {
		name    string
		id      string
		wantErr bool
		wantID  string
	}{
		{"found", "my-bucket", false, "my-bucket"},
		{"not found", "missing-bucket", true, ""},
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

func TestS3Provider_ContentTab(t *testing.T) {
	stub := &stubS3{}
	p := awspkg.NewS3ProviderWithClient(stub)
	item := awspkg.Item{ID: "my-bucket", Name: "my-bucket"}
	contentFetch := p.Tabs()[3].Fetch

	cases := []struct {
		name string
		key  string
		size int64
		want string
	}{
		{"no selection", "", 0, "(no object selected"},
		{"binary file", "image.png", 1000, "(binary file"},
		{"text file", "readme.txt", 100, "hello world"},
		{"too large", "big.txt", 11 * 1024 * 1024, "(file too large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p.SetSelectedObject(tc.key, tc.size)
			content, err := contentFetch(context.Background(), item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(content, tc.want) {
				t.Errorf("content missing %q\ngot: %s", tc.want, content)
			}
		})
	}
}
