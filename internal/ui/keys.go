package ui

import "github.com/gdamore/tcell/v2"

// setupKeys attaches the global keyboard handler to the application.
//
// Bindings:
//
//	Tab         — focus next panel (suppressed while search input is active)
//	Shift+Tab   — focus previous panel (suppressed while search input is active)
//	/           — enter search mode (suppressed while search input is active)
//	j / ↓       — move down in focused list
//	k / ↑       — move up in focused list
//	[           — previous tab in detail pane
//	]           — next tab in detail pane
//	q           — quit
//	r           — refresh current resource list
func setupKeys(a *App) {
	a.tapp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		searchActive := a.tapp.GetFocus() == a.panels.searchInput

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
		}

		if searchActive {
			return event
		}

		switch event.Rune() {
		case '/':
			a.enterSearch()
			return nil
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
