package main

import (
	"context"
	"flag"
	"log"

	awspkg "github.com/bryanl/lazyaws/internal/aws"
	"github.com/bryanl/lazyaws/internal/ui"
)

func main() {
	local := flag.Bool("local", false, "point at LocalStack (http://localhost:4566)")
	flag.Parse()

	ctx := context.Background()

	cfg, err := awspkg.LoadConfig(ctx)
	if err != nil {
		log.Fatalf("aws config: %v", err)
	}

	theme := ui.DetectTheme()
	awspkg.ActiveTags = awspkg.ColorTags{Header: theme.HeaderTag, Link: theme.LinkTag}

	providers := []awspkg.Provider{
		awspkg.NewS3Provider(cfg, *local),
		awspkg.NewLambdaProvider(cfg, *local),
		awspkg.NewSNSProvider(cfg, *local),
		awspkg.NewSQSProvider(cfg, *local),
		awspkg.NewCloudFormationProvider(cfg, *local),
		awspkg.NewIAMProvider(cfg, *local),
		awspkg.NewIAMPoliciesProvider(cfg, *local),
		awspkg.NewSecretsManagerProvider(cfg, *local),
		awspkg.NewAPIGatewayProvider(cfg, *local),
		awspkg.NewRoute53Provider(cfg, *local),
		awspkg.NewACMProvider(cfg, *local),
		awspkg.NewDynamoDBProvider(cfg, *local),
		awspkg.NewKinesisProvider(cfg, *local),
		awspkg.NewKMSProvider(cfg, *local),
		awspkg.NewStepFunctionsProvider(cfg, *local),
		awspkg.NewCloudWatchProvider(cfg, *local),
		awspkg.NewCloudWatchLogsProvider(cfg, *local),
		awspkg.NewEventBridgeProvider(cfg, *local),
		awspkg.NewEC2Provider(cfg, *local),
	}

	app := ui.NewApp(providers, theme)
	if err := app.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
