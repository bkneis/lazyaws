package ui

import (
	"context"
	"fmt"
	"log"

	awspkg "github.com/bryanl/lazyaws/internal/aws"
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
