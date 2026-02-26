package renderer

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type terminalTheme struct {
	background [3]uint8
	foreground [3]uint8
	cursor     [3]uint8
	ansi16     [16][3]uint8
}

var (
	darkTheme = terminalTheme{
		background: [3]uint8{0x1E, 0x1E, 0x1E},
		foreground: [3]uint8{0xE0, 0xE0, 0xE0},
		cursor:     [3]uint8{0xE0, 0xE0, 0xE0},
		ansi16: [16][3]uint8{
			{0x30, 0x30, 0x30}, // 0 - lifted higher for better contrast on dark backgrounds
			{0xE0, 0x6C, 0x75}, // 1
			{0x98, 0xC3, 0x79}, // 2
			{0xE5, 0xC0, 0x7B}, // 3
			{0x61, 0xAF, 0xEF}, // 4
			{0xC6, 0x78, 0xDD}, // 5
			{0x56, 0xB6, 0xC2}, // 6
			{0xDC, 0xDF, 0xE4}, // 7
			{0x7F, 0x84, 0x8E}, // 8
			{0xF7, 0x76, 0x8E}, // 9
			{0xB1, 0xD7, 0x94}, // 10
			{0xF2, 0xCD, 0x8D}, // 11
			{0x7C, 0xC8, 0xFF}, // 12
			{0xD5, 0x8F, 0xE7}, // 13
			{0x71, 0xCF, 0xDD}, // 14
			{0xFF, 0xFF, 0xFF}, // 15
		},
	}
	lightTheme = terminalTheme{
		background: [3]uint8{0xFA, 0xFA, 0xFA},
		foreground: [3]uint8{0x1A, 0x1A, 0x1A},
		cursor:     [3]uint8{0x1A, 0x1A, 0x1A},
		ansi16: [16][3]uint8{
			{0x20, 0x20, 0x20}, // 0
			{0xB0, 0x00, 0x20}, // 1
			{0x1B, 0x70, 0x30}, // 2
			{0x8A, 0x6A, 0x00}, // 3
			{0x00, 0x4A, 0xAA}, // 4
			{0x8A, 0x32, 0x8C}, // 5
			{0x00, 0x6A, 0x82}, // 6
			{0xD9, 0xD9, 0xD9}, // 7
			{0x60, 0x60, 0x60}, // 8
			{0xD7, 0x2B, 0x43}, // 9
			{0x2F, 0x8C, 0x45}, // 10
			{0xA4, 0x7E, 0x00}, // 11
			{0x1B, 0x65, 0xC2}, // 12
			{0x9E, 0x48, 0xA3}, // 13
			{0x00, 0x86, 0xA3}, // 14
			{0xF5, 0xF5, 0xF5}, // 15
		},
	}
)

func detectDarkMode() bool {
	// Optional manual override: TERM_THEME=dark|light|system
	if forced := strings.ToLower(strings.TrimSpace(os.Getenv("TERM_THEME"))); forced != "" {
		if forced == "system" {
			// Continue with system detection.
		} else {
			return forced == "dark"
		}
	}

	if runtime.GOOS == "darwin" {
		// Try defaults first as it's faster
		out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
		if err == nil {
			return strings.EqualFold(strings.TrimSpace(string(out)), "dark")
		}

		// Fallback for macOS variants where defaults output is inconsistent or missing (for Light mode)
		out, err = exec.Command("osascript", "-e", "tell application \"System Events\" to tell appearance preferences to get dark mode").Output()
		if err == nil {
			return strings.EqualFold(strings.TrimSpace(string(out)), "true")
		}
	}

	return false
}

func activeTheme(isDark bool) terminalTheme {
	if isDark {
		return darkTheme
	}
	return lightTheme
}
