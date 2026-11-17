package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const serviceName = "gamepad-osk.service"

func runQuiet(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // G204: fixed commands from our code
	return strings.TrimSpace(string(out)), err
}

func configHome() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

// systemdUserAvailable reports whether a systemd user manager is running.
func systemdUserAvailable() bool {
	_, err := runQuiet("systemctl", "--user", "show-environment")
	return err == nil
}

func xdgAutostartPath() string {
	return filepath.Join(configHome(), "autostart", "gamepad-osk.desktop")
}

// userServiceFile returns the path of our user unit, creating it if missing.
func ensureUserService() error {
	path := filepath.Join(configHome(), "systemd", "user", serviceName)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if _, err := os.Stat("/usr/lib/systemd/user/" + serviceName); err == nil {
		return nil // packaged unit
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // G301: user config dir
		return err
	}
	unit := fmt.Sprintf(`[Unit]
Description=gamepad-osk - on-screen keyboard and mouse for game controllers
After=graphical-session.target
PartOf=graphical-session.target

[Service]
Type=simple
ExecStartPre=/bin/sleep 3
ExecStart=%s --daemon
Restart=on-failure
RestartSec=5

[Install]
WantedBy=graphical-session.target
`, exe)
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil { //nolint:gosec // G306: unit files are world-readable
		return err
	}
	_, _ = runQuiet("systemctl", "--user", "daemon-reload")
	return nil
}

// AutostartEnabled reports whether gamepad-osk starts at login.
func AutostartEnabled() bool {
	if systemdUserAvailable() {
		out, _ := runQuiet("systemctl", "--user", "is-enabled", serviceName)
		if out == "enabled" {
			return true
		}
	}
	_, err := os.Stat(xdgAutostartPath())
	return err == nil
}

// SetAutostart turns starting at login on or off. Turning it on also starts
// the keyboard now; turning it off leaves a running keyboard alone.
func SetAutostart(on bool) error {
	if systemdUserAvailable() {
		if on {
			if err := ensureUserService(); err != nil {
				return err
			}
			if out, err := runQuiet("systemctl", "--user", "enable", "--now", serviceName); err != nil {
				return fmt.Errorf("%v: %s", err, out)
			}
			return nil
		}
		_, _ = runQuiet("systemctl", "--user", "disable", serviceName)
		_ = os.Remove(xdgAutostartPath())
		return nil
	}
	// No systemd user session: XDG autostart file
	path := xdgAutostartPath()
	if !on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // G301: user config dir
		return err
	}
	desk := "[Desktop Entry]\nType=Application\nName=gamepad-osk\nComment=On-screen keyboard and mouse for game controllers\nExec=" +
		exe + " --daemon\nX-GNOME-Autostart-enabled=true\n"
	return os.WriteFile(path, []byte(desk), 0o644) //nolint:gosec // G306: desktop files are world-readable
}

// RestartKeyboard restarts a running keyboard so new settings apply.
// Returns true if it was restarted.
func RestartKeyboard() bool {
	if systemdUserAvailable() {
		if out, _ := runQuiet("systemctl", "--user", "is-active", serviceName); out == "active" || out == "activating" {
			_, err := runQuiet("systemctl", "--user", "restart", serviceName)
			return err == nil
		}
	}
	return false
}

// launchDetached starts a command outside our process group and, with
// systemd, outside our service (a restart of the service must not kill it).
func launchDetached(args ...string) error {
	if _, err := exec.LookPath("systemd-run"); err == nil && systemdUserAvailable() {
		full := append([]string{"--user", "--collect", "--quiet"}, args...)
		return exec.Command("systemd-run", full...).Start() //nolint:gosec // G204: our own binary
	}
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // G204: our own binary
	return cmd.Start()
}
