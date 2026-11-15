package main

import (
	"bytes"
	"testing"
)

// Settings written by the settings window must load back unchanged.
func TestWriteINIRoundTripForkKeys(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Theme.Name = "night"
	cfg.Window.Legend = false
	cfg.Keys.Layout = "de"
	cfg.Gamepad.Controller = Profile8BitDo
	cfg.Gamepad.SwapAB = true
	cfg.Gamepad.ToggleCombo = "select+start"
	cfg.Gamepad.MouseStick = "left"
	cfg.Mouse.Enabled = false
	cfg.Mouse.Sensitivity = 23
	cfg.Keys.Scale = 65

	var buf bytes.Buffer
	if err := writeINI(&buf, cfg); err != nil {
		t.Fatal(err)
	}
	got := DefaultConfig()
	if err := parseINI(&buf, &got); err != nil {
		t.Fatal(err)
	}
	if got != cfg {
		t.Errorf("round trip mismatch:\n got  %+v\n want %+v", got, cfg)
	}
}

func TestSwapABExchangesFunctions(t *testing.T) {
	g := DefaultConfig().Gamepad
	if buttonTableFor(g)["a"].EvdevBtn != BTN_SOUTH {
		t.Fatal("a should be BTN_SOUTH without swap")
	}
	g.SwapAB = true
	tbl := buttonTableFor(g)
	if tbl["a"].EvdevBtn != BTN_EAST || tbl["b"].EvdevBtn != BTN_SOUTH {
		t.Errorf("swap_ab: a=%#x b=%#x", tbl["a"].EvdevBtn, tbl["b"].EvdevBtn)
	}
	am := BuildActionMap(g)
	if am.BtnPress[BTN_EAST] != ActionPress {
		t.Error("with swap_ab the right button should type")
	}
}

func TestLegendFollowsSwapAB(t *testing.T) {
	cfg := DefaultConfig()
	p := GetControllerProfile(ProfileXbox)
	first := BuildLegend(cfg, p, "en", true)[0]
	if first.Chips[0].Label != "A" || first.Text != "Type" {
		t.Errorf("first legend item = %+v", first)
	}
	cfg.Gamepad.SwapAB = true
	first = BuildLegend(cfg, p, "en", true)[0]
	if first.Chips[0].Label != "B" {
		t.Errorf("with swap_ab typing should show B, got %q", first.Chips[0].Label)
	}
}

func TestDetectProfile(t *testing.T) {
	cases := []struct {
		info   deviceInfo
		dinput bool
		want   string
	}{
		{deviceInfo{Driver: "xpad"}, false, ProfileXbox},
		{deviceInfo{Driver: "hid-playstation"}, false, ProfilePlayStation},
		{deviceInfo{Driver: "hid-nintendo"}, false, ProfileNintendo},
		{deviceInfo{Driver: "hid-generic", Vendor: "2dc8", Name: "8BitDo Ultimate 2C Wireless"}, true, Profile8BitDo},
		{deviceInfo{Driver: "hid-generic", Vendor: "054c"}, false, ProfilePlayStation},
		{deviceInfo{Driver: "hid-generic", Vendor: "1234", Name: "Some Pad"}, true, ProfileGeneric},
		{deviceInfo{Driver: "hid-generic", Vendor: "1234", Name: "Some Pad"}, false, ProfileXbox},
	}
	for _, c := range cases {
		if got := detectProfile(c.info, c.dinput); got != c.want {
			t.Errorf("detectProfile(%+v, %v) = %s, want %s", c.info, c.dinput, got, c.want)
		}
	}
}

func TestGermanLayoutDetection(t *testing.T) {
	for in, want := range map[string]bool{
		"de": true, "de-nodeadkeys": true, "de(nodeadkeys)": true, "at": true,
		"us": false, "fr": false, "dvorak": false,
	} {
		if got := isGermanLayout(in); got != want {
			t.Errorf("isGermanLayout(%q) = %v, want %v", in, got, want)
		}
	}
	if ResolveLayout("us") != "us" || ResolveLayout("de") != "de" {
		t.Error("explicit layouts must win over detection")
	}
}

// Every layout row must be as wide as the others (15 units) so the
// keyboard keeps its shape.
func TestLayoutRowWidths(t *testing.T) {
	for name, l := range map[string][][]KeyDef{"us": LayoutQWERTY, "de": LayoutQWERTZ} {
		for i, row := range l {
			sum := 0.0
			for _, k := range row {
				sum += k.Width
			}
			if sum != 15 {
				t.Errorf("%s row %d is %.2f units wide, want 15", name, i, sum)
			}
		}
	}
}

func TestQWERTZUndoRedoUseSwappedCodes(t *testing.T) {
	// German keymap: KEY_Y types z, so Undo (Ctrl+Z) must send KEY_Y
	row := LayoutQWERTZ[0]
	if row[2].Code != KEY_Y || row[3].Code != KEY_Z {
		t.Errorf("undo/redo codes = %d/%d", row[2].Code, row[3].Code)
	}
	if KeyAction(row[1]) != "paste" || KeyAction(LayoutQWERTZ[5][len(LayoutQWERTZ[5])-1]) != "cfg" {
		t.Error("translated keys must keep their actions")
	}
}
