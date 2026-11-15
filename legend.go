package main

import (
	"os"
	"strings"
)

// LegendChip is one controller button drawn as a small coloured label.
type LegendChip struct {
	Label string
	Color Color // A = 0: neutral chip in theme colours
}

// LegendItem is a group of chips followed by what they do ("B  Tippen").
type LegendItem struct {
	Chips []LegendChip
	Text  string
}

// UILanguage returns "de" or "en" from the layout and the locale.
func UILanguage(layout string) string {
	for _, v := range []string{os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG")} {
		if v == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(v), "de") {
			return "de"
		}
		return "en"
	}
	if layout == "de" {
		return "de"
	}
	return "en"
}

var legendTexts = map[string]map[string]string{
	"de": {
		"press": "Tippen", "backspace": "Löschen", "space": "Leerzeichen", "enter": "Enter",
		"shift": "Groß (halten)", "close": "Schließen", "hide": "Ausblenden",
		"left_click": "Linksklick", "right_click": "Rechtsklick",
		"mouse": "Maus", "nav": "Taste wählen", "position": "Oben/Unten", "toggle": "Ein/Aus",
		"lstick": "L-Stick", "rstick": "R-Stick",
		"mousemode": "Nur Maus", "click": "Klick", "scroll": "Scrollen", "end": "Beenden",
	},
	"en": {
		"press": "Type", "backspace": "Delete", "space": "Space", "enter": "Enter",
		"shift": "Shift (hold)", "close": "Close", "hide": "Hide",
		"left_click": "Left click", "right_click": "Right click",
		"mouse": "Mouse", "nav": "Select key", "position": "Top/Bottom", "toggle": "On/Off",
		"lstick": "L stick", "rstick": "R stick",
		"mousemode": "Mouse only", "click": "Click", "scroll": "Scroll", "end": "Exit",
	},
}

// BuildLegend lists what each controller button does, in the order a
// newcomer needs them: typing first, then mouse, then show/hide.
func BuildLegend(cfg Config, p ControllerProfile, lang string, daemon bool) []LegendItem {
	tx := legendTexts[lang]
	if tx == nil {
		tx = legendTexts["en"]
	}
	g := cfg.Gamepad
	b := g.Buttons
	// With swap_ab the "a" function sits on the physical B position
	physical := func(button string) string {
		if g.SwapAB {
			switch button {
			case "a":
				return "b"
			case "b":
				return "a"
			}
		}
		return button
	}
	chip := func(button string) LegendChip {
		pos := physical(button)
		return LegendChip{Label: p.Label(pos), Color: p.Colors[pos]}
	}
	var items []LegendItem
	add := func(button, textKey string) {
		if button == "" {
			return
		}
		items = append(items, LegendItem{Chips: []LegendChip{chip(button)}, Text: tx[textKey]})
	}

	add(b.Press, "press")
	add(b.Backspace, "backspace")
	add(b.Space, "space")
	add(b.Enter, "enter")
	add(b.Shift, "shift")
	if daemon {
		add(b.Close, "hide")
	} else {
		add(b.Close, "close")
	}

	mouseStick, navStick := tx["rstick"], tx["lstick"]
	if g.MouseStick == "left" {
		mouseStick, navStick = navStick, mouseStick
	}
	if cfg.Mouse.Enabled {
		items = append(items, LegendItem{Chips: []LegendChip{{Label: mouseStick}}, Text: tx["mouse"]})
		add(b.LeftClick, "left_click")
		add(b.RightClick, "right_click")
	}
	items = append(items, LegendItem{
		Chips: []LegendChip{{Label: "✚"}, {Label: navStick}}, Text: tx["nav"]})
	add(b.PositionToggle, "position")

	if combo := strings.TrimSpace(g.ToggleCombo); combo != "" {
		var chips []LegendChip
		for _, part := range strings.Split(combo, "+") {
			// Combos are positional (not affected by swap_ab)
			pos := strings.TrimSpace(strings.ToLower(part))
			chips = append(chips, LegendChip{Label: p.Label(pos), Color: p.Colors[pos]})
		}
		items = append(items, LegendItem{Chips: chips, Text: tx["toggle"]})
	}
	if combo := strings.TrimSpace(g.MouseCombo); combo != "" {
		var chips []LegendChip
		for _, part := range strings.Split(combo, "+") {
			pos := strings.TrimSpace(strings.ToLower(part))
			chips = append(chips, LegendChip{Label: p.Label(pos), Color: p.Colors[pos]})
		}
		items = append(items, LegendItem{Chips: chips, Text: tx["mousemode"]})
	}
	return items
}

// BuildMouseLegend lists the buttons of mouse mode for the small badge.
func BuildMouseLegend(cfg Config, p ControllerProfile, lang string) []LegendItem {
	tx := legendTexts[lang]
	if tx == nil {
		tx = legendTexts["en"]
	}
	g := cfg.Gamepad
	chip := func(button string) LegendChip {
		pos := button
		if g.SwapAB && (button == "a" || button == "b") {
			pos = map[string]string{"a": "b", "b": "a"}[button]
		}
		return LegendChip{Label: p.Label(pos), Color: p.Colors[pos]}
	}
	var items []LegendItem
	if g.Buttons.Press != "" {
		items = append(items, LegendItem{Chips: []LegendChip{chip(g.Buttons.Press)}, Text: tx["click"]})
	}
	if g.Buttons.RightClick != "" {
		items = append(items, LegendItem{Chips: []LegendChip{chip(g.Buttons.RightClick)}, Text: tx["right_click"]})
	}
	navStick := tx["rstick"]
	if g.MouseStick != "left" {
		navStick = tx["lstick"]
	}
	items = append(items, LegendItem{Chips: []LegendChip{{Label: navStick}}, Text: tx["scroll"]})
	if g.Buttons.Close != "" {
		items = append(items, LegendItem{Chips: []LegendChip{chip(g.Buttons.Close)}, Text: tx["end"]})
	}
	return items
}

// legendMetrics holds sizes shared by layout and drawing.
type legendMetrics struct {
	lineH, chipH, chipPad, chipGap, textGap, itemGap int32
}

func newLegendMetrics(unit int32) legendMetrics {
	lineH := max32(22, int32(float64(unit)*0.5))
	return legendMetrics{
		lineH:   lineH,
		chipH:   int32(float64(lineH) * 0.78),
		chipPad: max32(5, lineH/4),
		chipGap: 3,
		textGap: 5,
		itemGap: max32(14, lineH*2/3),
	}
}

// itemWidth measures one legend item with the given text measurer.
func (m legendMetrics) itemWidth(it LegendItem, measureChip, measureText func(string) int32) int32 {
	var w int32
	for i, c := range it.Chips {
		if i > 0 {
			w += m.chipGap
		}
		w += max32(m.chipH, measureChip(c.Label)+2*m.chipPad)
	}
	return w + m.textGap + measureText(it.Text)
}

// layoutLegend wraps items into lines no wider than maxW.
func layoutLegend(items []LegendItem, m legendMetrics, maxW int32, measureChip, measureText func(string) int32) [][]LegendItem {
	var lines [][]LegendItem
	var cur []LegendItem
	var curW int32
	for _, it := range items {
		w := m.itemWidth(it, measureChip, measureText)
		if len(cur) > 0 && curW+m.itemGap+w > maxW {
			lines = append(lines, cur)
			cur, curW = nil, 0
		}
		if len(cur) > 0 {
			curW += m.itemGap
		}
		cur = append(cur, it)
		curW += w
	}
	if len(cur) > 0 {
		lines = append(lines, cur)
	}
	return lines
}
