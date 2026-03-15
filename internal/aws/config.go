package aws

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// LoadConfig returns an aws.Config loaded from the default credential chain
// (environment variables, ~/.aws/credentials, IAM role, etc.).
func LoadConfig(ctx context.Context) (awssdk.Config, error) {
	return config.LoadDefaultConfig(ctx)
}
