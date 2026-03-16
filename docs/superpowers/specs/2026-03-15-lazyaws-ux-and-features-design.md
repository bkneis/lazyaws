# lazyaws UX & Feature Improvements Design

**Date:** 2026-03-15
**Status:** Approved

---

## Overview

Four features to improve developer productivity in a serverless/SAM context:

1. **Tab UX** — visual highlight + mouse-clickable tabs in the Detail pane
2. **IAM Policies provider** — new provider + general-purpose Expand panel
3. **Cross-resource linking** — navigable links between related AWS resources
4. **S3 object browser** — selectable rows in the Objects tab with open/download

Both IAM Roles and Secrets Manager providers already exist in the codebase and are registered in `main.go`.

---

## Feature 1: Tab UX

### Goal
Make the active tab clearly identifiable and allow direct mouse click to switch tabs.

### Detail pane split

To cleanly separate tab-click handling from link-click handling (Feature 3), the right column becomes a vertical `tview.Flex` containing:

1. **Tab bar** — a 2-row `tview.TextView` (label row + underline row), `SetDynamicColors(true)`, `SetRegions(true)`
2. **Content** — the existing scrollable `tview.TextView`, `SetDynamicColors(true)`, `SetRegions(true)` (already enabled for color output)

```
┌─────────────── right column (vertical Flex) ────────────────┐
│  Overview   Routes   Stages              ← tab bar (2 rows) │
│ ─────────                                                    │
│ Key         Value                        ← content TextView  │
│ ...                                                          │
└──────────────────────────────────────────────────────────────┘
```

### Tab bar rendering

Active tab: `[aqua::bu]Overview[-::-]` (cyan bold + underline, matching existing aqua focus colour).
Inactive tab: `[gray]Routes[-]`.
Line 2: underline drawn with `─` chars spanning the width of the active tab label only.

This is distinct from link styling (Feature 3 uses plain `[aqua::u]` without bold) so they are visually differentiated.

### Clickable tabs

Tab bar TextView uses `SetMouseCapture`. On `tview.MouseLeftClick`:
- Read the clicked cell column from the event
- Map column offset to tab index using **display-column offsets** (the rendered label widths without tview tag characters, since tview strips tags before display and the mouse event reports display-cell position)
- Precompute label start/end display columns when rendering the tab bar string; store as `tabBarOffsets []int` in App
- Call `app.selectTab(idx)`

Keyboard `[` / `]` bindings unchanged.

### `panels` struct changes

```go
tabBar   *tview.TextView   // new: 2-row tab bar
detail   *tview.TextView   // existing: content area
```

Right column layout: `tview.NewFlex().SetDirection(tview.FlexRow)` with tabBar (fixed 2 rows) + detail (flex 1) + expand (flex 0 initially, see Feature 2).

**Tab cycle**: The `expand` panel is **not** part of the Tab/Shift+Tab focus cycle (`panels.primitives()` remains `[resources, items, detail]`). Focus on `expand` is managed exclusively via Enter (open) and Esc (close), keeping `panels.focused` index valid at all times.

### Files affected
- `internal/ui/panels.go` — `newPanels()`: split right column into tabBar + detail + expand Flex; wire tabBar mouse capture
- `internal/ui/app.go` — `renderDetail()`: write tab bar string to tabBar widget, content to detail widget; expose `selectTab(idx int)`; store `tabBarOffsets []int`

---

## Feature 2: IAM Policies Provider + General Expansion Panel

### IAM Policies provider

New file `internal/aws/iam_policies.go`. Registered in `main.go` after IAM Roles.

- `ListItems`: calls `ListPolicies` with `Scope: Local` (customer-managed only; avoids thousands of AWS-managed policies)
- Item: `ID=PolicyArn`, `Name=PolicyName`, `Meta["defaultVersionId"]=DefaultVersionId`
  - `DefaultVersionId` is returned by `ListPolicies` — no extra API call needed
- **Tabs:**
  - **Overview**: ARN, Attachment count, Created date, Description
  - **Document**: call `GetPolicyVersion(PolicyArn, DefaultVersionId)` using `item.Meta["defaultVersionId"]`; pretty-print JSON

The existing `Item` struct has `Meta map[string]string`, so carrying `DefaultVersionId` requires no struct change.

### General Expansion Panel

A third child of the right column vertical Flex, below the content TextView:

```
right column Flex (vertical)
├── tabBar     (fixed 2 rows)
├── detail     (flex proportion 2)
└── expand     (flex proportion 0 initially → 1 when visible)
```

**Showing/hiding**: change the expand item's proportion via `rightFlex.ResizeItem(expand, 0, 0)` (hidden) or `rightFlex.ResizeItem(expand, 0, 1)` (visible, equal share with detail). This avoids needing to know terminal height and avoids a fixed row count.

**Optional interface** in `internal/aws/provider.go`:

```go
type Expandable interface {
    Expand(ctx context.Context, item Item) (string, error)
}
```

**Enter key on Items list**: override with `items.SetInputCapture`:
- Intercept `tcell.KeyEnter`
- If active provider implements `Expandable`: call `Expand()` async → on result, make expand panel visible, write content, move focus to expand
- Otherwise: pass event through (S3Provider intentionally does not implement `Expandable`; S3 object Enter is handled in the content pane when focus=Detail and activeTab=Objects, as described in Feature 4)

**Esc key priority** (highest to lowest):
1. If expand panel is visible → hide it, return focus to Items, consume event
2. If navStack is non-empty → pop navStack, restore provider/item/tab, consume event
3. Otherwise → pass through

**Focus after expand open**: focus moves to expand TextView so the user can scroll long JSON. Esc returns focus to Items and hides the panel.

### Files affected
- `internal/aws/provider.go` — add `Expandable` interface
- `internal/aws/iam_policies.go` — new file, implements `Provider` + `Expandable`
- `internal/ui/panels.go` — add `expand *tview.TextView`, `rightFlex *tview.Flex` to struct; update layout
- `internal/ui/app.go` — `expandVisible bool`; Enter/Esc handlers; `showExpand()`, `hideExpand()`
- `main.go` — register IAM Policies provider

---

## Feature 3: Cross-resource Linking

### Goal
Clickable underlined links in the Detail content pane that navigate to related AWS resources, with Esc-based back navigation.

### Link helper

New function in `internal/aws/format.go`:

```go
// Link returns a tview-formatted string for a clickable cross-resource link.
// providerName must match the target provider's Name() return value exactly.
// targetID must match the target Item's ID field exactly.
func Link(label, providerName, targetID string) string {
    region := "link:" + providerName + ":" + targetID
    return `["` + region + `"][aqua::u]` + label + `[white::-]["]`
}
```

Styling: `[aqua::u]` (underline, no bold) — visually distinct from active tab (`[aqua::bu]`).

### Navigation stack

```go
type navState struct {
    providerIdx int
    itemIdx     int  // index within the Items list at time of navigation
    tabIdx      int
}
```

`App` gains `navStack []navState`.

**Follow a link:**
1. Push `{activeProvider, currentItemIdx, activeTab}` onto `navStack`
2. Resolve `providerName` → `providerIdx` by iterating `app.providers` and matching `p.Name()`; if no match, show status bar error "No provider for: {name}" and abort
3. If the target provider's item list is not yet loaded, call `loadItems(providerIdx, "")` synchronously (or show "navigating…" and load async — see below)
4. Search loaded items for `item.ID == targetID`; if not found, show status bar error "Resource not found: {id}" and abort
5. Call `selectItem(providerIdx, foundItem)`

**Async loading on navigate**: if the target provider hasn't been loaded, run `ListItems` in a goroutine showing "navigating…" in the status bar, then continue with steps 4–5 inside `QueueUpdateDraw`.

**Esc (navStack pop)**: restore `activeProvider`, re-select the item at `itemIdx`, restore `activeTab`. Does not re-fetch (uses cached item list and tab cache).

### Mouse/keyboard handling on content TextView

Content TextView's `SetMouseCapture`: on `tview.MouseLeftClick`, tview internally updates the highlighted region for a regions-enabled TextView when the user clicks. After the click event, call `tv.GetHighlights()` (returns `[]string` of currently highlighted region IDs). If the first result starts with `"link:"`, parse `providerName:targetID` from the suffix and call `navigateTo()`. Tab regions (`"tab-"` prefix) will never appear here as tabs are rendered in the separate tabBar widget.

Keyboard: `Enter` on the content TextView calls `tv.GetHighlights()` and follows the first `"link:"` region if present. tview's built-in Tab/Backtab region cycling can be enabled to allow keyboard navigation between links.

### Link inventory

| Provider | Tab | Field | Target provider | Target ID |
|---|---|---|---|---|
| API Gateway | Routes | Lambda target (V2 direct) | Lambda | function name |
| API Gateway | Routes | Lambda target (V2 via integrations/{id}) | Lambda | function name (resolved) |
| API Gateway | Routes | Lambda target (V1 REST via method integration) | Lambda | function name (resolved) |
| Lambda | Overview | Role ARN | IAM Roles | role name (last ARN segment) |
| Lambda | Triggers | Source ARN | SQS or SNS | queue/topic name (last ARN segment) |
| CloudFormation | Resources | Logical ID (mapped types only) | see type map | physical resource ID |
| SQS | DLQ | DLQ ARN | SQS | queue URL derived from ARN: `https://sqs.{region}.amazonaws.com/{accountId}/{queueName}` |

**CloudFormation type → provider map** (unmapped types render as plain text, no link):

```
AWS::Lambda::Function       → Lambda          (PhysicalResourceId = function name; Item.ID = function name ✓)
AWS::SQS::Queue             → SQS             (PhysicalResourceId = queue URL; Item.ID = queue URL ✓)
AWS::SNS::Topic             → SNS             (PhysicalResourceId = topic ARN; Item.ID = topic ARN ✓)
AWS::S3::Bucket             → S3              (PhysicalResourceId = bucket name; Item.ID = bucket name ✓)
AWS::ApiGateway::RestApi    → API Gateway     (PhysicalResourceId = API ID; Item.ID = ApiId ✓)
AWS::ApiGatewayV2::Api      → API Gateway     (PhysicalResourceId = API ID; Item.ID = ApiId ✓)
AWS::IAM::Role              → IAM Roles       (PhysicalResourceId = role name; Item.ID = role name ✓)
AWS::SecretsManager::Secret → Secrets Manager (PhysicalResourceId = secret ARN; Item.ID = secret ARN ✓)
```

CloudFormation `PhysicalResourceId` is the deployed resource identifier. The `targetID` passed to `Link()` is the raw `PhysicalResourceId` — it matches `Item.ID` directly in each target provider (verified in the table above). No transformation required.

**Secrets Manager note**: `PhysicalResourceId` for a `AWS::SecretsManager::Secret` is consistently the secret ARN when the stack was deployed without an explicit `Name` property. ARN-only matching is acceptable; no name fallback is implemented.

**API Gateway Lambda URI parsing:**

V2 HTTP/WebSocket integration URI (direct Lambda):
```
arn:aws:lambda:{region}:{account}:function:{name}
```

V2 integration backend URI (Lambda proxy):
```
arn:aws:apigateway:{region}:lambda:path/2015-03-31/functions/arn:aws:lambda:{region}:{account}:function:{name}/invocations
```
Extract the embedded Lambda ARN by splitting on `functions/` and `/invocations`.

V1 REST integration URI format (same as V2 proxy backend URI above).

Function name = last segment of the extracted Lambda ARN (after final `:`).

### Files affected
- `internal/aws/format.go` — add `Link()`
- `internal/aws/apigateway.go` — expand `APIGatewayV2API` interface with `GetIntegration`; expand `APIGatewayV1API` interface with `GetMethod` and `GetIntegration`; Routes tab: resolve integrations, parse URI, emit `Link()`
- `internal/aws/lambda.go` — Overview: wrap role ARN; Triggers tab: wrap source ARN
- `internal/aws/cloudformation.go` — Resources tab: apply type map, emit `Link()` for mapped types; unmapped types render as plain text
- `internal/aws/sqs.go` — DLQ tab: wrap DLQ ARN using full queue URL as targetID (matches `Item.ID` which is also the queue URL)
- `internal/ui/app.go` — `navStack`, `navigateTo()`, `navigateBack()`, content TextView mouse/Enter handler

---

## Feature 4: S3 Object Browser

### Goal
Make the Objects tab rows navigable and actionable without loading more than 50 objects.

### Row selection state

`App` gains:
```go
selectedObjectRow int
cachedObjects     []s3Object  // parallel to tabCache for the Objects tab
```

`selectedObjectRow` resets to 0 in both `selectItem()` (item change) and `loadItems()` (provider/refresh). `cachedObjects` is also cleared in `loadItems()` to match the existing `tabLoaded`/`tabCache` reset pattern. On refresh, `selectedObjectRow` is clamped to `[0, len(cachedObjects)-1]` after reload.

The Objects tab fetch function populates both `tabCache[tabIdx]` (formatted string) and `app.cachedObjects`. Re-render highlights the selected row by re-building the table string with cyan on the selected row index.

### j/k in content pane

`j`/`k` move the selected object row **only when**:
- Focus is on the content TextView (Detail pane), AND
- The active tab index is the Objects tab index

This is implemented in the existing `tapp.SetInputCapture` handler in `keys.go` with those two guard conditions. After moving, re-render the Objects tab in-place (no re-fetch).

### Enter flow

```
Enter on selected object row (guard: focus=Detail, activeTab=Objects):

  size := cachedObjects[selectedObjectRow].Size
  name := cachedObjects[selectedObjectRow].Key
  isText := isTextFile(name)   // extension-only check (see below)

  if isText && size < 10MB:
      → download silently, show in Expand panel

  elif (isText && size >= 10MB) OR (not isText && size < 100MB):
      → status bar prompt: "Open {name}? [y/n]"
      → y: download, open (see below)
      → n: dismiss, restore status bar hints

  elif size >= 100MB:
      → status bar prompt: "File is {humanSize}. Download anyway? [y/n]"
      → y: download, open
      → n: dismiss
```

**Text detection** (extension only — `ListObjectsV2` does not return `Content-Type`):
```
.json .txt .yaml .yml .log .csv .xml .md .toml .ini .sh .env .conf .properties
```

**Open behaviour:**
- Text/JSON file → stream S3 object body to `os.CreateTemp("", "lazyaws-*")` via `io.Copy` (avoids full `[]byte` allocation), read file contents, show in Expand panel
- Binary file → stream to `os.CreateTemp("", "lazyaws-*")` via `io.Copy`, then `exec.Command("xdg-open", tmpPath)` in goroutine; on goroutine error, show "Failed to open: {err}" in status bar

**Download implementation**: `DownloadObject` streams the S3 response body to the temp file using `io.Copy`. Signature: `DownloadObject(ctx context.Context, bucketName, key string, w io.Writer) error`. This avoids holding large files in memory and applies to all file sizes.

**Temp file cleanup**: keep reference in `app.tmpFiles []string`; delete all on app exit via `defer`. Expand panel close does not delete the file (it may be open in an external application).

**Cache staleness**: size data comes from the cached object list (populated at Objects tab load time). If the actual object changes between tab load and Enter, the user may see a stale size. This is an accepted trade-off; no re-fetch is performed on Enter.

### Status bar prompt

Reuse the existing `statusPages` pattern. Add a third page `"prompt"` containing a `tview.TextView` with the prompt text and `SetInputCapture` for `y`/`n`. When showing the prompt, explicitly call `tapp.SetFocus(promptWidget)` (mirroring how `enterSearch()` calls `tapp.SetFocus(searchInput)`). After `y`/`n` response, switch back to `"hints"` page and restore focus to the content TextView.

### Files affected
- `internal/aws/s3.go` — expose `[]s3Object` slice from Objects tab fetch; add `DownloadObject(ctx context.Context, bucketName, key string, w io.Writer) error` method (streaming)
- `internal/ui/app.go` — `selectedObjectRow`, `cachedObjects`, `tmpFiles`; Enter handler; `openObject()`; `showPrompt()`/`hidePrompt()`
- `internal/ui/panels.go` — add `"prompt"` page to `statusPages`

---

## Shared Infrastructure Summary

| Component | Change |
|---|---|
| `internal/aws/provider.go` | Add `Expandable` interface |
| `internal/aws/format.go` | Add `Link()` helper |
| `internal/ui/panels.go` | Split right column into tabBar + detail + expand Flex; add prompt page to statusPages |
| `internal/ui/app.go` | `navStack`, `expandVisible`, `selectedObjectRow`, `cachedObjects`, `tmpFiles`; unified Esc priority; Enter handler; mouse capture on content and tabBar |

---

## Verification

1. **Tab UX**: Click each tab with mouse — confirm direct jump. Active tab renders aqua bold+underline with ASCII underline row. Keyboard `[`/`]` still works.
2. **IAM Policies**: Navigate to "IAM Policies" provider → select a policy → Document tab shows JSON. Press `Enter` → Expand panel opens with policy JSON. Press `Esc` → panel hides, focus returns to Items.
3. **Cross-resource linking**: API Gateway → Routes tab → click a Lambda link → provider switches to Lambda, correct function selected. Press `Esc` → returns to API Gateway. Test CloudFormation → Lambda, Lambda → IAM Role, SQS DLQ → SQS. Verify unmapped CloudFormation types render as plain text (no link). Verify missing provider/ID shows status bar error rather than panic.
4. **S3 object browser**: Select S3 bucket → Objects tab → navigate rows with `j`/`k` → cyan highlight moves. Press `Enter` on `.json` < 10MB → Expand panel shows content, no prompt. Press `Enter` on binary file → prompt appears, `y` downloads and invokes xdg-open. Press `Enter` on file ≥ 100MB → size warning prompt. Press `n` → dismisses.
5. **Esc priority**: With Expand open + navStack non-empty → first Esc hides Expand, second Esc pops navStack.
6. **Tests**: `go test ./...` — all existing tests pass. New IAM Policies provider has table-driven tests mirroring `iam_test.go`.
