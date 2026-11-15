package main

import (
	"errors"
	"sync"
	"strings"
	"time"
	"unsafe"
)

const promptFontPath = "/usr/share/fonts/TTF/promptfont.ttf" // Arch package path

type texCacheKey struct {
	text string
	font uintptr // font pointer as identity
	color Color
}

type texCacheEntry struct {
	tex  *Texture
	w, h float32
}

type Renderer struct {
	renderer       *SDLRenderer
	theme          Theme
	unit           int32
	pad            int32
	statusH        int32 // height of modifier status bar
	font           *Font
	fontSmall      *Font
	fontGlyph      *Font
	fontChip       *Font // bold, for legend chips
	fonts          *FontSet
	flashText      string
	flashGlyphText string // rendered with Promptfont, placed before flashText
	flashEnd       time.Time
	texCache       map[texCacheKey]texCacheEntry
	dirtyFrames    int // redraw counter for double/triple buffering

	legend  [][]LegendItem // wrapped legend lines (nil = no legend)
	legendM legendMetrics

	// Embedded: draw into a viewport of another window (settings preview):
	// no clear/present, background as rounded panel of EmbedW x EmbedH.
	Embedded       bool
	EmbedW, EmbedH int32
}

// outW is the drawing width used for centring.
func (r *Renderer) outW() int32 {
	if r.Embedded {
		return r.EmbedW
	}
	w, _ := SDL3GetRenderOutputSize(r.renderer)
	return w
}

// FontSet holds all fonts plus the fallback fonts they reference.
type FontSet struct {
	Font, Small, Chip, Glyph *Font
	HasPromptFont            bool    // Glyph is Promptfont (controller icons available)
	owned                    []*Font // everything to close, fallbacks included
}

// fallbackCodepoints are symbols used in layouts and the legend that many
// UI fonts lack (arrows, erase, PlayStation shapes, d-pad, euro, micro).
var fallbackCodepoints = []string{"2190", "232b", "2715", "25cb", "25a1", "25b3", "271a", "2212", "20ac", "00b5", "00b2", "27f9", "27fe"}

var (
	fallbackOnce  sync.Once
	fallbackFiles []string
)

// fallbackFontFiles returns fonts that cover fallbackCodepoints, deduplicated,
// in the order fontconfig suggests them. Looked up once per process.
func fallbackFontFiles(mainFont string) []string {
	fallbackOnce.Do(func() {
		seen := map[string]bool{mainFont: true}
		for _, cp := range fallbackCodepoints {
			if p := fcMatch("sans-serif:charset=" + cp); p != "" && !seen[p] {
				seen[p] = true
				fallbackFiles = append(fallbackFiles, p)
			}
		}
		if p := findFontFile("DejaVuSans.ttf", "NotoSansSymbols2-Regular.ttf"); p != "" && !seen[p] {
			fallbackFiles = append(fallbackFiles, p)
		}
	})
	return fallbackFiles
}

var (
	uiFontOnce sync.Once
	uiFontPath string
)

// LoadFonts opens the UI font at sizes derived from the key unit and wires
// up fallback fonts for symbols. fontSize > 0 overrides the label size.
func LoadFonts(unit int32, fontSize int) (*FontSet, error) {
	if err := TTF3Init(); err != nil {
		return nil, err
	}
	uiFontOnce.Do(func() { uiFontPath = findUIFont() })
	fontPath := uiFontPath
	if fontPath == "" {
		return nil, errors.New("no suitable font found (install e.g. DejaVu Sans or Noto Sans)")
	}
	Debugf("UI font: %s", fontPath)

	size := max32(12, int32(float64(unit)*0.34))
	if fontSize > 0 {
		size = int32(fontSize) //nolint:gosec // G115: font size fits in int32
	}
	small := max32(10, size-5)
	glyphSize := max32(12, int32(float64(unit)*0.28))

	fs := &FontSet{}
	open := func(path string, pt int32) *Font {
		f, err := TTF3OpenFont(path, float32(pt))
		if err != nil {
			return nil
		}
		fs.owned = append(fs.owned, f)
		return f
	}

	fs.Font = open(fontPath, size)
	fs.Small = open(fontPath, small)
	fs.Chip = open(fontPath, small)
	if fs.Font == nil || fs.Small == nil || fs.Chip == nil {
		fs.Close()
		return nil, errors.New("cannot open font " + fontPath)
	}
	TTF3SetBold(fs.Chip)

	for _, p := range fallbackFontFiles(fontPath) {
		for _, pair := range []struct {
			f  *Font
			pt int32
		}{{fs.Font, size}, {fs.Small, small}, {fs.Chip, small}} {
			if fb := open(p, pair.pt); fb != nil {
				TTF3AddFallback(pair.f, fb)
			}
		}
	}

	if pf := findPromptFont(); pf != "" {
		fs.Glyph = open(pf, glyphSize)
		fs.HasPromptFont = fs.Glyph != nil
	}
	if fs.Glyph == nil {
		fs.Glyph = fs.Small
	}
	return fs, nil
}

// Close releases all fonts of the set.
func (fs *FontSet) Close() {
	if fs == nil {
		return
	}
	for _, f := range fs.owned {
		TTF3CloseFont(f)
	}
	fs.owned = nil
}

// MeasureLegend wraps the legend for the given width using the set's fonts.
func (fs *FontSet) MeasureLegend(items []LegendItem, unit, maxW int32) ([][]LegendItem, legendMetrics) {
	m := newLegendMetrics(unit)
	chipW := func(s string) int32 { w, _ := TTF3GetStringSize(fs.Chip, s); return w }
	textW := func(s string) int32 { w, _ := TTF3GetStringSize(fs.Small, s); return w }
	return layoutLegend(items, m, maxW, chipW, textW), m
}

// LegendHeight is the extra window height the wrapped legend needs.
func LegendHeight(lines [][]LegendItem, m legendMetrics, pad int32) int32 {
	if len(lines) == 0 {
		return 0
	}
	return int32(len(lines))*m.lineH + pad //nolint:gosec // G115: line count fits in int32
}

func NewRenderer(r *SDLRenderer, theme Theme, unitSize, padding int32, fs *FontSet,
	legend [][]LegendItem, lm legendMetrics) *Renderer {
	return &Renderer{
		renderer:    r,
		theme:       theme,
		unit:        unitSize,
		pad:         padding,
		statusH:     max32(20, int32(float64(unitSize)*0.4)),
		font:        fs.Font,
		fontSmall:   fs.Small,
		fontGlyph:   fs.Glyph,
		fontChip:    fs.Chip,
		fonts:       fs,
		texCache:    make(map[texCacheKey]texCacheEntry),
		dirtyFrames: 3, // force initial draw
		legend:      legend,
		legendM:     lm,
	}
}

func (r *Renderer) Draw(kb *KeyboardState) {
	if r.dirtyFrames <= 0 {
		return
	}
	// Wayland: don't render until layer-shell configure is received
	if isWayland && !IsLayerShellReady() {
		return
	}
	r.dirtyFrames--

	t := r.theme
	if r.Embedded {
		r.fillRoundRect(FRect{W: float32(r.EmbedW), H: float32(r.EmbedH)}, float32(r.unit)/4, t.Bg)
	} else {
		setColor(r.renderer, t.Bg)
		SDL3RenderClear(r.renderer)
	}

	// Draw modifier status bar
	r.drawModifierBar(kb)

	// Draw keyboard rows
	y := r.pad + r.statusH
	for ri, row := range kb.Layout {
		x := r.pad
		for ci, key := range row {
			isCursor := ri == kb.CursorRow && ci == kb.CursorCol
			r.drawKey(key, x, y, isCursor, kb)
			x += int32(key.Width*float64(r.unit)) + r.pad
		}
		y += r.unit + r.pad
	}

	// Button legend below the keys
	if len(r.legend) > 0 {
		r.drawLegend(y)
	}

	// Accent popup
	if kb.AccentPopup != nil {
		r.drawAccentPopup(kb)
	}

	if !r.Embedded {
		SDL3RenderPresent(r.renderer)
	}
}

// MarkDirty signals that the display needs updating.
// Redraws for 3 frames to cover double/triple buffering.
func (r *Renderer) MarkDirty() {
	r.dirtyFrames = 3
}

func (r *Renderer) drawModifierBar(kb *KeyboardState) {
	// Modifier pills (left-aligned)
	var labels []string
	if kb.ShiftActive {
		labels = append(labels, "SHIFT")
	}
	if kb.CapsActive {
		labels = append(labels, "CAPS")
	}
	if kb.CtrlActive {
		labels = append(labels, "CTRL")
	}
	if kb.AltActive {
		labels = append(labels, "ALT")
	}
	if kb.MetaActive {
		labels = append(labels, "SUPER")
	}
	if kb.AltTabHeld {
		labels = append(labels, "ALT-TAB")
	}

	x := r.pad
	tc := r.theme.ModifierActiveText
	for _, label := range labels {
		r.drawPill(label, x, r.pad, r.theme.ModifierActiveBg, tc)
		tw, _ := TTF3GetStringSize(r.fontSmall, label)
		x += tw + 10 + 6
	}

	// Status flash (right-aligned, 2 seconds)
	if (r.flashText != "" || r.flashGlyphText != "") && time.Now().Before(r.flashEnd) {
		rightEdge := float32(r.outW() - r.pad)

		// Render text part (right-aligned)
		if r.flashText != "" {
			entry := r.cachedText(r.fontSmall, r.flashText, r.theme.KeyText)
			if entry.tex != nil {
				dst := FRect{X: rightEdge - entry.w, Y: float32(r.pad + 2), W: entry.w, H: entry.h}
				SDL3RenderTexture(r.renderer, entry.tex, nil, &dst)
				rightEdge = dst.X - 4
			}
		}

		// Render glyph part (Promptfont, left of text)
		if r.flashGlyphText != "" {
			entry := r.cachedText(r.fontGlyph, r.flashGlyphText, r.theme.KeyText)
			if entry.tex != nil {
				dst := FRect{X: rightEdge - entry.w, Y: float32(r.pad + 2), W: entry.w, H: entry.h}
				SDL3RenderTexture(r.renderer, entry.tex, nil, &dst)
			}
		}
	}
}

func (r *Renderer) drawPill(label string, x, y int32, bg Color, fg Color) {
	entry := r.cachedText(r.fontSmall, label, fg)
	if entry.tex == nil {
		return
	}
	pw := int32(entry.w) + 10
	ph := int32(entry.h) + 4
	pill := FRect{X: float32(x), Y: float32(y), W: float32(pw), H: float32(ph)}
	if r.theme.Radius > 0 {
		r.fillRoundRect(pill, float32(ph)/2, bg)
	} else {
		setColor(r.renderer, bg)
		SDL3RenderFillRect(r.renderer, pill)
	}
	dst := FRect{X: float32(x + 5), Y: float32(y + 2), W: entry.w, H: entry.h}
	SDL3RenderTexture(r.renderer, entry.tex, nil, &dst)
}

// cachedText returns a cached texture entry, rasterizing on first use.
func (r *Renderer) cachedText(font *Font, text string, color Color) texCacheEntry {
	if text == "" || font == nil {
		return texCacheEntry{}
	}
	key := texCacheKey{text: text, font: uintptr(unsafe.Pointer(font)), color: color} //nolint:gosec // G103: pointer used as identity key only, not dereferenced
	if entry, ok := r.texCache[key]; ok {
		return entry
	}
	surf, err := TTF3RenderTextBlended(font, text, color)
	if err != nil {
		return texCacheEntry{}
	}
	tex := SDL3CreateTextureFromSurface(r.renderer, surf)
	w := float32(SDL3SurfaceWidth(surf))
	h := float32(SDL3SurfaceHeight(surf))
	SDL3DestroySurface(surf)
	if tex == nil {
		return texCacheEntry{}
	}
	entry := texCacheEntry{tex: tex, w: w, h: h}
	r.texCache[key] = entry
	return entry
}

func (r *Renderer) drawKey(key KeyDef, x, y int32, isCursor bool, kb *KeyboardState) {
	w := int32(key.Width * float64(r.unit))
	h := r.unit
	rect := FRect{X: float32(x), Y: float32(y), W: float32(w), H: float32(h)}
	t := r.theme

	flashed := kb.IsFlashed(key)
	var bg, border Color
	switch {
	case isCursor || flashed:
		bg, border = t.HighlightBg, t.HighlightBorder
	case key.IsModifier && isModActive(key, kb):
		bg, border = t.ModifierActiveBg, t.HighlightBorder
	case key.IsModifier:
		bg, border = t.ModifierBg, t.KeyBorder
	case strings.HasPrefix(key.Label, "F") && len(key.Label) > 1 && key.Label[1] >= '0' && key.Label[1] <= '9':
		bg, border = t.FnKeyBg, t.KeyBorder
	default:
		bg, border = t.KeyBg, t.KeyBorder
	}

	if t.Radius > 0 {
		// Rounded key with a soft shadow line below (phone-style themes)
		rad := float32(t.Radius * float64(h))
		if t.KeyShadow.A > 0 && !isCursor {
			sh := rect
			sh.Y += max32f(1, float32(h)/40)
			r.fillRoundRect(sh, rad, t.KeyShadow)
		}
		r.fillRoundRect(rect, rad, bg)
	} else {
		// Fill
		setColor(r.renderer, bg)
		SDL3RenderFillRect(r.renderer, rect)
		// Border
		setColor(r.renderer, border)
		SDL3RenderRect(r.renderer, rect)
	}

	// Label
	label := kb.DisplayLabel(key)
	tc := t.KeyText
	if isCursor || flashed {
		tc = Color{R: 255, G: 255, B: 255, A: 255}
		if t.CursorText.A > 0 {
			tc = t.CursorText
		}
	} else if key.IsModifier && isModActive(key, kb) {
		tc = t.ModifierActiveText
	}
	// Use Promptfont for labels containing its codepoints (e.g. mouse speed icons)
	labelFont := r.font
	if r.fontGlyph != nil && hasPromptfontRune(label) {
		labelFont = r.fontGlyph
	}
	r.renderText(labelFont, label, tc, rect, AlignCenter)

	// Shift hint (top-right)
	if key.ShiftLabel != "" && !key.IsModifier && !isCursor {
		if kb.ShiftActive == kb.CapsActive {
			hintFont := r.fontSmall
			if r.fontGlyph != nil && hasPromptfontRune(key.ShiftLabel) {
				hintFont = r.fontGlyph
			}
			r.renderText(hintFont, key.ShiftLabel, t.ModifierText,
				FRect{X: float32(x), Y: float32(y), W: float32(w - 4), H: float32(h)}, AlignTopRight)
		}
	}

	// AltGr hint (bottom-left), German layout: @ € { [ ] } \ ~ | ...
	if key.AltGrLabel != "" && !isCursor && !kb.AltGrActive {
		r.renderText(r.fontSmall, key.AltGrLabel, t.ModifierText,
			FRect{X: float32(x + 4), Y: float32(y), W: float32(w - 4), H: float32(h - 2)}, AlignBottomLeft)
	}

	// Controller glyph (bottom-right); only with Promptfont, the legend
	// explains the buttons otherwise
	if glyph, ok := KeyGlyphs[key.Code]; ok && r.fonts.HasPromptFont {
		r.renderText(r.fontGlyph, glyph, t.GlyphColor,
			FRect{X: float32(x), Y: float32(y), W: float32(w - 3), H: float32(h - 2)}, AlignBottomRight)
	}
}

func (r *Renderer) drawAccentPopup(kb *KeyboardState) {
	accents := kb.AccentPopup.Accents
	sel := kb.AccentPopup.Selected
	row := kb.Layout[kb.CursorRow]

	xo := r.pad
	for i := range kb.CursorCol {
		xo += int32(row[i].Width*float64(r.unit)) + r.pad
	}
	ky := r.pad + r.statusH + int32(kb.CursorRow)*(r.unit+r.pad) //nolint:gosec // G115: cursor index fits in int32
	py := ky - r.unit - r.pad

	tw := int32(len(accents))*(r.unit+r.pad) + r.pad //nolint:gosec // G115: accent count fits in int32
	pr := FRect{X: float32(xo), Y: float32(py), W: float32(tw), H: float32(r.unit + r.pad*2)}
	rounded := r.theme.Radius > 0
	rad := float32(r.theme.Radius * float64(r.unit))
	if rounded {
		if r.theme.KeyShadow.A > 0 {
			sh := pr
			sh.Y += 2
			r.fillRoundRect(sh, rad+float32(r.pad), r.theme.KeyShadow)
		}
		r.fillRoundRect(pr, rad+float32(r.pad), r.theme.AccentPopupBg)
	} else {
		setColor(r.renderer, r.theme.AccentPopupBg)
		SDL3RenderFillRect(r.renderer, pr)
		setColor(r.renderer, r.theme.HighlightBorder)
		SDL3RenderRect(r.renderer, pr)
	}

	ax := xo + r.pad
	for i, accent := range accents {
		ar := FRect{X: float32(ax), Y: float32(py + r.pad), W: float32(r.unit), H: float32(r.unit)}
		tc := r.theme.AccentPopupText
		if i == sel {
			if rounded {
				r.fillRoundRect(ar, rad, r.theme.AccentHighlightBg)
				if r.theme.CursorText.A > 0 {
					tc = r.theme.CursorText
				}
			} else {
				setColor(r.renderer, r.theme.AccentHighlightBg)
				SDL3RenderFillRect(r.renderer, ar)
			}
		}
		r.renderText(r.font, accent.Label, tc, ar, AlignCenter)
		ax += r.unit + r.pad
	}
}

type TextAlign int

const (
	AlignCenter TextAlign = iota
	AlignTopRight
	AlignBottomRight
	AlignBottomLeft
	AlignLeft
)

func (r *Renderer) renderText(font *Font, text string, color Color, rect FRect, align TextAlign) {
	entry := r.cachedText(font, text, color)
	if entry.tex == nil {
		return
	}

	// Shrink labels that do not fit (long translated labels, small keys)
	tw, th := entry.w, entry.h
	if maxW := rect.W - 6; align == AlignCenter && maxW > 0 && tw > maxW {
		th *= maxW / tw
		tw = maxW
	}
	entry.w, entry.h = tw, th

	var dst FRect
	switch align {
	case AlignCenter:
		dst = FRect{
			X: rect.X + (rect.W-entry.w)/2,
			Y: rect.Y + (rect.H-entry.h)/2,
			W: entry.w, H: entry.h,
		}
	case AlignTopRight:
		dst = FRect{
			X: rect.X + rect.W - entry.w,
			Y: rect.Y + 2,
			W: entry.w, H: entry.h,
		}
	case AlignBottomRight:
		dst = FRect{
			X: rect.X + rect.W - entry.w,
			Y: rect.Y + rect.H - entry.h,
			W: entry.w, H: entry.h,
		}
	case AlignBottomLeft:
		dst = FRect{
			X: rect.X,
			Y: rect.Y + rect.H - entry.h,
			W: entry.w, H: entry.h,
		}
	case AlignLeft:
		dst = FRect{
			X: rect.X,
			Y: rect.Y + (rect.H-entry.h)/2,
			W: entry.w, H: entry.h,
		}
	}
	SDL3RenderTexture(r.renderer, entry.tex, nil, &dst)
}

// SetLegend replaces the legend lines (e.g. after a different controller connected).
func (r *Renderer) SetLegend(lines [][]LegendItem) {
	r.legend = lines
	r.flushTexCache()
	r.MarkDirty()
}

func (r *Renderer) SetTheme(t Theme) {
	r.theme = t
	r.flushTexCache()
	r.Flash(t.Name)
}

func (r *Renderer) flushTexCache() {
	for k, entry := range r.texCache {
		if entry.tex != nil {
			SDL3DestroyTexture(entry.tex)
		}
		delete(r.texCache, k)
	}
}

func (r *Renderer) Flash(text string) {
	r.flashText = text
	r.flashGlyphText = ""
	r.flashEnd = time.Now().Add(2 * time.Second)
	r.MarkDirty()
}

// FlashGlyph renders a Promptfont glyph followed by text in the normal font.
func (r *Renderer) FlashGlyph(glyph string, text string) {
	r.flashGlyphText = glyph
	r.flashText = text
	r.flashEnd = time.Now().Add(2 * time.Second)
	r.MarkDirty()
}

func (r *Renderer) Destroy() {
	r.flushTexCache()
	r.fonts.Close()
}

func CalcUnitSize(layout [][]KeyDef, screenWidth int32, cfg Config) int32 {
	if cfg.Keys.UnitSize > 0 {
		return int32(cfg.Keys.UnitSize) //nolint:gosec // G115: unit size fits in int32
	}
	var maxUnits float64
	var maxKeys int
	for _, row := range layout {
		ru := 0.0
		for _, k := range row {
			ru += k.Width
		}
		if ru > maxUnits {
			maxUnits = ru
			maxKeys = len(row)
		}
	}
	scale := float64(cfg.Keys.Scale) / 100.0
	if scale <= 0 {
		scale = 0.70
	}
	target := float64(screenWidth) * scale
	pad := float64(cfg.Keys.Padding)
	unit := (target - float64(maxKeys+1)*pad) / maxUnits
	if unit < 30 {
		unit = 30
	}
	return int32(unit)
}

func CalcWindowSize(layout [][]KeyDef, unit, pad, statusH int32) (int32, int32) {
	var maxW int32
	for _, row := range layout {
		rw := pad
		for _, key := range row {
			rw += int32(key.Width*float64(unit)) + pad
		}
		if rw > maxW {
			maxW = rw
		}
	}
	h := pad + statusH + int32(len(layout))*(unit+pad) //nolint:gosec // G115: layout row count fits in int32
	return maxW, h
}

func setColor(r *SDLRenderer, c Color) {
	SDL3SetRenderDrawColor(r, c)
}

func isModActive(key KeyDef, kb *KeyboardState) bool {
	switch key.ModifierType {
	case "shift":
		return kb.ShiftActive
	case "caps":
		return kb.CapsActive
	case "ctrl":
		return kb.CtrlActive
	case "alt":
		return kb.AltActive
	case "meta":
		return kb.MetaActive
	case "altgr":
		return kb.AltGrActive
	}
	return false
}

// hasPromptfontRune returns true if the string contains codepoints from
// Promptfont's mapped range (U+2600-U+27FF).
func hasPromptfontRune(s string) bool {
	for _, r := range s {
		if r >= 0x2600 && r <= 0x27FF {
			return true
		}
	}
	return false
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
