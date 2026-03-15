package ui

import (
	"context"
	"fmt"
	"strings"

	awspkg "github.com/bryanl/lazyaws/internal/aws"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// navState records the TUI position before following a cross-resource link.
type navState struct {
	providerIdx int
	itemIdx     int
	tabIdx      int
}

// App is the root TUI application.
type App struct {
	tapp              *tview.Application
	panels            *panels
	providers         []awspkg.Provider
	loadedItems       []awspkg.Item // mirrors items list for Enter handler
	activeProvider    int
	activeTab         int
	tabLoaded         []bool
	tabCache          []string
	currentItem       awspkg.Item
	preFocusIdx       int
	tabBarOffsets     []int // display-column start per tab (for mouse click)
	expandVisible     bool  // whether expand panel is shown
	navStack          []navState
	selectedObjectRow int
	cachedObjects     []awspkg.S3ObjectItem
	tmpFiles          []string // temp files to clean up on exit (used in Task 10)
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
			a.panels.searchInput.SetText("")
			a.panels.statusPages.SwitchToPage("hints")
			a.resetHints()
			a.loadItems(i, "")
		})
	}

	if len(a.providers) > 0 {
		a.activeProvider = 0
		a.loadItems(0, "")
	}

	a.panels.searchInput.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			a.executeSearch(a.panels.searchInput.GetText())
		case tcell.KeyEscape:
			a.clearSearch()
		}
	})

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

	leftCol := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.panels.resources, 0, 1, true).
		AddItem(a.panels.items, 0, 2, false)

	layout := tview.NewFlex().
		AddItem(leftCol, 25, 0, true).
		AddItem(a.panels.rightFlex, 0, 1, false)

	// Wire tabBar mouse capture for clickable tabs
	a.panels.tabBar.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			screenCol, _ := event.Position()            // screen-absolute column
			widgetX, _, _, _ := a.panels.tabBar.GetRect() // widget's left edge on screen
			a.selectTabByColumn(screenCol - widgetX)
			return action, nil
		}
		return action, event
	})

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

	outer := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(layout, 0, 1, true).
		AddItem(a.panels.statusPages, 1, 0, false)

	setupKeys(a)

	a.tapp.EnableMouse(true)
	a.tapp.SetRoot(outer, true).SetFocus(a.panels.resources)
}

// loadItems fetches items for provider i in a background goroutine.
// Pass query="" to load all items with no filter.
func (a *App) loadItems(i int, query string) {
	a.tabLoaded = nil
	a.tabCache = nil
	a.loadedItems = nil
	a.activeTab = 0
	a.currentItem = awspkg.Item{}
	a.selectedObjectRow = 0
	a.cachedObjects = nil
	a.panels.items.Clear()
	a.panels.detail.SetText("Loading...")

	go func() {
		items, err := a.providers[i].ListItems(context.Background(), query)
		a.tapp.QueueUpdateDraw(func() {
			a.panels.items.Clear()
			a.panels.detail.Clear()
			a.loadedItems = items // assign before error check so it's always set

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

// enterSearch activates the search input, saving current focus to restore later.
func (a *App) enterSearch() {
	a.preFocusIdx = a.panels.focused
	a.panels.statusPages.SwitchToPage("search")
	a.tapp.SetFocus(a.panels.searchInput)
}

// executeSearch runs a search with the given query and restores panel focus.
func (a *App) executeSearch(query string) {
	a.panels.searchInput.SetText("")
	a.panels.statusPages.SwitchToPage("hints")
	a.panels.status.SetText(" [cyan]search:[-] " + query + "   [cyan]esc[-]: clear")
	a.loadItems(a.activeProvider, query)
	a.restoreFocus()
}

// clearSearch cancels search mode, clears the input, and reloads the full list.
func (a *App) clearSearch() {
	a.panels.searchInput.SetText("")
	a.panels.statusPages.SwitchToPage("hints")
	a.resetHints()
	a.loadItems(a.activeProvider, "")
	a.restoreFocus()
}

// resetHints restores the status bar to the default key-binding hints.
func (a *App) resetHints() {
	a.panels.status.SetText(hintsText)
}

// restoreFocus returns focus to the panel that was active before search.
func (a *App) restoreFocus() {
	a.panels.focused = a.preFocusIdx
	a.tapp.SetFocus(a.panels.primitives()[a.preFocusIdx])
}

// selectItem resets tab state and loads the first tab for the selected item.
func (a *App) selectItem(providerIdx int, item awspkg.Item) {
	a.currentItem = item
	a.activeTab = 0
	a.selectedObjectRow = 0
	a.cachedObjects = nil
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
			// Cache S3 objects for row selection when the Objects tab finishes loading.
			if s3p, ok := a.providers[providerIdx].(*awspkg.S3Provider); ok {
				tabs := a.providers[providerIdx].Tabs()
				if tabIdx < len(tabs) && tabs[tabIdx].Label == "Objects" {
					a.cachedObjects = s3p.GetLastObjects()
				}
			}
			if a.activeTab == tabIdx {
				a.renderDetail()
			}
		})
	}()
}

// renderDetail writes the tab bar to tabBar widget and content to detail widget.
func (a *App) renderDetail() {
	tabs := a.providers[a.activeProvider].Tabs()
	if len(tabs) == 0 {
		return
	}
	a.renderTabBar(tabs)
	content := "  ... fetching"
	if a.activeTab < len(a.tabLoaded) && a.tabLoaded[a.activeTab] {
		if len(a.cachedObjects) > 0 && tabs[a.activeTab].Label == "Objects" {
			content = a.renderObjectsWithHighlight()
		} else {
			content = a.tabCache[a.activeTab]
		}
	}
	a.panels.detail.SetText(content).ScrollToBeginning()
}

// renderObjectsWithHighlight rebuilds the Objects tab table with the selected
// row highlighted in aqua.
func (a *App) renderObjectsWithHighlight() string {
	if len(a.cachedObjects) == 0 {
		return a.tabCache[a.activeTab]
	}
	headers := []string{"Key", "Size", "Last Modified"}
	rows := make([][]string, len(a.cachedObjects))
	for i, obj := range a.cachedObjects {
		rows[i] = []string{obj.Key, obj.SizeFormatted, obj.LastModified}
	}

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

// isS3ObjectsTabFocused returns true when focus is on the detail pane,
// the active provider is S3, the active tab is Objects, and objects are cached.
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

// moveObjectRow adjusts selectedObjectRow by delta (wraps) and re-renders.
func (a *App) moveObjectRow(delta int) {
	n := len(a.cachedObjects)
	if n == 0 {
		return
	}
	a.selectedObjectRow = (a.selectedObjectRow + delta + n) % n
	a.renderDetail()
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

// nextTab advances to the next tab, fetching if not yet loaded.
func (a *App) nextTab() {
	tabs := a.providers[a.activeProvider].Tabs()
	if len(tabs) == 0 || len(a.tabLoaded) == 0 {
		return
	}
	a.selectTab((a.activeTab + 1) % len(tabs))
}

// prevTab retreats to the previous tab, fetching if not yet loaded.
func (a *App) prevTab() {
	tabs := a.providers[a.activeProvider].Tabs()
	if len(tabs) == 0 || len(a.tabLoaded) == 0 {
		return
	}
	n := len(tabs)
	a.selectTab((a.activeTab + n - 1) % n)
}

// refresh reloads the currently active provider's item list with no filter.
func (a *App) refresh() {
	a.panels.searchInput.SetText("")
	a.panels.statusPages.SwitchToPage("hints")
	a.resetHints()
	a.loadItems(a.activeProvider, "")
}

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

// handleEsc implements the Esc priority: expand > navStack > pass-through.
// Returns true if the event was consumed.
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
	// Item selection is reset to first item by loadItems; user re-selects manually
	return true
}

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

// Run starts the tview event loop.
func (a *App) Run() error {
	return a.tapp.Run()
}
