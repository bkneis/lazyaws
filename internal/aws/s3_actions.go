package aws

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3ActionsAPI defines write operations needed by the S3 actions menu.
type S3ActionsAPI interface {
	CreateBucket(ctx context.Context, in *s3.CreateBucketInput, opts ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	DeleteBucket(ctx context.Context, in *s3.DeleteBucketInput, opts ...func(*s3.Options)) (*s3.DeleteBucketOutput, error)
	PutObject(ctx context.Context, in *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, opts ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	PutBucketVersioning(ctx context.Context, in *s3.PutBucketVersioningInput, opts ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error)
}

// Actions implements Actionable for S3Provider.
func (p *S3Provider) Actions(item Item) []ActionDef {
	wc, ok := p.client.(S3ActionsAPI)
	if !ok {
		return nil
	}

	actions := []ActionDef{
		{
			Label: "Create bucket",
			Key:   'c',
			Func: func(ctx context.Context, item Item, ac ActionContext) error {
				ac.PromptInput("Bucket name", "", func(name string) {
					go func() {
						in := &s3.CreateBucketInput{Bucket: awssdk.String(name)}
						// us-east-1 must NOT include CreateBucketConfiguration.
						if region := p.region; region != "" && region != "us-east-1" {
							in.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{
								LocationConstraint: s3types.BucketLocationConstraint(region),
							}
						}
						_, err := wc.CreateBucket(context.Background(), in)
						if err != nil {
							ac.ShowError(err)
							return
						}
						ac.Refresh()
					}()
				})
				return nil
			},
		},
	}

	if item.ID != "" {
		actions = append(actions,
			ActionDef{
				Label: "Delete bucket",
				Key:   'd',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.ConfirmDelete(item.ID, func() {
						go func() {
							_, err := wc.DeleteBucket(context.Background(), &s3.DeleteBucketInput{
								Bucket: awssdk.String(item.ID),
							})
							if err != nil {
								ac.ShowError(err)
								return
							}
							ac.Refresh()
						}()
					})
					return nil
				},
			},
			ActionDef{
				Label: "Upload file",
				Key:   'u',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.PromptInput("Local file path", "", func(localPath string) {
						ac.PromptInput("S3 key", filepath.Base(localPath), func(key string) {
							go func() {
								f, err := os.Open(localPath)
								if err != nil {
									ac.ShowError(fmt.Errorf("open file: %w", err))
									return
								}
								defer f.Close()
								_, err = wc.PutObject(context.Background(), &s3.PutObjectInput{
									Bucket: awssdk.String(item.ID),
									Key:    awssdk.String(key),
									Body:   f,
								})
								if err != nil {
									ac.ShowError(err)
									return
								}
								ac.Refresh()
							}()
						})
					})
					return nil
				},
			},
			ActionDef{
				Label: "Toggle versioning",
				Key:   'v',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					ac.Confirm(fmt.Sprintf("Toggle versioning on bucket %q?", item.ID), func() {
						go func() {
							// Check current status via GetBucketVersioning (already in S3API).
							out, err := p.client.GetBucketVersioning(context.Background(), &s3.GetBucketVersioningInput{
								Bucket: awssdk.String(item.ID),
							})
							if err != nil {
								ac.ShowError(err)
								return
							}
							status := s3types.BucketVersioningStatusEnabled
							if out.Status == s3types.BucketVersioningStatusEnabled {
								status = s3types.BucketVersioningStatusSuspended
							}
							_, err = wc.PutBucketVersioning(context.Background(), &s3.PutBucketVersioningInput{
								Bucket: awssdk.String(item.ID),
								VersioningConfiguration: &s3types.VersioningConfiguration{
									Status: status,
								},
							})
							if err != nil {
								ac.ShowError(err)
								return
							}
							ac.Refresh()
						}()
					})
					return nil
				},
			},
		)

		// Delete object — only shown when an object is selected.
		if key := item.Meta["selectedObjectKey"]; key != "" {
			actions = append(actions, ActionDef{
				Label: "Delete object",
				Key:   'x',
				Func: func(ctx context.Context, item Item, ac ActionContext) error {
					objKey := item.Meta["selectedObjectKey"]
					ac.ConfirmDelete(objKey, func() {
						go func() {
							_, err := wc.DeleteObject(context.Background(), &s3.DeleteObjectInput{
								Bucket: awssdk.String(item.ID),
								Key:    awssdk.String(objKey),
							})
							if err != nil {
								ac.ShowError(err)
								return
							}
							ac.Refresh()
						}()
					})
					return nil
				},
			})
		}
	}

	return actions
}
