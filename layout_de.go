package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// KEY_102ND is the extra key left of Y on ISO keyboards (< > | on German layouts).
const KEY_102ND = 86 //nolint:revive // Match Linux evdev naming

// altgr adds an AltGr label to a key (sent as RightAlt + key).
func altgr(kd KeyDef, label string) KeyDef {
	kd.AltGrLabel = label
	return kd
}

// action tags a key with a built-in function independent of its label.
func action(kd KeyDef, name string) KeyDef {
	kd.Action = name
	return kd
}

var (
	accentAde = []AccentDef{{"à", 0xe0}, {"á", 0xe1}, {"â", 0xe2}, {"å", 0xe5}, {"æ", 0xe6}}
	accentEde = []AccentDef{{"è", 0xe8}, {"é", 0xe9}, {"ê", 0xea}, {"ë", 0xeb}}
	accentOde = []AccentDef{{"ò", 0xf2}, {"ó", 0xf3}, {"ô", 0xf4}, {"ø", 0xf8}}
	accentUde = []AccentDef{{"ù", 0xf9}, {"ú", 0xfa}, {"û", 0xfb}}
)

// LayoutQWERTZ is the German layout. Key codes are physical (evdev) positions;
// the desktop's German keymap turns them into the printed characters, so
// KEY_Y types "z" and KEY_Z types "y". Undo/Redo use those swapped codes too.
var LayoutQWERTZ = [][]KeyDef{
	// Row 0: Shortcuts (shift shows F-keys)
	{
		k("Esc", KEY_ESC, 1.5),
		action(k("Einfg", KEY_V, 1.0, "Kopie"), "paste"),
		combo("Rückg", KEY_Y, []int{KEY_LEFTCTRL}, "F1", KEY_F1, 1.0),
		combo("Wiedh", KEY_Z, []int{KEY_LEFTCTRL}, "F2", KEY_F2, 1.0),
		combo("Aussch", KEY_X, []int{KEY_LEFTCTRL}, "F3", KEY_F3, 1.0),
		combo("Alles", KEY_A, []int{KEY_LEFTCTRL}, "F4", KEY_F4, 1.0),
		action(combo("AltTab", KEY_TAB, []int{KEY_LEFTALT}, "F5", KEY_F5, 1.0), "alttab"),
		combo("Pos1", KEY_HOME, nil, "F6", KEY_F6, 1.0),
		combo("Ende", KEY_END, nil, "F7", KEY_F7, 1.0),
		combo("Bild↑", KEY_PAGEUP, nil, "F8", KEY_F8, 1.0),
		combo("Bild↓", KEY_PAGEDOWN, nil, "F9", KEY_F9, 1.0),
		combo("Play", KEY_PLAYPAUSE, nil, "F10", KEY_F10, 1.0),
		combo("Stumm", KEY_MUTE, nil, "F11", KEY_F11, 1.0),
		k("Entf", KEY_DELETE, 1.5),
	},
	// Row 1: Numbers
	{
		k("^", KEY_GRAVE, "°"), k("1", KEY_1, "!"), altgr(k("2", KEY_2, "\""), "²"),
		altgr(k("3", KEY_3, "§"), "³"), k("4", KEY_4, "$"), k("5", KEY_5, "%"),
		k("6", KEY_6, "&"), altgr(k("7", KEY_7, "/"), "{"), altgr(k("8", KEY_8, "("), "["),
		altgr(k("9", KEY_9, ")"), "]"), altgr(k("0", KEY_0, "="), "}"), altgr(k("ß", KEY_MINUS, "?"), "\\"),
		k("´", KEY_EQUAL, "`"), k("⌫", KEY_BACKSPACE, 2.0),
	},
	// Row 2: QWERTZ
	{
		k("Tab", KEY_TAB, 1.5),
		altgr(k("q", KEY_Q, "Q"), "@"), k("w", KEY_W, "W"),
		altgr(k("e", KEY_E, "E", accentEde), "€"), k("r", KEY_R, "R"), k("t", KEY_T, "T"),
		k("z", KEY_Y, "Z"), k("u", KEY_U, "U", accentUde),
		k("i", KEY_I, "I", accentI), k("o", KEY_O, "O", accentOde),
		k("p", KEY_P, "P"), k("ü", KEY_LEFTBRACE, "Ü"),
		altgr(k("+", KEY_RIGHTBRACE, "*"), "~"), k("#", KEY_BACKSLASH, 1.5, "'"),
	},
	// Row 3: Home row
	{
		mod("Caps", KEY_CAPSLOCK, 1.75, "caps"),
		k("a", KEY_A, "A", accentAde), k("s", KEY_S, "S"),
		k("d", KEY_D, "D"), k("f", KEY_F, "F"), k("g", KEY_G, "G"),
		k("h", KEY_H, "H"), k("j", KEY_J, "J"), k("k", KEY_K, "K"),
		k("l", KEY_L, "L"), k("ö", KEY_SEMICOLON, "Ö"),
		k("ä", KEY_APOSTROPHE, "Ä"), k("Enter", KEY_ENTER, 2.25),
	},
	// Row 4: Bottom alpha (ISO: extra < > | key)
	{
		mod("Shift", KEY_LEFTSHIFT, 1.25, "shift"),
		altgr(k("<", KEY_102ND, ">"), "|"),
		k("y", KEY_Z, "Y"), k("x", KEY_X, "X"),
		k("c", KEY_C, "C", accentC), k("v", KEY_V, "V"), k("b", KEY_B, "B"),
		k("n", KEY_N, "N", accentN), altgr(k("m", KEY_M, "M"), "µ"),
		k(",", KEY_COMMA, ";"), k(".", KEY_DOT, ":"), k("-", KEY_SLASH, "_"),
		mod("Shift", KEY_RIGHTSHIFT, 2.75, "shift"),
	},
	// Row 5: Space bar
	{
		mod("Strg", KEY_LEFTCTRL, 1.5, "ctrl"),
		mod("Super", KEY_LEFTMETA, 1.25, "meta"),
		mod("Alt", KEY_LEFTALT, 1.25, "alt"),
		k("Leertaste", KEY_SPACE, 5.0),
		mod("AltGr", KEY_RIGHTALT, 1.25, "altgr"),
		k("←", KEY_LEFT, 0.75), k("→", KEY_RIGHT, 0.75), k("↑", KEY_UP, 0.75, "⟹"), k("↓", KEY_DOWN, 0.75, "⟾"),
		action(k("Einstell.", 0, 1.75, "Design"), "cfg"),
	},
}

// LayoutByName returns the key layout for "us" or "de".
func LayoutByName(name string) [][]KeyDef {
	if name == "de" {
		return LayoutQWERTZ
	}
	return LayoutQWERTY
}

// ResolveLayout turns the config value into "us" or "de".
func ResolveLayout(cfgValue string) string {
	switch cfgValue {
	case "us", "de":
		return cfgValue
	}
	if l := detectSystemLayout(); l != "" {
		Debugf("Detected desktop keyboard layout: %s", l)
		if isGermanLayout(l) {
			return "de"
		}
	}
	return "us"
}

func isGermanLayout(xkb string) bool {
	xkb = strings.ToLower(strings.TrimSpace(xkb))
	return xkb == "de" || xkb == "at" || strings.HasPrefix(xkb, "de-") || strings.HasPrefix(xkb, "de_") ||
		strings.HasPrefix(xkb, "de(") || strings.HasPrefix(xkb, "at(")
}

// detectSystemLayout returns the first XKB layout of the desktop, or "".
// Sources, in order: XKB_DEFAULT_LAYOUT, KDE (kxkbrc), GNOME (gsettings),
// the system keyboard config (localectl/Debian), the console keymap.
func detectSystemLayout() string {
	if l := os.Getenv("XKB_DEFAULT_LAYOUT"); l != "" {
		return firstOf(l)
	}
	if home, err := os.UserHomeDir(); err == nil {
		cfgHome := os.Getenv("XDG_CONFIG_HOME")
		if cfgHome == "" {
			cfgHome = filepath.Join(home, ".config")
		}
		if l := iniValue(filepath.Join(cfgHome, "kxkbrc"), "LayoutList"); l != "" {
			return firstOf(l)
		}
	}
	if l := gnomeLayout(); l != "" {
		return l
	}
	if l := xorgConfLayout("/etc/X11/xorg.conf.d/00-keyboard.conf"); l != "" {
		return firstOf(l)
	}
	if l := iniValue("/etc/default/keyboard", "XKBLAYOUT"); l != "" {
		return firstOf(l)
	}
	if l := iniValue("/etc/vconsole.conf", "KEYMAP"); l != "" {
		return l
	}
	return ""
}

func firstOf(list string) string {
	list = strings.Trim(list, "\"' ")
	if i := strings.IndexAny(list, ","); i >= 0 {
		list = list[:i]
	}
	return strings.TrimSpace(list)
}

// iniValue reads key=value from a simple config file (sections ignored).
func iniValue(path, key string) string {
	f, err := os.Open(path) //nolint:gosec // G304: fixed system/user config paths
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), "\"'")
		}
	}
	return ""
}

func xorgConfLayout(path string) string {
	f, err := os.Open(path) //nolint:gosec // G304: fixed system config path
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 3 && fields[0] == "Option" && strings.Trim(fields[1], "\"") == "XkbLayout" {
			return strings.Trim(fields[2], "\"")
		}
	}
	return ""
}

// gnomeLayout reads the first input source, e.g. [('xkb', 'de+nodeadkeys')].
func gnomeLayout() string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.input-sources", "sources").Output()
	if err != nil {
		return ""
	}
	s := string(out)
	i := strings.Index(s, "'xkb', '")
	if i < 0 {
		return ""
	}
	s = s[i+len("'xkb', '"):]
	if j := strings.IndexAny(s, "+'"); j >= 0 {
		s = s[:j]
	}
	return s
}
