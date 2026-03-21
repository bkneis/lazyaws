package aws

import (
	"context"
	"strings"
)

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
	// ListItems returns the top-level list of resources, optionally filtered by name.
	// An empty query returns all items.
	ListItems(ctx context.Context, query string) ([]Item, error)
	// GetDetail returns a formatted string for the detail panel (legacy, kept for compat).
	GetDetail(ctx context.Context, item Item) (string, error)
	// Tabs returns the tab definitions for the detail pane.
	Tabs() []TabDef
}

// LinkNavigator is an optional capability a Provider may implement to fetch
// a single resource directly by its ID, bypassing a full list scan.
// navigateTo in the UI uses this when available.
type LinkNavigator interface {
	FetchItem(ctx context.Context, id string) (Item, error)
}

// ColorTags holds tview markup tags for color-rendering in provider output.
// Set ActiveTags once at startup before the TUI starts.
type ColorTags struct {
	Header string // e.g. "[cyan]" — KV keys and table headers
	Link   string // e.g. "[aqua::u]" — cross-resource links
}

// ActiveTags is the package-level color scheme used by KV, Table, and Link.
// Defaults to the original cyan/aqua scheme.
var ActiveTags = ColorTags{Header: "[cyan]", Link: "[aqua::u]"}

// filterItems returns items whose Name contains query (case-insensitive).
// Returns items unchanged when query is empty.
func filterItems(items []Item, query string) []Item {
	if query == "" {
		return items
	}
	q := strings.ToLower(query)
	var out []Item
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Name), q) {
			out = append(out, item)
		}
	}
	return out
}

// ActionContext is passed to action funcs to interact with the UI.
// All methods are safe to call from goroutines — they schedule via QueueUpdateDraw.
type ActionContext interface {
	// Confirm shows a Yes/No dialog. Default focused button is No.
	Confirm(message string, onConfirm func())
	// ConfirmDelete shows a delete dialog requiring the user to type "delete me"
	// before the Delete button is enabled. Default focus is Cancel.
	ConfirmDelete(resourceName string, onConfirm func())
	// PromptInput shows a single-field input dialog.
	PromptInput(label string, placeholder string, onSubmit func(value string))
	// ShowError displays an error modal.
	ShowError(err error)
	// ShowInfo displays an informational message modal.
	ShowInfo(message string)
	// Refresh reloads the current provider's item list.
	Refresh()
	// OpenMultiGroupPicker opens a multi-select modal showing all loaded log groups.
	// onConfirm is called with the confirmed selection; the App activates the Tail tab.
	OpenMultiGroupPicker(onConfirm func(selected []string))
	// SuspendAndRun suspends the TUI, runs fn synchronously (blocking), then resumes.
	// Use for launching external processes (e.g. $EDITOR) that need terminal control.
	SuspendAndRun(fn func())
}

// ActionDef describes a single operation shown in the x actions menu.
type ActionDef struct {
	Label string // e.g. "Delete bucket"
	Key   rune   // optional shortcut shown in menu; 0 = none
	Func  func(ctx context.Context, item Item, ac ActionContext) error
}

// Actionable is an optional interface providers implement to expose write ops.
// Actions is called with the currently selected item (or Item{} if none selected).
// Returning nil/empty means no menu is shown.
type Actionable interface {
	Actions(item Item) []ActionDef
}
