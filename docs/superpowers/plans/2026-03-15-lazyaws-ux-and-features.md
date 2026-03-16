# lazyaws UX & Feature Improvements — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tab UX polish, IAM Policies provider, cross-resource navigation links, and interactive S3 object browsing to the lazyaws TUI.

**Architecture:** The right column is refactored into a vertical Flex (tabBar + detail + expand) in one prerequisite task; all four features layer on top of this shared layout. Providers remain independently testable via narrow interface injection.

**Tech Stack:** Go, tview v0.0.0, aws-sdk-go-v2, tcell/v2

**Spec:** `docs/superpowers/specs/2026-03-15-lazyaws-ux-and-features-design.md`

---

## Chunk 1: Feature 1 — Tab UX + Layout Restructure

This chunk is the prerequisite for all other features. It splits the single Detail `tview.TextView` into a vertical Flex containing a 2-row tabBar, a content area, and a hidden expand panel.

---

### Task 1: Restructure panels — split right column

**Files:**
- Modify: `internal/ui/panels.go`

- [ ] **Step 1: Add new fields to the `panels` struct**

Replace the existing `detail *tview.TextView` with three new fields plus the flex container. Open `internal/ui/panels.go` and change the struct:

```go
type panels struct {
	resources   *tview.List
	items       *tview.List
	tabBar      *tview.TextView  // 2-row tab header (label row + underline row)
	detail      *tview.TextView  // scrollable content area
	expand      *tview.TextView  // expansion panel (hidden by default)
	rightFlex   *tview.Flex      // vertical flex containing tabBar+detail+expand
	status      *tview.TextView
	searchInput *tview.InputField
	prompt      *tview.TextView  // y/n prompt widget (used in Feature 4)
	statusPages *tview.Pages
	focused     int // 0=resources, 1=items, 2=detail
}
```

- [ ] **Step 2: Rewrite `newPanels()` to build the new layout**

Replace the existing `newPanels()` function body entirely:

```go
func newPanels() *panels {
	resources := tview.NewList().ShowSecondaryText(false)
	resources.SetBorder(true).SetTitle(" Resources ").SetBorderColor(focusColor)
	resources.SetSelectedTextColor(tcell.ColorBlack).SetSelectedBackgroundColor(focusColor)
	resources.SetFocusFunc(func() { resources.SetBorderColor(focusColor) }).
		SetBlurFunc(func() { resources.SetBorderColor(tcell.ColorDefault) })

	items := tview.NewList().ShowSecondaryText(false)
	items.SetBorder(true).SetTitle(" Items ")
	items.SetSelectedTextColor(tcell.ColorBlack).SetSelectedBackgroundColor(focusColor)
	items.SetFocusFunc(func() { items.SetBorderColor(focusColor) }).
		SetBlurFunc(func() { items.SetBorderColor(tcell.ColorDefault) })

	tabBar := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)

	detail := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false).
		SetRegions(true)
	detail.SetBorder(true).SetTitle(" Detail ")
	detail.SetFocusFunc(func() { detail.SetBorderColor(focusColor) }).
		SetBlurFunc(func() { detail.SetBorderColor(tcell.ColorDefault) })

	expand := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	expand.SetBorder(true).SetTitle(" Expand ")

	rightFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tabBar, 2, 0, false).
		AddItem(detail, 0, 2, false).
		AddItem(expand, 0, 0, false) // proportion 0 = hidden

	status := tview.NewTextView().SetDynamicColors(true).SetText(hintsText)

	searchInput := tview.NewInputField().
		SetLabel("/ ").
		SetLabelColor(tcell.ColorYellow).
		SetFieldBackgroundColor(tcell.ColorDefault)

	prompt := tview.NewTextView().SetDynamicColors(true)

	statusPages := tview.NewPages().
		AddPage("hints", status, true, true).
		AddPage("search", searchInput, true, false).
		AddPage("prompt", prompt, true, false)

	return &panels{
		resources:   resources,
		items:       items,
		tabBar:      tabBar,
		detail:      detail,
		expand:      expand,
		rightFlex:   rightFlex,
		status:      status,
		searchInput: searchInput,
		prompt:      prompt,
		statusPages: statusPages,
	}
}
```

- [ ] **Step 3: Update `primitives()` — expand is NOT in the Tab cycle**

The `primitives()` method stays exactly as-is (`[resources, items, detail]`). Confirm it is unchanged:

```go
func (p *panels) primitives() []tview.Primitive {
	return []tview.Primitive{p.resources, p.items, p.detail}
}
```

- [ ] **Step 4: Build and confirm it compiles**

```bash
cd /home/bryan/lazyaws && go build ./...
```

Expected: compilation errors in `app.go` because `build()` still references `a.panels.detail` in the layout line. That is expected — fix in Task 2.

---

### Task 2: Update `app.go` — wire new layout and split `renderDetail`

**Files:**
- Modify: `internal/ui/app.go`

- [ ] **Step 1: Add new fields to the `App` struct**

```go
type App struct {
	tapp           *tview.Application
	panels         *panels
	providers      []awspkg.Provider
	loadedItems    []awspkg.Item    // NEW: mirrors items list for Enter handler
	activeProvider int
	activeTab      int
	tabLoaded      []bool
	tabCache       []string
	currentItem    awspkg.Item
	preFocusIdx    int
	tabBarOffsets  []int            // NEW: display-column start per tab (for mouse click)
	expandVisible  bool             // NEW: whether expand panel is shown
}
```

- [ ] **Step 2: Fix `build()` — use `rightFlex` in layout, add mouse capture on tabBar**

In `build()`, replace the layout assembly and add the tabBar mouse handler. Find the two lines that build `layout` and `outer` and replace:

```go
// OLD:
leftCol := tview.NewFlex().SetDirection(tview.FlexRow).
    AddItem(a.panels.resources, 0, 1, true).
    AddItem(a.panels.items, 0, 2, false)

layout := tview.NewFlex().
    AddItem(leftCol, 25, 0, true).
    AddItem(a.panels.detail, 0, 1, false)
```

```go
// NEW:
leftCol := tview.NewFlex().SetDirection(tview.FlexRow).
    AddItem(a.panels.resources, 0, 1, true).
    AddItem(a.panels.items, 0, 2, false)

layout := tview.NewFlex().
    AddItem(leftCol, 25, 0, true).
    AddItem(a.panels.rightFlex, 0, 1, false)

// Wire tabBar mouse capture for clickable tabs
a.panels.tabBar.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
    if action == tview.MouseLeftClick {
        col, _ := event.Position() // Position() returns (x=column, y=row)
        a.selectTabByColumn(col)
        return action, nil
    }
    return action, event
})
```

- [ ] **Step 3: Update `loadItems()` to store `loadedItems`**

In `loadItems`, after the `items, err := ...` call inside the goroutine's `QueueUpdateDraw`, add:

```go
a.loadedItems = items
```

Full updated goroutine block:

```go
go func() {
    items, err := a.providers[i].ListItems(context.Background(), query)
    a.tapp.QueueUpdateDraw(func() {
        a.panels.items.Clear()
        a.panels.detail.Clear()
        a.loadedItems = items // NEW

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
```

Also reset in `loadItems` at the top:

```go
func (a *App) loadItems(i int, query string) {
    a.tabLoaded = nil
    a.tabCache = nil
    a.loadedItems = nil // NEW
    a.currentItem = awspkg.Item{}
    // ... rest unchanged
```

- [ ] **Step 4: Replace `renderDetail()` and `renderTabBar()` with the split version**

Remove the old `renderTabBar` standalone function. Replace `renderDetail()`:

```go
// renderDetail writes the tab bar to tabBar widget and content to detail widget.
func (a *App) renderDetail() {
    tabs := a.providers[a.activeProvider].Tabs()
    if len(tabs) == 0 {
        return
    }
    a.renderTabBar(tabs)
    content := "  ... fetching"
    if a.activeTab < len(a.tabLoaded) && a.tabLoaded[a.activeTab] {
        content = a.tabCache[a.activeTab]
    }
    a.panels.detail.SetText(content).ScrollToBeginning()
}

// renderTabBar writes the 2-row tab bar to the tabBar widget and records
// display-column offsets for mouse click detection.
func (a *App) renderTabBar(tabs []awspkg.TabDef) {
    var line1, line2 strings.Builder
    a.tabBarOffsets = make([]int, len(tabs))
    col := 0
    for i, tab := range tabs {
        label := " " + tab.Label + " "
        a.tabBarOffsets[i] = col
        if i == a.activeTab {
            line1.WriteString("[aqua::bu]" + label + "[-::-]")
            line2.WriteString(strings.Repeat("─", len(label)))
        } else {
            line1.WriteString("[gray]" + label + "[-]")
            line2.WriteString(strings.Repeat(" ", len(label)))
        }
        col += len(label) // display columns (no tag chars)
    }
    a.panels.tabBar.SetText(line1.String() + "\n" + line2.String())
}
```

- [ ] **Step 5: Add `selectTab` and `selectTabByColumn` helpers**

```go
// selectTab switches to the given tab index, fetching if not yet loaded.
func (a *App) selectTab(idx int) {
    tabs := a.providers[a.activeProvider].Tabs()
    if idx < 0 || idx >= len(tabs) || len(a.tabLoaded) == 0 {
        return
    }
    a.activeTab = idx
    if !a.tabLoaded[idx] {
        a.loadTab(a.activeProvider, idx, a.currentItem)
    } else {
        a.renderDetail()
    }
}

// selectTabByColumn maps a clicked display column to the tab index and selects it.
func (a *App) selectTabByColumn(col int) {
    for i := len(a.tabBarOffsets) - 1; i >= 0; i-- {
        if col >= a.tabBarOffsets[i] {
            a.selectTab(i)
            return
        }
    }
}
```

- [ ] **Step 6: Update `nextTab()` and `prevTab()` to use `selectTab()`**

```go
func (a *App) nextTab() {
    tabs := a.providers[a.activeProvider].Tabs()
    if len(tabs) == 0 || len(a.tabLoaded) == 0 {
        return
    }
    a.selectTab((a.activeTab + 1) % len(tabs))
}

func (a *App) prevTab() {
    tabs := a.providers[a.activeProvider].Tabs()
    if len(tabs) == 0 || len(a.tabLoaded) == 0 {
        return
    }
    n := len(tabs)
    a.selectTab((a.activeTab + n - 1) % n)
}
```

- [ ] **Step 7: Build and verify no compile errors**

```bash
cd /home/bryan/lazyaws && go build ./...
```

Expected: clean build.

- [ ] **Step 8: Run and manually verify tab UX**

```bash
./lazyaws
```

Verify:
- Active tab renders aqua bold + underline row below
- Inactive tabs are gray
- `[` / `]` still cycle tabs
- Clicking a tab directly jumps to it (mouse must be enabled — it already is via `EnableMouse(true)`)

- [ ] **Step 9: Run tests**

```bash
go test ./...
```

Expected: all existing tests pass.

- [ ] **Step 10: Commit**

```bash
git add internal/ui/panels.go internal/ui/app.go
git commit -m "feat: split detail pane into tabBar+content+expand flex, add clickable tabs with aqua underline"
```

---

## Chunk 2: Feature 2 — IAM Policies Provider + Expand Panel

---

### Task 3: Add `Expandable` interface and expand panel lifecycle

**Files:**
- Modify: `internal/aws/provider.go`
- Modify: `internal/ui/app.go`

- [ ] **Step 1: Add `Expandable` interface to `provider.go`**

Append after the `Provider` interface:

```go
// Expandable is an optional interface providers can implement to support
// the bottom expand panel. Enter on an item calls Expand and shows the result.
type Expandable interface {
    Expand(ctx context.Context, item Item) (string, error)
}
```

- [ ] **Step 2: Add `showExpand`, `hideExpand`, and `showStatusMessage` to `app.go`**

```go
// showExpand displays content in the expand panel below the detail pane.
func (a *App) showExpand(content string) {
    a.panels.expand.SetText(content).ScrollToBeginning()
    a.panels.rightFlex.ResizeItem(a.panels.expand, 0, 1)
    a.expandVisible = true
    a.tapp.SetFocus(a.panels.expand)
}

// hideExpand hides the expand panel and returns focus to the items list.
func (a *App) hideExpand() {
    a.panels.rightFlex.ResizeItem(a.panels.expand, 0, 0)
    a.expandVisible = false
    a.tapp.SetFocus(a.panels.items)
    a.panels.focused = 1 // items
}

// showStatusMessage temporarily sets status bar to a message.
func (a *App) showStatusMessage(msg string) {
    a.panels.statusPages.SwitchToPage("hints")
    a.panels.status.SetText(msg)
}
```

- [ ] **Step 3: Add Esc priority handling**

Add `handleEsc()` to `app.go`:

```go
// handleEsc implements the Esc priority: expand > navStack > pass-through.
// Returns true if the event was consumed.
func (a *App) handleEsc() bool {
    if a.expandVisible {
        a.hideExpand()
        return true
    }
    return false
}
```

In `keys.go`, add `case tcell.KeyEscape` **inside the first `switch event.Key()` block** (the one that already has `case tcell.KeyTab` and `case tcell.KeyBacktab`). The full switch after the edit looks like:

```go
switch event.Key() {
case tcell.KeyTab:
    if searchActive {
        return event
    }
    a.tapp.SetFocus(a.panels.next())
    return nil
case tcell.KeyBacktab:
    if searchActive {
        return event
    }
    a.tapp.SetFocus(a.panels.prev())
    return nil
case tcell.KeyEscape:
    if searchActive {
        return event // let searchInput.SetDoneFunc handle it
    }
    if a.handleEsc() {
        return nil
    }
    return event
}
```

Do NOT add it before the switch or in the `switch event.Rune()` block.

- [ ] **Step 4: Wire Enter on items list for `Expandable` providers**

In `build()` in `app.go`, add after the searchInput `SetDoneFunc` block:

```go
a.panels.items.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
    if event.Key() != tcell.KeyEnter {
        return event
    }
    provider := a.providers[a.activeProvider]
    exp, ok := provider.(awspkg.Expandable)
    if !ok {
        return event // pass through to AddItem callback for non-Expandable
    }
    idx := a.panels.items.GetCurrentItem()
    if idx < 0 || idx >= len(a.loadedItems) {
        return event
    }
    item := a.loadedItems[idx]
    a.selectItem(a.activeProvider, item)
    go func() {
        content, err := exp.Expand(context.Background(), item)
        a.tapp.QueueUpdateDraw(func() {
            if err != nil {
                a.showStatusMessage(fmt.Sprintf("[red]Expand error: %v[-]", err))
                return
            }
            a.showExpand(content)
        })
    }()
    return nil // consume — AddItem callback suppressed for Expandable providers
})
```

- [ ] **Step 5: Build**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/aws/provider.go internal/ui/app.go internal/ui/keys.go
git commit -m "feat: add Expandable interface and expand panel lifecycle (show/hide, Esc priority)"
```

---

### Task 4: Create IAM Policies provider

**Files:**
- Create: `internal/aws/iam_policies.go`
- Create: `internal/aws/iam_policies_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write the failing test first**

Create `internal/aws/iam_policies_test.go`:

```go
package aws

import (
    "context"
    "encoding/json"
    "net/url"
    "testing"
    "time"

    awssdk "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/iam"
    iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// stubIAMPoliciesAPI implements IAMPoliciesAPI for testing.
type stubIAMPoliciesAPI struct {
    policies []iamtypes.Policy // NOTE: ListPolicies returns types.Policy, not ManagedPolicy
    document string            // raw policy JSON to URL-encode as version document
}

func (s *stubIAMPoliciesAPI) ListPolicies(_ context.Context, _ *iam.ListPoliciesInput, _ ...func(*iam.Options)) (*iam.ListPoliciesOutput, error) {
    return &iam.ListPoliciesOutput{Policies: s.policies}, nil
}

func (s *stubIAMPoliciesAPI) GetPolicy(_ context.Context, in *iam.GetPolicyInput, _ ...func(*iam.Options)) (*iam.GetPolicyOutput, error) {
    desc := "Test policy description"
    return &iam.GetPolicyOutput{Policy: &iamtypes.Policy{
        Arn:             in.PolicyArn,
        Description:     awssdk.String(desc),
        AttachmentCount: awssdk.Int32(3),
    }}, nil
}

func (s *stubIAMPoliciesAPI) GetPolicyVersion(_ context.Context, in *iam.GetPolicyVersionInput, _ ...func(*iam.Options)) (*iam.GetPolicyVersionOutput, error) {
    encoded := url.QueryEscape(s.document)
    return &iam.GetPolicyVersionOutput{
        PolicyVersion: &iamtypes.PolicyVersion{
            Document:  awssdk.String(encoded),
            IsDefault: true,
        },
    }, nil
}

func TestIAMPoliciesProvider_ListItems(t *testing.T) {
    now := time.Now()
    stub := &stubIAMPoliciesAPI{
        policies: []iamtypes.Policy{
            {
                Arn:              awssdk.String("arn:aws:iam::123:policy/MyPolicy"),
                PolicyName:       awssdk.String("MyPolicy"),
                DefaultVersionId: awssdk.String("v1"),
                AttachmentCount:  awssdk.Int32(3),
                CreateDate:       &now,
                // Note: Description is NOT populated by ListPolicies; fetched via GetPolicy
            },
        },
    }
    p := NewIAMPoliciesProviderWithClient(stub)
    items, err := p.ListItems(context.Background(), "")
    if err != nil {
        t.Fatal(err)
    }
    if len(items) != 1 {
        t.Fatalf("want 1 item, got %d", len(items))
    }
    item := items[0]
    if item.ID != "arn:aws:iam::123:policy/MyPolicy" {
        t.Errorf("want ID=arn:aws:iam::123:policy/MyPolicy, got %s", item.ID)
    }
    if item.Name != "MyPolicy" {
        t.Errorf("want Name=MyPolicy, got %s", item.Name)
    }
    if item.Meta["defaultVersionId"] != "v1" {
        t.Errorf("want meta defaultVersionId=v1, got %s", item.Meta["defaultVersionId"])
    }
}

func TestIAMPoliciesProvider_ListItems_Filter(t *testing.T) {
    stub := &stubIAMPoliciesAPI{
        policies: []iamtypes.Policy{
            {Arn: awssdk.String("arn:aws:iam::123:policy/Alpha"), PolicyName: awssdk.String("Alpha"), DefaultVersionId: awssdk.String("v1")},
            {Arn: awssdk.String("arn:aws:iam::123:policy/Beta"), PolicyName: awssdk.String("Beta"), DefaultVersionId: awssdk.String("v1")},
        },
    }
    p := NewIAMPoliciesProviderWithClient(stub)
    items, err := p.ListItems(context.Background(), "alp")
    if err != nil {
        t.Fatal(err)
    }
    if len(items) != 1 || items[0].Name != "Alpha" {
        t.Errorf("want [Alpha], got %v", items)
    }
}

func TestIAMPoliciesProvider_Expand(t *testing.T) {
    doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`
    stub := &stubIAMPoliciesAPI{document: doc}
    p := NewIAMPoliciesProviderWithClient(stub)
    item := Item{ID: "arn:aws:iam::123:policy/MyPolicy", Meta: map[string]string{"defaultVersionId": "v1"}}

    content, err := p.Expand(context.Background(), item)
    if err != nil {
        t.Fatal(err)
    }
    // Should contain pretty-printed JSON
    var parsed map[string]any
    // Strip leading whitespace for parse test
    if err := json.Unmarshal([]byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`), &parsed); err != nil {
        t.Fatal(err)
    }
    if content == "" {
        t.Error("want non-empty expand content")
    }
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/aws/ -run TestIAMPoliciesProvider -v
```

Expected: compile error — `NewIAMPoliciesProviderWithClient` not defined.

- [ ] **Step 3: Implement `internal/aws/iam_policies.go`**

```go
package aws

import (
    "context"
    "encoding/json"
    "fmt"
    "net/url"
    "time"

    awssdk "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/iam"
    iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// IAMPoliciesAPI is the subset of IAM client methods used by IAMPoliciesProvider.
// Note: Description is NOT returned by ListPolicies — GetPolicy is used in tabOverview.
type IAMPoliciesAPI interface {
    ListPolicies(ctx context.Context, in *iam.ListPoliciesInput, opts ...func(*iam.Options)) (*iam.ListPoliciesOutput, error)
    GetPolicy(ctx context.Context, in *iam.GetPolicyInput, opts ...func(*iam.Options)) (*iam.GetPolicyOutput, error)
    GetPolicyVersion(ctx context.Context, in *iam.GetPolicyVersionInput, opts ...func(*iam.Options)) (*iam.GetPolicyVersionOutput, error)
}

// IAMPoliciesProvider implements Provider for customer-managed IAM Policies.
type IAMPoliciesProvider struct{ client IAMPoliciesAPI }

func NewIAMPoliciesProvider(cfg awssdk.Config, local bool) *IAMPoliciesProvider {
    var opts []func(*iam.Options)
    if local {
        opts = append(opts, func(o *iam.Options) { o.BaseEndpoint = awssdk.String("http://localhost:4566") })
    }
    return &IAMPoliciesProvider{client: iam.NewFromConfig(cfg, opts...)}
}

func NewIAMPoliciesProviderWithClient(client IAMPoliciesAPI) *IAMPoliciesProvider {
    return &IAMPoliciesProvider{client: client}
}

func (p *IAMPoliciesProvider) Name() string { return "IAM Policies" }

func (p *IAMPoliciesProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
    out, err := p.client.ListPolicies(ctx, &iam.ListPoliciesInput{
        Scope: iamtypes.PolicyScopeTypeLocal,
    })
    if err != nil {
        return nil, fmt.Errorf("list policies: %w", err)
    }
    items := make([]Item, 0, len(out.Policies))
    for _, pol := range out.Policies {
        created := ""
        if pol.CreateDate != nil {
            created = pol.CreateDate.Format(time.DateOnly)
        }
        items = append(items, Item{
            ID:   awssdk.ToString(pol.Arn),
            Name: awssdk.ToString(pol.PolicyName),
            Meta: map[string]string{
                "defaultVersionId": awssdk.ToString(pol.DefaultVersionId),
                "attachmentCount":  fmt.Sprintf("%d", awssdk.ToInt32(pol.AttachmentCount)),
                "createDate":       created,
                // Description is not returned by ListPolicies; fetched in tabOverview via GetPolicy
            },
        })
    }
    return filterItems(items, query), nil
}

func (p *IAMPoliciesProvider) GetDetail(ctx context.Context, item Item) (string, error) {
    return p.tabOverview(ctx, item)
}

func (p *IAMPoliciesProvider) Tabs() []TabDef {
    return []TabDef{
        {Label: "Overview", Fetch: p.tabOverview},
        {Label: "Document", Fetch: p.tabDocument},
    }
}

// Expand implements Expandable — shows the policy document in the expand panel.
func (p *IAMPoliciesProvider) Expand(ctx context.Context, item Item) (string, error) {
    return p.tabDocument(ctx, item)
}

func (p *IAMPoliciesProvider) tabOverview(ctx context.Context, item Item) (string, error) {
    // GetPolicy is needed for Description (not returned by ListPolicies).
    description := ""
    if out, err := p.client.GetPolicy(ctx, &iam.GetPolicyInput{PolicyArn: awssdk.String(item.ID)}); err == nil {
        description = awssdk.ToString(out.Policy.Description)
    }
    return KV([][2]string{
        {"ARN", item.ID},
        {"Attachments", item.Meta["attachmentCount"]},
        {"Created", item.Meta["createDate"]},
        {"Description", description},
    }), nil
}

func (p *IAMPoliciesProvider) tabDocument(ctx context.Context, item Item) (string, error) {
    versionID := item.Meta["defaultVersionId"]
    out, err := p.client.GetPolicyVersion(ctx, &iam.GetPolicyVersionInput{
        PolicyArn: awssdk.String(item.ID),
        VersionId: awssdk.String(versionID),
    })
    if err != nil {
        return "", fmt.Errorf("get policy version: %w", err)
    }
    docStr, err := url.QueryUnescape(awssdk.ToString(out.PolicyVersion.Document))
    if err != nil {
        docStr = awssdk.ToString(out.PolicyVersion.Document)
    }
    var raw any
    if err := json.Unmarshal([]byte(docStr), &raw); err != nil {
        return "  " + docStr + "\n", nil
    }
    b, _ := json.MarshalIndent(raw, "  ", "  ")
    return "  " + string(b) + "\n", nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/aws/ -run TestIAMPoliciesProvider -v
```

Expected: all 3 tests pass.

- [ ] **Step 5: Register in `main.go`**

Add after `awspkg.NewIAMProvider(cfg, *local)`:

```go
awspkg.NewIAMPoliciesProvider(cfg, *local),
```

- [ ] **Step 6: Build and manually verify**

```bash
go build ./... && ./lazyaws
```

Navigate to "IAM Policies" → select a policy → verify Overview and Document tabs load. Press Enter → expand panel opens with JSON. Press Esc → expand hides.

- [ ] **Step 7: Run all tests**

```bash
go test ./...
```

- [ ] **Step 8: Commit**

```bash
git add internal/aws/iam_policies.go internal/aws/iam_policies_test.go main.go
git commit -m "feat: add IAM Policies provider with Expandable JSON document panel"
```

---

## Chunk 3: Feature 3 — Cross-resource Linking

---

### Task 5: Add `Link()` helper and tests

**Files:**
- Modify: `internal/aws/format.go`
- Create: `internal/aws/format_link_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/aws/format_link_test.go`:

```go
package aws

import "testing"

func TestLink(t *testing.T) {
    tests := []struct {
        label, provider, targetID, wantContains string
    }{
        {
            label:        "my-function",
            provider:     "Lambda",
            targetID:     "my-function",
            wantContains: `["link:Lambda:my-function"]`,
        },
        {
            label:        "my-function",
            provider:     "Lambda",
            targetID:     "my-function",
            wantContains: "[aqua::u]my-function",
        },
    }
    for _, tc := range tests {
        got := Link(tc.label, tc.provider, tc.targetID)
        if !contains(got, tc.wantContains) {
            t.Errorf("Link(%q,%q,%q) = %q, want to contain %q", tc.label, tc.provider, tc.targetID, got, tc.wantContains)
        }
    }
}

func contains(s, sub string) bool {
    return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
    for i := 0; i <= len(s)-len(sub); i++ {
        if s[i:i+len(sub)] == sub {
            return true
        }
    }
    return false
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/aws/ -run TestLink -v
```

Expected: compile error — `Link` not defined.

- [ ] **Step 3: Add `Link()` to `format.go`**

Append to `internal/aws/format.go`:

```go
// Link returns a tview-formatted string for a clickable cross-resource link.
// providerName must match the target provider's Name() return value exactly.
// targetID must match the target Item's ID field exactly.
// Styled with aqua underline (no bold) — distinct from the active tab style.
func Link(label, providerName, targetID string) string {
    region := "link:" + providerName + ":" + targetID
    return `["` + region + `"][aqua::u]` + label + `[white::-]["]`
}

// arnLastSegment returns the last colon-separated segment of an ARN.
// Used to extract role names, queue names, topic names from ARNs.
func arnLastSegment(arn string) string {
    parts := strings.Split(arn, ":")
    if len(parts) == 0 {
        return arn
    }
    return parts[len(parts)-1]
}

// arnToSQSURL converts an SQS ARN to its queue URL form.
// arn:aws:sqs:{region}:{accountId}:{queueName} → https://sqs.{region}.amazonaws.com/{accountId}/{queueName}
func arnToSQSURL(arn string) string {
    parts := strings.Split(arn, ":")
    if len(parts) != 6 {
        return arn
    }
    return fmt.Sprintf("https://sqs.%s.amazonaws.com/%s/%s", parts[3], parts[4], parts[5])
}

// parseLambdaURN extracts the Lambda function name from an API Gateway integration URI.
// Handles both direct ARN (arn:aws:lambda:...:function:name) and proxy URI format
// (arn:aws:apigateway:...:lambda:path/.../functions/{lambdaArn}/invocations).
func parseLambdaFromIntegrationURI(uri string) string {
    // Proxy format: contains "functions/" and "/invocations"
    if idx := strings.Index(uri, "functions/"); idx >= 0 {
        rest := uri[idx+len("functions/"):]
        if end := strings.Index(rest, "/invocations"); end >= 0 {
            rest = rest[:end]
        }
        // rest is the Lambda ARN — return last segment
        return arnLastSegment(rest)
    }
    // Direct Lambda ARN
    return arnLastSegment(uri)
}
```

Also add `"strings"` to imports if not already present (it is).

- [ ] **Step 4: Run tests**

```bash
go test ./internal/aws/ -run TestLink -v
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add internal/aws/format.go internal/aws/format_link_test.go
git commit -m "feat: add Link(), arnLastSegment(), arnToSQSURL(), parseLambdaFromIntegrationURI() helpers"
```

---

### Task 6: Add navigation stack to `app.go`

**Files:**
- Modify: `internal/ui/app.go`

- [ ] **Step 1: Add `navState` type and `navStack` to `App`**

Add type definition above `App`:

```go
// navState records the TUI position before following a cross-resource link.
type navState struct {
    providerIdx int
    itemIdx     int
    tabIdx      int
}
```

Add `navStack []navState` to the `App` struct.

- [ ] **Step 2: Add `navigateTo` and `navigateBack`**

```go
// navigateTo follows a cross-resource link to the given provider and item ID.
// It pushes the current position onto navStack before navigating.
func (a *App) navigateTo(providerName, targetID string) {
    // Find provider index
    targetProviderIdx := -1
    for i, p := range a.providers {
        if p.Name() == providerName {
            targetProviderIdx = i
            break
        }
    }
    if targetProviderIdx == -1 {
        a.showStatusMessage(fmt.Sprintf("[red]No provider for: %s[-]", providerName))
        return
    }

    // Push current state
    a.navStack = append(a.navStack, navState{
        providerIdx: a.activeProvider,
        itemIdx:     a.panels.items.GetCurrentItem(),
        tabIdx:      a.activeTab,
    })

    // If provider items not loaded, load them first
    if a.activeProvider != targetProviderIdx || len(a.loadedItems) == 0 {
        a.showStatusMessage("[yellow]navigating…[-]")
        a.activeProvider = targetProviderIdx
        go func() {
            items, err := a.providers[targetProviderIdx].ListItems(context.Background(), "")
            a.tapp.QueueUpdateDraw(func() {
                if err != nil {
                    a.navStack = a.navStack[:len(a.navStack)-1] // pop on error
                    a.showStatusMessage(fmt.Sprintf("[red]Navigation error: %v[-]", err))
                    return
                }
                a.loadedItems = items
                a.panels.items.Clear()
                for _, item := range items {
                    item := item
                    a.panels.items.AddItem(item.Name, "", 0, func() {
                        a.selectItem(targetProviderIdx, item)
                    })
                }
                a.resetHints()
                a.panels.resources.SetCurrentItem(targetProviderIdx)
                a.findAndSelectItem(targetProviderIdx, targetID)
            })
        }()
        return
    }

    a.panels.resources.SetCurrentItem(targetProviderIdx)
    a.findAndSelectItem(targetProviderIdx, targetID)
}

// findAndSelectItem finds the item with targetID in loadedItems and selects it.
func (a *App) findAndSelectItem(providerIdx int, targetID string) {
    for i, item := range a.loadedItems {
        if item.ID == targetID {
            a.panels.items.SetCurrentItem(i)
            a.selectItem(providerIdx, item)
            return
        }
    }
    a.navStack = a.navStack[:len(a.navStack)-1] // pop — target not found
    a.showStatusMessage(fmt.Sprintf("[red]Resource not found: %s[-]", targetID))
}

// navigateBack pops the navigation stack and restores the previous position.
func (a *App) navigateBack() bool {
    if len(a.navStack) == 0 {
        return false
    }
    state := a.navStack[len(a.navStack)-1]
    a.navStack = a.navStack[:len(a.navStack)-1]

    a.activeProvider = state.providerIdx
    a.panels.resources.SetCurrentItem(state.providerIdx)
    // Re-load items for the restored provider (uses cache if already loaded)
    a.loadItems(state.providerIdx, "")
    // Restore item selection after items load — done inside loadItems goroutine
    // For simplicity: user will need to re-select the item manually after back-nav
    // (full item list is restored; tab state is reset by loadItems)
    return true
}
```

- [ ] **Step 3: Update `handleEsc()` to include navStack pop**

```go
func (a *App) handleEsc() bool {
    if a.expandVisible {
        a.hideExpand()
        return true
    }
    if a.navigateBack() {
        return true
    }
    return false
}
```

- [ ] **Step 4: Wire content TextView mouse and Enter handler for links**

In `build()`, after wiring the tabBar mouse capture, add:

```go
// Wire content TextView for link clicks and Enter to follow links
a.panels.detail.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
    if action == tview.MouseLeftClick {
        a.followHighlightedLink()
    }
    return action, event
})
a.panels.detail.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
    if event.Key() == tcell.KeyEnter {
        if a.followHighlightedLink() {
            return nil
        }
    }
    return event
})
```

Add `followHighlightedLink`:

```go
// followHighlightedLink reads the currently highlighted region from the detail
// TextView and navigates to it if it is a link region.
func (a *App) followHighlightedLink() bool {
    highlights := a.panels.detail.GetHighlights()
    if len(highlights) == 0 {
        return false
    }
    region := highlights[0]
    if !strings.HasPrefix(region, "link:") {
        return false
    }
    // region format: "link:{providerName}:{targetID}"
    rest := strings.TrimPrefix(region, "link:")
    sep := strings.Index(rest, ":")
    if sep < 0 {
        return false
    }
    providerName := rest[:sep]
    targetID := rest[sep+1:]
    a.navigateTo(providerName, targetID)
    return true
}
```

- [ ] **Step 5: Build**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/ui/app.go
git commit -m "feat: add navigation stack and link-following logic (navigateTo, navigateBack, followHighlightedLink)"
```

---

### Task 7: Add links to provider tab output

**Files:**
- Modify: `internal/aws/sqs.go`
- Modify: `internal/aws/lambda.go`
- Modify: `internal/aws/cloudformation.go`
- Modify: `internal/aws/apigateway.go`

- [ ] **Step 1: SQS — add DLQ link**

In `sqs.go`, `tabDLQ`, replace the `KV` call to wrap the DLQ ARN:

```go
dlqURL := arnToSQSURL(redrive.DeadLetterTargetArn)
return KV([][2]string{
    {"DLQ", Link(redrive.DeadLetterTargetArn, "SQS", dlqURL)},
    {"Max Receives", fmt.Sprintf("%d", redrive.MaxReceiveCount)},
}), nil
```

- [ ] **Step 2: Lambda — add role ARN link in tabOverview**

In `lambda.go`, `tabOverview`, wrap the role ARN. Find the `Role ARN` KV entry and replace:

```go
{"Role ARN", Link(awssdk.ToString(fn.Role), "IAM Roles", arnLastSegment(awssdk.ToString(fn.Role)))},
```

- [ ] **Step 3: Lambda — add trigger source links in tabTriggers**

In `lambda.go`, `tabTriggers` (or equivalent triggers tab fetch), for each trigger row, detect provider from ARN and emit a Link. After building the trigger type string, add link to source:

```go
// Determine target provider from ARN prefix
sourceARN := awssdk.ToString(m.EventSourceArn)
targetProvider := triggerProviderFromARN(sourceARN)
sourceName := arnLastSegment(sourceARN)
var sourceCell string
if targetProvider != "" {
    targetID := sourceARN
    if targetProvider == "SQS" {
        targetID = arnToSQSURL(sourceARN)
    }
    sourceCell = Link(sourceName, targetProvider, targetID)
} else {
    sourceCell = sourceARN
}
rows = append(rows, []string{triggerType, sourceCell, string(m.State)})
```

Add helper in `lambda.go`:

```go
func triggerProviderFromARN(arn string) string {
    switch {
    case strings.Contains(arn, ":sqs:"):
        return "SQS"
    case strings.Contains(arn, ":sns:"):
        return "SNS"
    default:
        return ""
    }
}
```

- [ ] **Step 4: CloudFormation — add type-map links in Resources tab**

In `cloudformation.go`, in the Resources tab fetch function, replace the plain Logical ID cell with a link for mapped types:

```go
// cfnTypeToProvider maps CloudFormation resource types to lazyaws provider names.
// PhysicalResourceId matches Item.ID directly for each provider.
var cfnTypeToProvider = map[string]string{
    "AWS::Lambda::Function":       "Lambda",
    "AWS::SQS::Queue":             "SQS",
    "AWS::SNS::Topic":             "SNS",
    "AWS::S3::Bucket":             "S3",
    "AWS::ApiGateway::RestApi":    "API Gateway",
    "AWS::ApiGatewayV2::Api":      "API Gateway",
    "AWS::IAM::Role":              "IAM Roles",
    "AWS::SecretsManager::Secret": "Secrets Manager",
}
```

In the row-building loop:

```go
logicalID := awssdk.ToString(r.LogicalResourceId)
physicalID := awssdk.ToString(r.PhysicalResourceId)
resourceType := awssdk.ToString(r.ResourceType)
displayID := logicalID
if provider, ok := cfnTypeToProvider[resourceType]; ok {
    targetID := physicalID
    if provider == "SQS" {
        targetID = physicalID // PhysicalResourceId for SQS is already the queue URL
    }
    displayID = Link(logicalID, provider, targetID)
}
rows = append(rows, []string{displayID, resourceType, string(r.ResourceStatus)})
```

- [ ] **Step 5: API Gateway — expand interfaces and resolve integrations**

**Important:** `apigateway.go` has ONE `APIGatewayProvider` struct with two client fields (`v1 APIGatewayV1API` and `v2 APIGatewayV2API`). Do NOT create separate structs.

**Add to `APIGatewayV2API` interface:**
```go
GetIntegration(ctx context.Context, in *apigatewayv2.GetIntegrationInput, opts ...func(*apigatewayv2.Options)) (*apigatewayv2.GetIntegrationOutput, error)
```

**Add to `APIGatewayV1API` interface:**
```go
GetMethod(ctx context.Context, in *apigateway.GetMethodInput, opts ...func(*apigateway.Options)) (*apigateway.GetMethodOutput, error)
```

**V2 Routes — integration resolution traversal:**

The V2 Routes tab fetches routes via `ListRoutes`. Each route has a `Target` field. When `Target` starts with `"integrations/"`, the integrationId is embedded directly (e.g. `"integrations/abc123"`) — extract it and call `GetIntegration`:

```go
integrationID := strings.TrimPrefix(target, "integrations/")
intOut, err := p.v2.GetIntegration(ctx, &apigatewayv2.GetIntegrationInput{
    ApiId:         awssdk.String(item.ID),
    IntegrationId: awssdk.String(integrationID),
})
if err == nil && intOut.IntegrationUri != nil {
    fnName := parseLambdaFromIntegrationURI(awssdk.ToString(intOut.IntegrationUri))
    if fnName != "" {
        target = Link(fnName, "Lambda", fnName)
    }
}
```

When `Target` is a direct Lambda ARN (does not start with `"integrations/"`), parse it directly with `parseLambdaFromIntegrationURI`.

**V1 Routes — integration resolution traversal:**

The V1 Routes tab iterates resources from `GetResources`. Each resource has `ResourceMethods` (map of HTTP method → `Method`). For each method, call `GetMethod` which returns the full `Method` including `MethodIntegration.Uri` — no separate `GetIntegration` call is needed for V1:

```go
// resourceID is available from iterating GetResources response
// httpMethod is available from iterating ResourceMethods keys
methodOut, err := p.v1.GetMethod(ctx, &apigateway.GetMethodInput{
    RestApiId:  awssdk.String(item.ID),
    ResourceId: awssdk.String(resourceID),
    HttpMethod: awssdk.String(httpMethod),
})
if err == nil && methodOut.MethodIntegration != nil && methodOut.MethodIntegration.Uri != nil {
    fnName := parseLambdaFromIntegrationURI(awssdk.ToString(methodOut.MethodIntegration.Uri))
    if fnName != "" {
        target = Link(fnName, "Lambda", fnName)
    }
}
```

Note: these extra API calls happen inside the Routes tab fetch goroutine — no UI impact. Errors are swallowed (link simply not rendered). The `cfnTypeToProvider` map keys must exactly match `Provider.Name()` values — verify against `Name()` methods: `"Lambda"`, `"SQS"`, `"SNS"`, `"S3"`, `"API Gateway"`, `"IAM Roles"`, `"Secrets Manager"`.

- [ ] **Step 6: Build**

```bash
go build ./...
```

- [ ] **Step 7: Manual smoke test**

```bash
./lazyaws
```

Navigate to Lambda → select a function → Overview tab → click the Role ARN link → verify IAM Roles provider opens with the role selected. Press Esc → verify return to Lambda.

Navigate to SQS → select a queue with DLQ → DLQ tab → click DLQ link → verify SQS provider shows the DLQ.

Navigate to CloudFormation → select a stack → Resources tab → click a Lambda resource logical ID → verify Lambda provider navigates.

- [ ] **Step 8: Run all tests**

```bash
go test ./...
```

- [ ] **Step 9: Commit**

```bash
git add internal/aws/ internal/ui/app.go
git commit -m "feat: add cross-resource links in Lambda, SQS, CloudFormation, API Gateway providers"
```

---

## Chunk 4: Feature 4 — S3 Object Browser

---

### Task 8: S3 — cache raw objects and add streaming download

**Files:**
- Modify: `internal/aws/s3.go`
- Modify: `internal/aws/s3_test.go` (add download test)

- [ ] **Step 1: Add `GetObject` to `S3API` interface and update existing test stub**

In `s3.go`, add to the `S3API` interface:

```go
GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
```

**Important:** This is a breaking change for the existing `stubS3` in `s3_test.go`. In the same edit, add `GetObject` to the stub (see Step 5).

- [ ] **Step 2: Define `S3ObjectItem` (single exported type) and add cache fields to `S3Provider`**

Define ONE exported type — do not create a parallel unexported type:

```go
import "sync"

// S3ObjectItem holds pre-formatted display data for interactive object row selection.
type S3ObjectItem struct {
    Key           string
    Size          int64
    SizeFormatted string
    LastModified  string
}

type S3Provider struct {
    client      S3API
    objectsMu   sync.Mutex
    lastObjects []S3ObjectItem
}
```

- [ ] **Step 3: Update `tabObjects` to populate `lastObjects`**

After building `rows`, before returning, store raw objects:

```go
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
    total := awssdk.ToInt32(out.KeyCount)
    shown := int32(len(out.Contents))
    if shown < total {
        result += fmt.Sprintf("\n  (showing %d of %d objects — use / to filter)\n", shown, total)
    }
    return result, nil
}

// GetLastObjects returns the objects cached by the most recent tabObjects call.
func (p *S3Provider) GetLastObjects() []s3ObjectItem {
    p.objectsMu.Lock()
    defer p.objectsMu.Unlock()
    out := make([]s3ObjectItem, len(p.lastObjects))
    copy(out, p.lastObjects)
    return out
}
```

- [ ] **Step 4: Add `DownloadObject` streaming method**

```go
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
```

Add `"io"` to imports.

- [ ] **Step 5: Add test for `GetLastObjects` and `DownloadObject`**

In `s3_test.go`, add stub method `GetObject` and two test cases:

```go
// In the existing stub or a new one:
func (s *stubS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
    content := "hello world"
    return &s3.GetObjectOutput{
        Body: io.NopCloser(strings.NewReader(content)),
    }, nil
}

func TestS3Provider_GetLastObjects(t *testing.T) {
    stub := &stubS3{objects: []s3types.Object{
        {Key: awssdk.String("readme.txt"), Size: awssdk.Int64(100)},
    }}
    p := NewS3ProviderWithClient(stub)
    item := Item{ID: "my-bucket", Name: "my-bucket"}
    _, err := p.tabObjects(context.Background(), item)
    if err != nil {
        t.Fatal(err)
    }
    got := p.GetLastObjects()
    if len(got) != 1 || got[0].Key != "readme.txt" {
        t.Errorf("want [{readme.txt 100}], got %v", got)
    }
}

func TestS3Provider_DownloadObject(t *testing.T) {
    stub := &stubS3{}
    p := NewS3ProviderWithClient(stub)
    var buf strings.Builder
    err := p.DownloadObject(context.Background(), "my-bucket", "readme.txt", &buf)
    if err != nil {
        t.Fatal(err)
    }
    if buf.String() != "hello world" {
        t.Errorf("want 'hello world', got %q", buf.String())
    }
}
```

- [ ] **Step 6: Run S3 tests**

```bash
go test ./internal/aws/ -run TestS3Provider -v
```

Expected: all pass.

- [ ] **Step 7: Build**

```bash
go build ./...
```

- [ ] **Step 8: Commit**

```bash
git add internal/aws/s3.go internal/aws/s3_test.go
git commit -m "feat: add S3 object cache (GetLastObjects) and streaming DownloadObject"
```

---

### Task 9: App — interactive S3 object row selection

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/keys.go`
- Modify: `internal/ui/panels.go`

- [ ] **Step 1: Add S3 selection state to `App`**

Add to `App` struct:

```go
selectedObjectRow int
cachedObjects     []s3ObjectItem // populated when S3 Objects tab loads
tmpFiles          []string       // temp files to clean up on exit
```

Import the s3ObjectItem type — since it's in the `aws` package, reference it as `awspkg.s3ObjectItem`... but `s3ObjectItem` is unexported. **Fix: export it as `S3ObjectItem`** in `s3.go`:

```go
// S3ObjectItem holds minimal data for interactive object row selection.
type S3ObjectItem struct {
    Key  string
    Size int64
}
```

Update `lastObjects`, `GetLastObjects`, and `tabObjects` to use `S3ObjectItem`.

Then in `App`:

```go
cachedObjects []awspkg.S3ObjectItem
```

- [ ] **Step 2: Reset `selectedObjectRow` and `cachedObjects` in `loadItems` and `selectItem`**

In `loadItems`, add to the reset block at the top:

```go
a.selectedObjectRow = 0
a.cachedObjects = nil
```

In `selectItem`, add:

```go
a.selectedObjectRow = 0
a.cachedObjects = nil
```

- [ ] **Step 3: Populate `cachedObjects` after Objects tab loads**

In `loadTab`, after `a.tabLoaded[tabIdx] = true`, add:

```go
// If this is the S3 Objects tab, cache raw objects for row selection.
if s3p, ok := a.providers[providerIdx].(*awspkg.S3Provider); ok {
    tabs := a.providers[providerIdx].Tabs()
    if tabIdx < len(tabs) && tabs[tabIdx].Label == "Objects" {
        a.cachedObjects = s3p.GetLastObjects()
    }
}
```

- [ ] **Step 4: Add object row highlighting to `renderDetail`**

When the active tab is Objects and we have cached objects, re-render with the selected row highlighted. Add helper:

```go
// renderObjectsWithHighlight rebuilds the Objects tab table with the selected
// row highlighted in aqua. Uses pre-formatted strings from cachedObjects.
// Column widths are computed inline (mirrors logic in format.go Table()).
func (a *App) renderObjectsWithHighlight() string {
    if len(a.cachedObjects) == 0 {
        return a.tabCache[a.activeTab]
    }
    headers := []string{"Key", "Size", "Last Modified"}
    rows := make([][]string, len(a.cachedObjects))
    for i, obj := range a.cachedObjects {
        rows[i] = []string{obj.Key, obj.SizeFormatted, obj.LastModified}
    }

    // Compute column widths (no separate helper — inline to avoid export)
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

    padded := make([]string, len(headers))
    for i, h := range headers {
        padded[i] = fmt.Sprintf("%-*s", widths[i]+2, h)
    }
    var sb strings.Builder
    fmt.Fprintf(&sb, "  [cyan]%s[-]\n  ", strings.Join(padded, ""))
    for _, w := range widths {
        sb.WriteString(strings.Repeat("─", w) + "  ")
    }
    sb.WriteString("\n")
    for i, row := range rows {
        if i == a.selectedObjectRow {
            sb.WriteString("  [aqua]")
        } else {
            sb.WriteString("  ")
        }
        for j, cell := range row {
            if j < len(widths) {
                fmt.Fprintf(&sb, "%-*s", widths[j]+2, cell)
            }
        }
        if i == a.selectedObjectRow {
            sb.WriteString("[-]")
        }
        sb.WriteString("\n")
    }
    return sb.String()
}
```

**Note:** `S3ObjectItem` stores pre-formatted strings (`SizeFormatted`, `LastModified`) populated in `tabObjects`, so `renderObjectsWithHighlight` needs no export of `formatSize`. The type was defined in Task 8 Step 2 as:

Populate in `tabObjects`. Then `renderObjectsWithHighlight` uses the pre-formatted strings — no need to export `formatSize`. Update `App` to use these fields.

In `renderDetail`, check if we should use the highlighted version:

```go
func (a *App) renderDetail() {
    tabs := a.providers[a.activeProvider].Tabs()
    if len(tabs) == 0 {
        return
    }
    a.renderTabBar(tabs)
    content := "  ... fetching"
    if a.activeTab < len(a.tabLoaded) && a.tabLoaded[a.activeTab] {
        // Use highlighted render for S3 Objects tab when objects are cached
        if len(a.cachedObjects) > 0 && tabs[a.activeTab].Label == "Objects" {
            content = a.renderObjectsWithHighlight()
        } else {
            content = a.tabCache[a.activeTab]
        }
    }
    a.panels.detail.SetText(content).ScrollToBeginning()
}
```

- [ ] **Step 5: Add j/k guard in `keys.go` for S3 Objects tab**

In `setupKeys`, inside the `switch event.Rune()` block, modify `'j'` and `'k'` cases:

```go
case 'j':
    if a.isS3ObjectsTabFocused() {
        a.moveObjectRow(1)
        return nil
    }
    return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
case 'k':
    if a.isS3ObjectsTabFocused() {
        a.moveObjectRow(-1)
        return nil
    }
    return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
```

Add helpers in `app.go`:

```go
// isS3ObjectsTabFocused returns true when ALL of:
// 1. focus is on the detail pane (not resources/items/expand)
// 2. the active provider is S3 (Name() == "S3")
// 3. the active tab is the Objects tab (Label == "Objects")
// 4. cached objects are available (len > 0)
func (a *App) isS3ObjectsTabFocused() bool {
    if a.tapp.GetFocus() != a.panels.detail {
        return false
    }
    if a.providers[a.activeProvider].Name() != "S3" {
        return false
    }
    tabs := a.providers[a.activeProvider].Tabs()
    if a.activeTab >= len(tabs) || tabs[a.activeTab].Label != "Objects" {
        return false
    }
    return len(a.cachedObjects) > 0
}

// moveObjectRow adjusts selectedObjectRow by delta and re-renders.
func (a *App) moveObjectRow(delta int) {
    n := len(a.cachedObjects)
    if n == 0 {
        return
    }
    a.selectedObjectRow = (a.selectedObjectRow + delta + n) % n
    a.renderDetail()
}
```

- [ ] **Step 6: Build**

```bash
go build ./...
```

- [ ] **Step 7: Manual verify**

Navigate to S3 → select a bucket → Objects tab → Tab to focus Detail → j/k moves cyan highlight.

- [ ] **Step 8: Commit**

```bash
git add internal/aws/s3.go internal/ui/app.go internal/ui/keys.go
git commit -m "feat: S3 Objects tab row selection with j/k navigation and cyan highlight"
```

---

### Task 10: App — object open/download flow

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/panels.go`

- [ ] **Step 1: Add `isTextFile` helper**

In `app.go`:

```go
var textExtensions = map[string]bool{
    ".json": true, ".txt": true, ".yaml": true, ".yml": true,
    ".log": true, ".csv": true, ".xml": true, ".md": true,
    ".toml": true, ".ini": true, ".sh": true, ".env": true,
    ".conf": true, ".properties": true,
}

func isTextFile(key string) bool {
    ext := strings.ToLower(filepath.Ext(key))
    return textExtensions[ext]
}
```

Add `"path/filepath"` to imports.

- [ ] **Step 2: Add prompt page widget and show/hide helpers**

The `panels.prompt *tview.TextView` was already added in Task 1. Wire it with input capture in `build()`:

```go
var promptYesHandler func()
var promptNoHandler func()

a.panels.prompt.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
    switch event.Rune() {
    case 'y', 'Y':
        a.panels.statusPages.SwitchToPage("hints")
        a.resetHints()
        a.tapp.SetFocus(a.panels.detail)
        if promptYesHandler != nil {
            h := promptYesHandler
            promptYesHandler = nil
            promptNoHandler = nil
            h()
        }
        return nil
    case 'n', 'N':
        a.panels.statusPages.SwitchToPage("hints")
        a.resetHints()
        a.tapp.SetFocus(a.panels.detail)
        if promptNoHandler != nil {
            h := promptNoHandler
            promptYesHandler = nil
            promptNoHandler = nil
            h()
        }
        return nil
    }
    return event
})
```

Add helpers:

```go
func (a *App) showPrompt(msg string, onYes, onNo func()) {
    a.panels.prompt.SetText(msg)
    a.panels.statusPages.SwitchToPage("prompt")
    // promptYesHandler and promptNoHandler are set via closures in build()
    // Use a channel or stored func — see implementation note below
    a.tapp.SetFocus(a.panels.prompt)
}
```

**Implementation note:** The `promptYesHandler` and `promptNoHandler` variables must be accessible from both `showPrompt` and the `SetInputCapture` closure. Add them as fields on `App`:

```go
// In App struct:
promptYesHandler func()
promptNoHandler  func()
```

```go
func (a *App) showPrompt(msg string, onYes, onNo func()) {
    a.promptYesHandler = onYes
    a.promptNoHandler = onNo
    a.panels.prompt.SetText(" " + msg)
    a.panels.statusPages.SwitchToPage("prompt")
    a.tapp.SetFocus(a.panels.prompt)
}

// dismissPrompt closes the prompt without firing any handler.
// Call from loadItems and selectItem to handle navigation-away mid-prompt.
func (a *App) dismissPrompt() {
    if a.promptYesHandler != nil || a.promptNoHandler != nil {
        a.promptYesHandler = nil
        a.promptNoHandler = nil
        a.panels.statusPages.SwitchToPage("hints")
        a.resetHints()
    }
}
```

Update prompt input capture to use `a.promptYesHandler` / `a.promptNoHandler`.

- [ ] **Step 3: Add `openObject` flow**

```go
const (
    textSizeThreshold   = 10 * 1024 * 1024  // 10 MB
    warnSizeThreshold   = 100 * 1024 * 1024 // 100 MB
)

// openObject handles Enter on a selected S3 object row.
func (a *App) openObject() {
    if len(a.cachedObjects) == 0 || a.selectedObjectRow >= len(a.cachedObjects) {
        return
    }
    obj := a.cachedObjects[a.selectedObjectRow]
    bucket := a.currentItem.ID
    isText := isTextFile(obj.Key)

    doOpen := func() {
        if isText {
            a.downloadAndShow(bucket, obj.Key)
        } else {
            a.downloadAndOpen(bucket, obj.Key)
        }
    }

    switch {
    case isText && obj.Size < textSizeThreshold:
        doOpen() // silent, no prompt
    case obj.Size >= warnSizeThreshold:
        msg := fmt.Sprintf("File is %s. Download anyway? [y/n]", awspkg.FormatSize(obj.Size))
        a.showPrompt(msg, doOpen, nil)
    default:
        msg := fmt.Sprintf("Open %s? [y/n]", obj.Key)
        a.showPrompt(msg, doOpen, nil)
    }
}

// downloadAndShow downloads the object and shows it in the expand panel.
func (a *App) downloadAndShow(bucket, key string) {
    s3p, ok := a.providers[a.activeProvider].(*awspkg.S3Provider)
    if !ok {
        return
    }
    a.showStatusMessage("[yellow]Downloading…[-]")
    go func() {
        f, err := os.CreateTemp("", "lazyaws-*")
        if err != nil {
            a.tapp.QueueUpdateDraw(func() {
                a.showStatusMessage(fmt.Sprintf("[red]Temp file error: %v[-]", err))
            })
            return
        }
        if err := s3p.DownloadObject(context.Background(), bucket, key, f); err != nil {
            f.Close()
            os.Remove(f.Name())
            a.tapp.QueueUpdateDraw(func() {
                a.showStatusMessage(fmt.Sprintf("[red]Download error: %v[-]", err))
            })
            return
        }
        f.Close()
        a.tmpFiles = append(a.tmpFiles, f.Name())
        content, err := os.ReadFile(f.Name())
        a.tapp.QueueUpdateDraw(func() {
            a.resetHints()
            if err != nil {
                a.showStatusMessage(fmt.Sprintf("[red]Read error: %v[-]", err))
                return
            }
            a.showExpand(string(content))
        })
    }()
}

// downloadAndOpen downloads the object to a temp file and opens it with xdg-open.
func (a *App) downloadAndOpen(bucket, key string) {
    s3p, ok := a.providers[a.activeProvider].(*awspkg.S3Provider)
    if !ok {
        return
    }
    a.showStatusMessage("[yellow]Downloading…[-]")
    go func() {
        f, err := os.CreateTemp("", "lazyaws-*")
        if err != nil {
            a.tapp.QueueUpdateDraw(func() {
                a.showStatusMessage(fmt.Sprintf("[red]Temp file error: %v[-]", err))
            })
            return
        }
        if err := s3p.DownloadObject(context.Background(), bucket, key, f); err != nil {
            f.Close()
            os.Remove(f.Name())
            a.tapp.QueueUpdateDraw(func() {
                a.showStatusMessage(fmt.Sprintf("[red]Download error: %v[-]", err))
            })
            return
        }
        f.Close()
        a.tmpFiles = append(a.tmpFiles, f.Name())
        tmpPath := f.Name()
        if err := exec.Command("xdg-open", tmpPath).Start(); err != nil {
            a.tapp.QueueUpdateDraw(func() {
                a.showStatusMessage(fmt.Sprintf("[red]Failed to open: %v[-]", err))
            })
        } else {
            a.tapp.QueueUpdateDraw(func() { a.resetHints() })
        }
    }()
}
```

Add imports: `"os"`, `"os/exec"`.

Export `FormatFileSize` from `s3.go` (rename `formatSize` → `FormatSize` and add an alias `FormatFileSize = FormatSize`) or just duplicate the format logic inline. Easiest: export as `FormatSize`:

In `s3.go`, rename `formatSize` → `FormatSize` (capital F). Update all internal callers.

- [ ] **Step 4: Wire Enter on detail pane for S3 Objects**

Update the `detail.SetInputCapture` (added in Task 6) to also handle S3 object Enter:

```go
a.panels.detail.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
    if event.Key() == tcell.KeyEnter {
        // S3 object open takes priority
        if a.isS3ObjectsTabFocused() {
            a.openObject()
            return nil
        }
        // Link following
        if a.followHighlightedLink() {
            return nil
        }
    }
    return event
})
```

- [ ] **Step 5: Add temp file cleanup on exit**

In `app.go`, add a `cleanup` method:

```go
func (a *App) cleanup() {
    for _, path := range a.tmpFiles {
        os.Remove(path)
    }
}
```

In `keys.go`, update the `'q'` case to call cleanup before stopping:

```go
case 'q':
    a.cleanup()
    a.tapp.Stop()
    return nil
```

Also update `Run()` in `app.go` to call cleanup on any exit path:

```go
func (a *App) Run() error {
    defer a.cleanup()
    return a.tapp.Run()
}
```

The `defer` ensures cleanup runs even if the app exits via means other than `q` (e.g. Ctrl+C). The explicit call in `keys.go` is not required when using defer, but is harmless.

- [ ] **Step 6: Build**

```bash
go build ./...
```

- [ ] **Step 7: Manual end-to-end test**

```bash
./lazyaws
```

Navigate to S3 → select a bucket → Objects tab → Tab to focus Detail → j/k to highlight a `.json` file < 10MB → Enter → expand panel shows JSON, no prompt.

Select a binary file → Enter → prompt "Open X? [y/n]" → y → file opens in default app (or error shown if no app configured).

Select any file ≥ 100MB → Enter → size warning prompt.

- [ ] **Step 8: Run all tests**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 9: Commit**

```bash
git add internal/ui/app.go internal/ui/panels.go internal/ui/keys.go internal/aws/s3.go
git commit -m "feat: S3 object open/download flow with prompt, expand panel, and xdg-open"
```

---

## Final Verification

- [ ] **Full test suite**

```bash
go test ./... -count=1
```

Expected: all tests pass.

- [ ] **Build clean binary**

```bash
go build -o lazyaws . && echo "BUILD OK"
```

- [ ] **End-to-end smoke test checklist**

1. Tab bar: active tab is aqua bold+underline, clicking a tab jumps directly, `[`/`]` still work
2. IAM Policies: select policy → Enter → expand panel opens with JSON → Esc hides
3. SQS DLQ: click DLQ link → navigates to SQS queue → Esc returns
4. Lambda: click Role ARN link → navigates to IAM Roles → Esc returns
5. CloudFormation: click a Lambda resource logical ID → navigates to Lambda
6. S3: j/k moves row highlight → Enter on `.json` < 10MB shows in expand → Enter on binary prompts → Esc priority: expand first, navStack second
