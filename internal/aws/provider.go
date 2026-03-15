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
