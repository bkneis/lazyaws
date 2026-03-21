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
//	R           — open region picker
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
		case tcell.KeyEscape:
			if searchActive {
				return event // let searchInput.SetDoneFunc handle it
			}
			if a.handleEsc() {
				return nil
			}
			return event
		}

		if searchActive {
			return event
		}

		// Pass all rune events through to the modal if one is visible.
		if name, _ := a.rootPages.GetFrontPage(); name != "main" {
			return event
		}

		switch event.Rune() {
		case 'x':
			a.openActionsMenu()
			return nil
		case '/':
			a.enterSearch()
			return nil
		case 'q':
			a.tapp.Stop()
			return nil
		case 'r':
			a.refresh()
			return nil
		case 'R':
			a.openRegionPicker()
			return nil
		case 'g':
			if a.isCWLogsActive() {
				a.openInGonzo()
				return nil
			}
		case 'j':
			if a.isCWStreamsTabFocused() {
				a.moveCWStreamRow(1)
				return nil
			}
			if a.isDynamoItemsTabFocused() {
				a.moveDynamoItemRow(1)
				return nil
			}
			if a.isS3ObjectsTabFocused() {
				a.moveObjectRow(1)
				return nil
			}
			if a.isKinesisShardsTabFocused() {
				a.moveShardRow(1)
				return nil
			}
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case 'k':
			if a.isCWStreamsTabFocused() {
				a.moveCWStreamRow(-1)
				return nil
			}
			if a.isDynamoItemsTabFocused() {
				a.moveDynamoItemRow(-1)
				return nil
			}
			if a.isS3ObjectsTabFocused() {
				a.moveObjectRow(-1)
				return nil
			}
			if a.isKinesisShardsTabFocused() {
				a.moveShardRow(-1)
				return nil
			}
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		case 'n':
			if a.isDynamoItemsTabFocused() {
				a.advanceDynamoPage(true)
				return nil
			}
		case 'p':
			if a.isDynamoItemsTabFocused() {
				a.advanceDynamoPage(false)
				return nil
			}
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
