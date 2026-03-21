# Multi-Service Expansion & Tabbed Detail Pane — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend lazyaws to support 10 AWS services with a lazydocker-inspired tabbed detail pane replacing the static JSON view.

**Architecture:** Extend the `Provider` interface with `Tabs() []TabDef`; tab fetch functions are closures capturing the provider's client. The UI adds tab state to `App` and renders a tab bar as the first line of pane 3. New format helpers in `internal/aws/format.go` produce aligned KV and table output used by all providers.

**Tech Stack:** Go 1.26, AWS SDK v2, tview v0.42, tcell v2

**Spec:** `docs/superpowers/specs/2026-03-11-multi-service-tabs-design.md`

---

## Chunk 1: Foundation — Types, Format Helpers, UI Tab State

### Task 1: Extend core types and add format helpers

**Files:**
- Modify: `internal/aws/provider.go`
- Create: `internal/aws/format.go`
- Create: `internal/aws/format_test.go`
- Modify: `internal/aws/s3.go` (add stub `Tabs()`)
- Modify: `internal/aws/lambda.go` (add stub `Tabs()`)

- [ ] **Step 1: Write failing tests for format helpers**

```go
// internal/aws/format_test.go
package aws_test

import (
	"strings"
	"testing"

	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

func TestKV(t *testing.T) {
	cases := []struct {
		name   string
		pairs  [][2]string
		expect []string
	}{
		{
			name:   "aligns values",
			pairs:  [][2]string{{"Region", "us-east-1"}, {"Versioning", "Enabled"}},
			expect: []string{"Region:", "us-east-1", "Versioning:", "Enabled"},
		},
		{
			name:   "empty",
			pairs:  nil,
			expect: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := awspkg.KV(tc.pairs)
			for _, want := range tc.expect {
				if !strings.Contains(out, want) {
					t.Errorf("KV output missing %q\ngot:\n%s", want, out)
				}
			}
		})
	}
}

func TestTable(t *testing.T) {
	headers := []string{"Name", "Size", "Modified"}
	rows := [][]string{
		{"images/hero.png", "2.3 MB", "2024-11-01"},
		{"data/export.csv", "892 KB", "2024-11-05"},
	}
	out := awspkg.Table(headers, rows)
	for _, want := range []string{"Name", "Size", "images/hero.png", "892 KB", "──"} {
		if !strings.Contains(out, want) {
			t.Errorf("Table output missing %q\ngot:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... 2>&1 | head -20
```
Expected: compile error — `KV` and `Table` undefined.

- [ ] **Step 3: Update provider.go with Meta, TabDef, updated Provider interface**

```go
// internal/aws/provider.go
package aws

import "context"

// Item represents a single resource returned by a Provider.
type Item struct {
	ID   string
	Name string
	Meta map[string]string // provider-specific context, e.g. {"type": "REST"}
}

// TabDef describes a single tab in the detail pane.
type TabDef struct {
	Label string
	Fetch func(ctx context.Context, item Item) (string, error)
}

// Provider lists and describes a category of AWS resources.
type Provider interface {
	// Name is the display label shown in the resource-type panel.
	Name() string
	// ListItems returns the top-level list of resources.
	ListItems(ctx context.Context) ([]Item, error)
	// GetDetail returns a formatted string for the detail panel (legacy, kept for compat).
	GetDetail(ctx context.Context, item Item) (string, error)
	// Tabs returns the tab definitions for the detail pane.
	Tabs() []TabDef
}
```

- [ ] **Step 4: Add stub Tabs() to s3.go and lambda.go to restore compilation**

In `internal/aws/s3.go`, add at the end:
```go
// Tabs returns S3 tab definitions (stub — real tabs added in a later task).
func (p *S3Provider) Tabs() []TabDef { return nil }
```

In `internal/aws/lambda.go`, add at the end:
```go
// Tabs returns Lambda tab definitions (stub — real tabs added in a later task).
func (p *LambdaProvider) Tabs() []TabDef { return nil }
```

- [ ] **Step 5: Create format.go**

```go
// internal/aws/format.go
package aws

import (
	"fmt"
	"strings"
)

// KV renders key-value pairs as left-aligned columns with keys right-padded.
// Keys are rendered with a trailing colon.
func KV(pairs [][2]string) string {
	maxKey := 0
	for _, p := range pairs {
		if len(p[0]) > maxKey {
			maxKey = len(p[0])
		}
	}
	var sb strings.Builder
	for _, p := range pairs {
		fmt.Fprintf(&sb, "  %-*s  %s\n", maxKey+1, p[0]+":", p[1])
	}
	return sb.String()
}

// Table renders a header row, a separator line, and data rows.
// Each column width is the max of its header and all its values.
func Table(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("  ")
	for i, h := range headers {
		fmt.Fprintf(&sb, "%-*s", widths[i]+2, h)
	}
	sb.WriteString("\n  ")
	for _, w := range widths {
		sb.WriteString(strings.Repeat("─", w) + "  ")
	}
	sb.WriteString("\n")
	for _, row := range rows {
		sb.WriteString("  ")
		for i, cell := range row {
			if i < len(widths) {
				fmt.Fprintf(&sb, "%-*s", widths[i]+2, cell)
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
```

- [ ] **Step 6: Run tests — format helpers pass, existing tests still pass**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -v 2>&1 | tail -20
```
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
cd /home/bryan/lazyaws && git add internal/aws/provider.go internal/aws/format.go internal/aws/format_test.go internal/aws/s3.go internal/aws/lambda.go
git commit -m "feat: extend Provider interface with TabDef/Tabs, add format helpers"
```

---

### Task 2: UI — tab state, rendering, and `[`/`]` keybindings

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/keys.go`
- Create: `internal/ui/app_test.go`

- [ ] **Step 1: Write failing tests for tab bar rendering and tab cycling**

```go
// internal/ui/app_test.go
package ui

import (
	"strings"
	"testing"

	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

func TestRenderTabBar(t *testing.T) {
	tabs := []awspkg.TabDef{
		{Label: "Overview"},
		{Label: "Objects"},
		{Label: "Policy"},
	}
	cases := []struct {
		active int
		expect string
	}{
		{0, "[Overview]  Objects  Policy"},
		{1, "Overview  [Objects]  Policy"},
		{2, "Overview  Objects  [Policy]"},
	}
	for _, tc := range cases {
		got := renderTabBar(tabs, tc.active)
		if !strings.Contains(got, tc.expect) {
			t.Errorf("active=%d: got %q, want to contain %q", tc.active, got, tc.expect)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd /home/bryan/lazyaws && go test ./internal/ui/... 2>&1 | head -10
```
Expected: compile error — `renderTabBar` undefined.

- [ ] **Step 3: Rewrite app.go with tab state and rendering**

```go
// internal/ui/app.go
package ui

import (
	"context"
	"fmt"
	"strings"

	awspkg "github.com/bkneis/lazyaws/internal/aws"
	"github.com/rivo/tview"
)

// App is the root TUI application.
type App struct {
	tapp           *tview.Application
	panels         *panels
	providers      []awspkg.Provider
	activeProvider int
	activeTab      int
	tabLoaded      []bool
	tabCache       []string
	currentItem    awspkg.Item
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
		a.panels.resources.AddItem(p.Name(), "", 0, func() {
			a.activeProvider = i
			a.loadItems(i)
		})
	}

	if len(a.providers) > 0 {
		a.activeProvider = 0
		a.loadItems(0)
	}

	layout := tview.NewFlex().
		AddItem(a.panels.resources, 20, 0, true).
		AddItem(a.panels.items, 30, 0, false).
		AddItem(a.panels.detail, 0, 1, false)

	setupKeys(a)

	a.tapp.SetRoot(layout, true).SetFocus(a.panels.resources)
}

// loadItems fetches items for provider i in a background goroutine.
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
				a.panels.items.AddItem(item.Name, "", 0, func() {
					a.selectItem(i, item)
				})
			}
		})
	}()
}

// selectItem resets tab state and loads the first tab for the selected item.
func (a *App) selectItem(providerIdx int, item awspkg.Item) {
	a.currentItem = item
	a.activeTab = 0
	tabs := a.providers[providerIdx].Tabs()
	a.tabLoaded = make([]bool, len(tabs))
	a.tabCache = make([]string, len(tabs))
	a.loadTab(providerIdx, 0, item)
}

// loadTab fetches a tab's content asynchronously.
func (a *App) loadTab(providerIdx, tabIdx int, item awspkg.Item) {
	tabs := a.providers[providerIdx].Tabs()
	if len(tabs) == 0 || tabIdx >= len(tabs) {
		return
	}
	a.renderDetail() // show "... fetching" immediately

	go func() {
		content, err := tabs[tabIdx].Fetch(context.Background(), item)
		a.tapp.QueueUpdateDraw(func() {
			if err != nil {
				a.tabCache[tabIdx] = fmt.Sprintf("[red]Error: %v[-]", err)
			} else {
				a.tabCache[tabIdx] = content
			}
			a.tabLoaded[tabIdx] = true
			if a.activeTab == tabIdx {
				a.renderDetail()
			}
		})
	}()
}

// renderDetail writes the tab bar + current tab content to pane 3.
func (a *App) renderDetail() {
	tabs := a.providers[a.activeProvider].Tabs()
	if len(tabs) == 0 {
		return
	}
	bar := renderTabBar(tabs, a.activeTab)
	content := "  ... fetching"
	if a.activeTab < len(a.tabLoaded) && a.tabLoaded[a.activeTab] {
		content = a.tabCache[a.activeTab]
	}
	a.panels.detail.SetText(bar + "\n\n" + content).ScrollToBeginning()
}

// renderTabBar builds the tab bar string with active tab in [brackets].
func renderTabBar(tabs []awspkg.TabDef, active int) string {
	parts := make([]string, len(tabs))
	for i, t := range tabs {
		if i == active {
			parts[i] = "[" + t.Label + "]"
		} else {
			parts[i] = t.Label
		}
	}
	return " " + strings.Join(parts, "  ")
}

// nextTab advances to the next tab, fetching if not yet loaded.
func (a *App) nextTab() {
	tabs := a.providers[a.activeProvider].Tabs()
	if len(tabs) == 0 {
		return
	}
	a.activeTab = (a.activeTab + 1) % len(tabs)
	if !a.tabLoaded[a.activeTab] {
		a.loadTab(a.activeProvider, a.activeTab, a.currentItem)
	} else {
		a.renderDetail()
	}
}

// prevTab retreats to the previous tab, fetching if not yet loaded.
func (a *App) prevTab() {
	tabs := a.providers[a.activeProvider].Tabs()
	if len(tabs) == 0 {
		return
	}
	n := len(tabs)
	a.activeTab = (a.activeTab + n - 1) % n
	if !a.tabLoaded[a.activeTab] {
		a.loadTab(a.activeProvider, a.activeTab, a.currentItem)
	} else {
		a.renderDetail()
	}
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

- [ ] **Step 4: Add `[` and `]` to keys.go**

```go
// internal/ui/keys.go
package ui

import "github.com/gdamore/tcell/v2"

// setupKeys attaches the global keyboard handler to the application.
//
// Bindings:
//
//	Tab         — focus next panel
//	Shift+Tab   — focus previous panel
//	j / ↓       — move down in focused list
//	k / ↑       — move up in focused list
//	[           — previous tab in detail pane
//	]           — next tab in detail pane
//	q           — quit
//	r           — refresh current resource list
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
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case 'k':
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		case ']':
			a.nextTab()
			return nil
		case '[':
			a.prevTab()
			return nil
		}

		return event
	})
}
```

- [ ] **Step 5: Run tests**

```bash
cd /home/bryan/lazyaws && go test ./internal/ui/... -v 2>&1
```
Expected: all PASS including `TestRenderTabBar`.

- [ ] **Step 6: Verify build**

```bash
cd /home/bryan/lazyaws && go build ./...
```
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
cd /home/bryan/lazyaws && git add internal/ui/app.go internal/ui/keys.go internal/ui/app_test.go
git commit -m "feat: add tab state, renderDetail, and [/] keybindings to UI"
```

---

## Chunk 2: Update Existing Providers — S3 and Lambda

### Task 3: S3 provider — real tabs

**Files:**
- Modify: `internal/aws/s3.go`
- Modify: `internal/aws/s3_test.go`

The S3API interface needs new methods. The `Tabs()` stub is replaced with real tabs.

- [ ] **Step 1: Write failing tab tests in s3_test.go**

Add to `internal/aws/s3_test.go` after existing tests:

```go
// Add new methods to stubS3:
func (s *stubS3) GetBucketVersioning(_ context.Context, in *s3.GetBucketVersioningInput, _ ...func(*s3.Options)) (*s3.GetBucketVersioningOutput, error) {
	return &s3.GetBucketVersioningOutput{
		Status: s3types.BucketVersioningStatusEnabled,
	}, nil
}

func (s *stubS3) GetPublicAccessBlock(_ context.Context, in *s3.GetPublicAccessBlockInput, _ ...func(*s3.Options)) (*s3.GetPublicAccessBlockOutput, error) {
	t := true
	return &s3.GetPublicAccessBlockOutput{
		PublicAccessBlockConfiguration: &s3types.PublicAccessBlockConfiguration{
			BlockPublicAcls: &t,
		},
	}, nil
}

func (s *stubS3) GetBucketEncryption(_ context.Context, in *s3.GetBucketEncryptionInput, _ ...func(*s3.Options)) (*s3.GetBucketEncryptionOutput, error) {
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

func (s *stubS3) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	sz := int64(1024)
	mod := time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC)
	return &s3.ListObjectsV2Output{
		Contents: []s3types.Object{
			{Key: aws.String("images/hero.png"), Size: &sz, LastModified: &mod},
		},
		KeyCount: aws.Int32(1),
	}, nil
}

func (s *stubS3) GetBucketPolicy(_ context.Context, in *s3.GetBucketPolicyInput, _ ...func(*s3.Options)) (*s3.GetBucketPolicyOutput, error) {
	return &s3.GetBucketPolicyOutput{
		Policy: aws.String(`{"Version":"2012-10-17","Statement":[]}`),
	}, nil
}

func TestS3Provider_Tabs(t *testing.T) {
	created := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	stub := &stubS3{
		buckets:  []s3types.Bucket{{Name: aws.String("my-bucket"), CreationDate: &created}},
		location: "us-east-1",
	}
	p := awspkg.NewS3ProviderWithClient(stub)
	tabs := p.Tabs()

	if len(tabs) != 3 {
		t.Fatalf("got %d tabs, want 3", len(tabs))
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

// TestS3Provider_TabOverview_EmptyRegion verifies that an empty LocationConstraint
// (which AWS returns for buckets in us-east-1) is displayed as "us-east-1".
func TestS3Provider_TabOverview_EmptyRegion(t *testing.T) {
	created := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	stub := &stubS3{
		buckets:  []s3types.Bucket{{Name: aws.String("my-bucket"), CreationDate: &created}},
		location: "", // empty = us-east-1
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
```

- [ ] **Step 2: Run to verify failure**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... 2>&1 | head -20
```
Expected: compile error — new methods not in S3API interface.

- [ ] **Step 3: Update s3.go with new interface methods and real Tabs()**

Replace `internal/aws/s3.go`:

```go
package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
}

// S3Provider implements Provider for Amazon S3.
type S3Provider struct {
	client S3API
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
	detail := map[string]any{"Name": item.ID, "Location": string(loc.LocationConstraint)}
	b, err := json.MarshalIndent(detail, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal detail: %w", err)
	}
	return string(b), nil
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

	rows := make([][]string, len(out.Contents))
	for i, obj := range out.Contents {
		size := formatSize(awssdk.ToInt64(obj.Size))
		mod := ""
		if obj.LastModified != nil {
			mod = obj.LastModified.Format(time.DateOnly)
		}
		rows[i] = []string{awssdk.ToString(obj.Key), size, mod}
	}

	result := Table([]string{"Key", "Size", "Last Modified"}, rows)
	total := awssdk.ToInt32(out.KeyCount)
	shown := int32(len(out.Contents))
	if shown < total {
		result += fmt.Sprintf("\n  (showing %d of %d objects)\n", shown, total)
	}
	return result, nil
}

func (p *S3Provider) tabPolicy(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: awssdk.String(item.ID)})
	if err != nil {
		// NoSuchBucketPolicy is expected for buckets without a policy
		if strings.Contains(err.Error(), "NoSuchBucketPolicy") {
			return "  (no bucket policy)\n", nil
		}
		return "", err
	}
	// Pretty-print the JSON
	var raw any
	if err := json.Unmarshal([]byte(awssdk.ToString(out.Policy)), &raw); err != nil {
		return awssdk.ToString(out.Policy), nil
	}
	b, _ := json.MarshalIndent(raw, "  ", "  ")
	return "  " + string(b) + "\n", nil
}

func formatSize(bytes int64) string {
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
```

- [ ] **Step 4: Run tests**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -run TestS3 -v
```
Expected: all S3 tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/bryan/lazyaws && git add internal/aws/s3.go internal/aws/s3_test.go
git commit -m "feat: S3 provider — Overview, Objects, Policy tabs"
```

---

### Task 4: Lambda provider — real tabs

**Files:**
- Modify: `internal/aws/lambda.go`
- Modify: `internal/aws/lambda_test.go`

- [ ] **Step 1: Write failing tab tests in lambda_test.go**

Read existing `internal/aws/lambda_test.go` first, then add to the stub and add tab tests:

```go
// Add to stubLambda (the existing test stub):
func (s *stubLambda) GetFunctionConfiguration(_ context.Context, in *lambda.GetFunctionConfigurationInput, _ ...func(*lambda.Options)) (*lambda.GetFunctionConfigurationOutput, error) {
	for _, f := range s.functions {
		if aws.ToString(f.FunctionName) == aws.ToString(in.FunctionName) {
			return &lambda.GetFunctionConfigurationOutput{
				FunctionName: f.FunctionName,
				Runtime:      f.Runtime,
				MemorySize:   f.MemorySize,
				Timeout:      f.Timeout,
				Handler:      f.Handler,
				Role:         f.Role,
				Environment: &lambdatypes.EnvironmentResponse{
					Variables: map[string]string{
						"DB_HOST":   "localhost",
						"LOG_LEVEL": "INFO",
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("function not found")
}

func (s *stubLambda) ListEventSourceMappings(_ context.Context, in *lambda.ListEventSourceMappingsInput, _ ...func(*lambda.Options)) (*lambda.ListEventSourceMappingsOutput, error) {
	arn := "arn:aws:sqs:us-east-1:123:my-queue"
	state := "Enabled"
	return &lambda.ListEventSourceMappingsOutput{
		EventSourceMappings: []lambdatypes.EventSourceMappingConfiguration{
			{EventSourceArn: &arn, State: &state},
		},
	}, nil
}

func TestLambdaProvider_Tabs(t *testing.T) {
	// ... (build stub with one function, call Tabs(), test each tab)
	// Pattern identical to TestS3Provider_Tabs
	// Tab 0 "Overview": contains runtime string
	// Tab 1 "Env": contains "DB_HOST"
	// Tab 2 "Triggers": contains "my-queue"
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -run TestLambda 2>&1 | head -20
```

- [ ] **Step 3: Update lambda.go with new interface methods and real Tabs()**

Extend `LambdaAPI` interface to add `GetFunctionConfiguration` and `ListEventSourceMappings`. Add `tabOverview`, `tabEnv`, `tabTriggers` methods. Replace stub `Tabs()` with real implementation.

Key logic notes:
- `tabOverview`: call `GetFunction`, extract `Configuration` fields, render with `KV`
- `tabEnv`: call `GetFunctionConfiguration`, iterate `Environment.Variables`, render with `KV`; show `(no environment variables)` if empty
- `tabTriggers`: call `ListEventSourceMappings(FunctionName: item.ID)`, derive source type from ARN prefix (`arn:aws:sqs` → "SQS", `arn:aws:events` → "EventBridge", etc.), render with `Table`

- [ ] **Step 4: Run tests**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -run TestLambda -v
```

- [ ] **Step 5: Commit**

```bash
cd /home/bryan/lazyaws && git add internal/aws/lambda.go internal/aws/lambda_test.go
git commit -m "feat: Lambda provider — Overview, Env, Triggers tabs"
```

---

## Chunk 3: New Providers — SNS, SQS, CloudFormation, IAM

All new providers follow the same pattern:
1. Define `XxxAPI` interface with the methods needed
2. Implement `XxxProvider` struct with `NewXxxProvider`, `NewXxxProviderWithClient`, `Name`, `ListItems`, `GetDetail` (minimal), `Tabs`
3. Write stub + table tests for `ListItems` and each tab's `Fetch`

### Task 5: SNS provider

**Files:**
- Create: `internal/aws/sns.go`
- Create: `internal/aws/sns_test.go`

SNS API methods needed:
- `ListTopics` → `ListItems` (name = last segment of ARN)
- `GetTopicAttributes` → tab Overview (SubscriptionsConfirmed, SubscriptionsPending, SubscriptionsDeleted, FifoTopic, KmsMasterKeyId)
- `ListSubscriptionsByTopic` → tab Subscriptions

- [ ] **Step 1: Write failing tests**

```go
// internal/aws/sns_test.go
package aws_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	awspkg "github.com/bkneis/lazyaws/internal/aws"
)

type stubSNS struct{}

func (s *stubSNS) ListTopics(_ context.Context, _ *sns.ListTopicsInput, _ ...func(*sns.Options)) (*sns.ListTopicsOutput, error) {
	return &sns.ListTopicsOutput{
		Topics: []snstypes.Topic{
			{TopicArn: aws.String("arn:aws:sns:us-east-1:123:order-events")},
		},
	}, nil
}

func (s *stubSNS) GetTopicAttributes(_ context.Context, _ *sns.GetTopicAttributesInput, _ ...func(*sns.Options)) (*sns.GetTopicAttributesOutput, error) {
	return &sns.GetTopicAttributesOutput{
		Attributes: map[string]string{
			"TopicArn":                     "arn:aws:sns:us-east-1:123:order-events",
			"SubscriptionsConfirmed":        "3",
			"SubscriptionsPending":          "1",
			"SubscriptionsDeleted":          "0",
			"FifoTopic":                     "false",
		},
	}, nil
}

func (s *stubSNS) ListSubscriptionsByTopic(_ context.Context, _ *sns.ListSubscriptionsByTopicInput, _ ...func(*sns.Options)) (*sns.ListSubscriptionsByTopicOutput, error) {
	return &sns.ListSubscriptionsByTopicOutput{
		Subscriptions: []snstypes.Subscription{
			{
				Protocol:        aws.String("sqs"),
				Endpoint:        aws.String("arn:aws:sqs:us-east-1:123:order-queue"),
				SubscriptionArn: aws.String("arn:aws:sns:us-east-1:123:order-events:abc"),
			},
		},
	}, nil
}

func TestSNSProvider_ListItems(t *testing.T) {
	p := awspkg.NewSNSProviderWithClient(&stubSNS{})
	items, err := p.ListItems(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Name != "order-events" {
		t.Errorf("got name %q, want order-events", items[0].Name)
	}
}

func TestSNSProvider_Tabs(t *testing.T) {
	p := awspkg.NewSNSProviderWithClient(&stubSNS{})
	tabs := p.Tabs()
	if len(tabs) != 2 {
		t.Fatalf("got %d tabs, want 2", len(tabs))
	}
	item := awspkg.Item{ID: "arn:aws:sns:us-east-1:123:order-events", Name: "order-events"}

	cases := []struct {
		idx   int
		label string
		want  string
	}{
		{0, "Overview", "order-events"},
		{1, "Subscriptions", "order-queue"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if tabs[tc.idx].Label != tc.label {
				t.Errorf("tab %d label = %q, want %q", tc.idx, tabs[tc.idx].Label, tc.label)
			}
			content, err := tabs[tc.idx].Fetch(context.Background(), item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(content, tc.want) {
				t.Errorf("tab %d missing %q\ngot:\n%s", tc.idx, tc.want, content)
			}
		})
	}
}

// TestSNSProvider_SubscriptionStatus verifies the status derivation from SubscriptionArn.
func TestSNSProvider_TabSubscriptions_Status(t *testing.T) {
	// stubSNSStatus returns subscriptions with the three SubscriptionArn sentinel values.
	// Confirmed = full ARN, PendingConfirmation = literal "PendingConfirmation", Deleted = "Deleted".
	// The test verifies each maps to the correct Status column value.
	// Implement stubSNSStatus inline in the test file with ListSubscriptionsByTopic returning
	// three subscriptions with the above SubscriptionArn values.
	cases := []struct {
		subArn string
		want   string
	}{
		{"arn:aws:sns:us-east-1:123:order-events:abc123", "Confirmed"},
		{"PendingConfirmation", "Pending"},
		{"Deleted", "Deleted"},
	}
	// For each case, build a stub returning one subscription, call tabSubscriptions, check output.
	_ = cases // implementer fills this in
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -run TestSNS 2>&1 | head -10
```

- [ ] **Step 3: Implement sns.go**

```go
// internal/aws/sns.go
package aws

import (
	"context"
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
)

type SNSAPI interface {
	ListTopics(ctx context.Context, in *sns.ListTopicsInput, opts ...func(*sns.Options)) (*sns.ListTopicsOutput, error)
	GetTopicAttributes(ctx context.Context, in *sns.GetTopicAttributesInput, opts ...func(*sns.Options)) (*sns.GetTopicAttributesOutput, error)
	ListSubscriptionsByTopic(ctx context.Context, in *sns.ListSubscriptionsByTopicInput, opts ...func(*sns.Options)) (*sns.ListSubscriptionsByTopicOutput, error)
}

type SNSProvider struct{ client SNSAPI }

func NewSNSProvider(cfg awssdk.Config, local bool) *SNSProvider {
	var opts []func(*sns.Options)
	if local {
		opts = append(opts, func(o *sns.Options) { o.BaseEndpoint = awssdk.String("http://localhost:4566") })
	}
	return &SNSProvider{client: sns.NewFromConfig(cfg, opts...)}
}

func NewSNSProviderWithClient(client SNSAPI) *SNSProvider { return &SNSProvider{client: client} }

func (p *SNSProvider) Name() string { return "SNS" }

func (p *SNSProvider) ListItems(ctx context.Context) ([]Item, error) {
	out, err := p.client.ListTopics(ctx, &sns.ListTopicsInput{})
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	items := make([]Item, len(out.Topics))
	for i, t := range out.Topics {
		arn := awssdk.ToString(t.TopicArn)
		name := arn
		if parts := strings.Split(arn, ":"); len(parts) > 0 {
			name = parts[len(parts)-1]
		}
		items[i] = Item{ID: arn, Name: name}
	}
	return items, nil
}

func (p *SNSProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	content, err := p.tabOverview(ctx, item)
	return content, err
}

func (p *SNSProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Subscriptions", Fetch: p.tabSubscriptions},
	}
}

func (p *SNSProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.GetTopicAttributes(ctx, &sns.GetTopicAttributesInput{TopicArn: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	attrs := out.Attributes
	topicType := "Standard"
	if attrs["FifoTopic"] == "true" {
		topicType = "FIFO"
	}
	kms := attrs["KmsMasterKeyId"]
	if kms == "" {
		kms = "(none)"
	}
	return KV([][2]string{
		{"ARN", item.ID},
		{"Type", topicType},
		{"Confirmed", attrs["SubscriptionsConfirmed"]},
		{"Pending", attrs["SubscriptionsPending"]},
		{"Deleted", attrs["SubscriptionsDeleted"]},
		{"KMS Key", kms},
	}), nil
}

func (p *SNSProvider) tabSubscriptions(ctx context.Context, item Item) (string, error) {
	out, err := p.client.ListSubscriptionsByTopic(ctx, &sns.ListSubscriptionsByTopicInput{TopicArn: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	if len(out.Subscriptions) == 0 {
		return "  (no subscriptions)\n", nil
	}
	rows := make([][]string, len(out.Subscriptions))
	for i, s := range out.Subscriptions {
		status := subscriptionStatus(s)
		rows[i] = []string{awssdk.ToString(s.Protocol), awssdk.ToString(s.Endpoint), status}
	}
	return Table([]string{"Protocol", "Endpoint", "Status"}, rows), nil
}

func subscriptionStatus(s snstypes.Subscription) string {
	arn := awssdk.ToString(s.SubscriptionArn)
	switch arn {
	case "PendingConfirmation":
		return "Pending"
	case "Deleted":
		return "Deleted"
	default:
		if strings.HasPrefix(arn, "arn:") {
			return "Confirmed"
		}
		return arn
	}
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -run TestSNS -v
```

- [ ] **Step 5: Commit**

```bash
cd /home/bryan/lazyaws && git add internal/aws/sns.go internal/aws/sns_test.go
git commit -m "feat: SNS provider — Overview, Subscriptions tabs"
```

---

### Task 6: SQS provider

**Files:**
- Create: `internal/aws/sqs.go`
- Create: `internal/aws/sqs_test.go`

SQS API methods needed:
- `ListQueues` → `ListItems` (name = last segment of URL)
- `GetQueueAttributes` → all three tabs (QueueArn, ApproximateNumber*, VisibilityTimeout, MessageRetentionPeriod, MaximumMessageSize, DelaySeconds, ReceiveMessageWaitTimeSeconds, SqsManagedSseEnabled, RedrivePolicy)

Note: All three tabs call `GetQueueAttributes` with `AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameAll}`. Cache can be avoided since it's fast — each tab re-fetches.

- [ ] **Step 1: Write failing tests** (pattern identical to SNS — stub with `ListQueues` and `GetQueueAttributes`)

Test tab 0 "Overview" contains message counts, tab 1 "Config" contains "VisibilityTimeout" label, tab 2 "DLQ" contains either DLQ ARN or "(no dead-letter queue)".

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement sqs.go**

Key logic:
- `ListItems`: `ListQueues`, extract queue name from URL last segment
- `tabOverview`: `GetQueueAttributes(All)`, show type (FIFO if `.fifo` suffix), approximate message counts, ARN
- `tabConfig`: same attributes call, show VisibilityTimeout, MessageRetentionPeriod (convert seconds to human-readable), MaximumMessageSize (bytes to KB), DelaySeconds, ReceiveMessageWaitTimeSeconds, encryption
- `tabDLQ`: parse `RedrivePolicy` JSON attribute (contains `deadLetterTargetArn` and `maxReceiveCount`); if missing → show "(no dead-letter queue configured)"; then call `GetQueueAttributes` on DLQ URL for message count

- [ ] **Step 4: Run tests**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -run TestSQS -v
```

- [ ] **Step 5: Commit**

```bash
cd /home/bryan/lazyaws && git add internal/aws/sqs.go internal/aws/sqs_test.go
git commit -m "feat: SQS provider — Overview, Config, DLQ tabs"
```

---

### Task 7: CloudFormation provider

**Files:**
- Create: `internal/aws/cloudformation.go`
- Create: `internal/aws/cloudformation_test.go`

CF API methods needed:
- `DescribeStacks` → `ListItems` (stack name) and tabs Overview, Outputs, Parameters
- `ListStackResources` → tab Resources

- [ ] **Step 1: Write failing tests**

Test `ListItems` returns stack names. Test each of 4 tabs contains expected strings.

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement cloudformation.go**

```go
// SNCAPI interface, CFProvider struct, Name="CloudFormation"
// ListItems: DescribeStacks (nil StackName = all stacks), Item{ID: stackName, Name: stackName}
// tabOverview: DescribeStacks(StackName), KV with Name/Status/Created/LastUpdated/Description
// tabResources: ListStackResources, Table with LogicalResourceId/ResourceType/ResourceStatus
// tabOutputs: DescribeStacks Outputs field, KV pairs; "(no outputs)" if empty
// tabParameters: DescribeStacks Parameters field, KV pairs; "(no parameters)" if empty
```

- [ ] **Step 4: Run tests**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -run TestCloudFormation -v
```

- [ ] **Step 5: Commit**

```bash
cd /home/bryan/lazyaws && git add internal/aws/cloudformation.go internal/aws/cloudformation_test.go
git commit -m "feat: CloudFormation provider — Overview, Resources, Outputs, Parameters tabs"
```

---

### Task 8: IAM Roles provider

**Files:**
- Create: `internal/aws/iam.go`
- Create: `internal/aws/iam_test.go`

IAM API methods needed:
- `ListRoles` → `ListItems`
- `GetRole` → tabs Overview and Trust
- `ListAttachedRolePolicies` + `ListRolePolicies` → tab Policies

- [ ] **Step 1: Write failing tests**

Test `ListItems` returns role names. Test Overview contains ARN. Test Policies tab contains both managed and inline policy names. Test Trust tab contains "lambda.amazonaws.com".

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement iam.go**

```go
// Name="IAM Roles"
// tabOverview: GetRole, KV with Name/ARN/Created/MaxSession/Description
// tabPolicies: ListAttachedRolePolicies (type=Managed) + ListRolePolicies (type=Inline, names only)
//   Table with Type/Name columns
// tabTrust: GetRole, parse AssumeRolePolicyDocument JSON
//   Extract first Statement's Principal service and Condition
//   KV with Principal/Action/Condition
//   If Principal is a map with "Service" key, show the service value
```

MaxSessionDuration is in seconds; convert to human (e.g. 3600 → "1h").

- [ ] **Step 4: Run tests**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -run TestIAM -v
```

- [ ] **Step 5: Commit**

```bash
cd /home/bryan/lazyaws && git add internal/aws/iam.go internal/aws/iam_test.go
git commit -m "feat: IAM Roles provider — Overview, Policies, Trust tabs"
```

---

## Chunk 4: New Providers — SecretsManager, APIGateway, Route53, ACM

### Task 9: Secrets Manager provider

**Files:**
- Create: `internal/aws/secretsmanager.go`
- Create: `internal/aws/secretsmanager_test.go`

SM API methods needed:
- `ListSecrets` → `ListItems`
- `DescribeSecret` → tab Overview
- `GetSecretValue` → tab Value
- `ListSecretVersionIds` → tab Versions

- [ ] **Step 1: Write failing tests**

Include a test specifically for the masking logic: key "db_password" should have value masked, key "db_host" should not.

```go
func TestSecretsManagerProvider_TabValue_Masking(t *testing.T) {
	// stub GetSecretValue returns JSON: {"db_host":"localhost","db_password":"s3cr3t"}
	// expect content to contain "db_host" and "localhost"
	// expect content to contain "db_password" and "••••"
	// expect content NOT to contain "s3cr3t"
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement secretsmanager.go**

Key masking logic:
```go
var sensitiveKeys = []string{"password", "secret", "token", "key"}

func isSensitiveKey(k string) bool {
	lower := strings.ToLower(k)
	for _, s := range sensitiveKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
```

`tabValue`:
1. Call `GetSecretValue`
2. Try `json.Unmarshal(SecretString)` into `map[string]any`
3. If successful: iterate sorted keys, mask value if `isSensitiveKey(key)`, nested map → render as `[object]`
4. If not JSON: render raw `SecretString`

- [ ] **Step 4: Run tests including masking test**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -run TestSecretsManager -v
```

- [ ] **Step 5: Commit**

```bash
cd /home/bryan/lazyaws && git add internal/aws/secretsmanager.go internal/aws/secretsmanager_test.go
git commit -m "feat: SecretsManager provider — Overview, Value (with masking), Versions tabs"
```

---

### Task 10: API Gateway provider

**Files:**
- Create: `internal/aws/apigateway.go`
- Create: `internal/aws/apigateway_test.go`

This provider uses **two** AWS SDK packages: `apigatewayv2` (HTTP/WebSocket) and `apigateway` (REST). The `Meta["type"]` field (`"HTTP"`, `"WEBSOCKET"`, `"REST"`) drives branching in all tab fetch functions.

API methods needed:
- `apigatewayv2.GetApis` + `apigateway.GetRestApis` → `ListItems`
- `apigatewayv2.GetApi` / `apigateway.GetRestApi` → tab Overview
- `apigatewayv2.GetRoutes` / `apigateway.GetResources` → tab Routes
- `apigatewayv2.GetStages` / `apigateway.GetStages` (v1) → tab Stages

Define two interfaces: `APIGatewayV2API` and `APIGatewayV1API`. Provider holds both clients.

```go
type APIGatewayProvider struct {
	v2 APIGatewayV2API
	v1 APIGatewayV1API
}
```

- [ ] **Step 1: Write failing tests**

Write stubs for both v1 and v2 interfaces. Test `ListItems` merges both lists with correct `Meta["type"]`. Test Overview tab branches on type. Test Routes tab shows routes for HTTP and resources for REST.

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement apigateway.go**

`ListItems`: call both, merge, set Meta["type"]. Use combined name as display: `{name} ({type})`.

`tabOverview`:
```go
if item.Meta["type"] != "REST" {
    // apigatewayv2.GetApi
} else {
    // apigateway.GetRestApi
}
```

`tabRoutes` (HTTP/WS): `GetRoutes`, rows = RouteKey + Target (lambda function name extracted from integration ARN if available).
`tabRoutes` (REST): `GetResources`, flatten resource paths × methods into rows.

`tabStages`: branch similarly.

- [ ] **Step 4: Run tests**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -run TestAPIGateway -v
```

- [ ] **Step 5: Commit**

```bash
cd /home/bryan/lazyaws && git add internal/aws/apigateway.go internal/aws/apigateway_test.go
git commit -m "feat: API Gateway provider — Overview, Routes, Stages tabs (HTTP/WebSocket/REST)"
```

---

### Task 11: Route53 provider

**Files:**
- Create: `internal/aws/route53.go`
- Create: `internal/aws/route53_test.go`

Route53 API methods needed:
- `ListHostedZones` → `ListItems`; Item.ID = zone ID (strip `/hostedzone/` prefix), Meta["name"] = zone name for subsequent calls
- `GetHostedZone` → tab Overview
- `ListResourceRecordSets` → tab Records

- [ ] **Step 1: Write failing tests**

Test `ListItems` returns zones, IDs stripped of `/hostedzone/`. Test Records tab shows record name, type, TTL, value. For ALIAS records (no TTL), TTL column shows "-".

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement route53.go**

`tabRecords` logic:
- For each `ResourceRecordSet`, TTL = "-" if nil (ALIAS), else formatted as seconds
- Value = first ResourceRecord.Value, or ALIAS target if AliasTarget is set
- Truncate long values to 40 chars with "..."

```go
func formatTTL(ttl *int64) string {
	if ttl == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *ttl)
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -run TestRoute53 -v
```

- [ ] **Step 5: Commit**

```bash
cd /home/bryan/lazyaws && git add internal/aws/route53.go internal/aws/route53_test.go
git commit -m "feat: Route53 provider — Overview, Records tabs"
```

---

### Task 12: ACM provider

**Files:**
- Create: `internal/aws/acm.go`
- Create: `internal/aws/acm_test.go`

ACM API methods needed:
- `ListCertificates` → `ListItems`; Item.ID = ARN, Name = primary domain
- `DescribeCertificate` → all three tabs (Overview, Domains, Validation)

- [ ] **Step 1: Write failing tests**

Test Overview contains domain name, status, expiry with days remaining. Test Domains tab lists all SANs with "(primary)" next to first. Test Validation tab shows DNS CNAME record info.

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement acm.go**

`tabOverview` key logic:
- Expiry: format as `2025-11-03  (357 days)` using `time.Until`
- InUseBy: list resource ARNs, extract service name from ARN (e.g. `arn:aws:cloudfront` → "CloudFront")

`tabValidation`:
- Show Method first
- For each DomainValidationOption with ResourceRecord, show domain / record name / record value (truncate to 30 chars)
- If validation is EMAIL-based, show email validation domains instead

- [ ] **Step 4: Run tests**

```bash
cd /home/bryan/lazyaws && go test ./internal/aws/... -run TestACM -v
```

- [ ] **Step 5: Commit**

```bash
cd /home/bryan/lazyaws && git add internal/aws/acm.go internal/aws/acm_test.go
git commit -m "feat: ACM provider — Overview, Domains, Validation tabs"
```

---

## Chunk 5: Wiring

### Task 13: Register all providers in main.go and add SDK dependencies

**Files:**
- Modify: `main.go`
- Run: `go get` for new dependencies
- Run: `go mod tidy`

- [ ] **Step 1: Add new SDK dependencies**

```bash
cd /home/bryan/lazyaws && go get \
  github.com/aws/aws-sdk-go-v2/service/sns \
  github.com/aws/aws-sdk-go-v2/service/sqs \
  github.com/aws/aws-sdk-go-v2/service/cloudformation \
  github.com/aws/aws-sdk-go-v2/service/iam \
  github.com/aws/aws-sdk-go-v2/service/secretsmanager \
  github.com/aws/aws-sdk-go-v2/service/apigatewayv2 \
  github.com/aws/aws-sdk-go-v2/service/apigateway \
  github.com/aws/aws-sdk-go-v2/service/route53 \
  github.com/aws/aws-sdk-go-v2/service/acm
```

- [ ] **Step 2: Update main.go**

```go
package main

import (
	"context"
	"flag"
	"log"

	awspkg "github.com/bkneis/lazyaws/internal/aws"
	"github.com/bkneis/lazyaws/internal/ui"
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
		awspkg.NewSNSProvider(cfg, *local),
		awspkg.NewSQSProvider(cfg, *local),
		awspkg.NewCloudFormationProvider(cfg, *local),
		awspkg.NewIAMProvider(cfg, *local),
		awspkg.NewSecretsManagerProvider(cfg, *local),
		awspkg.NewAPIGatewayProvider(cfg, *local),
		awspkg.NewRoute53Provider(cfg, *local),
		awspkg.NewACMProvider(cfg, *local),
	}

	app := ui.NewApp(providers)
	if err := app.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
```

- [ ] **Step 3: go mod tidy**

```bash
cd /home/bryan/lazyaws && go mod tidy
```

- [ ] **Step 4: Full build + test**

```bash
cd /home/bryan/lazyaws && go build ./... && go test ./...
```
Expected: all PASS, binary builds cleanly.

- [ ] **Step 5: Commit**

```bash
cd /home/bryan/lazyaws && git add main.go go.mod go.sum
git commit -m "feat: wire all 10 providers in main.go, add SDK dependencies"
```

---

## Notes for Implementers

### Patterns to follow
- All providers in `internal/aws/` use `package aws` (not `package aws_test`)
- All test files use `package aws_test` (external test package)
- Stubs implement only the methods in the `XxxAPI` interface — nothing else
- Use `awssdk.ToString`, `awssdk.ToInt32`, `awssdk.ToBool`, `awssdk.ToInt64` for pointer dereference
- Human-readable durations: `formatDuration(seconds int64) string` — add to `format.go`
- `formatSize` is defined in `s3.go` and can be moved to `format.go` if reused

### LocalStack notes
- IAM and Route53 are global services; LocalStack may require region override — use same `--local` pattern
- ACM in LocalStack may not support all certificate operations; errors should surface as tab error state

### Error display
- Tab fetch errors: display as `[red]Error: <message>[-]` in tab content (same pattern as loadItems)
- Do not return errors silently or swallow them

### Testing the masking function
The `isSensitiveKey` function should be exported as `IsSensitiveKey` and tested directly in `format_test.go` to avoid depending on the full secretsmanager provider in that test.
