# Search Feature Design

**Date:** 2026-03-15
**Status:** Approved

## Overview

Add a `/`-triggered search mode to lazyaws that filters the current provider's item list by passing the query string to the provider's list call. Works from any panel focus. Esc clears search and restores the full list.

## UI Layer

### Status area swap via `tview.Pages`

The bottom row of the outer flex currently holds a `tview.TextView` (`status`) at fixed height 1. This is replaced with a `tview.Pages` (`statusPages`) also at height 1, containing two named pages:

- `"hints"` — the existing `tview.TextView` with key binding hints
- `"search"` — a `tview.InputField` labelled `"/ "`

`panels` gains two new fields:
```go
searchInput  *tview.InputField
statusPages  *tview.Pages
```

`newPanels()` constructs both, populates `statusPages`, and returns `statusPages` as the widget to be placed in the layout (not `status` directly). `app.go`'s `build()` uses `a.panels.statusPages` in the outer flex instead of `a.panels.status`.

The `status` field is retained; `statusPages` wraps it alongside `searchInput`.

### Key event routing in tview

`tview.Application.SetInputCapture` fires for all key events **after** the focused primitive's own handlers have run. `tview.InputField` consumes rune events (including `j`, `k`, `[`, `]`, `q`, `r`, `/`) before they reach the global capture. The InputField-focused guard on `/` (and Tab/Shift+Tab) is **required** regardless — it is the definitive protection and must not be treated as optional. If tview forwards runes from InputField to global capture, the guard prevents double-firing; if tview consumes them, the guard is a no-op safety net.

`tcell.KeyTab` and `tcell.KeyBacktab` are **not** consumed by `tview.InputField`; the global capture will fire them. The InputField-focused guard must be applied to the `Tab`/`Shift+Tab` handlers to prevent `panels.focused` being mutated while search is active.

`tcell.KeyEscape` via `SetDoneFunc`: `SetDoneFunc` is invoked with the termination key (`tview.KeyEnter` or `tview.KeyEscape`) and the implementation must branch on the key value — `tview.InputField` has a single `SetDoneFunc` callback, not separate callbacks per key. `clearSearch` is called only from `SetDoneFunc` (Esc branch); `executeSearch` from the Enter branch. The global capture does not handle Escape — this avoids double-firing.

### InputField focus guard pattern

At the top of the global `SetInputCapture` handler, add:

```go
searchActive := a.tapp.GetFocus() == a.panels.searchInput
```

All bindings that must not fire during search — `/`, `Tab`, `Shift+Tab`, and all rune cases — check `searchActive` and return `event` unchanged if true.

### Search lifecycle

**Enter search mode (`/` pressed globally):**
- Guard: skip if `searchActive` (InputField already focused).
- Record the current focus index: `a.preFocusIdx = a.panels.focused`.
- `statusPages.SwitchToPage("search")`
- `tapp.SetFocus(searchInput)`

`preFocusIdx int` is a field on `App`. It reads `panels.focused` at the moment search is entered; `panels.focused` is not mutated during search (Tab/Shift+Tab are suppressed while InputField is focused). On exit, `panels.focused` is restored to `preFocusIdx` and focus is set to `panels.primitives()[preFocusIdx]`.

**Execute search (Enter in InputField — via `SetDoneFunc`, `tview.KeyEnter` branch):**
- Read query from InputField.
- `statusPages.SwitchToPage("hints")`
- Update hints text to show `search: <query>  [esc: clear]`.
- Call `loadItems(activeProvider, query)`.
- Restore: `panels.focused = preFocusIdx`, `tapp.SetFocus(panels.primitives()[preFocusIdx])`.

**Clear search (Esc in InputField — via `SetDoneFunc`, `tview.KeyEscape` branch):**
- `statusPages.SwitchToPage("hints")`
- Restore original hints text via `resetHints()`.
- Clear InputField text.
- Call `loadItems(activeProvider, "")`.
- Restore focus as above.

**Provider switch while search is active (user selects a different resource in Resources panel):**
- The `AddItem` callback in `build()` calls `loadItems(i, "")` — always passing empty query. Switching providers resets the search. `resetHints()` is also called here to clear stale search hints text.

**`r` (refresh) while search is active:**
- `refresh()` calls `resetHints()` synchronously, then `loadItems(activeProvider, "")`. There is a brief cosmetic window where the hints show "normal" while the items list is still showing filtered results (since `loadItems` is async). This is acceptable and expected.

### Stale tab/detail state on search

`loadItems` already clears `a.panels.items` and sets the detail pane to `"Loading..."`. In addition, when `loadItems` is called with any query (including `""`), `tabLoaded`, `tabCache`, and `currentItem` must be reset to zero values before the goroutine fires. This prevents `renderDetail` from showing stale cached content if called before the user selects a new item from the filtered list.

```go
func (a *App) loadItems(i int, query string) {
    a.tabLoaded = nil
    a.tabCache = nil
    a.currentItem = awspkg.Item{}
    // ... existing goroutine
}
```

### Status bar hint text

Normal mode (constant `hintsText` in `panels.go`):
```
Tab/S-Tab: panel   j/k: navigate   [·]: tab   /: search   r: refresh   q: quit
```

Active filter mode:
```
search: <query>  [esc: clear]
```

`resetHints()` on `App` sets `a.panels.status.SetText(hintsText)` and is called from `clearSearch`, `refresh`, and the provider-switch callback in `build()`.

### Go loop variable capture

The `build()` loop that registers resource providers uses a closure over the loop variable `i`. The module declares `go 1.26.1` in `go.mod`, which is ≥ 1.22, so per-iteration loop variable semantics apply and there is no capture bug. No `i := i` shadow is needed.

### `query` value capture in goroutine

`query string` is passed by value to `loadItems` and captured by value in the goroutine closure. Go strings are immutable value types; no copy or pointer concern.

## Provider Interface

### Signature change

```go
// Before
ListItems(ctx context.Context) ([]Item, error)

// After
ListItems(ctx context.Context, query string) ([]Item, error)
```

Empty string means no filter — returns all items, preserving current behaviour.

### Filtering strategy

Most AWS list APIs do not support server-side name filtering. All providers apply **case-insensitive client-side contains** after fetching the full list:
```go
strings.Contains(strings.ToLower(item.Name), strings.ToLower(query))
```
This is consistent and simple. The filter is applied inside each provider's `ListItems` implementation, not at the call site in `app.go`.

Providers affected (all existing ones):
- `s3`, `lambda`, `sns`, `sqs`, `secretsmanager`, `iam`, `route53`, `acm`, `apigateway`, `apigatewayv2`, `cloudformation`

### Call sites in app.go

`loadItems` gains a `query string` parameter:
```go
func (a *App) loadItems(i int, query string)
```

All call sites:
| Location | Query passed |
|----------|-------------|
| `build()` initial load | `""` |
| `build()` resource-switch callback | `""` (resets search on provider switch) |
| `refresh()` | `""` (resets search) |
| `executeSearch()` | typed query string |
| `clearSearch()` | `""` |

## Files Changed

| File | Change |
|------|--------|
| `internal/ui/panels.go` | Add `searchInput`, `statusPages`; construct Pages; extract `hintsText` constant |
| `internal/ui/app.go` | Add `preFocusIdx`; add `enterSearch`, `executeSearch`, `clearSearch`, `resetHints`; update `loadItems` signature + all call sites (including `build()` callbacks and stale-state reset); update outer flex to use `panels.statusPages` |
| `internal/ui/keys.go` | Add `/` binding with `searchActive` guard; add `searchActive` guard to Tab/Shift+Tab handlers |
| `internal/aws/provider.go` | Update `Provider` interface `ListItems` signature |
| `internal/aws/*.go` (all providers) | Update `ListItems` signature + add client-side contains filter |
| `internal/aws/*_test.go` | Update test calls to pass query string |

## Out of Scope

- Server-side SDK filtering (APIs don't uniformly support it)
- Fuzzy matching
- Search across multiple providers simultaneously
- Search history
