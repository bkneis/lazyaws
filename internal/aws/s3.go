package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3API is the subset of the S3 client methods used by S3Provider.
type S3API interface {
	ListBuckets(ctx context.Context, in *s3.ListBucketsInput, opts ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
	GetBucketLocation(ctx context.Context, in *s3.GetBucketLocationInput, opts ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error)
	GetBucketVersioning(ctx context.Context, in *s3.GetBucketVersioningInput, opts ...func(*s3.Options)) (*s3.GetBucketVersioningOutput, error)
	GetPublicAccessBlock(ctx context.Context, in *s3.GetPublicAccessBlockInput, opts ...func(*s3.Options)) (*s3.GetPublicAccessBlockOutput, error)
	GetBucketEncryption(ctx context.Context, in *s3.GetBucketEncryptionInput, opts ...func(*s3.Options)) (*s3.GetBucketEncryptionOutput, error)
	ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, opts ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	GetBucketPolicy(ctx context.Context, in *s3.GetBucketPolicyInput, opts ...func(*s3.Options)) (*s3.GetBucketPolicyOutput, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// S3ObjectItem holds pre-formatted display data for interactive object row selection.
type S3ObjectItem struct {
	Key           string
	Size          int64
	SizeFormatted string
	LastModified  string
}

// S3Provider implements Provider for Amazon S3.
type S3Provider struct {
	client      S3API
	objectsMu   sync.RWMutex
	lastObjects []S3ObjectItem
}

func NewS3Provider(cfg awssdk.Config, local bool) *S3Provider {
	var opts []func(*s3.Options)
	if local {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = awssdk.String("http://localhost:4566")
			o.UsePathStyle = true
		})
	}
	return &S3Provider{client: s3.NewFromConfig(cfg, opts...)}
}

func NewS3ProviderWithClient(client S3API) *S3Provider { return &S3Provider{client: client} }

func (p *S3Provider) Name() string { return "S3" }

func (p *S3Provider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	items := make([]Item, len(out.Buckets))
	for i, b := range out.Buckets {
		name := awssdk.ToString(b.Name)
		items[i] = Item{ID: name, Name: name}
	}
	return filterItems(items, query), nil
}

func (p *S3Provider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *S3Provider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Objects", Fetch: p.tabObjects},
		{Label: "Policy", Fetch: p.tabPolicy},
	}
}

func (p *S3Provider) tabOverview(ctx context.Context, item Item) (string, error) {
	loc, err := p.client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	region := string(loc.LocationConstraint)
	if region == "" {
		region = "us-east-1"
	}

	versioning := "Disabled"
	if v, err := p.client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: awssdk.String(item.ID)}); err == nil {
		if v.Status == s3types.BucketVersioningStatusEnabled {
			versioning = "Enabled"
		} else if v.Status == s3types.BucketVersioningStatusSuspended {
			versioning = "Suspended"
		}
	}

	public := "Unknown"
	if pa, err := p.client.GetPublicAccessBlock(ctx, &s3.GetPublicAccessBlockInput{Bucket: awssdk.String(item.ID)}); err == nil && pa.PublicAccessBlockConfiguration != nil {
		cfg := pa.PublicAccessBlockConfiguration
		if awssdk.ToBool(cfg.BlockPublicAcls) && awssdk.ToBool(cfg.BlockPublicPolicy) {
			public = "All access blocked"
		} else {
			public = "Public access allowed"
		}
	}

	encryption := "None"
	if enc, err := p.client.GetBucketEncryption(ctx, &s3.GetBucketEncryptionInput{Bucket: awssdk.String(item.ID)}); err == nil && enc.ServerSideEncryptionConfiguration != nil {
		if len(enc.ServerSideEncryptionConfiguration.Rules) > 0 {
			algo := enc.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault
			if algo != nil {
				encryption = string(algo.SSEAlgorithm)
			}
		}
	}

	return KV([][2]string{
		{"Region", region},
		{"Versioning", versioning},
		{"Public", public},
		{"Encryption", encryption},
	}), nil
}

func (p *S3Provider) tabObjects(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  awssdk.String(item.ID),
		MaxKeys: awssdk.Int32(50),
	})
	if err != nil {
		return "", err
	}

	raw := make([]S3ObjectItem, len(out.Contents))
	rows := make([][]string, len(out.Contents))
	for i, obj := range out.Contents {
		key := awssdk.ToString(obj.Key)
		size := awssdk.ToInt64(obj.Size)
		mod := ""
		if obj.LastModified != nil {
			mod = obj.LastModified.Format(time.DateOnly)
		}
		raw[i] = S3ObjectItem{Key: key, Size: size, SizeFormatted: FormatSize(size), LastModified: mod}
		rows[i] = []string{key, FormatSize(size), mod}
	}

	p.objectsMu.Lock()
	p.lastObjects = raw
	p.objectsMu.Unlock()

	result := Table([]string{"Key", "Size", "Last Modified"}, rows)
	if awssdk.ToBool(out.IsTruncated) {
		result += "\n  (showing first 50 objects — use / to filter)\n"
	}
	return result, nil
}

// GetLastObjects returns the objects cached by the most recent tabObjects call.
func (p *S3Provider) GetLastObjects() []S3ObjectItem {
	p.objectsMu.RLock()
	defer p.objectsMu.RUnlock()
	out := make([]S3ObjectItem, len(p.lastObjects))
	copy(out, p.lastObjects)
	return out
}

// DownloadObject streams the S3 object body to w. The caller is responsible
// for closing the destination after writing.
func (p *S3Provider) DownloadObject(ctx context.Context, bucketName, key string, w io.Writer) error {
	out, err := p.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: awssdk.String(bucketName),
		Key:    awssdk.String(key),
	})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	defer out.Body.Close()
	if _, err := io.Copy(w, out.Body); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	return nil
}

func (p *S3Provider) tabPolicy(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: awssdk.String(item.ID)})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchBucketPolicy") {
			return "  (no bucket policy)\n", nil
		}
		return "", err
	}
	var raw any
	if err := json.Unmarshal([]byte(awssdk.ToString(out.Policy)), &raw); err != nil {
		return awssdk.ToString(out.Policy), nil
	}
	b, _ := json.MarshalIndent(raw, "  ", "  ")
	return "  " + string(b) + "\n", nil
}

func FormatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
