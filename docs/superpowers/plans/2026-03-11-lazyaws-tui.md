# lazyaws TUI Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a lazydocker-style terminal UI for browsing AWS resources (S3, Lambda), with LocalStack support via `--local` flag.

**Architecture:** 3-panel tview layout (resource type selector | item list | detail view). A `Provider` interface decouples AWS fetching from the UI layer. S3 and Lambda are the v1 implementations; the architecture is intentionally extensible so additional providers (API Gateway, IAM, etc.) can be added by dropping a new file into `internal/aws/` and registering it in `main.go`. A `--local` flag applies a `BaseEndpoint` override per service client to point at LocalStack (`http://localhost:4566`).

**Tech Stack:** Go, [tview](https://github.com/rivo/tview), [tcell/v2](https://github.com/gdamore/tcell), AWS SDK v2 (`github.com/aws/aws-sdk-go-v2`)

**Scope (v1):** S3 and Lambda. The spec lists 7 services; this plan builds 2 by explicit decision — the provider pattern makes adding the remaining 5 (API Gateway, IAM, Secrets Manager, SNS, SQS) a straightforward follow-up.

---

## File Map

| File | Responsibility |
|------|---------------|
| `main.go` | Entry point: parse `--local` flag, build providers, wire UI, run |
| `.gitignore` | Ignore build artifacts |
| `internal/aws/config.go` | Load `aws.Config` from default credential chain |
| `internal/aws/provider.go` | `Provider` interface and `Item` type |
| `internal/aws/s3.go` | S3 `Provider`: list buckets, get bucket detail |
| `internal/aws/s3_test.go` | Unit tests for S3 provider via stub client |
| `internal/aws/lambda.go` | Lambda `Provider`: list functions, get function detail |
| `internal/aws/lambda_test.go` | Unit tests for Lambda provider via stub client |
| `internal/ui/app.go` | tview `Application`, 3-panel `Flex` layout, async load helpers |
| `internal/ui/panels.go` | Panel state: tview primitives + focus cycling |
| `internal/ui/panels_test.go` | Unit tests for focus cycling logic |
| `internal/ui/keys.go` | Input capture: Tab/Shift+Tab, j/k, q, r |

---

## Chunk 1: Foundation

### Task 1: Project Scaffold

**Files:**
- Create: `main.go`
- Create: `.gitignore`
- Create: `go.mod`
- Create: `internal/aws/config.go`
- Create: `internal/aws/provider.go`

- [ ] **Step 1: Init Go module**

```bash
cd /home/bryan/lazyaws
go mod init github.com/bryanl/lazyaws
```

Expected: `go.mod` created.

- [ ] **Step 2: Install dependencies**

```bash
go get github.com/rivo/tview@latest
go get github.com/aws/aws-sdk-go-v2@latest
go get github.com/aws/aws-sdk-go-v2/config@latest
go get github.com/aws/aws-sdk-go-v2/service/s3@latest
go get github.com/aws/aws-sdk-go-v2/service/lambda@latest
```

- [ ] **Step 3: Create directory structure**

```bash
mkdir -p internal/aws internal/ui docs/superpowers/plans
```

- [ ] **Step 4: Create `.gitignore`**

```
lazyaws
*.test
```

- [ ] **Step 5: Write `internal/aws/provider.go`**

```go
package aws

import "context"

// Item represents a single resource returned by a Provider.
type Item struct {
	ID   string
	Name string
}

// Provider lists and describes a category of AWS resources.
type Provider interface {
	// Name is the display label shown in the resource-type panel.
	Name() string
	// ListItems returns the top-level list of resources.
	ListItems(ctx context.Context) ([]Item, error)
	// GetDetail returns a formatted string (typically JSON) for the detail panel.
	GetDetail(ctx context.Context, item Item) (string, error)
}
```

- [ ] **Step 6: Write `internal/aws/config.go`**

```go
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
```

> Note: LocalStack endpoint injection happens at the individual service-client level
> (per AWS SDK v2 recommendation), not at the config level. Each Provider constructor
> accepts a `local bool` and applies `o.BaseEndpoint = aws.String("http://localhost:4566")`
> when constructing its SDK client.

- [ ] **Step 7: Write `main.go`**

```go
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

	providers := []awspkg.Provider{
		awspkg.NewS3Provider(cfg, *local),
		awspkg.NewLambdaProvider(cfg, *local),
	}

	app := ui.NewApp(providers)
	if err := app.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
```

- [ ] **Step 8: Confirm partial build (missing packages expected)**

```bash
go build ./... 2>&1 | head -20
```

Expected: errors for missing `ui` and provider packages — fine, scaffold only at this stage.

- [ ] **Step 9: Commit scaffold**

```bash
git add .gitignore go.mod go.sum internal/aws/provider.go internal/aws/config.go
git commit -m "chore: project scaffold — module, provider interface, aws config loader"
```

---

### Task 2: Config Smoke Test

**Files:**
- Create: `internal/aws/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
package aws_test

import (
	"context"
	"testing"

	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

func TestLoadConfig_returnsValidConfig(t *testing.T) {
	cfg, err := awspkg.LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A valid config must have a region or at least not panic on use.
	_ = cfg
}
```

- [ ] **Step 2: Run — verify it passes (no network call, just credential chain resolution)**

```bash
go test ./internal/aws/ -run TestLoadConfig -v
```

Expected: PASS. (The SDK resolves config without making network calls.)

- [ ] **Step 3: Commit**

```bash
git add internal/aws/config_test.go
git commit -m "test: smoke test for LoadConfig"
```

---

## Chunk 2: Resource Providers

### Task 3: S3 Provider

**Files:**
- Create: `internal/aws/s3.go`
- Create: `internal/aws/s3_test.go`

- [ ] **Step 1: Write the failing test first**

Create `internal/aws/s3_test.go`:

```go
package aws_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

// stubS3 implements awspkg.S3API using in-memory data.
type stubS3 struct {
	buckets  []s3types.Bucket
	location string
}

func (s *stubS3) ListBuckets(_ context.Context, _ *s3.ListBucketsInput, _ ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	return &s3.ListBucketsOutput{Buckets: s.buckets}, nil
}

func (s *stubS3) GetBucketLocation(_ context.Context, _ *s3.GetBucketLocationInput, _ ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error) {
	return &s3.GetBucketLocationOutput{
		LocationConstraint: s3types.BucketLocationConstraint(s.location),
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
	items, err := p.ListItems(context.Background())
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
	items, _ := p.ListItems(context.Background())

	detail, err := p.GetDetail(context.Background(), items[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(detail), &m); err != nil {
		t.Fatalf("detail is not valid JSON: %v\nraw: %s", err, detail)
	}
	if m["Name"] != "my-bucket" {
		t.Errorf("got Name %v, want my-bucket", m["Name"])
	}
	if m["Location"] != "eu-west-1" {
		t.Errorf("got Location %v, want eu-west-1", m["Location"])
	}
}
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/aws/ -run TestS3 -v
```

Expected: FAIL — `S3API`, `NewS3ProviderWithClient` not defined.

- [ ] **Step 3: Implement `internal/aws/s3.go`**

```go
package aws

import (
	"context"
	"encoding/json"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3API is the subset of the S3 client methods used by S3Provider.
type S3API interface {
	ListBuckets(ctx context.Context, in *s3.ListBucketsInput, opts ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
	GetBucketLocation(ctx context.Context, in *s3.GetBucketLocationInput, opts ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error)
}

// S3Provider implements Provider for Amazon S3.
type S3Provider struct {
	client S3API
}

// NewS3Provider creates an S3Provider backed by a real AWS SDK client.
// When local is true the client endpoint is overridden to LocalStack.
func NewS3Provider(cfg awssdk.Config, local bool) *S3Provider {
	opts := []func(*s3.Options){}
	if local {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = awssdk.String("http://localhost:4566")
			o.UsePathStyle = true // LocalStack requires path-style S3 URLs
		})
	}
	return &S3Provider{client: s3.NewFromConfig(cfg, opts...)}
}

// NewS3ProviderWithClient creates an S3Provider with a custom client (for testing).
func NewS3ProviderWithClient(client S3API) *S3Provider {
	return &S3Provider{client: client}
}

func (p *S3Provider) Name() string { return "S3" }

func (p *S3Provider) ListItems(ctx context.Context) ([]Item, error) {
	out, err := p.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}

	items := make([]Item, len(out.Buckets))
	for i, b := range out.Buckets {
		name := awssdk.ToString(b.Name)
		items[i] = Item{ID: name, Name: name}
	}
	return items, nil
}

func (p *S3Provider) GetDetail(ctx context.Context, item Item) (string, error) {
	loc, err := p.client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
		Bucket: awssdk.String(item.ID),
	})
	if err != nil {
		return "", fmt.Errorf("get bucket location: %w", err)
	}

	detail := map[string]any{
		"Name":     item.ID,
		"Location": string(loc.LocationConstraint),
	}
	b, err := json.MarshalIndent(detail, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
```

- [ ] **Step 4: Run tests — verify they pass**

```bash
go test ./internal/aws/ -run TestS3 -v
```

Expected: PASS for `TestS3Provider_ListItems` and `TestS3Provider_GetDetail`.

- [ ] **Step 5: Commit**

```bash
git add internal/aws/s3.go internal/aws/s3_test.go
git commit -m "feat: S3 provider — list buckets, get bucket location detail"
```

---

### Task 4: Lambda Provider

**Files:**
- Create: `internal/aws/lambda.go`
- Create: `internal/aws/lambda_test.go`

- [ ] **Step 1: Write the failing test first**

Create `internal/aws/lambda_test.go`:

```go
package aws_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	awspkg "github.com/bryanl/lazyaws/internal/aws"
)

// stubLambda implements awspkg.LambdaAPI using in-memory data.
type stubLambda struct {
	functions []lambdatypes.FunctionConfiguration
}

func (s *stubLambda) ListFunctions(_ context.Context, _ *lambda.ListFunctionsInput, _ ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error) {
	return &lambda.ListFunctionsOutput{Functions: s.functions}, nil
}

func (s *stubLambda) GetFunction(_ context.Context, in *lambda.GetFunctionInput, _ ...func(*lambda.Options)) (*lambda.GetFunctionOutput, error) {
	for _, f := range s.functions {
		if aws.ToString(f.FunctionName) == aws.ToString(in.FunctionName) {
			fc := f
			return &lambda.GetFunctionOutput{Configuration: &fc}, nil
		}
	}
	return &lambda.GetFunctionOutput{}, nil
}

func TestLambdaProvider_ListItems(t *testing.T) {
	stub := &stubLambda{
		functions: []lambdatypes.FunctionConfiguration{
			{FunctionName: aws.String("my-function"), Runtime: lambdatypes.RuntimeProvidedal2023},
		},
	}

	p := awspkg.NewLambdaProviderWithClient(stub)
	items, err := p.ListItems(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Name != "my-function" {
		t.Errorf("got name %q, want my-function", items[0].Name)
	}
}

func TestLambdaProvider_GetDetail(t *testing.T) {
	stub := &stubLambda{
		functions: []lambdatypes.FunctionConfiguration{
			{FunctionName: aws.String("my-function"), MemorySize: aws.Int32(128)},
		},
	}

	p := awspkg.NewLambdaProviderWithClient(stub)
	items, _ := p.ListItems(context.Background())

	detail, err := p.GetDetail(context.Background(), items[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(detail), &m); err != nil {
		t.Fatalf("detail is not valid JSON: %v\nraw: %s", err, detail)
	}
	if m["FunctionName"] != "my-function" {
		t.Errorf("got FunctionName %v, want my-function", m["FunctionName"])
	}
}
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/aws/ -run TestLambda -v
```

Expected: FAIL — `LambdaAPI`, `NewLambdaProviderWithClient` not defined.

- [ ] **Step 3: Implement `internal/aws/lambda.go`**

```go
package aws

import (
	"context"
	"encoding/json"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
)

// LambdaAPI is the subset of the Lambda client methods used by LambdaProvider.
type LambdaAPI interface {
	ListFunctions(ctx context.Context, in *lambda.ListFunctionsInput, opts ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error)
	GetFunction(ctx context.Context, in *lambda.GetFunctionInput, opts ...func(*lambda.Options)) (*lambda.GetFunctionOutput, error)
}

// LambdaProvider implements Provider for AWS Lambda.
type LambdaProvider struct {
	client LambdaAPI
}

// NewLambdaProvider creates a LambdaProvider backed by a real AWS SDK client.
// When local is true the client endpoint is overridden to LocalStack.
func NewLambdaProvider(cfg awssdk.Config, local bool) *LambdaProvider {
	opts := []func(*lambda.Options){}
	if local {
		opts = append(opts, func(o *lambda.Options) {
			o.BaseEndpoint = awssdk.String("http://localhost:4566")
		})
	}
	return &LambdaProvider{client: lambda.NewFromConfig(cfg, opts...)}
}

// NewLambdaProviderWithClient creates a LambdaProvider with a custom client (for testing).
func NewLambdaProviderWithClient(client LambdaAPI) *LambdaProvider {
	return &LambdaProvider{client: client}
}

func (p *LambdaProvider) Name() string { return "Lambda" }

func (p *LambdaProvider) ListItems(ctx context.Context) ([]Item, error) {
	out, err := p.client.ListFunctions(ctx, &lambda.ListFunctionsInput{})
	if err != nil {
		return nil, fmt.Errorf("list functions: %w", err)
	}

	items := make([]Item, len(out.Functions))
	for i, f := range out.Functions {
		name := awssdk.ToString(f.FunctionName)
		items[i] = Item{ID: name, Name: name}
	}
	return items, nil
}

func (p *LambdaProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: awssdk.String(item.ID),
	})
	if err != nil {
		return "", fmt.Errorf("get function: %w", err)
	}

	b, err := json.MarshalIndent(out.Configuration, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
```

- [ ] **Step 4: Run all provider tests**

```bash
go test ./internal/aws/... -v
```

Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/aws/lambda.go internal/aws/lambda_test.go
git commit -m "feat: Lambda provider — list functions, get function configuration detail"
```

---

## Chunk 3: UI

### Task 5: Panels (with tests)

**Files:**
- Create: `internal/ui/panels.go`
- Create: `internal/ui/panels_test.go`

- [ ] **Step 1: Write failing focus-cycling tests**

Create `internal/ui/panels_test.go`:

```go
package ui

import (
	"testing"
)

func TestPanels_next(t *testing.T) {
	cases := []struct {
		start    int
		expected int
	}{
		{start: 0, expected: 1},
		{start: 1, expected: 2},
		{start: 2, expected: 0}, // wraps around
	}
	for _, tc := range cases {
		p := newPanels()
		p.focused = tc.start
		p.next()
		if p.focused != tc.expected {
			t.Errorf("next from %d: got %d, want %d", tc.start, p.focused, tc.expected)
		}
	}
}

func TestPanels_prev(t *testing.T) {
	cases := []struct {
		start    int
		expected int
	}{
		{start: 2, expected: 1},
		{start: 1, expected: 0},
		{start: 0, expected: 2}, // wraps around
	}
	for _, tc := range cases {
		p := newPanels()
		p.focused = tc.start
		p.prev()
		if p.focused != tc.expected {
			t.Errorf("prev from %d: got %d, want %d", tc.start, p.focused, tc.expected)
		}
	}
}
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/ui/ -run TestPanels -v
```

Expected: FAIL — `newPanels`, `panels` not defined.

- [ ] **Step 3: Implement `internal/ui/panels.go`**

```go
package ui

import "github.com/rivo/tview"

// panels holds the three tview widgets and the currently focused index.
type panels struct {
	resources *tview.List
	items     *tview.List
	detail    *tview.TextView
	focused   int // 0=resources, 1=items, 2=detail
}

func newPanels() *panels {
	resources := tview.NewList().ShowSecondaryText(false)
	resources.SetBorder(true).SetTitle(" Resources ")

	items := tview.NewList().ShowSecondaryText(false)
	items.SetBorder(true).SetTitle(" Items ")

	detail := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	detail.SetBorder(true).SetTitle(" Detail ")

	return &panels{
		resources: resources,
		items:     items,
		detail:    detail,
	}
}

// primitives returns the panels in Tab-cycle order.
func (p *panels) primitives() []tview.Primitive {
	return []tview.Primitive{p.resources, p.items, p.detail}
}

// current returns the currently focused primitive.
func (p *panels) current() tview.Primitive {
	return p.primitives()[p.focused]
}

// next advances focus by one (wraps around) and returns the new focus target.
func (p *panels) next() tview.Primitive {
	p.focused = (p.focused + 1) % 3
	return p.current()
}

// prev retreats focus by one (wraps around) and returns the new focus target.
func (p *panels) prev() tview.Primitive {
	p.focused = (p.focused + 2) % 3
	return p.current()
}
```

- [ ] **Step 4: Run tests — verify they pass**

```bash
go test ./internal/ui/ -run TestPanels -v
```

Expected: PASS for all 6 cases (3 next, 3 prev).

- [ ] **Step 5: Commit**

```bash
git add internal/ui/panels.go internal/ui/panels_test.go
git commit -m "feat: tview panels with focus cycling; test next/prev wrap-around"
```

---

### Task 6: Keys

**Files:**
- Create: `internal/ui/keys.go`

No unit tests for keybinding wiring — this is pure tview event plumbing, verified by manual smoke test.

- [ ] **Step 1: Implement `internal/ui/keys.go`**

```go
package ui

import "github.com/gdamore/tcell/v2"

// setupKeys attaches the global keyboard handler to the application.
//
// Bindings:
//   Tab         — focus next panel
//   Shift+Tab   — focus previous panel
//   j / ↓       — move down in focused list (tview handles arrows natively;
//                  j requires explicit forwarding as a Down key event)
//   k / ↑       — move up in focused list
//   q           — quit
//   r           — refresh current resource list
func setupKeys(a *App) {
	a.tapp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			a.tapp.SetFocus(a.panels.next())
			return nil
		case tcell.KeyBacktab:
			a.tapp.SetFocus(a.panels.prev())
			return nil
		}

		switch event.Rune() {
		case 'q':
			a.tapp.Stop()
			return nil
		case 'r':
			a.refresh()
			return nil
		case 'j':
			// Forward j as Down so tview List handles navigation.
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case 'k':
			// Forward k as Up so tview List handles navigation.
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		}

		return event
	})
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/ui/keys.go
git commit -m "feat: keybindings — Tab, j/k, q, r"
```

---

### Task 7: App (async loading)

**Files:**
- Create: `internal/ui/app.go`

- [ ] **Step 1: Implement `internal/ui/app.go`**

```go
package ui

import (
	"context"
	"fmt"

	awspkg "github.com/bryanl/lazyaws/internal/aws"
	"github.com/rivo/tview"
)

// App is the root TUI application.
type App struct {
	tapp            *tview.Application
	panels          *panels
	providers       []awspkg.Provider
	activeProvider  int
}

// NewApp constructs the App with the given resource providers.
func NewApp(providers []awspkg.Provider) *App {
	a := &App{
		tapp:      tview.NewApplication(),
		panels:    newPanels(),
		providers: providers,
	}
	a.build()
	return a
}

func (a *App) build() {
	for i, p := range a.providers {
		i, p := i, p
		a.panels.resources.AddItem(p.Name(), "", 0, func() {
			a.activeProvider = i
			a.loadItems(i)
		})
	}

	if len(a.providers) > 0 {
		a.loadItems(0)
	}

	layout := tview.NewFlex().
		AddItem(a.panels.resources, 20, 0, true).
		AddItem(a.panels.items, 30, 0, false).
		AddItem(a.panels.detail, 0, 1, false)

	setupKeys(a)

	a.tapp.SetRoot(layout, true).SetFocus(a.panels.resources)
}

// loadItems fetches items for provider i in a background goroutine and
// updates the items panel on the tview event loop via QueueUpdateDraw.
func (a *App) loadItems(i int) {
	a.panels.items.Clear()
	a.panels.detail.SetText("Loading...")

	go func() {
		items, err := a.providers[i].ListItems(context.Background())
		a.tapp.QueueUpdateDraw(func() {
			a.panels.items.Clear()
			a.panels.detail.Clear()

			if err != nil {
				a.panels.detail.SetText(fmt.Sprintf("[red]Error: %v[-]", err))
				return
			}

			for _, item := range items {
				item := item
				a.panels.items.AddItem(item.Name, "", 0, func() {
					a.loadDetail(i, item)
				})
			}
		})
	}()
}

// loadDetail fetches the detail for item in a background goroutine.
func (a *App) loadDetail(providerIdx int, item awspkg.Item) {
	a.panels.detail.SetText("Loading...")

	go func() {
		detail, err := a.providers[providerIdx].GetDetail(context.Background(), item)
		a.tapp.QueueUpdateDraw(func() {
			if err != nil {
				a.panels.detail.SetText(fmt.Sprintf("[red]Error: %v[-]", err))
				return
			}
			a.panels.detail.SetText(detail).ScrollToBeginning()
		})
	}()
}

// refresh reloads the currently active provider's item list.
func (a *App) refresh() {
	a.loadItems(a.activeProvider)
}

// Run starts the tview event loop.
func (a *App) Run() error {
	return a.tapp.Run()
}
```

- [ ] **Step 2: Build end-to-end**

```bash
go build -o lazyaws .
```

Expected: `lazyaws` binary produced with no errors.

- [ ] **Step 3: Run all tests**

```bash
go test ./... -v
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/app.go
git commit -m "feat: tview app — 3-panel layout, async AWS loading, refresh"
```

---

### Task 8: Smoke Tests & Final Commit

- [ ] **Step 1: Smoke test against real AWS**

```bash
./lazyaws
```

Verify:
- 3-panel layout renders
- Tab cycles focus between panels
- j/k and arrow keys navigate lists
- Selecting S3 loads bucket list asynchronously ("Loading..." appears briefly)
- Selecting a bucket shows JSON detail in right panel
- r refreshes the list
- q quits

- [ ] **Step 2: Smoke test against LocalStack**

```bash
# Start LocalStack (separate terminal or background):
docker run --rm -p 4566:4566 localstack/localstack

./lazyaws --local
```

Verify: app launches and returns empty lists (or pre-seeded LocalStack data) without errors.

- [ ] **Step 3: Final commit**

```bash
git add main.go
git commit -m "feat: lazyaws v1 — S3 and Lambda TUI with LocalStack support"
```

---

## Key Bindings

| Key | Action |
|-----|--------|
| Tab | Focus next panel |
| Shift+Tab | Focus previous panel |
| ↑ / k | Move up in focused list |
| ↓ / j | Move down in focused list |
| Enter | Select item |
| r | Refresh current resource list |
| q | Quit |

## Adding More Providers Later

To add a new AWS resource (e.g. SQS):

1. Create `internal/aws/sqs.go` — define `SQSAPI` interface, implement `SQSProvider`
2. Create `internal/aws/sqs_test.go` — write stub + table tests following the S3/Lambda pattern
3. Register in `main.go`: append `awspkg.NewSQSProvider(cfg, *local)` to the `providers` slice

No UI changes required — the panel system is provider-agnostic.
