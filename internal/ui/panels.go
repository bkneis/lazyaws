package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const focusColor = tcell.ColorAqua

const hintsText = " [cyan]Tab[-]/[cyan]S-Tab[-]: panel   [cyan]j/k[-]: navigate   [cyan][[]·][-]: tab   [cyan]/[-]: search   [cyan]r[-]: refresh   [cyan]q[-]: quit"

// panels holds the three tview widgets and the currently focused index.
type panels struct {
	resources   *tview.List
	items       *tview.List
	tabBar      *tview.TextView  // 2-row tab header (label row + underline row)
	detail      *tview.TextView  // scrollable content area
	expand      *tview.TextView  // expansion panel (hidden by default)
	rightFlex   *tview.Flex      // vertical flex containing tabBar+detail+expand
	status      *tview.TextView
	searchInput *tview.InputField
	prompt      *tview.TextView  // y/n prompt widget (used in Feature 4)
	statusPages *tview.Pages
	focused     int // 0=resources, 1=items, 2=detail
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

	tabBar := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)

	detail := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false).
		SetRegions(true)
	detail.SetBorder(true).SetTitle(" Detail ")
	detail.SetFocusFunc(func() { detail.SetBorderColor(focusColor) }).
		SetBlurFunc(func() { detail.SetBorderColor(tcell.ColorDefault) })

	expand := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	expand.SetBorder(true).SetTitle(" Expand ")

	rightFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tabBar, 2, 0, false).
		AddItem(detail, 0, 2, false).
		AddItem(expand, 0, 0, false) // proportion 0 = hidden

	status := tview.NewTextView().SetDynamicColors(true).SetText(hintsText)

	searchInput := tview.NewInputField().
		SetLabel("/ ").
		SetLabelColor(tcell.ColorYellow).
		SetFieldBackgroundColor(tcell.ColorDefault)

	prompt := tview.NewTextView().SetDynamicColors(true)

	statusPages := tview.NewPages().
		AddPage("hints", status, true, true).
		AddPage("search", searchInput, true, false).
		AddPage("prompt", prompt, true, false)

	return &panels{
		resources:   resources,
		items:       items,
		tabBar:      tabBar,
		detail:      detail,
		expand:      expand,
		rightFlex:   rightFlex,
		status:      status,
		searchInput: searchInput,
		prompt:      prompt,
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
