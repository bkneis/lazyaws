package ui

import (
	"context"
	"fmt"
	"log"
	"strings"

	awspkg "github.com/bkneis/lazyaws/internal/aws"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const modalPageName = "modal"

// appActionContext implements awspkg.ActionContext for use by action funcs.
type appActionContext struct {
	app  *App
	item awspkg.Item
}

// Confirm shows a Yes/No modal. Default focus is "No".
func (ac *appActionContext) Confirm(message string, onConfirm func()) {
	a := ac.app
	a.tapp.QueueUpdateDraw(func() {
		modal := tview.NewModal().
			SetText(message).
			AddButtons([]string{"No", "Yes"}).
			SetDoneFunc(func(_ int, buttonLabel string) {
				a.popModal()
				if buttonLabel == "Yes" {
					go onConfirm()
				}
			})
		a.rootPages.AddPage(modalPageName, modal, true, true)
		a.tapp.SetFocus(modal)
	})
}

// ConfirmDelete shows a delete confirmation modal. The Delete button only works
// when the user has typed "delete me" in the input field. Default focus is Cancel.
func (ac *appActionContext) ConfirmDelete(resourceName string, onConfirm func()) {
	a := ac.app
	a.tapp.QueueUpdateDraw(func() {
		inputValue := ""
		form := tview.NewForm()
		form.SetBorder(true).SetTitle(fmt.Sprintf(` Delete "%s" `, resourceName))
		form.AddInputField(`Type "delete me" to confirm:`, "", 24, nil, func(text string) {
			inputValue = text
		})
		form.AddButton("Delete", func() {
			if inputValue != "delete me" {
				return
			}
			a.popModal()
			go onConfirm()
		})
		form.AddButton("Cancel", func() {
			a.popModal()
		})
		// Focus Cancel (item 0 + button 0 = index 1, button 1 = index 2)
		form.SetFocus(2)
		a.pushModal(form, 62, 8)
	})
}

// PromptInput shows a single-field input modal. Default focus is Cancel.
func (ac *appActionContext) PromptInput(label string, placeholder string, onSubmit func(value string)) {
	a := ac.app
	a.tapp.QueueUpdateDraw(func() {
		inputValue := placeholder
		form := tview.NewForm()
		form.SetBorder(true).SetTitle(" " + label + " ")
		form.AddInputField(label+":", placeholder, 40, nil, func(text string) {
			inputValue = text
		})
		form.AddButton("Submit", func() {
			val := inputValue
			a.popModal()
			go onSubmit(val)
		})
		form.AddButton("Cancel", func() {
			a.popModal()
		})
		// Focus Submit (item 0, button 0 = index 1)
		form.SetFocus(0)
		a.pushModal(form, 66, 8)
	})
}

// ShowError displays an error message modal.
func (ac *appActionContext) ShowError(err error) {
	log.Printf("action error: %v", err)
	a := ac.app
	a.tapp.QueueUpdateDraw(func() {
		modal := tview.NewModal().
			SetText(fmt.Sprintf("[red]Error:[-] %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(_ int, _ string) {
				a.popModal()
			})
		a.rootPages.AddPage(modalPageName, modal, true, true)
		a.tapp.SetFocus(modal)
	})
}

// ShowInfo displays an informational message modal.
func (ac *appActionContext) ShowInfo(message string) {
	a := ac.app
	a.tapp.QueueUpdateDraw(func() {
		modal := tview.NewModal().
			SetText(message).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(_ int, _ string) {
				a.popModal()
			})
		a.rootPages.AddPage(modalPageName, modal, true, true)
		a.tapp.SetFocus(modal)
	})
}

// Refresh reloads the active provider's item list.
func (ac *appActionContext) Refresh() {
	ac.app.tapp.QueueUpdateDraw(func() {
		ac.app.refresh()
	})
}

// OpenMultiGroupPicker opens the multi-select log group picker modal.
func (ac *appActionContext) OpenMultiGroupPicker(onConfirm func([]string)) {
	groups := make([]string, len(ac.app.loadedItems))
	for i, item := range ac.app.loadedItems {
		groups[i] = item.ID
	}
	ac.app.tapp.QueueUpdateDraw(func() {
		ac.app.openMultiGroupPicker(groups, onConfirm)
	})
}

// openActionsMenu opens the x actions menu for the current provider/item.
// Returns true if the menu was opened, false if not (e.g., provider is not Actionable).
func (a *App) openActionsMenu() bool {
	// Guard: don't open if a modal is already visible.
	if name, _ := a.rootPages.GetFrontPage(); name != "main" {
		return false
	}

	provider := a.providers[a.activeProvider]
	actionable, ok := provider.(awspkg.Actionable)
	if !ok {
		return false
	}

	item := a.enrichItemMeta(a.currentItem)
	actions := actionable.Actions(item)
	if len(actions) == 0 {
		return false
	}

	ac := &appActionContext{app: a, item: item}

	list := tview.NewList().ShowSecondaryText(false)
	list.SetBorder(true).SetTitle(" Actions ")
	list.SetSelectedTextColor(a.theme.SelectionText).SetSelectedBackgroundColor(a.theme.FocusColor)

	for _, action := range actions {
		action := action
		label := action.Label
		if action.Key != 0 {
			label = fmt.Sprintf("%c  %s", action.Key, action.Label)
		}
		list.AddItem(label, "", action.Key, func() {
			log.Printf("action selected: %s", action.Label)
			a.popModal()
			go func() {
				if err := action.Func(context.Background(), item, ac); err != nil {
					ac.ShowError(err)
				}
			}()
		})
	}

	height := len(actions) + 2
	if height < 4 {
		height = 4
	}
	a.pushModal(list, 34, height)
	return true
}

// enrichItemMeta copies item and adds sub-item context (e.g. selected S3 object key) to Meta.
func (a *App) enrichItemMeta(item awspkg.Item) awspkg.Item {
	m := make(map[string]string, len(item.Meta)+4)
	for k, v := range item.Meta {
		m[k] = v
	}
	item.Meta = m
	// S3: enrich with selected object key when on the Objects tab.
	if _, ok := a.providers[a.activeProvider].(*awspkg.S3Provider); ok {
		tabs := a.providers[a.activeProvider].Tabs()
		if a.activeTab < len(tabs) && tabs[a.activeTab].Label == "Objects" &&
			len(a.cachedObjects) > 0 && a.selectedObjectRow < len(a.cachedObjects) {
			item.Meta["selectedObjectKey"] = a.cachedObjects[a.selectedObjectRow].Key
		}
	}
	return item
}

// pushModal places a primitive in a centered grid overlay and focuses it.
func (a *App) pushModal(p tview.Primitive, width, height int) {
	centered := tview.NewGrid().
		SetColumns(0, width, 0).
		SetRows(0, height, 0).
		AddItem(p, 1, 1, 1, 1, 0, 0, true)
	a.rootPages.AddPage(modalPageName, centered, true, true)
	a.tapp.SetFocus(p)
}

// popModal removes the top modal page and returns focus to the items panel.
func (a *App) popModal() {
	a.rootPages.RemovePage(modalPageName)
	a.panels.focused = 1
	a.tapp.SetFocus(a.panels.items)
}

// openMultiGroupPicker opens a two-panel multi-select modal for picking log groups.
func (a *App) openMultiGroupPicker(allGroups []string, onConfirm func([]string)) {
	var selected []string
	filterQuery := ""

	filteredGroups := func() []string {
		q := strings.ToLower(filterQuery)
		var result []string
		for _, g := range allGroups {
			if q == "" || strings.Contains(strings.ToLower(g), q) {
				result = append(result, g)
				if len(result) == 10 {
					break
				}
			}
		}
		return result
	}

	availList := tview.NewList().ShowSecondaryText(false)
	availList.SetSelectedTextColor(a.theme.SelectionText).SetSelectedBackgroundColor(a.theme.FocusColor)

	selectedList := tview.NewList().ShowSecondaryText(false)
	selectedList.SetSelectedTextColor(a.theme.SelectionText).SetSelectedBackgroundColor(a.theme.FocusColor)

	filterInput := tview.NewInputField().SetLabel("/ ").SetLabelColor(tcell.ColorYellow)

	rebuildAvail := func() {
		availList.Clear()
		for _, g := range filteredGroups() {
			g := g
			availList.AddItem(g, "", 0, nil)
		}
		title := fmt.Sprintf(" Available (%d shown) ", availList.GetItemCount())
		availList.SetBorder(true).SetTitle(title)
	}

	rebuildSelected := func() {
		selectedList.Clear()
		for _, g := range selected {
			g := g
			selectedList.AddItem(g, "", 0, nil)
		}
		selectedList.SetBorder(true).SetTitle(fmt.Sprintf(" Selected (%d) ", len(selected)))
	}

	confirm := func() {
		if len(selected) == 0 {
			return
		}
		a.popModal()
		a.cwTailGroups = selected
		a.cwTailEvents = nil
		tabs := a.providers[a.activeProvider].Tabs()
		for i, t := range tabs {
			if t.Label == "Tail" {
				a.selectTab(i)
				break
			}
		}
		onConfirm(selected)
	}

	availList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case ' ':
			cur := availList.GetCurrentItem()
			if cur >= 0 && cur < availList.GetItemCount() {
				name, _ := availList.GetItemText(cur)
				if !containsStr(selected, name) {
					selected = append(selected, name)
					rebuildSelected()
				}
			}
			return nil
		case '/':
			a.tapp.SetFocus(filterInput)
			return nil
		}
		if event.Key() == tcell.KeyTab {
			a.tapp.SetFocus(selectedList)
			return nil
		}
		if event.Key() == tcell.KeyEnter {
			confirm()
			return nil
		}
		return event
	})

	selectedList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == ' ' {
			cur := selectedList.GetCurrentItem()
			if cur >= 0 && cur < selectedList.GetItemCount() {
				name, _ := selectedList.GetItemText(cur)
				selected = removeStr(selected, name)
				rebuildSelected()
			}
			return nil
		}
		if event.Key() == tcell.KeyTab {
			a.tapp.SetFocus(availList)
			return nil
		}
		if event.Key() == tcell.KeyEnter {
			confirm()
			return nil
		}
		return event
	})

	filterInput.SetChangedFunc(func(text string) {
		filterQuery = text
		rebuildAvail()
	})
	filterInput.SetDoneFunc(func(_ tcell.Key) {
		a.tapp.SetFocus(availList)
	})

	leftFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(availList, 0, 1, true).
		AddItem(filterInput, 1, 0, false)

	flex := tview.NewFlex().
		AddItem(leftFlex, 0, 1, true).
		AddItem(selectedList, 0, 1, false)
	flex.SetBorder(true).SetTitle(" Stream multiple log groups  (space: toggle · /: filter · tab: switch · enter: start) ")

	rebuildAvail()
	rebuildSelected()
	a.pushModal(flex, 90, 20)
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func removeStr(ss []string, s string) []string {
	out := ss[:0:0]
	for _, v := range ss {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}
