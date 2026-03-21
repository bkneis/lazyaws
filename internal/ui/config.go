package ui

import (
	"os"
	"path/filepath"

	"github.com/gdamore/tcell/v2"
	"gopkg.in/yaml.v3"
)

type fileConfig struct {
	Theme struct {
		FocusColor     string `yaml:"focus_color"`
		SelectionText  string `yaml:"selection_text"`
		HighlightTag   string `yaml:"highlight_tag"`
		HeaderTag      string `yaml:"header_tag"`
		ActiveTabTag   string `yaml:"active_tab_tag"`
		InactiveTabTag string `yaml:"inactive_tab_tag"`
		LinkTag        string `yaml:"link_tag"`
	} `yaml:"theme"`
}

func configPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "lazyaws", "config.yaml")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "lazyaws", "config.yaml")
}

// LoadConfigTheme reads the config file and returns a Theme with overrides applied.
// Returns nil if no config file exists or the file has no theme overrides.
func LoadConfigTheme(base Theme) *Theme {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil
	}
	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return nil
	}
	t := fc.Theme
	if t.FocusColor == "" && t.SelectionText == "" && t.HighlightTag == "" &&
		t.HeaderTag == "" && t.ActiveTabTag == "" && t.InactiveTabTag == "" && t.LinkTag == "" {
		return nil
	}
	result := base
	if t.FocusColor != "" {
		result.FocusColor = tcell.GetColor(t.FocusColor)
	}
	if t.SelectionText != "" {
		result.SelectionText = tcell.GetColor(t.SelectionText)
	}
	if t.HighlightTag != "" {
		result.HighlightTag = t.HighlightTag
	}
	if t.HeaderTag != "" {
		result.HeaderTag = t.HeaderTag
	}
	if t.ActiveTabTag != "" {
		result.ActiveTabTag = t.ActiveTabTag
	}
	if t.InactiveTabTag != "" {
		result.InactiveTabTag = t.InactiveTabTag
	}
	if t.LinkTag != "" {
		result.LinkTag = t.LinkTag
	}
	return &result
}
