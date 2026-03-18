package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	awspkg "github.com/bryanl/lazyaws/internal/aws"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type linkRef struct{ provider, targetID string }

var linkMarkerRe = regexp.MustCompile("\x02([^\x03]+)\x03")

// parseLinkContent strips \x02...\x03 markers from content, returning
// cleaned displayable text and the ordered list of extracted links.
func parseLinkContent(content string) (string, []linkRef) {
	var links []linkRef
	for _, m := range linkMarkerRe.FindAllStringSubmatch(content, -1) {
		if parts := strings.SplitN(m[1], ":", 2); len(parts) == 2 {
			links = append(links, linkRef{provider: parts[0], targetID: parts[1]})
		}
	}
	return linkMarkerRe.ReplaceAllString(content, ""), links
}

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
	theme             Theme
	providers         []awspkg.Provider
	loadedItems       []awspkg.Item // mirrors items list for Enter handler
	activeProvider    int
	activeTab         int
	tabLoaded         []bool
	tabCache          []string
	currentItem       awspkg.Item
	preFocusIdx       int
	tabBarOffsets     []int       // display-column start per tab (for mouse click)
	tabLinks          [][]linkRef // per-tab extracted links, parallel to tabCache
	navStack           []navState
	selectedObjectRow  int
	cachedObjects      []awspkg.S3ObjectItem
	selectedDynamoRow  int
	cachedDynamoRows   []awspkg.DynamoDBItemRow
	dynamoHeaders      []string
	tmpFiles           []string // temp files to clean up on exit
	// CW Logs live tail state
	cachedCWLogStreams  []awspkg.CWLogStreamRow
	selectedCWStreamRow int
	cwTailEvents        []cwTailEvent
	cwTailCancel        context.CancelFunc
}

type cwTailEvent struct {
	ts     time.Time
	group  string
	stream string
	msg    string
}

// NewApp constructs the App with the given resource providers and color theme.
func NewApp(providers []awspkg.Provider, theme Theme) *App {
	a := &App{
		tapp:      tview.NewApplication(),
		panels:    newPanels(theme),
		theme:     theme,
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

	// Wire mouse click on detail to select S3 object rows or DynamoDB item rows.
	a.panels.detail.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			tabs := a.providers[a.activeProvider].Tabs()
			_, screenY := event.Position()
			_, widgetY, _, _ := a.panels.detail.GetRect()
			scrollRow, _ := a.panels.detail.GetScrollOffset()
			contentRow := (screenY - widgetY) + scrollRow

			if _, ok := a.providers[a.activeProvider].(*awspkg.S3Provider); ok {
				if a.activeTab < len(tabs) && tabs[a.activeTab].Label == "Objects" && len(a.cachedObjects) > 0 {
					objIdx := contentRow - 2 // skip header + separator rows
					if objIdx >= 0 && objIdx < len(a.cachedObjects) {
						a.selectedObjectRow = objIdx
						a.moveObjectRow(0)
						a.panels.focused = 2
						a.tapp.SetFocus(a.panels.detail)
					}
					return action, nil
				}
			}

			if _, ok := a.providers[a.activeProvider].(*awspkg.DynamoDBProvider); ok {
				if a.activeTab < len(tabs) && tabs[a.activeTab].Label == "Items" && len(a.cachedDynamoRows) > 0 {
					// 3 = page header line + blank line + table header; 4 = + separator line
					rowIdx := contentRow - 4
					if rowIdx >= 0 && rowIdx < len(a.cachedDynamoRows) {
						a.selectedDynamoRow = rowIdx
						a.moveDynamoItemRow(0)
						a.panels.focused = 2
						a.tapp.SetFocus(a.panels.detail)
					}
					return action, nil
				}
			}

			if _, ok := a.providers[a.activeProvider].(*awspkg.CloudWatchLogsProvider); ok {
				if a.activeTab < len(tabs) && tabs[a.activeTab].Label == "Streams" && len(a.cachedCWLogStreams) > 0 {
					rowIdx := contentRow - 2 // skip header + separator rows
					if rowIdx >= 0 && rowIdx < len(a.cachedCWLogStreams) {
						a.selectedCWStreamRow = rowIdx
						a.moveCWStreamRow(0)
						a.panels.focused = 2
						a.tapp.SetFocus(a.panels.detail)
					}
					return action, nil
				}
			}
		}
		return action, event
	})

	// Wire Enter on detail to follow the first cross-resource link.
	a.panels.detail.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			if a.followFirstLink() {
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
	a.stopCWTailStream()
	a.cachedCWLogStreams = nil
	a.selectedCWStreamRow = 0
	a.tabLoaded = nil
	a.tabCache = nil
	a.loadedItems = nil
	a.activeTab = 0
	a.currentItem = awspkg.Item{}
	a.selectedObjectRow = 0
	a.cachedObjects = nil
	a.selectedDynamoRow = 0
	a.cachedDynamoRows = nil
	a.dynamoHeaders = nil
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
			if len(items) > 0 {
				a.panels.focused = 1
				a.tapp.SetFocus(a.panels.items)
				a.selectItem(i, items[0])
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
	ht := a.theme.HeaderTag
	a.panels.status.SetText(" " + ht + "search:[-] " + query + "   " + ht + "esc[-]: clear")
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
	a.panels.status.SetText(a.panels.hintsText)
}

// restoreFocus returns focus to the panel that was active before search.
func (a *App) restoreFocus() {
	a.panels.focused = a.preFocusIdx
	a.tapp.SetFocus(a.panels.primitives()[a.preFocusIdx])
}

// selectItem resets tab state and loads the first tab for the selected item.
func (a *App) selectItem(providerIdx int, item awspkg.Item) {
	a.stopCWTailStream()
	a.cachedCWLogStreams = nil
	a.selectedCWStreamRow = 0
	a.currentItem = item
	a.activeTab = 0
	a.selectedObjectRow = 0
	a.cachedObjects = nil
	a.selectedDynamoRow = 0
	a.cachedDynamoRows = nil
	a.dynamoHeaders = nil
	tabs := a.providers[providerIdx].Tabs()
	a.tabLoaded = make([]bool, len(tabs))
	a.tabCache = make([]string, len(tabs))
	a.tabLinks = make([][]linkRef, len(tabs))
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
			if a.activeProvider != providerIdx {
				return // provider changed while fetch was in-flight
			}
			if err != nil {
				a.tabCache[tabIdx] = fmt.Sprintf("[red]Error: %v[-]", err)
				a.tabLinks[tabIdx] = nil
			} else {
				cleaned, links := parseLinkContent(content)
				a.tabCache[tabIdx] = cleaned
				a.tabLinks[tabIdx] = links
			}
			a.tabLoaded[tabIdx] = true
			// Cache S3 objects for row selection when the Objects tab finishes loading.
			if s3p, ok := a.providers[providerIdx].(*awspkg.S3Provider); ok {
				if tabIdx < len(tabs) && tabs[tabIdx].Label == "Objects" {
					a.cachedObjects = s3p.GetLastObjects()
					if len(a.cachedObjects) > 0 {
						s3p.SetSelectedObject(a.cachedObjects[0].Key, a.cachedObjects[0].Size)
					}
				}
			}
			// Cache DynamoDB items for row selection when the Items tab finishes loading.
			if dbp, ok := a.providers[providerIdx].(*awspkg.DynamoDBProvider); ok {
				if tabIdx < len(tabs) && tabs[tabIdx].Label == "Items" {
					a.cachedDynamoRows, a.dynamoHeaders = dbp.GetCurrentItems()
					a.selectedDynamoRow = 0
				}
			}
			// Cache CW Logs streams for row selection when the Streams tab finishes loading.
			if cwlp, ok := a.providers[providerIdx].(*awspkg.CloudWatchLogsProvider); ok {
				if tabIdx < len(tabs) && tabs[tabIdx].Label == "Streams" {
					a.cachedCWLogStreams = cwlp.GetLastStreams()
					a.selectedCWStreamRow = 0
				}
			}
			if a.activeTab == tabIdx {
				a.renderDetail()
				if a.panels.detail.HasFocus() {
					a.tapp.Sync() // force full terminal resync to fix left-panel rendering artifacts when detail is focused
				}
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

	// CW Logs Tail tab — rendered independently of tab load state.
	if a.isCWLogsTailActive() {
		a.panels.detail.SetText(a.renderCWLogTail()).ScrollToEnd()
		return
	}

	content := "  ... fetching"
	if a.activeTab < len(a.tabLoaded) && a.tabLoaded[a.activeTab] {
		switch {
		case len(a.cachedObjects) > 0 && tabs[a.activeTab].Label == "Objects":
			content = a.renderObjectsWithHighlight()
		case len(a.cachedDynamoRows) > 0 && tabs[a.activeTab].Label == "Items":
			content = a.renderDynamoItemsWithHighlight()
		case len(a.cachedCWLogStreams) > 0 && tabs[a.activeTab].Label == "Streams":
			content = a.renderCWStreamsWithHighlight()
		default:
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
	fmt.Fprintf(&sb, "  %s%s[-]\n  ", a.theme.HeaderTag, strings.Join(padded, ""))
	for _, w := range widths {
		sb.WriteString(strings.Repeat("─", w) + "  ")
	}
	sb.WriteString("\n")
	for i, row := range rows {
		if i == a.selectedObjectRow {
			sb.WriteString("  " + a.theme.HighlightTag)
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
	if _, ok := a.providers[a.activeProvider].(*awspkg.S3Provider); !ok {
		return false
	}
	tabs := a.providers[a.activeProvider].Tabs()
	if a.activeTab >= len(tabs) || tabs[a.activeTab].Label != "Objects" {
		return false
	}
	return len(a.cachedObjects) > 0
}

// isDynamoItemsTabFocused returns true when focus is on the detail pane,
// the active provider is DynamoDB, the active tab is Items, and items are cached.
func (a *App) isDynamoItemsTabFocused() bool {
	if a.tapp.GetFocus() != a.panels.detail {
		return false
	}
	if _, ok := a.providers[a.activeProvider].(*awspkg.DynamoDBProvider); !ok {
		return false
	}
	tabs := a.providers[a.activeProvider].Tabs()
	if a.activeTab >= len(tabs) || tabs[a.activeTab].Label != "Items" {
		return false
	}
	return len(a.cachedDynamoRows) > 0
}

// renderDynamoItemsWithHighlight rebuilds the Items tab table with the selected row highlighted.
func (a *App) renderDynamoItemsWithHighlight() string {
	if len(a.cachedDynamoRows) == 0 {
		return a.tabCache[a.activeTab]
	}

	// Compute column widths.
	widths := make([]int, len(a.dynamoHeaders))
	for i, h := range a.dynamoHeaders {
		widths[i] = len(h)
	}
	for _, row := range a.cachedDynamoRows {
		for i, cell := range row.Cells {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	padded := make([]string, len(a.dynamoHeaders))
	for i, h := range a.dynamoHeaders {
		padded[i] = fmt.Sprintf("%-*s", widths[i]+2, h)
	}

	var sb strings.Builder
	dbp := a.providers[a.activeProvider].(*awspkg.DynamoDBProvider)
	fmt.Fprintf(&sb, "  Items  (page %d, %d items)\n\n", dbp.ScanPage(), len(a.cachedDynamoRows))
	fmt.Fprintf(&sb, "  %s%s[-]\n  ", a.theme.HeaderTag, strings.Join(padded, ""))
	for _, w := range widths {
		sb.WriteString(strings.Repeat("─", w) + "  ")
	}
	sb.WriteString("\n")

	for i, row := range a.cachedDynamoRows {
		if i == a.selectedDynamoRow {
			sb.WriteString("  " + a.theme.HighlightTag)
		} else {
			sb.WriteString("  ")
		}
		for j, cell := range row.Cells {
			if j < len(widths) {
				fmt.Fprintf(&sb, "%-*s", widths[j]+2, cell)
			}
		}
		if i == a.selectedDynamoRow {
			sb.WriteString("[-]")
		}
		sb.WriteString("\n")
	}

	prevHint := "p: prev page"
	if !dbp.HasPrevPage() {
		prevHint = "[::d]p: prev page[::-]"
	}
	nextHint := "n: next page"
	if !dbp.HasNextPage() {
		nextHint = "[::d]n: next page[::-]"
	}
	fmt.Fprintf(&sb, "\n  [%s]  [%s]  [Enter: expand item]\n", prevHint, nextHint)
	return sb.String()
}

// moveDynamoItemRow adjusts selectedDynamoRow by delta (clamped, no wrap) and re-renders.
func (a *App) moveDynamoItemRow(delta int) {
	n := len(a.cachedDynamoRows)
	if n == 0 {
		return
	}
	a.selectedDynamoRow += delta
	if a.selectedDynamoRow < 0 {
		a.selectedDynamoRow = 0
	}
	if a.selectedDynamoRow >= n {
		a.selectedDynamoRow = n - 1
	}
	a.renderDetail()
}

// advanceDynamoPage loads the next or previous scan page asynchronously.
func (a *App) advanceDynamoPage(forward bool) {
	dbp, ok := a.providers[a.activeProvider].(*awspkg.DynamoDBProvider)
	if !ok {
		return
	}
	if forward && !dbp.HasNextPage() {
		return
	}
	if !forward && !dbp.HasPrevPage() {
		return
	}
	tableName := a.currentItem.ID
	a.showStatusMessage("[yellow]Loading…[-]")
	go func() {
		var rows []awspkg.DynamoDBItemRow
		var headers []string
		var err error
		if forward {
			rows, headers, err = dbp.NextPage(context.Background(), tableName)
		} else {
			rows, headers, err = dbp.PrevPage(context.Background(), tableName)
		}
		a.tapp.QueueUpdateDraw(func() {
			if err != nil {
				a.showStatusMessage(fmt.Sprintf("[red]Page error: %v[-]", err))
				return
			}
			a.cachedDynamoRows = rows
			a.dynamoHeaders = headers
			a.selectedDynamoRow = 0
			a.resetHints()
			a.renderDetail()
		})
	}()
}

// moveObjectRow adjusts selectedObjectRow by delta (wraps) and re-renders.
func (a *App) moveObjectRow(delta int) {
	n := len(a.cachedObjects)
	if n == 0 {
		return
	}
	a.selectedObjectRow = (a.selectedObjectRow + delta + n) % n
	if s3p, ok := a.providers[a.activeProvider].(*awspkg.S3Provider); ok {
		obj := a.cachedObjects[a.selectedObjectRow]
		s3p.SetSelectedObject(obj.Key, obj.Size)
		// Invalidate Content tab so it re-fetches with the new selection.
		for i, t := range a.providers[a.activeProvider].Tabs() {
			if t.Label == "Content" && i < len(a.tabLoaded) {
				a.tabLoaded[i] = false
			}
		}
	}
	a.renderDetail()
}

// renderTabBar writes a single-row tab bar to the tabBar widget and records
// display-column offsets for mouse click detection.
func (a *App) renderTabBar(tabs []awspkg.TabDef) {
	var line strings.Builder
	a.tabBarOffsets = make([]int, len(tabs))
	col := 0
	for i, tab := range tabs {
		label := " " + tab.Label + " "
		a.tabBarOffsets[i] = col
		if i == a.activeTab {
			line.WriteString(a.theme.ActiveTabTag + "[ " + tab.Label + " ][-]")
		} else {
			line.WriteString(a.theme.InactiveTabTag + label + "[-]")
		}
		col += len(label)
	}
	a.panels.tabBar.SetText(line.String())
}

// selectTab switches to the given tab index, fetching if not yet loaded.
// Focuses the detail panel so keyboard navigation works immediately after a tab click.
func (a *App) selectTab(idx int) {
	tabs := a.providers[a.activeProvider].Tabs()
	if idx < 0 || idx >= len(tabs) || len(a.tabLoaded) == 0 {
		return
	}

	// Stop tail stream when leaving the Tail tab.
	if a.isCWLogsTailActive() {
		a.stopCWTailStream()
	}

	a.activeTab = idx
	if !a.tabLoaded[idx] {
		a.loadTab(a.activeProvider, idx, a.currentItem)
	} else {
		a.renderDetail()
	}

	// Start live tail stream when entering the Tail tab.
	if _, ok := a.providers[a.activeProvider].(*awspkg.CloudWatchLogsProvider); ok {
		if tabs[idx].Label == "Tail" {
			a.startCWTailStream()
		}
	}

	a.panels.focused = 2
	a.tapp.SetFocus(a.panels.detail)
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

// showStatusMessage temporarily sets status bar to a message.
func (a *App) showStatusMessage(msg string) {
	a.panels.statusPages.SwitchToPage("hints")
	a.panels.status.SetText(msg)
}

// handleEsc implements the Esc priority: navStack > pass-through.
// Returns true if the event was consumed.
func (a *App) handleEsc() bool {
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

// followFirstLink navigates to the first cross-resource link in the active tab.
func (a *App) followFirstLink() bool {
	if a.activeTab >= len(a.tabLinks) || len(a.tabLinks[a.activeTab]) == 0 {
		return false
	}
	link := a.tabLinks[a.activeTab][0]
	a.navigateTo(link.provider, link.targetID)
	return true
}

// cleanup removes all temp files created during the session.
func (a *App) cleanup() {
	for _, path := range a.tmpFiles {
		os.Remove(path)
	}
}

// Run starts the tview event loop.
func (a *App) Run() error {
	defer a.cleanup()
	return a.tapp.Run()
}

// ── CW Logs streams + live tail ──────────────────────────────────────────────

// isCWLogsTailActive returns true when the active provider is CloudWatchLogsProvider
// and the active tab is "Tail". Used by renderDetail to override rendering.
func (a *App) isCWLogsTailActive() bool {
	if _, ok := a.providers[a.activeProvider].(*awspkg.CloudWatchLogsProvider); !ok {
		return false
	}
	tabs := a.providers[a.activeProvider].Tabs()
	return a.activeTab < len(tabs) && tabs[a.activeTab].Label == "Tail"
}

// isCWStreamsTabFocused returns true when the detail pane has focus, the active
// provider is CloudWatchLogsProvider, the active tab is Streams, and streams are cached.
func (a *App) isCWStreamsTabFocused() bool {
	if a.tapp.GetFocus() != a.panels.detail {
		return false
	}
	if _, ok := a.providers[a.activeProvider].(*awspkg.CloudWatchLogsProvider); !ok {
		return false
	}
	tabs := a.providers[a.activeProvider].Tabs()
	if a.activeTab >= len(tabs) || tabs[a.activeTab].Label != "Streams" {
		return false
	}
	return len(a.cachedCWLogStreams) > 0
}

// moveCWStreamRow adjusts selectedCWStreamRow by delta (clamped) and re-renders.
func (a *App) moveCWStreamRow(delta int) {
	n := len(a.cachedCWLogStreams)
	if n == 0 {
		return
	}
	a.selectedCWStreamRow += delta
	if a.selectedCWStreamRow < 0 {
		a.selectedCWStreamRow = 0
	}
	if a.selectedCWStreamRow >= n {
		a.selectedCWStreamRow = n - 1
	}
	a.renderDetail()
}

// startCWTailStream cancels any existing stream and starts a new one for the
// current log group, filtered to the selected stream (if any).
func (a *App) startCWTailStream() {
	if a.cwTailCancel != nil {
		a.cwTailCancel()
		a.cwTailCancel = nil
	}
	cwlp, ok := a.providers[a.activeProvider].(*awspkg.CloudWatchLogsProvider)
	if !ok {
		return
	}
	group := a.currentItem.ID
	stream := ""
	if len(a.cachedCWLogStreams) > 0 && a.selectedCWStreamRow < len(a.cachedCWLogStreams) {
		stream = a.cachedCWLogStreams[a.selectedCWStreamRow].Name
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cwTailCancel = cancel
	go func() {
		cwlp.StartTail(ctx, group, stream, func(ts int64, group, stream, msg string) { //nolint:errcheck
			a.tapp.QueueUpdateDraw(func() {
				a.cwTailEvents = append(a.cwTailEvents, cwTailEvent{
					ts:     time.UnixMilli(ts),
					group:  group,
					stream: stream,
					msg:    msg,
				})
				if len(a.cwTailEvents) > 200 {
					a.cwTailEvents = a.cwTailEvents[1:]
				}
				if a.isCWLogsTailActive() {
					a.renderDetail()
				}
			})
		})
	}()
}

// stopCWTailStream cancels the active tail stream and clears live event state.
func (a *App) stopCWTailStream() {
	if a.cwTailCancel != nil {
		a.cwTailCancel()
		a.cwTailCancel = nil
	}
	a.cwTailEvents = nil
}

// renderCWStreamsWithHighlight rebuilds the Streams tab table with the selected row highlighted.
func (a *App) renderCWStreamsWithHighlight() string {
	if len(a.cachedCWLogStreams) == 0 {
		return a.tabCache[a.activeTab]
	}
	headers := []string{"Stream Name", "Last Event"}
	rows := make([][]string, len(a.cachedCWLogStreams))
	for i, s := range a.cachedCWLogStreams {
		name := s.Name
		if len(name) > 50 {
			name = name[:47] + "..."
		}
		rows[i] = []string{name, s.LastEvent}
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
	fmt.Fprintf(&sb, "  %s%s[-]\n  ", a.theme.HeaderTag, strings.Join(padded, ""))
	for _, w := range widths {
		sb.WriteString(strings.Repeat("─", w) + "  ")
	}
	sb.WriteString("\n")
	for i, row := range rows {
		if i == a.selectedCWStreamRow {
			sb.WriteString("  " + a.theme.HighlightTag)
		} else {
			sb.WriteString("  ")
		}
		for j, cell := range row {
			if j < len(widths) {
				fmt.Fprintf(&sb, "%-*s", widths[j]+2, cell)
			}
		}
		if i == a.selectedCWStreamRow {
			sb.WriteString("[-]")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// isCWLogsActive returns true when the active provider is CloudWatchLogsProvider
// and a log group is selected.
func (a *App) isCWLogsActive() bool {
	if _, ok := a.providers[a.activeProvider].(*awspkg.CloudWatchLogsProvider); !ok {
		return false
	}
	return a.currentItem.ID != ""
}

// openInGonzo suspends the TUI, starts gonzo, and pipes the live tail of the
// current CloudWatch log group into gonzo's stdin. Resumes lazyaws when gonzo exits.
func (a *App) openInGonzo() {
	cwlp, ok := a.providers[a.activeProvider].(*awspkg.CloudWatchLogsProvider)
	if !ok {
		return
	}
	if _, err := exec.LookPath("gonzo"); err != nil {
		a.showStatusMessage("[red]gonzo not found in PATH[-]")
		return
	}

	group := a.currentItem.ID
	cmd := exec.Command("gonzo")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		a.showStatusMessage(fmt.Sprintf("[red]gonzo pipe: %v[-]", err))
		return
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	ctx, cancel := context.WithCancel(context.Background())

	a.tapp.Suspend(func() {
		if err := cmd.Start(); err != nil {
			cancel()
			return
		}
		go func() {
			cwlp.StartTail(ctx, group, "", func(_ int64, _, _, msg string) { //nolint:errcheck
				fmt.Fprintln(stdin, msg)
			})
			stdin.Close()
		}()
		cmd.Wait() //nolint:errcheck
		cancel()
	})
}

// renderCWLogTail generates the content for the CW Logs Tail tab.
func (a *App) renderCWLogTail() string {
	ht := a.theme.HeaderTag
	var sb strings.Builder

	stream := ""
	if len(a.cachedCWLogStreams) > 0 && a.selectedCWStreamRow < len(a.cachedCWLogStreams) {
		stream = a.cachedCWLogStreams[a.selectedCWStreamRow].Name
	}
	fmt.Fprintf(&sb, "  %sGroup%s   %s\n", ht, "[-]", a.currentItem.ID)
	if stream != "" {
		fmt.Fprintf(&sb, "  %sStream%s  %s\n", ht, "[-]", stream)
	}
	status := "streaming…"
	if a.cwTailCancel == nil {
		status = "idle"
	}
	fmt.Fprintf(&sb, "  %sStatus%s  %s\n\n", ht, "[-]", status)

	if len(a.cwTailEvents) == 0 && a.cwTailCancel != nil {
		sb.WriteString("  Waiting for events…\n")
	}
	for _, ev := range a.cwTailEvents {
		fmt.Fprintf(&sb, "  %s  %s\n", ev.ts.UTC().Format("15:04:05"), ev.msg)
	}
	return sb.String()
}
