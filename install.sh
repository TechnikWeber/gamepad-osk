#!/usr/bin/env bash
# gamepad-osk installer (TechnikWeber fork)
#
#   curl -fsSL https://raw.githubusercontent.com/TechnikWeber/gamepad-osk/main/install.sh | bash
#
# Installs for the current user only: binary to ~/.local/bin, autostart as a
# systemd user service. Root (sudo) is asked for only if the system really
# needs it: missing SDL3 libraries or missing access to /dev/uinput.
#
#   install.sh              install or update
#   install.sh --local F    install the binary F instead of downloading it
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

say()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

# Ask yes/no; defaults to yes. Without a terminal (curl | bash) read from /dev/tty.
ask() {
    local answer
    if [ -r /dev/tty ]; then
        read -r -p "$1 [Y/n] " answer </dev/tty || answer=n
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
    rm -f "$SVC" "$AUTOSTART" "$BIN"
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
    -h|--help) sed -n '2,14p' "$0"; exit 0 ;;
    "") ;;
    *) die "unknown option $1 (try --help)" ;;
esac

# --- 1. Architecture ---------------------------------------------------------
if [ -z "$LOCAL_BIN" ] && [ "$(uname -m)" != "x86_64" ]; then
    die "the ready-made binary is x86_64 only. Build from source: see README (go build)."
fi

# --- 2. SDL3 libraries -------------------------------------------------------
have_lib() { ldconfig -p 2>/dev/null | grep -q "$1" || ls /usr/lib*/"$1"* /usr/lib/*/"$1"* >/dev/null 2>&1; }
if ! have_lib libSDL3.so.0 || ! have_lib libSDL3_ttf.so.0; then
    if   command -v dnf    >/dev/null; then cmd="sudo dnf install -y SDL3 SDL3_ttf"
    elif command -v apt-get >/dev/null; then cmd="sudo apt-get install -y libsdl3-0 libsdl3-ttf-0"
    elif command -v pacman >/dev/null; then cmd="sudo pacman -S --needed sdl3 sdl3_ttf"
    elif command -v zypper >/dev/null; then cmd="sudo zypper install -y libSDL3-0 libSDL3_ttf0"
    else cmd=""; fi
    warn "SDL3 and SDL3_ttf are missing."
    if [ -n "$cmd" ] && ask "Install them now ($cmd)?"; then
        $cmd
    else
        die "please install SDL3 and SDL3_ttf with your package manager, then run this again."
    fi
fi

# --- 3. Binary ---------------------------------------------------------------
mkdir -p "$BIN_DIR"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
if [ -n "$LOCAL_BIN" ]; then
    say "Installing $LOCAL_BIN"
    cp "$LOCAL_BIN" "$tmp"
else
    say "Downloading the latest release"
    curl -fL --progress-bar -o "$tmp" "https://github.com/$REPO/releases/latest/download/gamepad-osk" \
        || die "download failed"
fi
chmod 755 "$tmp"
"$tmp" --version >/dev/null 2>&1 || die "the binary does not start here (missing libraries?). Try: ldd $tmp"
if has_systemd_user; then systemctl --user stop gamepad-osk.service 2>/dev/null || true; fi
mv -f "$tmp" "$BIN"
trap - EXIT
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

case ":$PATH:" in *":$BIN_DIR:"*) ;; *) warn "$BIN_DIR is not in your PATH (only matters for typing gamepad-osk by hand)." ;; esac

cat <<EOF

  Done. Show/hide the keyboard with  Guide + Select  on your controller
  (Xbox button + View, PS button + Create, Home + Minus).
  Settings: $CFG_HOME/gamepad-osk/config   Uninstall: install.sh --uninstall

EOF
