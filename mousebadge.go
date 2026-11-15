package main

// Mouse mode badge and see-through drawing (fork).
//
// In mouse mode the keyboard is hidden and only a small badge sits at the
// bottom centre: a mouse symbol, optionally followed by the buttons that
// work in this mode. The window is click-through on Wayland, so the badge
// never blocks what is behind it.

// beginFrame redirects drawing into an off-screen layer when the frame is
// to be drawn see-through. Returns whether the layer is in use.
func (r *Renderer) beginFrame() bool {
	if r.Embedded || r.Opacity <= 0 || r.Opacity >= 1 {
		return false
	}
	w, h := SDL3GetRenderOutputSize(r.renderer)
	if r.layer == nil || r.layerW != w || r.layerH != h {
		SDL3DestroyTexture(r.layer)
		r.layer = SDL3CreateTargetTexture(r.renderer, w, h)
		r.layerW, r.layerH = w, h
	}
	if r.layer == nil || !SDL3SetRenderTarget(r.renderer, r.layer) {
		return false
	}
	return true
}

// endFrame composes the layer onto the (transparent) window and presents.
func (r *Renderer) endFrame(layered bool) {
	if layered {
		SDL3SetRenderTarget(r.renderer, nil)
		setColor(r.renderer, Color{})
		SDL3RenderClear(r.renderer)
		SDL3SetTextureAlphaMod(r.layer, uint8(r.Opacity*255+0.5))
		SDL3RenderTexture(r.renderer, r.layer, nil, nil)
	}
	SDL3RenderPresent(r.renderer)
}

// badgeGeometry returns the badge metrics: legend metrics for the chips,
// total height, horizontal padding and the mouse icon size.
func (r *Renderer) badgeGeometry() (m legendMetrics, h, padX, iconW, iconH int32) {
	m = newLegendMetrics(r.unit)
	h = m.lineH + m.lineH*2/3
	padX = h * 2 / 5
	iconH = h * 11 / 20
	iconW = iconH * 13 / 20
	return
}

// MouseBadgeSize returns the window size needed for the badge.
func (r *Renderer) MouseBadgeSize(items []LegendItem) (int32, int32) {
	m, h, padX, iconW, _ := r.badgeGeometry()
	// Icon centred in the left round end, then a gap, the chips, right padding
	w := h/2 + iconW/2 + h/4 + padX
	if len(items) > 0 {
		chipW := func(s string) int32 { w, _ := TTF3GetStringSize(r.fontChip, s); return w }
		textW := func(s string) int32 { w, _ := TTF3GetStringSize(r.fontSmall, s); return w }
		for i, it := range items {
			if i > 0 {
				w += m.itemGap
			}
			w += m.itemWidth(it, chipW, textW)
		}
	} else {
		w = h // round badge with just the symbol
	}
	return w, h
}

// DrawMouseBadge draws the mouse-mode badge filling the window.
func (r *Renderer) DrawMouseBadge(items []LegendItem) {
	if r.dirtyFrames <= 0 {
		return
	}
	if isWayland && !IsLayerShellReady() {
		return
	}
	r.dirtyFrames--
	// The badge is small: keep it readable even with a very see-through keyboard
	if r.Opacity > 0 && r.Opacity < 0.85 {
		saved := r.Opacity
		r.Opacity = 0.85
		defer func() { r.Opacity = saved }()
	}
	layered := r.beginFrame()

	t := r.theme
	setColor(r.renderer, Color{})
	SDL3RenderClear(r.renderer)
	m, h, _, iconW, iconH := r.badgeGeometry()
	w, _ := SDL3GetRenderOutputSize(r.renderer)
	r.fillRoundRect(FRect{W: float32(w), H: float32(h)}, float32(h)/2, t.Bg)

	ix := h/2 - iconW/2
	if len(items) == 0 {
		ix = (w - iconW) / 2
	}
	r.drawMouseIcon(FRect{X: float32(ix), Y: float32((h - iconH) / 2), W: float32(iconW), H: float32(iconH)})
	if len(items) > 0 {
		r.drawLegendItems(items, m, h/2+iconW/2+h/4, (h-m.lineH)/2)
	}
	r.endFrame(layered)
}

// drawMouseIcon draws a simple two-button mouse in the accent colour.
func (r *Renderer) drawMouseIcon(b FRect) {
	t := r.theme
	r.fillRoundRect(b, b.W/2, t.HighlightBg)
	line := max32f(1.5, b.W/9)
	split := b.Y + b.H*0.42
	setColor(r.renderer, t.Bg)
	SDL3RenderFillRect(r.renderer, FRect{X: b.X + b.W/2 - line/2, Y: b.Y, W: line, H: split - b.Y})
	SDL3RenderFillRect(r.renderer, FRect{X: b.X, Y: split - line/2, W: b.W, H: line})
}
