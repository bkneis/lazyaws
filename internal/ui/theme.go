package ui

import (
	"os"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// Theme controls the visual color scheme of the TUI.
type Theme struct {
	FocusColor     tcell.Color // widget border + list selection background
	SelectionText  tcell.Color // text on selected list item
	HighlightTag   string      // tview tag for selected object row
	HeaderTag      string      // tview tag for column headers / KV keys
	ActiveTabTag   string      // tview tag for the active tab label
	InactiveTabTag string      // tview tag for inactive tab labels
	LinkTag        string      // tview tag for cross-resource links (with underline)
}

// DetectTheme infers the best color scheme from environment variables.
//
//   - TERM_PROGRAM=WarpTerminal → green (higher contrast in Warp)
//   - COLORFGBG background > 6 → blue (light terminal)
//   - default → aqua/cyan
func DetectTheme() Theme {
	if os.Getenv("TERM_PROGRAM") == "WarpTerminal" {
		return Theme{
			FocusColor:     tcell.ColorGreen,
			SelectionText:  tcell.ColorBlack,
			HighlightTag:   "[green]",
			HeaderTag:      "[green]",
			ActiveTabTag:   "[green]",
			InactiveTabTag: "[gray]",
			LinkTag:        "[green::u]",
		}
	}
	if fgbg := os.Getenv("COLORFGBG"); fgbg != "" {
		parts := strings.Split(fgbg, ";")
		if len(parts) >= 2 {
			if bg, err := strconv.Atoi(parts[len(parts)-1]); err == nil && bg > 6 {
				return Theme{
					FocusColor:     tcell.ColorBlue,
					SelectionText:  tcell.ColorWhite,
					HighlightTag:   "[blue]",
					HeaderTag:      "[blue]",
					ActiveTabTag:   "[blue]",
					InactiveTabTag: "[gray]",
					LinkTag:        "[blue::u]",
				}
			}
		}
	}
	return Theme{
		FocusColor:     tcell.ColorAqua,
		SelectionText:  tcell.ColorBlack,
		HighlightTag:   "[aqua]",
		HeaderTag:      "[cyan]",
		ActiveTabTag:   "[aqua]",
		InactiveTabTag: "[gray]",
		LinkTag:        "[aqua::u]",
	}
}
