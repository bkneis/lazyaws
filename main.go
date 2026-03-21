package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	awspkg "github.com/bryanl/lazyaws/internal/aws"
	"github.com/bryanl/lazyaws/internal/ui"
)

func setupLog() (*os.File, error) {
	dir := filepath.Join(os.Getenv("HOME"), ".local", "state", "lazyaws")
	os.MkdirAll(dir, 0755)
	f, err := os.OpenFile(filepath.Join(dir, "lazyaws.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("lazyaws started")
	return f, nil
}

func main() {
	local        := flag.Bool("local", false, "point at LocalStack (http://localhost:4566)")
	entrypointURL := flag.String("entrypoint-url", "", "custom endpoint URL, e.g. http://localhost:4566")
	flag.Parse()

	logFile, err := setupLog()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: could not open log file: %v\n", err)
	} else {
		defer logFile.Close()
		fmt.Fprintf(os.Stderr, "log: %s\n", logFile.Name())
	}

	endpointURL := *entrypointURL
	if endpointURL == "" && *local {
		endpointURL = "http://localhost:4566"
	}

	ctx := context.Background()

	cfg, err := awspkg.LoadConfig(ctx)
	if err != nil {
		log.Fatalf("aws config: %v", err)
	}

	theme := ui.DetectTheme()
	awspkg.ActiveTags = awspkg.ColorTags{Header: theme.HeaderTag, Link: theme.LinkTag}

	providers := []awspkg.Provider{
		awspkg.NewS3Provider(cfg, endpointURL),
		awspkg.NewLambdaProvider(cfg, endpointURL),
		awspkg.NewSNSProvider(cfg, endpointURL),
		awspkg.NewSQSProvider(cfg, endpointURL),
		awspkg.NewCloudFormationProvider(cfg, endpointURL),
		awspkg.NewIAMProvider(cfg, endpointURL),
		awspkg.NewIAMPoliciesProvider(cfg, endpointURL),
		awspkg.NewSecretsManagerProvider(cfg, endpointURL),
		awspkg.NewAPIGatewayProvider(cfg, endpointURL),
		awspkg.NewRoute53Provider(cfg, endpointURL),
		awspkg.NewACMProvider(cfg, endpointURL),
		awspkg.NewDynamoDBProvider(cfg, endpointURL),
		awspkg.NewKinesisProvider(cfg, endpointURL),
		awspkg.NewKMSProvider(cfg, endpointURL),
		awspkg.NewStepFunctionsProvider(cfg, endpointURL),
		awspkg.NewCloudWatchProvider(cfg, endpointURL),
		awspkg.NewCloudWatchLogsProvider(cfg, endpointURL),
		awspkg.NewEventBridgeProvider(cfg, endpointURL),
		awspkg.NewEC2Provider(cfg, endpointURL),
		awspkg.NewEC2VPCProvider(cfg, endpointURL),
		awspkg.NewEC2SGProvider(cfg, endpointURL),
		awspkg.NewEC2VolumesProvider(cfg, endpointURL),
		awspkg.NewEC2ImagesProvider(cfg, endpointURL),
		awspkg.NewELBProvider(cfg, endpointURL),
		awspkg.NewASGProvider(cfg, endpointURL),
		awspkg.NewRDSProvider(cfg, endpointURL),
	}

	app := ui.NewApp(providers, theme)
	if err := app.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
