package main

/*
#cgo pkg-config: sdl3
#include <SDL3/SDL.h>

typedef struct {
    Uint32 type;
    float x, y;
    int button;
    Uint32 key;
    Uint16 mod;
    float wheel_y;
} osk_event;

static bool osk_poll(osk_event *out) {
    SDL_Event e;
    if (!SDL_PollEvent(&e)) return false;
    out->type = e.type;
    out->x = 0; out->y = 0; out->button = 0; out->key = 0; out->mod = 0; out->wheel_y = 0;
    switch (e.type) {
    case SDL_EVENT_MOUSE_MOTION:
        out->x = e.motion.x; out->y = e.motion.y; break;
    case SDL_EVENT_MOUSE_BUTTON_DOWN:
    case SDL_EVENT_MOUSE_BUTTON_UP:
        out->x = e.button.x; out->y = e.button.y; out->button = e.button.button; break;
    case SDL_EVENT_MOUSE_WHEEL:
        out->x = e.wheel.mouse_x; out->y = e.wheel.mouse_y; out->wheel_y = e.wheel.y; break;
    case SDL_EVENT_KEY_DOWN:
        out->key = e.key.key; out->mod = e.key.mod; break;
    }
    return true;
}
*/
import "C"

// UIEvent is the subset of an SDL event the settings window needs.
type UIEvent struct {
	Type   uint32
	X, Y   float32
	Button int
	Key    uint32
	Ctrl   bool
	WheelY float32
}

//nolint:revive // Match SDL3 naming convention
const (
	SDL_EVENT_WINDOW_CLOSE_REQUESTED = uint32(C.SDL_EVENT_WINDOW_CLOSE_REQUESTED)
	SDL_EVENT_MOUSE_MOTION           = uint32(C.SDL_EVENT_MOUSE_MOTION)
	SDL_EVENT_MOUSE_BUTTON_DOWN      = uint32(C.SDL_EVENT_MOUSE_BUTTON_DOWN)
	SDL_EVENT_MOUSE_WHEEL            = uint32(C.SDL_EVENT_MOUSE_WHEEL)
	SDL_EVENT_KEY_DOWN               = uint32(C.SDL_EVENT_KEY_DOWN)
	SDL_EVENT_WINDOW_EXPOSED         = uint32(C.SDL_EVENT_WINDOW_EXPOSED)

	SDLK_UP     = uint32(C.SDLK_UP)
	SDLK_DOWN   = uint32(C.SDLK_DOWN)
	SDLK_LEFT   = uint32(C.SDLK_LEFT)
	SDLK_RIGHT  = uint32(C.SDLK_RIGHT)
	SDLK_RETURN = uint32(C.SDLK_RETURN)
	SDLK_SPACE  = uint32(C.SDLK_SPACE)
	SDLK_ESCAPE = uint32(C.SDLK_ESCAPE)
	SDLK_TAB    = uint32(C.SDLK_TAB)
	SDLK_S      = uint32(C.SDLK_S)
	SDLK_PAGEUP   = uint32(C.SDLK_PAGEUP)
	SDLK_PAGEDOWN = uint32(C.SDLK_PAGEDOWN)
)

// SDL3PollUIEvent returns the next event with mouse/key details.
func SDL3PollUIEvent() (UIEvent, bool) {
	var e C.osk_event
	if !C.osk_poll(&e) {
		return UIEvent{}, false
	}
	return UIEvent{
		Type:   uint32(e._type),
		X:      float32(e.x),
		Y:      float32(e.y),
		Button: int(e.button),
		Key:    uint32(e.key),
		Ctrl:   e.mod&C.SDL_KMOD_CTRL != 0,
		WheelY: float32(e.wheel_y),
	}, true
}

// SDL3SetRenderViewport limits drawing to rect (nil = whole target).
func SDL3SetRenderViewport(r *SDLRenderer, rect *Rect) {
	if rect == nil {
		C.SDL_SetRenderViewport((*C.SDL_Renderer)(r.ptr), nil)
		return
	}
	cr := C.SDL_Rect{x: C.int(rect.X), y: C.int(rect.Y), w: C.int(rect.W), h: C.int(rect.H)}
	C.SDL_SetRenderViewport((*C.SDL_Renderer)(r.ptr), &cr)
}
