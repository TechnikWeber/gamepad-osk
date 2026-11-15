package main

/*
#cgo pkg-config: sdl3
#include <SDL3/SDL.h>
*/
import "C"
import "unsafe"

// Fork additions: window resizing and off-screen render targets (used for
// see-through drawing on Wayland and the small mouse-mode badge).

func SDL3SetWindowSize(w *Window, width, height int32) {
	C.SDL_SetWindowSize((*C.SDL_Window)(w.ptr), C.int(width), C.int(height))
}

// SDL3CreateTargetTexture creates a texture the renderer can draw into.
func SDL3CreateTargetTexture(r *SDLRenderer, w, h int32) *Texture {
	t := C.SDL_CreateTexture((*C.SDL_Renderer)(r.ptr), C.SDL_PIXELFORMAT_ARGB8888,
		C.SDL_TEXTUREACCESS_TARGET, C.int(w), C.int(h))
	if t == nil {
		return nil
	}
	C.SDL_SetTextureBlendMode(t, C.SDL_BLENDMODE_BLEND)
	return &Texture{ptr: unsafe.Pointer(t)}
}

// SDL3SetRenderTarget redirects drawing into t (nil = the window again).
func SDL3SetRenderTarget(r *SDLRenderer, t *Texture) bool {
	var tp *C.SDL_Texture
	if t != nil {
		tp = (*C.SDL_Texture)(t.ptr)
	}
	return bool(C.SDL_SetRenderTarget((*C.SDL_Renderer)(r.ptr), tp))
}

func SDL3SetTextureAlphaMod(t *Texture, a uint8) {
	C.SDL_SetTextureAlphaMod((*C.SDL_Texture)(t.ptr), C.Uint8(a))
}
