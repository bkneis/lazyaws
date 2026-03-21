package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
	"github.com/bkneis/lazyaws/internal/ui"
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
	local         := flag.Bool("local", false, "point at LocalStack (http://localhost:4566)")
	entrypointURL := flag.String("entrypoint-url", "", "custom endpoint URL, e.g. http://localhost:4566")
	services      := flag.String("services", "", "comma-separated services to show, e.g. s3,sns,lambda")
	showVersion   := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("lazyaws %s\n", version)
		os.Exit(0)
	}

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
	if t := ui.LoadConfigTheme(theme); t != nil {
		theme = *t
	}
	awspkg.ActiveTags = awspkg.ColorTags{Header: theme.HeaderTag, Link: theme.LinkTag}

	buildProviders := func(c awssdk.Config, endpoint string) []awspkg.Provider {
		return []awspkg.Provider{
			awspkg.NewS3Provider(c, endpoint),
			awspkg.NewLambdaProvider(c, endpoint),
			awspkg.NewSNSProvider(c, endpoint),
			awspkg.NewSQSProvider(c, endpoint),
			awspkg.NewCloudFormationProvider(c, endpoint),
			awspkg.NewIAMProvider(c, endpoint),
			awspkg.NewIAMPoliciesProvider(c, endpoint),
			awspkg.NewSecretsManagerProvider(c, endpoint),
			awspkg.NewAPIGatewayProvider(c, endpoint),
			awspkg.NewRoute53Provider(c, endpoint),
			awspkg.NewACMProvider(c, endpoint),
			awspkg.NewDynamoDBProvider(c, endpoint),
			awspkg.NewKinesisProvider(c, endpoint),
			awspkg.NewKMSProvider(c, endpoint),
			awspkg.NewStepFunctionsProvider(c, endpoint),
			awspkg.NewCloudWatchProvider(c, endpoint),
			awspkg.NewCloudWatchLogsProvider(c, endpoint),
			awspkg.NewEventBridgeProvider(c, endpoint),
			awspkg.NewEC2Provider(c, endpoint),
			awspkg.NewEC2VPCProvider(c, endpoint),
			awspkg.NewEC2SGProvider(c, endpoint),
			awspkg.NewEC2VolumesProvider(c, endpoint),
			awspkg.NewEC2ImagesProvider(c, endpoint),
			awspkg.NewELBProvider(c, endpoint),
			awspkg.NewASGProvider(c, endpoint),
			awspkg.NewRDSProvider(c, endpoint),
		}
	}

	providers := buildProviders(cfg, endpointURL)

	if *services != "" {
		allowed := map[string]bool{}
		for _, s := range strings.Split(*services, ",") {
			allowed[strings.ToLower(strings.TrimSpace(s))] = true
		}
		filtered := make([]awspkg.Provider, 0, len(providers))
		for _, p := range providers {
			if allowed[strings.ToLower(p.Name())] {
				filtered = append(filtered, p)
			}
		}
		providers = filtered
	}

	rebuildFn := func(region string) []awspkg.Provider {
		c := cfg
		c.Region = region
		return buildProviders(c, endpointURL)
	}

	app := ui.NewApp(providers, theme, cfg.Region, rebuildFn)
	if err := app.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
