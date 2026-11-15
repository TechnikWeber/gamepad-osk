package main

import (
	"os"
	"path/filepath"
	"strings"
)

// ControllerProfile describes how a controller family labels its buttons.
// Config button names (a, b, x, y, lb, ...) are positional and follow the
// Xbox naming: a = bottom face button, b = right, x = left, y = top.
// A profile only changes what the user sees (legend), not the mapping.
type ControllerProfile struct {
	ID   string // config value: xbox, playstation, nintendo, generic
	Name string // human readable, for the log

	// Labels per positional config name.
	Labels map[string]string
	// Chip colours per positional config name (missing = neutral chip).
	Colors map[string]Color
	// Shapes draws PlayStation symbols instead of text for the face buttons.
	Shapes bool
}

const (
	ProfileAuto        = "auto"
	ProfileXbox        = "xbox"
	ProfilePlayStation = "playstation"
	ProfileNintendo    = "nintendo"
	Profile8BitDo      = "8bitdo"
	ProfileGeneric     = "generic"
)

// ControllerProfiles lists all selectable profiles.
var ControllerProfiles = map[string]ControllerProfile{
	ProfileXbox: {
		ID: ProfileXbox, Name: "Xbox",
		Labels: map[string]string{
			"a": "A", "b": "B", "x": "X", "y": "Y",
			"lb": "LB", "rb": "RB", "lt": "LT", "rt": "RT",
			"l3": "LS", "r3": "RS", "select": "View", "start": "Menu", "guide": "Xbox",
		},
		Colors: map[string]Color{
			"a": c(16, 140, 64), "b": c(214, 40, 40), "x": c(0, 110, 220), "y": c(232, 170, 0),
		},
	},
	ProfilePlayStation: {
		ID: ProfilePlayStation, Name: "PlayStation",
		Labels: map[string]string{
			"a": "✕", "b": "○", "x": "□", "y": "△",
			"lb": "L1", "rb": "R1", "lt": "L2", "rt": "R2",
			"l3": "L3", "r3": "R3", "select": "Create", "start": "Options", "guide": "PS",
		},
		Colors: map[string]Color{
			"a": c(90, 140, 220), "b": c(225, 75, 85), "x": c(210, 90, 190), "y": c(30, 170, 140),
		},
		Shapes: true,
	},
	Profile8BitDo: {
		// 8BitDo Ultimate family: Xbox positions (A bottom, B right),
		// Nintendo-style − + and Home. Retro pads with Nintendo print: swap_ab.
		ID: Profile8BitDo, Name: "8BitDo",
		Labels: map[string]string{
			"a": "A", "b": "B", "x": "X", "y": "Y",
			"lb": "LB", "rb": "RB", "lt": "LT", "rt": "RT",
			"l3": "LS", "r3": "RS", "select": "−", "start": "+", "guide": "Home",
		},
	},
	ProfileNintendo: {
		// Nintendo prints B at the bottom and A on the right.
		ID: ProfileNintendo, Name: "Nintendo",
		Labels: map[string]string{
			"a": "B", "b": "A", "x": "Y", "y": "X",
			"lb": "L", "rb": "R", "lt": "ZL", "rt": "ZR",
			"l3": "LS", "r3": "RS", "select": "−", "start": "+", "guide": "Home",
		},
	},
	ProfileGeneric: {
		ID: ProfileGeneric, Name: "Generic",
		Labels: map[string]string{
			"a": "A", "b": "B", "x": "X", "y": "Y",
			"lb": "LB", "rb": "RB", "lt": "LT", "rt": "RT",
			"l3": "L3", "r3": "R3", "select": "Select", "start": "Start", "guide": "Guide",
		},
	},
}

// Label returns the printed name of a positional config button.
func (p ControllerProfile) Label(button string) string {
	if l, ok := p.Labels[button]; ok {
		return l
	}
	switch button {
	case "up", "dpad_up":
		return "↑"
	case "down", "dpad_down":
		return "↓"
	case "left", "dpad_left":
		return "←"
	case "right", "dpad_right":
		return "→"
	}
	return strings.ToUpper(button)
}

// GetControllerProfile returns the profile for id, falling back to generic.
func GetControllerProfile(id string) ControllerProfile {
	if p, ok := ControllerProfiles[strings.ToLower(id)]; ok {
		return p
	}
	return ControllerProfiles[ProfileGeneric]
}

// deviceInfo collects what sysfs knows about an evdev node.
type deviceInfo struct {
	Name, Driver, Vendor string
	Bluetooth            bool
}

func readDeviceInfo(devPath string) deviceInfo {
	if rp, err := filepath.EvalSymlinks(devPath); err == nil {
		devPath = rp
	}
	base := filepath.Join("/sys/class/input", filepath.Base(devPath), "device")
	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(base, p)) //nolint:gosec // G304: sysfs path built from device name
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	info := deviceInfo{
		Name:      read("name"),
		Vendor:    strings.ToLower(read("id/vendor")),
		Bluetooth: read("id/bustype") == "0005",
	}
	if link, err := os.Readlink(filepath.Join(base, "device", "driver")); err == nil {
		info.Driver = filepath.Base(link)
	}
	return info
}

// detectProfile guesses the controller family from driver, vendor id and name.
// dinput is true when the pad uses the DInput/HID-generic axis layout.
func detectProfile(info deviceInfo, dinput bool) string {
	name := strings.ToLower(info.Name)
	switch info.Driver {
	case "xpad", "xpadneo", "xone":
		return ProfileXbox
	case "hid-playstation", "hid-sony", "sony", "playstation":
		return ProfilePlayStation
	case "hid-nintendo", "nintendo":
		return ProfileNintendo
	}
	switch info.Vendor {
	case "045e": // Microsoft
		return ProfileXbox
	case "054c": // Sony
		return ProfilePlayStation
	case "057e": // Nintendo
		return ProfileNintendo
	case "2dc8": // 8BitDo (as Xbox pad via xpad it is caught above)
		return Profile8BitDo
	}
	switch {
	case strings.Contains(name, "xbox") || strings.Contains(name, "x-box"):
		return ProfileXbox
	case strings.Contains(name, "dualsense") || strings.Contains(name, "dualshock") ||
		strings.Contains(name, "playstation"):
		return ProfilePlayStation
	case strings.Contains(name, "pro controller") || strings.Contains(name, "joy-con") ||
		strings.Contains(name, "nintendo"):
		return ProfileNintendo
	case strings.Contains(name, "8bitdo"):
		return Profile8BitDo
	}
	if dinput {
		return ProfileGeneric
	}
	return ProfileXbox // most pads without own driver mimic Xbox (XInput)
}
