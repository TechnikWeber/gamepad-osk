package main

import (
	"strings"
	"time"
)

// --- Settings page "Test": shows which button the controller reports ---
//
// Every button lights up on a controller picture while held, the sticks
// show their position, and the last pressed button is shown with what it
// does in gamepad-osk. Helps to spot swapped buttons and unknown symbols
// ("the 8BitDo logo is Home").

type testerState struct {
	held     map[string]bool // positional names held in the last frame
	last     string          // last pressed positional name
	lastTime time.Time
	backHeld bool // LB + RB were held (leave the page once on press)
}

// testerHeld returns the positional names of all buttons held right now.
func (s *settingsUI) testerHeld() map[string]bool {
	held := map[string]bool{}
	gp := s.gamepad
	if gp == nil || gp.fd == nil {
		return held
	}
	// evdev button -> positional name (raw positions, swap_ab not applied)
	for name, info := range buttonTable(isSwapXY(gp.config.Gamepad)) {
		if gp.btnHeld[info.EvdevBtn] {
			held[name] = true
		}
		if info.IsAxis && !gp.dinputLayout && gp.normalizeAxis(info.EvdevAxis, gp.axisState[info.EvdevAxis]) > -0.4 {
			if _, seen := gp.axisState[info.EvdevAxis]; seen {
				held[name] = true
			}
		}
	}
	if gp.btnHeld[0x13c] {
		held["guide"] = true
	}
	hx, hy := gp.axisState[ABS_HAT0X], gp.axisState[ABS_HAT0Y]
	held["up"] = hy < 0 || gp.btnHeld[BTN_DPAD_UP]
	held["down"] = hy > 0 || gp.btnHeld[BTN_DPAD_DOWN]
	held["left"] = hx < 0 || gp.btnHeld[BTN_DPAD_LEFT]
	held["right"] = hx > 0 || gp.btnHeld[BTN_DPAD_RIGHT]
	return held
}

// updateTester tracks newly pressed buttons and LB + RB (leave the page).
func (s *settingsUI) updateTester() {
	if s.page != 2 {
		s.tester.held = nil
		return
	}
	held := s.testerHeld()
	for name, on := range held {
		if on && !s.tester.held[name] {
			s.tester.last, s.tester.lastTime = name, time.Now()
		}
	}
	s.tester.held = held
	back := held["lb"] && held["rb"]
	if back && !s.tester.backHeld {
		s.tester.backHeld = true
		s.setPage(0)
		return
	}
	s.tester.backHeld = back
}

// testerLabel is the printed name of a positional button in the picture.
func (s *settingsUI) testerLabel(name string) string {
	switch name {
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	}
	return s.effectiveProfile().Label(name)
}

// testerFunctions lists what a positional button does with the current
// (unsaved) settings.
func (s *settingsUI) testerFunctions(name string) []string {
	var out []string
	bc := s.cfg.Gamepad.Buttons
	for _, def := range mappingDefs {
		if b := *def.field(&bc); b != "" && s.physical(b) == name {
			out = append(out, s.t(def.key))
		}
	}
	mouseClick, capsClick := "r3", "l3"
	if s.cfg.Gamepad.MouseStick == "left" {
		mouseClick, capsClick = "l3", "r3"
	}
	switch name {
	case mouseClick:
		out = append(out, s.t("b_left_click"))
	case capsClick:
		out = append(out, s.t("caps"))
	case "up", "down", "left", "right":
		out = append(out, s.t("nav"))
	}
	for _, c := range []struct{ combo, key string }{
		{s.cfg.Gamepad.ToggleCombo, "combo"},
		{s.cfg.Gamepad.MouseCombo, "mouse_combo"},
	} {
		parts := strings.Split(c.combo, "+")
		for i, p := range parts {
			if p != name || c.combo == "" {
				continue
			}
			var others []string
			for j, o := range parts {
				if j != i {
					others = append(others, s.testerLabel(o))
				}
			}
			label := strings.TrimSuffix(strings.TrimSuffix(s.t(c.key), " mit"), " with")
			out = append(out, label+" ("+s.t("part_of")+" "+strings.Join(others, " + ")+")")
		}
	}
	if len(out) == 0 {
		out = append(out, s.t("unused"))
	}
	return out
}

// drawTester draws the controller picture and the last pressed button.
func (s *settingsUI) drawTester(p FRect) {
	r := s.ui
	r.fillRoundRect(p, 12, setPanel)
	prof := s.effectiveProfile()
	held := s.tester.held

	chip := func(name string, x, y, w float32) {
		rect := FRect{X: p.X + x, Y: p.Y + y, W: w, H: 30}
		bg, fg := c(229, 229, 234), setText
		if held[name] {
			bg, fg = setBlue, c(255, 255, 255)
			if col, ok := prof.Colors[name]; ok && col.A > 0 {
				bg = col
			}
		}
		r.fillRoundRect(rect, 15, bg)
		r.renderText(s.uiFonts.Chip, s.testerLabel(name), fg, rect, AlignCenter)
	}

	// Shoulders and triggers
	chip("lt", 30, 14, 64)
	chip("lb", 30, 50, 64)
	chip("rt", 450, 14, 64)
	chip("rb", 450, 50, 64)
	// Centre: select, home, start
	chip("select", 170, 50, 64)
	chip("guide", 240, 50, 64)
	chip("start", 310, 50, 64)
	// D-pad
	dx, dy := float32(90), float32(140)
	chip("up", dx-22, dy-46, 44)
	chip("down", dx-22, dy+16, 44)
	chip("left", dx-72, dy-15, 44)
	chip("right", dx+28, dy-15, 44)
	// Face buttons (positions: a bottom, b right, x left, y top)
	fx, fy := float32(482), float32(140)
	chip("y", fx-22, fy-46, 44)
	chip("a", fx-22, fy+16, 44)
	chip("x", fx-72, fy-15, 44)
	chip("b", fx+28, fy-15, 44)

	// Sticks: circle, dot at the stick position, filled while clicked
	stick := func(cx, cy float32, axX, axY uint16, click string) {
		const rad = 42
		bg := c(229, 229, 234)
		if held[click] {
			bg = c(190, 220, 255)
		}
		r.fillRoundRect(FRect{X: p.X + cx - rad, Y: p.Y + cy - rad, W: 2 * rad, H: 2 * rad}, rad, bg)
		var nx, ny float32
		if gp := s.gamepad; gp != nil && gp.fd != nil {
			if _, ok := gp.axisState[axX]; ok {
				nx = float32(gp.normalizeAxis(axX, gp.axisState[axX]))
			}
			if _, ok := gp.axisState[axY]; ok {
				ny = float32(gp.normalizeAxis(axY, gp.axisState[axY]))
			}
		}
		const dot = 18
		r.fillRoundRect(FRect{X: p.X + cx + nx*(rad-dot/2) - dot/2, Y: p.Y + cy + ny*(rad-dot/2) - dot/2,
			W: dot, H: dot}, dot/2, setBlue)
		r.renderText(s.uiFonts.Small, s.testerLabel(click), setSubtle,
			FRect{X: p.X + cx - rad, Y: p.Y + cy + rad + 2, W: 2 * rad, H: 20}, AlignCenter)
	}
	rxAx, ryAx := uint16(ABS_RX), uint16(ABS_RY)
	if s.gamepad != nil && s.gamepad.dinputLayout {
		rxAx, ryAx = ABS_Z, ABS_RZ
	}
	stick(205, 175, ABS_X, ABS_Y, "l3")
	stick(355, 175, rxAx, ryAx, "r3")

	// Right: last pressed button and what it does
	ix := p.X + 580
	iw := p.W - 580 - 20
	setColor(s.sdl, setLine)
	SDL3RenderFillRect(s.sdl, FRect{X: ix - 20, Y: p.Y + 16, W: 1, H: p.H - 32})
	r.renderText(s.uiFonts.Small, s.t("last"), setSubtle, FRect{X: ix, Y: p.Y + 14, W: iw, H: 22}, AlignLeft)
	if s.tester.last == "" {
		r.renderText(s.uiFonts.Small, s.t("press_any"), setText, FRect{X: ix, Y: p.Y + 50, W: iw, H: 30}, AlignLeft)
		return
	}
	name := s.tester.last
	label := s.testerLabel(name)
	tw, _ := TTF3GetStringSize(s.uiFonts.Font, label)
	big := FRect{X: ix, Y: p.Y + 44, W: float32(max32(48, tw+28)), H: 44}
	bg := setBlue
	if col, ok := prof.Colors[name]; ok && col.A > 0 {
		bg = col
	}
	r.fillRoundRect(big, 22, bg)
	r.renderText(s.uiFonts.Font, label, c(255, 255, 255), big, AlignCenter)
	y := p.Y + 100
	for _, f := range s.testerFunctions(name) {
		r.renderText(s.uiFonts.Small, "→ "+f, setText, FRect{X: ix, Y: y, W: iw, H: 24}, AlignLeft)
		y += 26
	}
}
