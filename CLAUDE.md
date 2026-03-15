# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

`lazyaws` is a terminal UI for browsing AWS resources, inspired by lazygit/lazydocker. Built with Go using `tview` for the TUI and `aws-sdk-go-v2` for AWS access.

## Commands

```bash
# Build and run
go build -o lazyaws . && ./lazyaws

# Run against LocalStack (http://localhost:4566)
go run . -local

# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/aws/...

# Run a single test
go test ./internal/aws/ -run TestS3Provider_ListItems
```

## Architecture

```
main.go                     — wires providers + starts TUI
internal/aws/
  provider.go               — Provider interface + Item/TabDef types
  config.go                 — AWS config loader (default credential chain)
  format.go                 — KV() and Table() helpers for detail pane output
  <service>.go              — one file per AWS service (s3, lambda, sns, ...)
internal/ui/
  app.go                    — App struct: orchestrates provider/item/tab selection
  panels.go                 — tview widget construction + panel focus cycling
  keys.go                   — global keyboard bindings
```

### Provider Interface

`internal/aws/provider.go` defines the extension point:

```go
type Provider interface {
    Name() string
    ListItems(ctx context.Context) ([]Item, error)
    GetDetail(ctx context.Context, item Item) (string, error)  // legacy
    Tabs() []TabDef
}
```

Each service file defines a narrow interface over the SDK client (e.g. `S3API`), enabling constructor injection for tests (`NewS3ProviderWithClient(client S3API)`). `GetDetail` delegates to the first tab's fetch func and is kept for compatibility.

### Adding a New Provider

1. Create `internal/aws/<service>.go` — define a `<Service>API` interface, implement `Provider`, add `New<Service>Provider(cfg, local bool)` and `New<Service>ProviderWithClient(client)`.
2. Register in `main.go` — append `awspkg.New<Service>Provider(cfg, *local)` to the `providers` slice.
3. Use `KV()` for key-value output and `Table()` for tabular output in tab fetch funcs.

When asked to add a new provider (e.g. "add a lazyaws provider for DynamoDB"), follow this checklist in order:

- **Clarify before coding**: confirm the list API call, ID/Name fields, and desired tabs + their content. Do not write code until these are agreed.
- **Service file** (`internal/aws/<service>.go`):
  - Define a narrow `<Service>API` interface (only methods the provider calls).
  - Implement `Provider`: `Name()`, `ListItems()`, `GetDetail()` (delegates to first tab), `Tabs()`.
  - Two constructors: `New<Service>Provider(cfg aws.Config, local bool)` and `New<Service>ProviderWithClient(client <Service>API)`.
  - LocalStack endpoint override via `o.BaseEndpoint = aws.String("http://localhost:4566")` inside the `local` guard.
  - `GetDetail` must call the first `TabDef.Fetch`, not duplicate logic.
- **Register**: add `awspkg.New<Service>Provider(cfg, *local)` to the `providers` slice in `main.go`.
- **Tests** (`internal/aws/<service>_test.go`): table-driven, implement the narrow interface directly in the test file (no mocking library). Mirror the structure of `s3_test.go`.
- **go.mod**: if the SDK service package is new, run `go get github.com/aws/aws-sdk-go-v2/service/<service>` and commit the updated `go.mod`/`go.sum`.

### UI Layout

Three panels in a flex layout: **Resources** (left-top, provider list) → **Items** (left-bottom, resource list) → **Detail** (right, tabbed content). Tab cycling is `Tab`/`Shift+Tab`; detail tabs use `[`/`]`. All AWS fetches are async via goroutines + `tview.QueueUpdateDraw`.

### Testing Pattern

Tests use constructor injection via the `<Service>API` interface. Table-driven tests are standard. No mocking framework — implement the interface directly in test files.

### LocalStack

Pass `-local` flag to redirect all providers to `http://localhost:4566` with path-style addressing.
