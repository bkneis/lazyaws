package ui

import (
	"context"
	"fmt"
	"strings"

	awspkg "github.com/bryanl/lazyaws/internal/aws"
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

	leftCol := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.panels.resources, 0, 1, true).
		AddItem(a.panels.items, 0, 2, false)

	layout := tview.NewFlex().
		AddItem(leftCol, 25, 0, true).
		AddItem(a.panels.detail, 0, 1, false)

	outer := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(layout, 0, 1, true).
		AddItem(a.panels.status, 1, 0, false)

	setupKeys(a)

	a.tapp.EnableMouse(true)
	a.tapp.SetRoot(outer, true).SetFocus(a.panels.resources)
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

// renderTabBar builds the tab bar string with active tab highlighted in cyan.
func renderTabBar(tabs []awspkg.TabDef, active int) string {
	parts := make([]string, len(tabs))
	for i, t := range tabs {
		if i == active {
			parts[i] = "[cyan][[]" + t.Label + "][-]"
		} else {
			parts[i] = "[gray]" + t.Label + "[-]"
		}
	}
	return " " + strings.Join(parts, "  ")
}

// nextTab advances to the next tab, fetching if not yet loaded.
func (a *App) nextTab() {
	tabs := a.providers[a.activeProvider].Tabs()
	if len(tabs) == 0 || len(a.tabLoaded) == 0 {
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
	if len(tabs) == 0 || len(a.tabLoaded) == 0 {
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
