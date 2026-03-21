package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// panels holds the three tview widgets and the currently focused index.
type panels struct {
	resources   *tview.List
	items       *tview.List
	tabBar      *tview.TextView // single-row tab header
	detail      *tview.TextView // scrollable content area
	rightFlex   *tview.Flex     // vertical flex containing tabBar+detail
	status      *tview.TextView
	searchInput *tview.InputField
	statusPages *tview.Pages
	hintsText   string
	focused     int // 0=resources, 1=items, 2=detail
}

func newPanels(t Theme) *panels {
	fc := t.FocusColor
	st := t.SelectionText

	resources := tview.NewList().ShowSecondaryText(false)
	resources.SetBorder(true).SetTitle(" Resources ").SetBorderColor(fc)
	resources.SetSelectedTextColor(st).SetSelectedBackgroundColor(fc)
	resources.SetFocusFunc(func() { resources.SetBorderColor(fc) }).
		SetBlurFunc(func() { resources.SetBorderColor(tcell.ColorDefault) })

	items := tview.NewList().ShowSecondaryText(false)
	items.SetBorder(true).SetTitle(" Items ")
	items.SetSelectedTextColor(st).SetSelectedBackgroundColor(fc)
	items.SetFocusFunc(func() { items.SetBorderColor(fc) }).
		SetBlurFunc(func() { items.SetBorderColor(tcell.ColorDefault) })

	tabBar := tview.NewTextView().
		SetDynamicColors(true)

	detail := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)

	rightFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tabBar, 1, 0, false).
		AddItem(detail, 0, 1, false)
	rightFlex.SetBorder(true).SetBorderColor(tcell.ColorDefault)

	detail.SetFocusFunc(func() { rightFlex.SetBorderColor(fc) }).
		SetBlurFunc(func() { rightFlex.SetBorderColor(tcell.ColorDefault) })

	hints := t.HeaderTag + "Tab[-]/" + t.HeaderTag + "S-Tab[-]: panel   " +
		t.HeaderTag + "j/k[-]: navigate   " +
		t.HeaderTag + "[[]·][-]: tab   " +
		t.HeaderTag + "/[-]: search   " +
		t.HeaderTag + "r[-]: refresh   " +
		t.HeaderTag + "x[-]: actions   " +
		t.HeaderTag + "R[-]: region   " +
		t.HeaderTag + "q[-]: quit"
	status := tview.NewTextView().SetDynamicColors(true).SetText(" " + hints)

	searchInput := tview.NewInputField().
		SetLabel("/ ").
		SetLabelColor(tcell.ColorYellow).
		SetFieldBackgroundColor(tcell.ColorDefault)

	statusPages := tview.NewPages().
		AddPage("hints", status, true, true).
		AddPage("search", searchInput, true, false)

	return &panels{
		resources:   resources,
		items:       items,
		tabBar:      tabBar,
		detail:      detail,
		rightFlex:   rightFlex,
		status:      status,
		hintsText:   " " + hints,
		searchInput: searchInput,
		statusPages: statusPages,
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
