package aws

import (
	"context"
	"log"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/smithy-go/logging"
)

// LoadConfig returns an aws.Config loaded from the default credential chain
// (environment variables, ~/.aws/credentials, IAM role, etc.).
func LoadConfig(ctx context.Context) (awssdk.Config, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return cfg, err
	}
	cfg.Logger = logging.LoggerFunc(func(_ logging.Classification, format string, v ...interface{}) {
		log.Printf("[sdk] "+format, v...)
	})
	cfg.ClientLogMode = awssdk.LogRequest | awssdk.LogResponse | awssdk.LogRetries
	return cfg, nil
}
