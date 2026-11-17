package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// --- Settings window (gamepad-osk --settings) ---
//
// Drawn with the same SDL renderer as the keyboard: no toolkit needed, and it
// works with controller, mouse and keyboard. Top: live preview of the
// keyboard. Below: settings in two columns. Changes apply on "Save".

type rowKind int

const (
	rowChoice rowKind = iota
	rowToggle
	rowSlider
)

type settingsRow struct {
	label          string
	kind           rowKind
	options        func() []string // choice labels (evaluated when drawn)
	get            func() int      // choice index, toggle 0/1, slider value
	set            func(int)
	min, max, step int
	suffix         string
	zero           string      // slider: text shown for value 0 (e.g. "Off")
	skip           func() bool // choice: value not allowed, cycling steps over it
}

var settingsTexts = map[string]map[string]string{
	"de": {
		"title": "gamepad-osk – Einstellungen", "controller": "Controller", "none": "kein Controller gefunden",
		"autostart": "Beim Anmelden starten", "theme": "Design", "layout": "Tastaturlayout",
		"profile": "Controller-Beschriftung", "swap_ab": "A und B tauschen", "combo": "Ein-/Ausblenden mit",
		"mouse": "Maus", "speed": "Mausgeschwindigkeit", "legend": "Belegung anzeigen",
		"position": "Position", "size": "Größe", "save": "Speichern", "close": "Schließen",
		"auto": "Automatisch", "de": "Deutsch (QWERTZ)", "us": "Englisch (QWERTY)",
		"de_short": "Deutsch", "us_short": "Englisch",
		"rstick": "Rechter Stick", "lstick": "Linker Stick", "off": "Aus",
		"bottom": "Unten", "top": "Oben", "generic": "Allgemein",
		"light": "Hell", "night": "Dunkel", "matrix": "Matrix (Original)",
		"saved": "Gespeichert.", "restarted": "Gespeichert, Tastatur neu gestartet.",
		"unsaved": "Nicht gespeichert – nochmal zum Verwerfen.", "error": "Fehler: ",
		"preview": "Vorschau",
		"select": "wählen", "change_v": "ändern", "toggle_v": "umschalten", "page_v": "Seite",
		"save_v": "speichern", "close_v": "schließen",
		"page_general": "Allgemein", "page_buttons": "Belegung",
		"mouse_combo": "Nur Maus mit", "page_test": "Test",
		"test_hint": "Knöpfe drücken – sie leuchten auf · %s + %s zusammen: zurück", "last": "Zuletzt gedrückt", "press_any": "Drück einen Knopf …", "unused": "nicht belegt", "nav": "Taste wählen", "caps": "Feststelltaste", "mouse_move": "Maus bewegen", "part_of": "mit", "key_badges": "Buttons auf den Tasten", "opacity": "Transparenz",
		"mouse_curve": "Feinsteuerung Maus", "nav_curve": "Feinsteuerung Scrollen",
		"b_press": "Taste tippen", "b_backspace": "Löschen", "b_space": "Leerzeichen",
		"b_enter": "Enter", "b_shift": "Großschreiben (halten)", "b_close": "Ausblenden",
		"b_left_click": "Linksklick", "b_right_click": "Rechtsklick", "b_position": "Oben/Unten wechseln",
		"swapped": "Getauscht mit „%s“.",
	},
	"en": {
		"title": "gamepad-osk – Settings", "controller": "Controller", "none": "no controller found",
		"autostart": "Start at login", "theme": "Theme", "layout": "Keyboard layout",
		"profile": "Button names", "swap_ab": "Swap A and B", "combo": "Show/hide with",
		"mouse": "Mouse", "speed": "Mouse speed", "legend": "Show button legend",
		"position": "Position", "size": "Size", "save": "Save", "close": "Close",
		"auto": "Automatic", "de": "German (QWERTZ)", "us": "English (QWERTY)",
		"de_short": "German", "us_short": "English",
		"rstick": "Right stick", "lstick": "Left stick", "off": "Off",
		"bottom": "Bottom", "top": "Top", "generic": "Generic",
		"light": "Light", "night": "Dark", "matrix": "Matrix (original)",
		"saved": "Saved.", "restarted": "Saved, keyboard restarted.",
		"unsaved": "Not saved – press again to discard.", "error": "Error: ",
		"preview": "Preview",
		"select": "select", "change_v": "change", "toggle_v": "toggle", "page_v": "page",
		"save_v": "save", "close_v": "close",
		"page_general": "General", "page_buttons": "Buttons",
		"mouse_combo": "Mouse only with", "page_test": "Test",
		"test_hint": "Press buttons – they light up · %s + %s together: back", "last": "Last pressed", "press_any": "Press a button …", "unused": "not assigned", "nav": "Select key", "caps": "Caps Lock", "mouse_move": "Move mouse", "part_of": "with", "key_badges": "Buttons on the keys", "opacity": "Transparency",
		"mouse_curve": "Mouse fine control", "nav_curve": "Scroll fine control",
		"b_press": "Type key", "b_backspace": "Delete", "b_space": "Space",
		"b_enter": "Enter", "b_shift": "Shift (hold)", "b_close": "Hide",
		"b_left_click": "Left click", "b_right_click": "Right click", "b_position": "Top/bottom",
		"swapped": "Swapped with “%s”.",
	},
}

var comboChoices = []string{"guide+select", "guide+start", "select+start", "l3+r3", ""}

var profileChoices = []string{ProfileAuto, ProfileXbox, ProfilePlayStation, ProfileNintendo, Profile8BitDo, ProfileGeneric}

// settingsThemeOrder: friendly themes first, then the rest alphabetically.
func settingsThemeOrder() []string {
	order := []string{"light", "night", "matrix"}
	var rest []string
	for name := range Themes {
		if name != "light" && name != "night" && name != "matrix" {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(order, rest...)
}

type settingsUI struct {
	cfg, orig                Config
	autostart, origAutostart bool
	tx                       map[string]string
	rows                     []settingsRow // rows of the current page
	pages                    [3][]settingsRow // General, Buttons, Test (no rows)
	page                     int
	tabRects                 [3]FRect
	tester                   testerState
	focus                    int // rows, then save button, then close button

	gamepad      *GamepadReader
	padName      string
	detectedProf string
	detLayout    string

	sdl          *SDLRenderer
	ui           *Renderer // fonts and drawing helpers for the window
	uiFonts      *FontSet
	preview      *Renderer
	previewKB    *KeyboardState
	previewRect  Rect
	previewDirty bool

	rowRects   []FRect
	btnRects   [2]FRect
	winW, winH int32

	status       string
	statusUntil  time.Time
	confirmClose bool
	running      bool
}

func (s *settingsUI) t(key string) string {
	if v, ok := s.tx[key]; ok {
		return v
	}
	return key
}

func indexOf(list []string, v string) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return 0
}

func (s *settingsUI) effectiveProfile() ControllerProfile {
	if s.cfg.Gamepad.Controller == ProfileAuto || s.cfg.Gamepad.Controller == "" {
		if s.detectedProf != "" {
			return GetControllerProfile(s.detectedProf)
		}
		return GetControllerProfile(ProfileXbox)
	}
	return GetControllerProfile(s.cfg.Gamepad.Controller)
}

func (s *settingsUI) comboLabel(combo string) string {
	if combo == "" {
		return s.t("off")
	}
	p := s.effectiveProfile()
	var parts []string
	for _, b := range strings.Split(combo, "+") {
		parts = append(parts, p.Label(b))
	}
	return strings.Join(parts, " + ")
}

// physical returns the button position a config name ends up on (swap_ab).
func (s *settingsUI) physical(button string) string {
	if s.cfg.Gamepad.SwapAB {
		switch button {
		case "a":
			return "b"
		case "b":
			return "a"
		}
	}
	return button
}

// mappingButtons are the buttons a function can be put on ("" = off).
var mappingButtons = []string{"a", "b", "x", "y", "lb", "rb", "lt", "rt", "start", "select", ""}

type mappingDef struct {
	key      string
	field    func(*ButtonsConfig) *string
	noAxis   bool // triggers (lt/rt) not supported for this function
	required bool // cannot be switched off
}

var mappingDefs = []mappingDef{
	{"b_press", func(b *ButtonsConfig) *string { return &b.Press }, true, true},
	{"b_backspace", func(b *ButtonsConfig) *string { return &b.Backspace }, false, false},
	{"b_space", func(b *ButtonsConfig) *string { return &b.Space }, false, false},
	{"b_enter", func(b *ButtonsConfig) *string { return &b.Enter }, false, false},
	{"b_shift", func(b *ButtonsConfig) *string { return &b.Shift }, false, false},
	{"b_close", func(b *ButtonsConfig) *string { return &b.Close }, false, false},
	{"b_left_click", func(b *ButtonsConfig) *string { return &b.LeftClick }, true, false},
	{"b_right_click", func(b *ButtonsConfig) *string { return &b.RightClick }, true, false},
	{"b_position", func(b *ButtonsConfig) *string { return &b.PositionToggle }, false, false},
}

// mappingRow builds a settings row that puts one function on a button.
// A button already used by another function is swapped with it.
func (s *settingsUI) mappingRow(def mappingDef) settingsRow {
	var choices []string
	for _, b := range mappingButtons {
		if (def.noAxis && (b == "lt" || b == "rt")) || (def.required && b == "") {
			continue
		}
		choices = append(choices, b)
	}
	return settingsRow{label: s.t(def.key), kind: rowChoice,
		options: func() []string {
			p := s.effectiveProfile()
			out := make([]string, len(choices))
			for i, b := range choices {
				if b == "" {
					out[i] = s.t("off")
				} else {
					out[i] = p.Label(s.physical(b))
				}
			}
			return out
		},
		get: func() int { return indexOf(choices, *def.field(&s.cfg.Gamepad.Buttons)) },
		set: func(v int) {
			bc := &s.cfg.Gamepad.Buttons
			old, nb := *def.field(bc), choices[v]
			if nb != "" {
				for _, other := range mappingDefs {
					if other.key == def.key || *other.field(bc) != nb {
						continue
					}
					// The other function takes this one's old button, if it can
					if old == "" && other.required ||
						other.noAxis && (old == "lt" || old == "rt") {
						continue
					}
					*other.field(bc) = old
					s.setStatus(fmt.Sprintf(s.t("swapped"), s.t(other.key)))
				}
			}
			*def.field(bc) = nb
		}}
}

func (s *settingsUI) buildRows() {
	themes := settingsThemeOrder()
	layouts := []string{"auto", "de", "us"}
	comboOptions := func() []string {
		out := make([]string, len(comboChoices))
		for i, c := range comboChoices {
			out[i] = s.comboLabel(c)
		}
		return out
	}
	general := []settingsRow{
		{label: s.t("autostart"), kind: rowToggle,
			get: func() int { return b2i(s.autostart) }, set: func(v int) { s.autostart = v == 1 }},
		{label: s.t("theme"), kind: rowChoice,
			options: func() []string {
				out := make([]string, len(themes))
				for i, n := range themes {
					out[i] = s.t(n)
				}
				return out
			},
			get: func() int { return indexOf(themes, s.cfg.Theme.Name) },
			set: func(v int) { s.cfg.Theme.Name = themes[v] }},
		{label: s.t("layout"), kind: rowChoice,
			options: func() []string {
				return []string{s.t("auto") + ": " + s.t(s.detLayout+"_short"), s.t("de"), s.t("us")}
			},
			get: func() int { return indexOf(layouts, s.cfg.Keys.Layout) },
			set: func(v int) { s.cfg.Keys.Layout = layouts[v] }},
		{label: s.t("profile"), kind: rowChoice,
			options: func() []string {
				out := []string{s.t("auto") + ": " + GetControllerProfile(s.detectedOr()).Name}
				for _, id := range profileChoices[1:] {
					name := GetControllerProfile(id).Name
					if id == ProfileGeneric {
						name = s.t("generic")
					}
					out = append(out, name)
				}
				return out
			},
			get: func() int { return indexOf(profileChoices, s.cfg.Gamepad.Controller) },
			set: func(v int) { s.cfg.Gamepad.Controller = profileChoices[v] }},
		{label: s.t("combo"), kind: rowChoice, options: comboOptions,
			get: func() int { return indexOf(comboChoices, s.cfg.Gamepad.ToggleCombo) },
			set: func(v int) { s.cfg.Gamepad.ToggleCombo = comboChoices[v] },
			skip: func() bool {
				return s.cfg.Gamepad.ToggleCombo != "" && s.cfg.Gamepad.ToggleCombo == s.cfg.Gamepad.MouseCombo
			}},
		{label: s.t("mouse_combo"), kind: rowChoice, options: comboOptions,
			get: func() int { return indexOf(comboChoices, s.cfg.Gamepad.MouseCombo) },
			set: func(v int) { s.cfg.Gamepad.MouseCombo = comboChoices[v] },
			skip: func() bool {
				return s.cfg.Gamepad.MouseCombo != "" && s.cfg.Gamepad.ToggleCombo == s.cfg.Gamepad.MouseCombo
			}},
		{label: s.t("mouse"), kind: rowChoice,
			options: func() []string { return []string{s.t("rstick"), s.t("lstick"), s.t("off")} },
			get: func() int {
				switch {
				case !s.cfg.Mouse.Enabled:
					return 2
				case s.cfg.Gamepad.MouseStick == "left":
					return 1
				}
				return 0
			},
			set: func(v int) {
				s.cfg.Mouse.Enabled = v != 2
				if v == 1 {
					s.cfg.Gamepad.MouseStick = "left"
				} else if v == 0 {
					s.cfg.Gamepad.MouseStick = "right"
				}
			}},
		{label: s.t("speed"), kind: rowSlider, min: 1, max: 50, step: 1,
			get: func() int { return s.cfg.Mouse.Sensitivity }, set: func(v int) { s.cfg.Mouse.Sensitivity = v }},
		{label: s.t("mouse_curve"), kind: rowSlider, min: 0, max: 100, step: 10, suffix: " %", zero: s.t("off"),
			get: func() int { return s.cfg.Mouse.Curve }, set: func(v int) { s.cfg.Mouse.Curve = v }},
		{label: s.t("nav_curve"), kind: rowSlider, min: 0, max: 100, step: 10, suffix: " %", zero: s.t("off"),
			get: func() int { return s.cfg.Gamepad.NavCurve }, set: func(v int) { s.cfg.Gamepad.NavCurve = v }},
		{label: s.t("opacity"), kind: rowSlider, min: 0, max: 80, step: 5, suffix: " %",
			get: func() int { return int(100 - s.cfg.Window.Opacity*100 + 0.5) },
			set: func(v int) { s.cfg.Window.Opacity = float64(100-v) / 100 }},
		{label: s.t("legend"), kind: rowToggle,
			get: func() int { return b2i(s.cfg.Window.Legend) }, set: func(v int) { s.cfg.Window.Legend = v == 1 }},
		{label: s.t("position"), kind: rowChoice,
			options: func() []string { return []string{s.t("bottom"), s.t("top")} },
			get:     func() int { return indexOf([]string{"bottom", "top"}, s.cfg.Window.Position) },
			set:     func(v int) { s.cfg.Window.Position = []string{"bottom", "top"}[v] }},
		{label: s.t("size"), kind: rowSlider, min: 30, max: 100, step: 5, suffix: " %",
			get: func() int { return s.cfg.Keys.Scale }, set: func(v int) { s.cfg.Keys.Scale = v }},
	}
	buttons := []settingsRow{
		{label: s.t("swap_ab"), kind: rowToggle,
			get: func() int { return b2i(s.cfg.Gamepad.SwapAB) }, set: func(v int) { s.cfg.Gamepad.SwapAB = v == 1 }},
		{label: s.t("key_badges"), kind: rowToggle,
			get: func() int { return b2i(s.cfg.Window.KeyBadges) }, set: func(v int) { s.cfg.Window.KeyBadges = v == 1 }},
	}
	for _, def := range mappingDefs {
		buttons = append(buttons, s.mappingRow(def))
	}
	s.pages = [3][]settingsRow{general, buttons, nil}
	s.rows = s.pages[s.page]
}

// setPage switches between "General", "Buttons" and "Test".
func (s *settingsUI) setPage(p int) {
	p = (p%3 + 3) % 3
	if p == s.page {
		return
	}
	s.page = p
	s.rows = s.pages[p]
	s.focus = 0
	if p == 2 {
		s.focus = 1 // no rows: focus "Close"
	}
}

func (s *settingsUI) detectedOr() string {
	if s.detectedProf != "" {
		return s.detectedProf
	}
	return ProfileXbox
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// change moves a row's value by dir (-1/+1); toggles flip on any direction.
func (s *settingsUI) change(i, dir int) {
	if i < 0 || i >= len(s.rows) {
		return
	}
	r := s.rows[i]
	switch r.kind {
	case rowToggle:
		r.set(1 - r.get())
	case rowChoice:
		n := len(r.options())
		v := r.get()
		for tries := 0; tries < n; tries++ {
			v = ((v+dir)%n + n) % n
			r.set(v)
			if r.skip == nil || !r.skip() {
				break
			}
		}
	case rowSlider:
		v := r.get() + dir*r.step
		r.set(min(r.max, max(r.min, v)))
	}
	s.confirmClose = false
	s.previewDirty = true
}

func (s *settingsUI) changed() bool {
	return s.cfg != s.orig || s.autostart != s.origAutostart
}

func (s *settingsUI) setStatus(msg string) {
	s.status = msg
	s.statusUntil = time.Now().Add(4 * time.Second)
}

func (s *settingsUI) save() {
	path := UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // G301: config dir
		s.setStatus(s.t("error") + err.Error())
		return
	}
	f, err := os.Create(path) //nolint:gosec // G304: user config path
	if err != nil {
		s.setStatus(s.t("error") + err.Error())
		return
	}
	werr := writeINI(f, s.cfg)
	_ = f.Close()
	if werr != nil {
		s.setStatus(s.t("error") + werr.Error())
		return
	}
	if s.autostart != s.origAutostart {
		if err := SetAutostart(s.autostart); err != nil {
			s.setStatus(s.t("error") + err.Error())
			return
		}
	}
	s.orig, s.origAutostart = s.cfg, s.autostart
	s.confirmClose = false
	if RestartKeyboard() {
		s.setStatus(s.t("restarted"))
	} else {
		s.setStatus(s.t("saved"))
	}
	// Controller buttons in this window follow the saved mapping (e.g. A/B)
	s.reopenGamepad()
}

func (s *settingsUI) requestClose() {
	if s.changed() && !s.confirmClose {
		s.confirmClose = true
		s.setStatus(s.t("unsaved"))
		return
	}
	s.running = false
}

func (s *settingsUI) activate() {
	switch {
	case s.focus < len(s.rows):
		s.change(s.focus, +1)
	case s.focus == len(s.rows):
		s.save()
	default:
		s.requestClose()
	}
}

func (s *settingsUI) moveFocus(d int) {
	n := len(s.rows) + 2
	s.focus = ((s.focus+d)%n + n) % n
}

func (s *settingsUI) reopenGamepad() {
	if s.gamepad != nil {
		s.gamepad.Close()
	}
	gcfg := s.orig
	gcfg.Gamepad.Grab = false
	s.gamepad = NewGamepadReader(gcfg)
	if s.gamepad.Open("") {
		s.padName = s.gamepad.deviceName
		if s.padName == "" {
			s.padName = readDeviceInfo(s.gamepadPath()).Name
		}
		s.detectedProf = detectProfile(readDeviceInfo(s.gamepadPath()), s.gamepad.dinputLayout)
	}
}

func (s *settingsUI) gamepadPath() string {
	if s.gamepad == nil || s.gamepad.fd == nil {
		return ""
	}
	return s.gamepad.fd.Name()
}

// --- Layout and drawing ---

const (
	setMargin   = 24
	setHeaderH  = 64
	setRowH     = 44
	setColGap   = 16
	setButtonsH = 64
	setHintH    = 30
)

var (
	setBg      = c(242, 242, 247)
	setPanel   = c(255, 255, 255)
	setLine    = c(229, 229, 234)
	setText    = c(16, 16, 18)
	setSubtle  = c(110, 110, 118)
	setBlue    = c(10, 132, 255)
	setFocusBg = c(228, 239, 255)
	setGreen   = c(52, 199, 89)
	setOffGray = c(214, 214, 219)
)

func (s *settingsUI) rowsPerColumn() int { return (len(s.rows) + 1) / 2 }

// maxRowsPerColumn sizes the window for the longest page.
func (s *settingsUI) maxRowsPerColumn() int {
	return (max(len(s.pages[0]), len(s.pages[1])) + 1) / 2
}

// layoutWindow computes window size and the preview area for a screen height.
func (s *settingsUI) layoutWindow(screenH int32) {
	s.winW = 900
	rowsH := int32(s.maxRowsPerColumn()) * setRowH //nolint:gosec // G115: small count
	fixed := int32(setHeaderH + setMargin + rowsH + setMargin + setButtonsH + setHintH)
	previewW := s.winW - 2*setMargin
	previewH := previewW * 52 / 100 // keyboard + legend aspect
	if avail := screenH - 90 - fixed; avail < previewH && avail > 160 {
		previewH = avail
	}
	s.previewRect = Rect{X: setMargin, Y: setHeaderH, W: previewW, H: previewH}
	s.winH = fixed + previewH
}

// rebuildPreview renders the keyboard with the working config into the preview area.
func (s *settingsUI) rebuildPreview() {
	if s.preview != nil {
		s.preview.Destroy()
		s.preview = nil
	}
	cfg := s.cfg
	layoutName := ResolveLayout(cfg.Keys.Layout)
	layout := LayoutByName(layoutName)
	lang := UILanguage(layoutName)
	KeyGlyphs = BuildKeyGlyphs(cfg.Gamepad)

	pad := int32(5)
	area := s.previewRect
	// Largest unit that fits width and height (≈ 7.6 units tall incl. legend)
	unit := (area.W - 15*pad) / 15
	for unit > 20 {
		_, h := CalcWindowSize(layout, unit, pad, max32(20, int32(float64(unit)*0.4)))
		if cfg.Window.Legend {
			h += int32(float64(unit)*1.1) + pad
		}
		if h <= area.H {
			break
		}
		unit--
	}
	statusH := max32(20, int32(float64(unit)*0.4))
	w, h := CalcWindowSize(layout, unit, pad, statusH)
	fonts, err := LoadFonts(unit, 0)
	if err != nil {
		log.Printf("preview: %v", err)
		return
	}
	var lines [][]LegendItem
	var lm legendMetrics
	if cfg.Window.Legend {
		lines, lm = fonts.MeasureLegend(BuildLegend(cfg, s.effectiveProfile(), lang, true), unit, w-2*pad)
		h += LegendHeight(lines, lm, pad)
	}
	r := NewRenderer(s.sdl, GetTheme(cfg.Theme.Name), unit, pad, fonts, lines, lm)
	r.KeyBadges = BuildKeyBadges(cfg, s.effectiveProfile())
	r.Embedded = true
	r.EmbedW, r.EmbedH = w, h
	s.preview = r
	s.previewKB = NewKeyboardState(layout)
	s.previewKB.CursorRow, s.previewKB.CursorCol = 3, 6
	s.previewDirty = false
}

func (s *settingsUI) draw() {
	r := s.ui
	setColor(s.sdl, setBg)
	SDL3RenderClear(s.sdl)

	// Header
	r.renderText(s.uiFonts.Chip, s.t("title"), setText,
		FRect{X: setMargin, Y: 10, W: float32(s.winW), H: 26}, AlignLeft)
	sub := s.t("controller") + ": " + s.t("none")
	if s.padName != "" {
		sub = s.t("controller") + ": " + s.padName
	}
	r.renderText(s.uiFonts.Small, sub, setSubtle, FRect{X: setMargin, Y: 34, W: float32(s.winW), H: 22}, AlignLeft)

	// Page tabs (segmented control, top right)
	segW, segH := float32(150), float32(34)
	segW = 130
	seg := FRect{X: float32(s.winW-setMargin) - 3*segW, Y: 14, W: 3 * segW, H: segH}
	r.fillRoundRect(seg, 9, c(224, 224, 229))
	for i, key := range []string{"page_general", "page_buttons", "page_test"} {
		tr := FRect{X: seg.X + float32(i)*segW, Y: seg.Y, W: segW, H: segH}
		s.tabRects[i] = tr
		col := setSubtle
		if i == s.page {
			r.fillRoundRect(FRect{X: tr.X + 2, Y: tr.Y + 2, W: tr.W - 4, H: tr.H - 4}, 7, setPanel)
			col = setText
		}
		r.renderText(s.uiFonts.Small, s.t(key), col, tr, AlignCenter)
	}

	// Preview, centred in its area
	if s.preview != nil {
		area := s.previewRect
		vp := Rect{X: area.X + (area.W-s.preview.EmbedW)/2, Y: area.Y + (area.H-s.preview.EmbedH)/2,
			W: s.preview.EmbedW, H: s.preview.EmbedH}
		SDL3SetRenderViewport(s.sdl, &vp)
		s.preview.MarkDirty()
		s.preview.Draw(s.previewKB)
		SDL3SetRenderViewport(s.sdl, nil)
	}

	// Settings panel, two columns (test page: controller picture)
	top := s.previewRect.Y + s.previewRect.H + setMargin
	if s.page == 2 {
		s.drawTester(FRect{X: setMargin, Y: float32(top), W: float32(s.winW - 2*setMargin),
			H: float32(int32(s.maxRowsPerColumn()) * setRowH)}) //nolint:gosec // G115: small count
	}
	colW := (s.winW - 2*setMargin - setColGap) / 2
	per := s.rowsPerColumn()
	s.rowRects = s.rowRects[:0]
	for col := 0; col < 2; col++ {
		n := min(per, len(s.rows)-col*per)
		if n <= 0 {
			continue
		}
		px := float32(setMargin + int32(col)*(colW+setColGap))                                   //nolint:gosec // G115: col is 0/1
		panel := FRect{X: px, Y: float32(top), W: float32(colW), H: float32(int32(n) * setRowH)} //nolint:gosec // G115: small count
		r.fillRoundRect(panel, 12, setPanel)
		for k := 0; k < n; k++ {
			i := col*per + k
			rr := FRect{X: px, Y: float32(top) + float32(k*setRowH), W: float32(colW), H: setRowH}
			s.rowRects = append(s.rowRects, rr)
			s.drawRow(i, rr, k == 0, k == n-1)
		}
	}

	// Buttons (right aligned) and status (left)
	by := float32(top + int32(s.maxRowsPerColumn())*setRowH + 14) //nolint:gosec // G115: small count
	bw, bh := float32(150), float32(40)
	s.btnRects[0] = FRect{X: float32(s.winW-setMargin) - bw, Y: by, W: bw, H: bh}        // save
	s.btnRects[1] = FRect{X: float32(s.winW-setMargin) - 2*bw - 12, Y: by, W: bw, H: bh} // close
	s.drawButton(s.btnRects[0], s.t("save"), true, s.focus == len(s.rows), s.changed())
	s.drawButton(s.btnRects[1], s.t("close"), false, s.focus == len(s.rows)+1, true)
	if s.status != "" && time.Now().Before(s.statusUntil) {
		r.renderText(s.uiFonts.Small, s.status, setSubtle, FRect{X: setMargin, Y: by, W: 400, H: bh}, AlignLeft)
	}

	// Controller hint
	// (follows the saved mapping, which is what this window listens to)
	p := GetControllerProfile(s.detectedOr())
	ob := s.orig.Gamepad.Buttons
	label := func(b string) string {
		if s.orig.Gamepad.SwapAB && (b == "a" || b == "b") {
			b = map[string]string{"a": "b", "b": "a"}[b]
		}
		return p.Label(b)
	}
	parts := []string{"✚ ↑↓ " + s.t("select"), "✚ ←→ " + s.t("change_v"), label(ob.Press) + " " + s.t("toggle_v")}
	if ob.RightClick != "" && ob.LeftClick != "" {
		parts = append(parts, label(ob.RightClick)+"/"+label(ob.LeftClick)+" "+s.t("page_v"))
	}
	if ob.PositionToggle != "" {
		parts = append(parts, label(ob.PositionToggle)+" "+s.t("save_v"))
	}
	if ob.Close != "" {
		parts = append(parts, label(ob.Close)+" "+s.t("close_v"))
	}
	hint := strings.Join(parts, " · ")
	if s.page == 2 {
		hint = fmt.Sprintf(s.t("test_hint"), label("lb"), label("rb"))
	}
	r.renderText(s.uiFonts.Small, hint, setSubtle,
		FRect{X: 0, Y: by + bh + 10, W: float32(s.winW), H: setHintH}, AlignCenter)

	SDL3RenderPresent(s.sdl)
}

func (s *settingsUI) drawRow(i int, rr FRect, first, last bool) {
	r := s.ui
	row := s.rows[i]
	if i == s.focus {
		// Focus: tinted row, rounded where it meets the panel corners
		fr := rr
		r.fillRoundRect(FRect{X: fr.X + 4, Y: fr.Y + 3, W: fr.W - 8, H: fr.H - 6}, 9, setFocusBg)
	}
	if !last {
		setColor(s.sdl, setLine)
		SDL3RenderFillRect(s.sdl, FRect{X: rr.X + 16, Y: rr.Y + rr.H - 1, W: rr.W - 16, H: 1})
	}
	_ = first
	r.renderText(s.uiFonts.Small, row.label, setText, FRect{X: rr.X + 16, Y: rr.Y, W: rr.W / 2, H: rr.H}, AlignLeft)

	right := rr.X + rr.W - 14
	switch row.kind {
	case rowToggle:
		on := row.get() == 1
		sw := FRect{X: right - 50, Y: rr.Y + (rr.H-28)/2, W: 50, H: 28}
		bg := setOffGray
		if on {
			bg = setGreen
		}
		r.fillRoundRect(sw, 14, bg)
		kx := sw.X + 2
		if on {
			kx = sw.X + sw.W - 26
		}
		r.fillRoundRect(FRect{X: kx, Y: sw.Y + 2, W: 24, H: 24}, 12, c(255, 255, 255))
	case rowChoice:
		opts := row.options()
		val := opts[min(row.get(), len(opts)-1)]
		vw := float32(210)
		area := FRect{X: right - vw, Y: rr.Y, W: vw, H: rr.H}
		r.renderText(s.uiFonts.Font, "‹", setBlue, FRect{X: area.X, Y: rr.Y - 2, W: 24, H: rr.H}, AlignCenter)
		r.renderText(s.uiFonts.Font, "›", setBlue, FRect{X: area.X + area.W - 24, Y: rr.Y - 2, W: 24, H: rr.H}, AlignCenter)
		r.renderText(s.uiFonts.Small, val, setSubtle, FRect{X: area.X + 20, Y: rr.Y, W: area.W - 40, H: rr.H}, AlignCenter)
	case rowSlider:
		v := row.get()
		num := fmt.Sprintf("%d%s", v, row.suffix)
		if v == 0 && row.zero != "" {
			num = row.zero
		}
		tw := float32(140)
		track := FRect{X: right - tw - 56, Y: rr.Y + rr.H/2 - 3, W: tw, H: 6}
		r.fillRoundRect(track, 3, setOffGray)
		frac := float32(v-row.min) / float32(row.max-row.min)
		r.fillRoundRect(FRect{X: track.X, Y: track.Y, W: max32f(6, tw*frac), H: 6}, 3, setBlue)
		kx := track.X + tw*frac - 11
		r.fillRoundRect(FRect{X: kx, Y: rr.Y + rr.H/2 - 12, W: 24, H: 24}, 12, c(200, 200, 206))
		r.fillRoundRect(FRect{X: kx + 1, Y: rr.Y + rr.H/2 - 11, W: 22, H: 22}, 11, c(255, 255, 255))
		r.renderText(s.uiFonts.Small, num, setSubtle, FRect{X: right - 50, Y: rr.Y, W: 50, H: rr.H}, AlignCenter)
	}
}

func (s *settingsUI) drawButton(b FRect, label string, primary, focused, enabled bool) {
	r := s.ui
	bg, fg := c(229, 229, 234), setText
	if primary && enabled {
		bg, fg = setBlue, c(255, 255, 255)
	} else if primary {
		fg = setSubtle
	}
	if focused {
		r.fillRoundRect(FRect{X: b.X - 3, Y: b.Y - 3, W: b.W + 6, H: b.H + 6}, b.H/2+3, c(120, 180, 255))
	}
	r.fillRoundRect(b, b.H/2, bg)
	r.renderText(s.uiFonts.Chip, label, fg, b, AlignCenter)
}

// --- Input ---

func inRect(x, y float32, r FRect) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

func (s *settingsUI) handleUI(e UIEvent) {
	switch e.Type {
	case SDL_EVENT_QUIT, SDL_EVENT_WINDOW_CLOSE_REQUESTED:
		s.running = false
	case SDL_EVENT_MOUSE_MOTION:
		for i, rr := range s.rowRects {
			if inRect(e.X, e.Y, rr) {
				s.focus = i
			}
		}
		for i, br := range s.btnRects {
			if inRect(e.X, e.Y, br) {
				s.focus = len(s.rows) + i
			}
		}
	case SDL_EVENT_MOUSE_BUTTON_DOWN:
		for i, tr := range s.tabRects {
			if inRect(e.X, e.Y, tr) {
				s.setPage(i)
				return
			}
		}
		for i, rr := range s.rowRects {
			if !inRect(e.X, e.Y, rr) {
				continue
			}
			s.focus = i
			row := s.rows[i]
			right := rr.X + rr.W - 14
			switch row.kind {
			case rowToggle:
				s.change(i, 1)
			case rowChoice:
				dir := 1
				if e.X < right-105 || e.Button == 3 {
					dir = -1
				}
				s.change(i, dir)
			case rowSlider:
				tw := float32(140)
				tx := right - tw - 56
				if e.X >= tx-12 && e.X <= tx+tw+12 {
					frac := min(1, max(0, (e.X-tx)/tw))
					v := row.min + int(frac*float32(row.max-row.min)+0.5)
					v = (v-row.min)/row.step*row.step + row.min
					row.set(v)
					s.previewDirty = true
				} else if e.X > tx+tw {
					s.change(i, 1)
				} else {
					s.change(i, -1)
				}
			}
		}
		if inRect(e.X, e.Y, s.btnRects[0]) {
			s.save()
		}
		if inRect(e.X, e.Y, s.btnRects[1]) {
			s.requestClose()
		}
	case SDL_EVENT_MOUSE_WHEEL:
		for i, rr := range s.rowRects {
			if inRect(e.X, e.Y, rr) && s.rows[i].kind != rowToggle {
				if e.WheelY > 0 {
					s.change(i, 1)
				} else if e.WheelY < 0 {
					s.change(i, -1)
				}
			}
		}
	case SDL_EVENT_KEY_DOWN:
		switch e.Key {
		case SDLK_UP:
			s.moveFocus(-1)
		case SDLK_DOWN, SDLK_TAB:
			s.moveFocus(1)
		case SDLK_LEFT:
			s.change(s.focus, -1)
		case SDLK_RIGHT:
			s.change(s.focus, 1)
		case SDLK_RETURN, SDLK_SPACE:
			s.activate()
		case SDLK_ESCAPE:
			s.requestClose()
		case SDLK_PAGEUP:
			s.setPage(s.page - 1)
		case SDLK_PAGEDOWN:
			s.setPage(s.page + 1)
		case SDLK_S:
			if e.Ctrl {
				s.save()
			}
		}
	}
}

func (s *settingsUI) handleGamepad(a Action) {
	if s.page == 2 {
		return // test page: buttons only light up (LB + RB leaves, see updateTester)
	}
	switch a.Type {
	case ActionNavigate:
		if a.DY != 0 {
			s.moveFocus(a.DY)
		}
		if a.DX != 0 {
			if s.focus < len(s.rows) {
				s.change(s.focus, a.DX)
			} else {
				// left/right between the two buttons
				if s.focus == len(s.rows) {
					s.focus = len(s.rows) + 1
				} else {
					s.focus = len(s.rows)
				}
			}
		}
	case ActionPress:
		s.activate()
	case ActionClose:
		s.requestClose()
	case ActionPositionToggle:
		s.save()
	case ActionRightClick: // LB by default
		s.setPage(s.page - 1)
	case ActionLeftClick: // RB by default
		s.setPage(s.page + 1)
	}
}

// RunSettings opens the settings window and blocks until it is closed.
func RunSettings(cfg Config) error {
	runtime.LockOSThread()
	ValidateConfig(&cfg)

	s := &settingsUI{cfg: cfg, orig: cfg, running: true}
	s.autostart = AutostartEnabled()
	s.origAutostart = s.autostart
	detected := ResolveLayout("auto")
	s.detLayout = detected
	s.tx = settingsTexts[UILanguage(detected)]
	shot := os.Getenv("GAMEPAD_OSK_SETTINGS_SHOT") // docs screenshots: render off-screen
	if shot == "" {
		s.reopenGamepad()
	}
	s.buildRows()
	switch os.Getenv("GAMEPAD_OSK_SETTINGS_PAGE") { // docs screenshots
	case "buttons":
		s.setPage(1)
	case "test":
		s.setPage(2)
	}
	if shot != "" {
		return renderSettingsShot(s, shot)
	}

	if os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("SDL_VIDEODRIVER") == "" {
		_ = os.Setenv("SDL_VIDEODRIVER", "wayland")
	}
	SDL3SetHint("SDL_APP_ID", "gamepad-osk-settings")
	_ = os.Setenv("SDL_APP_ID", "gamepad-osk-settings")
	SDL3SetHint("SDL_VIDEO_ALLOW_SCREENSAVER", "1")
	if err := SDL3Init(SDL_INIT_VIDEO); err != nil {
		return err
	}
	defer SDL3Quit()

	screenH := int32(1080)
	if b, ok := SDL3GetDisplayBounds(SDL3GetPrimaryDisplay()); ok && b.H > 0 {
		screenH = b.H
	}
	s.layoutWindow(screenH)

	win, err := SDL3CreateWindow(s.t("title"), s.winW, s.winH, 0)
	if err != nil {
		return err
	}
	defer SDL3DestroyWindow(win)
	sdlr, err := SDL3CreateRenderer(win)
	if err != nil {
		return err
	}
	defer SDL3DestroyRenderer(sdlr)
	SDL3SetRenderVSync(sdlr, 1)
	SDL3SetRenderDrawBlendMode(sdlr, 1)
	s.sdl = sdlr

	s.uiFonts, err = LoadFonts(52, 0)
	if err != nil {
		return err
	}
	s.ui = NewRenderer(sdlr, GetTheme("light"), 52, 6, s.uiFonts, nil, legendMetrics{})
	s.ui.Embedded = true
	defer s.ui.Destroy()
	defer func() {
		if s.preview != nil {
			s.preview.Destroy()
		}
		if s.gamepad != nil {
			s.gamepad.Close()
		}
	}()
	s.rebuildPreview()

	lastReconnect := time.Now()
	for s.running {
		for {
			e, ok := SDL3PollUIEvent()
			if !ok {
				break
			}
			s.handleUI(e)
		}
		if s.gamepad != nil {
			if s.gamepad.Fd() >= 0 {
				for _, a := range s.gamepad.ReadEvents() {
					s.handleGamepad(a)
				}
				s.updateTester()
			} else if time.Since(lastReconnect) > 2*time.Second {
				lastReconnect = time.Now()
				before := s.detectedProf
				s.reopenGamepad()
				if s.detectedProf != before {
					s.previewDirty = true
				}
			}
		}
		if s.previewDirty {
			s.rebuildPreview()
		}
		s.draw()
		SDL3Delay(16)
	}
	return nil
}
