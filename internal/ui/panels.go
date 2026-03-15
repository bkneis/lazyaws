package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const focusColor = tcell.ColorAqua

// panels holds the three tview widgets and the currently focused index.
type panels struct {
	resources *tview.List
	items     *tview.List
	detail    *tview.TextView
	status    *tview.TextView
	focused   int // 0=resources, 1=items, 2=detail
}

func newPanels() *panels {
	resources := tview.NewList().ShowSecondaryText(false)
	resources.SetBorder(true).SetTitle(" Resources ").SetBorderColor(focusColor)
	resources.SetSelectedTextColor(tcell.ColorBlack).SetSelectedBackgroundColor(focusColor)
	resources.SetFocusFunc(func() { resources.SetBorderColor(focusColor) }).
		SetBlurFunc(func() { resources.SetBorderColor(tcell.ColorDefault) })

	items := tview.NewList().ShowSecondaryText(false)
	items.SetBorder(true).SetTitle(" Items ")
	items.SetSelectedTextColor(tcell.ColorBlack).SetSelectedBackgroundColor(focusColor)
	items.SetFocusFunc(func() { items.SetBorderColor(focusColor) }).
		SetBlurFunc(func() { items.SetBorderColor(tcell.ColorDefault) })

	detail := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	detail.SetBorder(true).SetTitle(" Detail ")
	detail.SetFocusFunc(func() { detail.SetBorderColor(focusColor) }).
		SetBlurFunc(func() { detail.SetBorderColor(tcell.ColorDefault) })

	status := tview.NewTextView().SetDynamicColors(true).
		SetText(" [cyan]Tab[-]/[cyan]S-Tab[-]: panel   [cyan]j/k[-]: navigate   [cyan][[]·][-]: tab   [cyan]r[-]: refresh   [cyan]q[-]: quit")

	return &panels{
		resources: resources,
		items:     items,
		detail:    detail,
		status:    status,
	}
}

// primitives returns the panels in Tab-cycle order.
func (p *panels) primitives() []tview.Primitive {
	return []tview.Primitive{p.resources, p.items, p.detail}
}

// current returns the currently focused primitive.
func (p *panels) current() tview.Primitive {
	return p.primitives()[p.focused]
}

// next advances focus by one (wraps around) and returns the new focus target.
func (p *panels) next() tview.Primitive {
	n := len(p.primitives())
	p.focused = (p.focused + 1) % n
	return p.current()
}

// prev retreats focus by one (wraps around) and returns the new focus target.
func (p *panels) prev() tview.Primitive {
	n := len(p.primitives())
	p.focused = (p.focused + n - 1) % n
	return p.current()
}
