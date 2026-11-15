package main

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// fontDirs lists the usual font roots. Distributions differ in the sub-folder
// layout (Arch: TTF/, Debian: truetype/<family>/, Fedora: <family>-fonts/),
// so they are searched recursively.
func fontDirs() []string {
	dirs := []string{"/usr/share/fonts", "/usr/local/share/fonts"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".local/share/fonts"),
			filepath.Join(home, ".fonts"))
	}
	return dirs
}

// fcMatch asks fontconfig for the file of the best match for pattern.
// Returns "" if fc-match is missing or the result is not a usable font file.
func fcMatch(pattern string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "fc-match", "--format=%{file}", pattern).Output() //nolint:gosec // G204: fixed binary, pattern from our code
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(out))
	if !isFontFile(path) {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

func isFontFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".ttf" || ext == ".otf"
}

// findFontFile searches the font directories recursively for one of the
// given file names (case-insensitive) and returns the first hit.
func findFontFile(names ...string) string {
	want := make(map[string]int, len(names))
	for i, n := range names {
		want[strings.ToLower(n)] = i
	}
	best, bestRank := "", len(names)
	for _, root := range fontDirs() {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr // unreadable entries are skipped
			}
			if rank, ok := want[strings.ToLower(d.Name())]; ok && rank < bestRank {
				best, bestRank = path, rank
			}
			return nil
		})
		if bestRank == 0 {
			break
		}
	}
	return best
}

// findUIFont returns the font used for key labels. Order: fontconfig's
// default sans-serif (matches the desktop, e.g. Noto Sans on Fedora), then
// well-known files anywhere below the font directories.
func findUIFont() string {
	if p := fcMatch("sans-serif:style=Regular"); p != "" {
		return p
	}
	return findFontFile(
		"DejaVuSans.ttf", "LiberationSans-Regular.ttf", "NotoSans-Regular.ttf",
		"Cantarell-Regular.otf", "FreeSans.ttf", "Roboto-Regular.ttf",
	)
}

// findPromptFont returns the Promptfont file for controller glyphs, or "".
func findPromptFont() string {
	if _, err := os.Stat(promptFontPath); err == nil {
		return promptFontPath
	}
	if p := findFontFile("promptfont.ttf", "promptfont.otf"); p != "" {
		return p
	}
	if p := fcMatch("promptfont"); p != "" && strings.Contains(strings.ToLower(p), "promptfont") {
		return p
	}
	return ""
}
