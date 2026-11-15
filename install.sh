#!/usr/bin/env bash
# gamepad-osk installer (TechnikWeber fork)
#
#   curl -fsSL https://raw.githubusercontent.com/TechnikWeber/gamepad-osk/main/install.sh | bash
#
# Installs for the current user only: binary to ~/.local/bin, autostart as a
# systemd user service. Root (sudo) is asked for only if the system really
# needs it: SDL3 from the package manager (optional, there is a build with
# SDL3 included) or missing access to /dev/uinput.
#
#   install.sh              install or update
#   install.sh --local F    install the binary F instead of downloading it
#   install.sh --portable   use the build with SDL3 included (automatic if
#                           the system has no SDL3 packages)
#   install.sh --uninstall  remove binary, service and udev rule (keeps config)
set -euo pipefail

REPO="TechnikWeber/gamepad-osk"
BIN_DIR="$HOME/.local/bin"
BIN="$BIN_DIR/gamepad-osk"
CFG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
SVC_DIR="$CFG_HOME/systemd/user"
SVC="$SVC_DIR/gamepad-osk.service"
AUTOSTART="$CFG_HOME/autostart/gamepad-osk.desktop"
UDEV_RULE="/etc/udev/rules.d/70-gamepad-osk.rules"
DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
ICON="$DATA_HOME/icons/hicolor/scalable/apps/gamepad-osk.svg"
MENU="$DATA_HOME/applications/gamepad-osk-settings.desktop"
LIB_DIR="$HOME/.local/lib/gamepad-osk" # SDL3 of the portable build
PORTABLE=0

say()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

# Ask yes/no; defaults to yes. Without a terminal (curl | bash) read from /dev/tty.
ask() {
    local answer
    if [ -r /dev/tty ]; then
        { read -r -p "$1 [Y/n] " answer </dev/tty; } 2>/dev/null || answer=n
    else
        return 1
    fi
    [[ -z "$answer" || "$answer" =~ ^[YyJj] ]]
}

has_systemd_user() {
    command -v systemctl >/dev/null && systemctl --user show-environment >/dev/null 2>&1
}

uninstall() {
    say "Removing gamepad-osk"
    if has_systemd_user; then
        systemctl --user disable --now gamepad-osk.service 2>/dev/null || true
    fi
    rm -f "$SVC" "$AUTOSTART" "$BIN" "$ICON" "$MENU"
    rm -rf "$LIB_DIR"
    has_systemd_user && systemctl --user daemon-reload || true
    if [ -f "$UDEV_RULE" ]; then
        say "Removing udev rule (needs sudo)"
        sudo rm -f "$UDEV_RULE" && sudo udevadm control --reload-rules || true
    fi
    say "Done. Your settings stay in $CFG_HOME/gamepad-osk/ (delete by hand if wanted)."
}

LOCAL_BIN=""
case "${1:-}" in
    --uninstall) uninstall; exit 0 ;;
    --local) LOCAL_BIN="${2:?--local needs a file}" ;;
    --portable) PORTABLE=1 ;;
    -h|--help) sed -n '2,17p' "$0"; exit 0 ;;
    "") ;;
    *) die "unknown option $1 (try --help)" ;;
esac

# --- 1. Architecture ---------------------------------------------------------
if [ -z "$LOCAL_BIN" ] && [ "$(uname -m)" != "x86_64" ]; then
    die "the ready-made binary is x86_64 only. Build from source: see README (go build)."
fi

# --- 2. SDL3 libraries -------------------------------------------------------
# Without SDL3 from the package manager the "portable" build is used: the
# same program with SDL3 and SDL3_ttf bundled (no root needed). It covers
# Ubuntu 24.04, Debian 12, Mint 22 and immutable systems like Bazzite.
have_lib() { ldconfig -p 2>/dev/null | grep -q "$1" || ls /usr/lib*/"$1"* /usr/lib/*/"$1"* >/dev/null 2>&1; }
if [ "$PORTABLE" = 0 ] && [ -z "$LOCAL_BIN" ] && { ! have_lib libSDL3.so.0 || ! have_lib libSDL3_ttf.so.0; }; then
    cmd=""
    if [ -e /run/ostree-booted ]; then
        PORTABLE=1 # Bazzite, Bluefin, Fedora Atomic: no layering and reboot needed
    elif command -v dnf >/dev/null; then cmd="sudo dnf install -y SDL3 SDL3_ttf"
    elif command -v apt-get >/dev/null; then
        if apt-cache show libsdl3-0 >/dev/null 2>&1; then
            cmd="sudo apt-get install -y libsdl3-0 libsdl3-ttf0"
        else
            PORTABLE=1 # Ubuntu 24.04, Debian 12, Mint 22: SDL3 not packaged
        fi
    elif command -v pacman >/dev/null; then cmd="sudo pacman -S --needed sdl3 sdl3_ttf"
    elif command -v zypper >/dev/null; then cmd="sudo zypper install -y libSDL3.so.0()(64bit) libSDL3_ttf.so.0()(64bit)"
    else PORTABLE=1; fi
    if [ -n "$cmd" ]; then
        warn "SDL3 and SDL3_ttf are missing."
        if ask "Install them now ($cmd)? No = use the build with SDL3 included"; then
            $cmd
        else
            PORTABLE=1
        fi
    fi
fi
[ "$PORTABLE" = 1 ] && say "Using the build with SDL3 included (no system SDL3 needed)"

# --- 3. Binary ---------------------------------------------------------------
mkdir -p "$BIN_DIR"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
if [ -n "$LOCAL_BIN" ]; then
    say "Installing $LOCAL_BIN"
    cp "$LOCAL_BIN" "$work/gamepad-osk"
elif [ "$PORTABLE" = 1 ]; then
    say "Downloading the latest release (with SDL3)"
    curl -fL --progress-bar "${GAMEPAD_OSK_PORTABLE_URL:-https://github.com/$REPO/releases/latest/download/gamepad-osk-portable-x86_64.tar.gz}" \
        | tar -xz -C "$work" --strip-components=1 || die "download failed"
else
    say "Downloading the latest release"
    curl -fL --progress-bar -o "$work/gamepad-osk" "https://github.com/$REPO/releases/latest/download/gamepad-osk" \
        || die "download failed"
fi
chmod 755 "$work/gamepad-osk"
"$work/gamepad-osk" --version >/dev/null 2>&1 \
    || die "the binary does not start here (missing libraries?). Try: ldd $work/gamepad-osk"
if has_systemd_user; then systemctl --user stop gamepad-osk.service 2>/dev/null || true; fi
rm -rf "$LIB_DIR"
if [ "$PORTABLE" = 1 ]; then
    # The binary looks for its libraries in ../lib/gamepad-osk (RPATH)
    mkdir -p "$LIB_DIR"
    cp "$work"/lib/*.so.* "$LIB_DIR/"
fi
mv -f "$work/gamepad-osk" "$BIN"
say "Installed $("$BIN" --version) to $BIN"

# --- 4. Permissions: /dev/uinput (typing) and the controller -----------------
# Most systemd desktops already give the logged-in user access to game
# controllers (uaccess). /dev/uinput differs between distributions.
if [ ! -w /dev/uinput ]; then
    warn "No write access to /dev/uinput, so the keyboard could not type."
    if ask "Add a udev rule for your login session (needs sudo)?"; then
        sudo tee "$UDEV_RULE" >/dev/null <<'RULE'
# gamepad-osk: give the logged-in user access to uinput (virtual keyboard/mouse)
# and to game controllers. uaccess = only the user at the seat, no group needed.
KERNEL=="uinput", SUBSYSTEM=="misc", OPTIONS+="static_node=uinput", TAG+="uaccess"
SUBSYSTEM=="input", ENV{ID_INPUT_JOYSTICK}=="1", TAG+="uaccess"
RULE
        sudo modprobe uinput 2>/dev/null || true
        sudo udevadm control --reload-rules
        sudo udevadm trigger --subsystem-match=misc --subsystem-match=input
        sleep 1
        [ -w /dev/uinput ] || warn "Still no access. Log out and back in once."
    else
        warn "Skipped. Run 'gamepad-osk --setup' later for help."
    fi
fi

# --- 5. Autostart -------------------------------------------------------------
if has_systemd_user; then
    mkdir -p "$SVC_DIR"
    cat >"$SVC" <<'UNIT'
[Unit]
Description=gamepad-osk - on-screen keyboard for game controllers
After=graphical-session.target
PartOf=graphical-session.target

[Service]
Type=simple
# Starts hidden: no grab, no mouse until you show it with the toggle combo
ExecStartPre=/bin/sleep 3
ExecStart=%h/.local/bin/gamepad-osk --daemon
Restart=on-failure
RestartSec=5

[Install]
WantedBy=graphical-session.target
UNIT
    systemctl --user daemon-reload
    systemctl --user enable --now gamepad-osk.service
    say "Autostart enabled (systemd user service, running now)"
else
    mkdir -p "$(dirname "$AUTOSTART")"
    cat >"$AUTOSTART" <<DESK
[Desktop Entry]
Type=Application
Name=gamepad-osk
Comment=On-screen keyboard for game controllers
Exec=$BIN --daemon
X-GNOME-Autostart-enabled=true
DESK
    say "Autostart added ($AUTOSTART). Starting it now."
    nohup "$BIN" --daemon >/dev/null 2>&1 &
fi

# --- 6. Start menu entry: settings window, right click toggles the keyboard ---
mkdir -p "$(dirname "$ICON")" "$(dirname "$MENU")"
cat >"$ICON" <<'SVG'
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128">
  <!-- gamepad-osk icon: a light keyboard tray with a controller in front -->
  <rect x="8" y="14" width="112" height="70" rx="16" fill="#d1d4db"/>
  <g fill="#ffffff">
    <rect x="18" y="24" width="16" height="14" rx="4"/><rect x="38" y="24" width="16" height="14" rx="4"/>
    <rect x="58" y="24" width="16" height="14" rx="4"/><rect x="78" y="24" width="16" height="14" rx="4"/>
    <rect x="98" y="24" width="12" height="14" rx="4"/>
    <rect x="18" y="42" width="16" height="14" rx="4"/><rect x="38" y="42" width="16" height="14" rx="4"/>
    <rect x="78" y="42" width="16" height="14" rx="4"/><rect x="98" y="42" width="12" height="14" rx="4"/>
    <rect x="34" y="60" width="60" height="14" rx="4"/>
  </g>
  <rect x="58" y="42" width="16" height="14" rx="4" fill="#0a84ff"/>
  <path d="M30 86c0-8 7-14 15-14h38c8 0 15 6 15 14l6 20c2 8-4 14-11 12l-12-6H47l-12 6c-7 2-13-4-11-12z" fill="#2c2c30"/>
  <rect x="38" y="88" width="14" height="4" rx="2" fill="#fff"/><rect x="43" y="83" width="4" height="14" rx="2" fill="#fff"/>
  <circle cx="86" cy="85" r="3.5" fill="#e8aa00"/><circle cx="93" cy="91" r="3.5" fill="#d62828"/>
  <circle cx="79" cy="91" r="3.5" fill="#006edc"/><circle cx="86" cy="97" r="3.5" fill="#108c40"/>
</svg>
SVG
cat >"$MENU" <<DESK
[Desktop Entry]
Type=Application
Name=gamepad-osk Settings
Name[de]=gamepad-osk Einstellungen
Comment=On-screen keyboard for game controllers: theme, layout, controller, autostart
Comment[de]=Bildschirmtastatur für Controller: Design, Layout, Controller, Autostart
Exec=$BIN --settings
Icon=gamepad-osk
Categories=Settings;Utility;
Keywords=keyboard;controller;gamepad;osk;Tastatur;
StartupWMClass=gamepad-osk-settings
Actions=toggle;

[Desktop Action toggle]
Name=Show/hide keyboard
Name[de]=Tastatur ein-/ausblenden
Exec=$BIN --toggle
DESK
command -v update-desktop-database >/dev/null && update-desktop-database -q "$DATA_HOME/applications" 2>/dev/null || true
say "Start menu entry added: gamepad-osk Settings"

case ":$PATH:" in *":$BIN_DIR:"*) ;; *) warn "$BIN_DIR is not in your PATH (only matters for typing gamepad-osk by hand)." ;; esac

cat <<EOF

  Done. Show/hide the keyboard with  Guide + Select  on your controller
  (Xbox button + View, PS button + Create, Home + Minus).
  Mouse only (keyboard hidden): Guide + Start (Home + Plus).
  Settings: start menu "gamepad-osk Settings" (or: gamepad-osk --settings)
  Uninstall: install.sh --uninstall

EOF
