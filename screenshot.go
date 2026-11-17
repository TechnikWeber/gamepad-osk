package main

import (
	"fmt"
	"os"
	"runtime"
)

// ScreenshotOptions controls an off-screen render of the keyboard.
type ScreenshotOptions struct {
	Path        string // output .bmp
	Profile     string // controller profile; "" or auto = xbox
	ScreenWidth int32  // pretend monitor width for auto-sizing (default 1920)
	CursorRow   int
	CursorCol   int
	Shift       bool
	MouseBadge  bool // render the mouse-mode badge instead of the keyboard
}

// RenderScreenshot draws the keyboard once into a BMP file. It needs no
// display, gamepad or uinput, so it runs in CI and on headless machines.
func RenderScreenshot(cfg Config, o ScreenshotOptions) error {
	runtime.LockOSThread()
	ValidateConfig(&cfg)
	layoutName := ResolveLayout(cfg.Keys.Layout)
	layout := LayoutByName(layoutName)
	lang := UILanguage(layoutName)

	if o.ScreenWidth <= 0 {
		o.ScreenWidth = 1920
	}
	profileID := o.Profile
	if profileID == "" || profileID == ProfileAuto {
		profileID = ProfileXbox
	}
	profile := GetControllerProfile(profileID)
	KeyGlyphs = BuildKeyGlyphs(cfg.Gamepad)

	unit := CalcUnitSize(layout, o.ScreenWidth, cfg)
	pad := int32(cfg.Keys.Padding) //nolint:gosec // G115: padding fits in int32
	statusH := max32(20, int32(float64(unit)*0.4))
	width, height := CalcWindowSize(layout, unit, pad, statusH)

	fonts, err := LoadFonts(unit, cfg.Keys.FontSize)
	if err != nil {
		return err
	}
	var lines [][]LegendItem
	var lm legendMetrics
	if cfg.Window.Legend {
		lines, lm = fonts.MeasureLegend(BuildLegend(cfg, profile, lang, true), unit, width-2*pad)
		height += LegendHeight(lines, lm, pad)
	}

	if o.MouseBadge {
		return renderMouseBadgeShot(cfg, o, profile, lang, unit, pad, fonts)
	}

	sdlr, surf, err := SDL3CreateSurfaceRenderer(width, height)
	if err != nil {
		fonts.Close()
		return err
	}
	defer SDL3DestroySurface(surf)
	defer SDL3DestroyRenderer(sdlr)
	SDL3SetRenderDrawBlendMode(sdlr, 1)

	rend := NewRenderer(sdlr, GetTheme(cfg.Theme.Name), unit, pad, fonts, lines, lm)
	rend.KeyBadges = BuildKeyBadges(cfg, profile)
	defer rend.Destroy()

	kb := NewKeyboardState(layout)
	if o.CursorRow >= 0 && o.CursorRow < len(layout) {
		kb.CursorRow = o.CursorRow
		if o.CursorCol >= 0 && o.CursorCol < len(layout[o.CursorRow]) {
			kb.CursorCol = o.CursorCol
		}
	}
	kb.ShiftActive = o.Shift
	rend.Draw(kb)

	if err := SDL3SaveBMP(surf, o.Path); err != nil {
		return err
	}
	fmt.Printf("Saved %dx%d %s (theme %s, layout %s, controller %s)\n",
		width, height, o.Path, cfg.Theme.Name, layoutName, profile.Name)
	return nil
}

// renderMouseBadgeShot draws the mouse-mode badge on a transparent
// background, nothing behind it (README/blog image).
func renderMouseBadgeShot(cfg Config, o ScreenshotOptions, profile ControllerProfile, lang string, unit, pad int32, fonts *FontSet) error {
	items := BuildMouseLegend(cfg, profile, lang)
	// Measure with a throwaway renderer, then draw at the exact size.
	mr, ms, err := SDL3CreateSurfaceRenderer(1, 1)
	if err != nil {
		fonts.Close()
		return err
	}
	measure := NewRenderer(mr, GetTheme(cfg.Theme.Name), unit, pad, fonts, nil, legendMetrics{})
	w, h := measure.MouseBadgeSize(items)
	SDL3DestroyRenderer(mr)
	SDL3DestroySurface(ms)

	sdlr, surf, err := SDL3CreateSurfaceRenderer(w, h)
	if err != nil {
		fonts.Close()
		return err
	}
	defer SDL3DestroySurface(surf)
	defer SDL3DestroyRenderer(sdlr)
	SDL3SetRenderDrawBlendMode(sdlr, 1)
	rend := NewRenderer(sdlr, GetTheme(cfg.Theme.Name), unit, pad, fonts, nil, legendMetrics{})
	defer rend.Destroy()
	rend.MarkDirty()
	rend.DrawMouseBadge(items)
	if err := SDL3SaveBMP(surf, o.Path); err != nil {
		return err
	}
	fmt.Printf("Saved %dx%d %s (mouse badge, theme %s, controller %s)\n", w, h, o.Path, cfg.Theme.Name, profile.Name)
	return nil
}

// renderSettingsShot draws the settings window once into a BMP file
// (README/blog images). GAMEPAD_OSK_SETTINGS_PAD sets the controller name
// shown in the header; [gamepad] controller sets the button names.
func renderSettingsShot(s *settingsUI, path string) error {
	s.padName = os.Getenv("GAMEPAD_OSK_SETTINGS_PAD")
	if p := s.cfg.Gamepad.Controller; p != "" && p != ProfileAuto {
		s.detectedProf = p
	}
	s.layoutWindow(1080)
	sdlr, surf, err := SDL3CreateSurfaceRenderer(s.winW, s.winH)
	if err != nil {
		return err
	}
	defer SDL3DestroySurface(surf)
	defer SDL3DestroyRenderer(sdlr)
	SDL3SetRenderDrawBlendMode(sdlr, 1)
	s.sdl = sdlr
	s.uiFonts, err = LoadFonts(52, 0)
	if err != nil {
		return err
	}
	s.ui = NewRenderer(sdlr, GetTheme("light"), 52, 6, s.uiFonts, nil, legendMetrics{})
	s.ui.Embedded = true
	defer s.ui.Destroy()
	s.rebuildPreview()
	defer func() {
		if s.preview != nil {
			s.preview.Destroy()
		}
	}()
	s.draw()
	if err := SDL3SaveBMP(surf, path); err != nil {
		return err
	}
	fmt.Printf("Saved %dx%d %s (settings page %d)\n", s.winW, s.winH, path, s.page)
	return nil
}
