package main

import "math"

func max32f(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// fillRoundRect fills a rectangle with rounded corners. Corners are drawn
// scanline by scanline with an anti-aliased edge pixel, which keeps the
// renderer-independent SDL API (no shaders) and still looks smooth.
func (r *Renderer) fillRoundRect(rect FRect, radius float32, col Color) {
	if radius*2 > rect.H {
		radius = rect.H / 2
	}
	if radius*2 > rect.W {
		radius = rect.W / 2
	}
	if radius < 1 {
		setColor(r.renderer, col)
		SDL3RenderFillRect(r.renderer, rect)
		return
	}
	x0, y0 := float32(math.Round(float64(rect.X))), float32(math.Round(float64(rect.Y)))
	w, h := float32(math.Round(float64(rect.W))), float32(math.Round(float64(rect.H)))
	rad := float32(math.Round(float64(radius)))

	// Middle band without corners
	setColor(r.renderer, col)
	SDL3RenderFillRect(r.renderer, FRect{X: x0, Y: y0 + rad, W: w, H: h - 2*rad})

	rows := int(rad)
	for i := 0; i < rows; i++ {
		// Distance of this scanline's centre from the corner circle centre
		dy := float64(rad) - (float64(i) + 0.5)
		inset := float64(rad) - math.Sqrt(math.Max(0, float64(rad*rad)-dy*dy))
		full := math.Ceil(inset)
		frac := full - inset // coverage of the partial edge pixel
		for _, y := range []float32{y0 + float32(i), y0 + h - 1 - float32(i)} {
			setColor(r.renderer, col)
			SDL3RenderFillRect(r.renderer, FRect{X: x0 + float32(full), Y: y, W: w - 2*float32(full), H: 1})
			if frac > 0.02 && full >= 1 {
				edge := col
				edge.A = uint8(float64(col.A) * frac)
				setColor(r.renderer, edge)
				SDL3RenderFillRect(r.renderer, FRect{X: x0 + float32(full) - 1, Y: y, W: 1, H: 1})
				SDL3RenderFillRect(r.renderer, FRect{X: x0 + w - float32(full), Y: y, W: 1, H: 1})
			}
		}
	}
}

// legendChipColors returns background and text colour of a legend chip.
func (r *Renderer) legendChipColors(c LegendChip) (Color, Color) {
	if c.Color.A > 0 {
		return c.Color, Color{R: 255, G: 255, B: 255, A: 255}
	}
	return r.theme.ModifierBg, r.theme.KeyText
}

// drawLegend draws the wrapped legend lines, centred, starting at y.
func (r *Renderer) drawLegend(y int32) {
	m := r.legendM
	sw := r.outW()
	chipW := func(s string) int32 { w, _ := TTF3GetStringSize(r.fontChip, s); return w }
	textW := func(s string) int32 { w, _ := TTF3GetStringSize(r.fontSmall, s); return w }

	for li, line := range r.legend {
		var total int32
		for i, it := range line {
			if i > 0 {
				total += m.itemGap
			}
			total += m.itemWidth(it, chipW, textW)
		}
		x := (sw - total) / 2
		ly := y + int32(li)*m.lineH //nolint:gosec // G115: line index fits in int32
		r.drawLegendItems(line, m, x, ly)
	}
}

// drawKeyBadge draws a small button chip into the top-left corner of a key.
func (r *Renderer) drawKeyBadge(chip LegendChip, x, y int32, onCursor bool, keyBg Color) {
	f := r.fonts.Badge
	tw, th := TTF3GetStringSize(f, chip.Label)
	h := th + 2
	w := max32(h, tw+h/2+2)
	inset := max32(3, r.unit/14)
	rect := FRect{X: float32(x + inset), Y: float32(y + inset), W: float32(w), H: float32(h)}
	bg, fg := r.legendChipColors(chip)
	if onCursor && chip.Color.A == 0 {
		// neutral chip on the blue focus: white chip, focus colour text
		bg, fg = Color{R: 255, G: 255, B: 255, A: 255}, r.theme.HighlightBg
	} else if bg == keyBg {
		// neutral chip on a key of the same colour (Shift, Caps): use the letter-key colour
		bg, fg = r.theme.KeyBg, r.theme.KeyText
	}
	if r.theme.Radius > 0 {
		r.fillRoundRect(rect, float32(h)/2, bg)
	} else {
		setColor(r.renderer, bg)
		SDL3RenderFillRect(r.renderer, rect)
	}
	r.renderText(f, chip.Label, fg, rect, AlignCenter)
}

// drawLegendItems draws one legend line starting at x; returns the end x.
func (r *Renderer) drawLegendItems(line []LegendItem, m legendMetrics, x, ly int32) int32 {
	chipW := func(s string) int32 { w, _ := TTF3GetStringSize(r.fontChip, s); return w }
	textW := func(s string) int32 { w, _ := TTF3GetStringSize(r.fontSmall, s); return w }
	chipY := ly + (m.lineH-m.chipH)/2
	for i, it := range line {
		if i > 0 {
			x += m.itemGap
		}
		for ci, c := range it.Chips {
			if ci > 0 {
				x += m.chipGap
			}
			cw := max32(m.chipH, chipW(c.Label)+2*m.chipPad)
			bg, fg := r.legendChipColors(c)
			chip := FRect{X: float32(x), Y: float32(chipY), W: float32(cw), H: float32(m.chipH)}
			if r.theme.Radius > 0 {
				r.fillRoundRect(chip, float32(m.chipH)/2, bg)
			} else {
				setColor(r.renderer, bg)
				SDL3RenderFillRect(r.renderer, chip)
			}
			r.renderText(r.fontChip, c.Label, fg, chip, AlignCenter)
			x += cw
		}
		x += m.textGap
		tw := textW(it.Text)
		r.renderText(r.fontSmall, it.Text, r.theme.ModifierText,
			FRect{X: float32(x), Y: float32(ly), W: float32(tw), H: float32(m.lineH)}, AlignLeft)
		x += tw
	}
	return x
}
