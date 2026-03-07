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
		background: [3]uint8{0x11, 0x14, 0x1A},
		foreground: [3]uint8{0xE6, 0xED, 0xF3},
		cursor:     [3]uint8{0xF8, 0xFA, 0xFC},
		ansi16: [16][3]uint8{
			{0x3B, 0x42, 0x4D}, // 0
			{0xF4, 0x70, 0x67}, // 1
			{0x8D, 0xD0, 0x8A}, // 2
			{0xE6, 0xC2, 0x4B}, // 3
			{0x6C, 0xB6, 0xFF}, // 4
			{0xD6, 0x8E, 0xF3}, // 5
			{0x5F, 0xCD, 0xD9}, // 6
			{0xD8, 0xE0, 0xEA}, // 7
			{0x69, 0x73, 0x86}, // 8
			{0xFF, 0x8A, 0x82}, // 9
			{0xA7, 0xE3, 0xA2}, // 10
			{0xF1, 0xD9, 0x6A}, // 11
			{0x8B, 0xC7, 0xFF}, // 12
			{0xE5, 0xAA, 0xFA}, // 13
			{0x86, 0xE1, 0xEB}, // 14
			{0xFF, 0xFF, 0xFF}, // 15
		},
	}
	lightTheme = terminalTheme{
		background: [3]uint8{0xF6, 0xF1, 0xE8},
		foreground: [3]uint8{0x20, 0x25, 0x2C},
		cursor:     [3]uint8{0x20, 0x25, 0x2C},
		ansi16: [16][3]uint8{
			{0x3F, 0x45, 0x4F}, // 0
			{0xC0, 0x3A, 0x2B}, // 1
			{0x2F, 0x8A, 0x43}, // 2
			{0xA5, 0x7B, 0x00}, // 3
			{0x1D, 0x67, 0xC9}, // 4
			{0xA5, 0x4F, 0xC8}, // 5
			{0x0B, 0x88, 0x9D}, // 6
			{0xDF, 0xE4, 0xEA}, // 7
			{0x70, 0x77, 0x84}, // 8
			{0xDB, 0x57, 0x47}, // 9
			{0x4B, 0xA7, 0x5A}, // 10
			{0xC2, 0x96, 0x1E}, // 11
			{0x3B, 0x83, 0xE0}, // 12
			{0xB6, 0x67, 0xD3}, // 13
			{0x2D, 0x9C, 0xB3}, // 14
			{0xFF, 0xFF, 0xFF}, // 15
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
