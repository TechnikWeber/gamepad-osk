package main

import (
	"fmt"
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
