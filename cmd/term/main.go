package main

import (
	"log"
	"os"

	"github.com/emranhossain/terminal_go/pkg/emulator"
	"github.com/emranhossain/terminal_go/pkg/pty"
	"github.com/emranhossain/terminal_go/pkg/renderer"
	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	// Initialize logging
	f, err := os.OpenFile("term.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(f)
		defer f.Close()
	}
	log.Println("Starting Ebitengine terminal...")

	// Fixed dimensions for now
	width, height := 80, 24

	// Initialize GUI Renderer
	rend, err := renderer.NewGUIRenderer("Gritty Terminal", width, height)
	if err != nil {
		log.Fatalf("failed to create renderer: %v", err)
	}

	// Initialize Terminal Emulator
	term := emulator.NewTerminal(width, height)
	parser := emulator.NewParser(term)

	// Initialize PTY Manager
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}

	// Set environment for the shell
	os.Setenv("TERM", "xterm-256color")
	os.Setenv("COLORTERM", "truecolor")
	os.Setenv("LANG", "en_US.UTF-8")
	os.Setenv("LC_ALL", "en_US.UTF-8")

	manager, err := pty.NewManager(shell, []string{"-l"})
	if err != nil {
		log.Fatalf("failed to start pty: %v", err)
	}
	defer manager.Close()

	manager.Resize(uint16(height), uint16(width))

	done := make(chan bool)

	// Keep terminal emulator and PTY in sync when the window is resized.
	rend.SetResizeHandler(func(cols, rows int) {
		term.Resize(cols, rows)
		manager.Resize(uint16(rows), uint16(cols))
		log.Printf("Resized terminal to %dx%d", cols, rows)
	})

	// Bridge GUI keys to PTY
	rend.SetKeyHandler(func(key ebiten.Key) {
		switch key {
		case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
			manager.Write([]byte{'\r'})
		case ebiten.KeyBackspace:
			manager.Write([]byte{0x7f})
		case ebiten.KeyTab:
			manager.Write([]byte{'\t'})
		case ebiten.KeyUp:
			manager.Write([]byte("\x1b[A"))
		case ebiten.KeyDown:
			manager.Write([]byte("\x1b[B"))
		case ebiten.KeyRight:
			manager.Write([]byte("\x1b[C"))
		case ebiten.KeyLeft:
			manager.Write([]byte("\x1b[D"))
		case ebiten.KeyHome:
			manager.Write([]byte("\x1b[H"))
		case ebiten.KeyEnd:
			manager.Write([]byte("\x1b[F"))
		case ebiten.KeyPageUp:
			manager.Write([]byte("\x1b[5~"))
		case ebiten.KeyPageDown:
			manager.Write([]byte("\x1b[6~"))
		case ebiten.KeyDelete:
			manager.Write([]byte("\x1b[3~"))
		case ebiten.KeyEscape:
			manager.Write([]byte{0x1b})
		}
	})

	rend.SetRuneHandler(func(rn rune) {
		manager.Write([]byte(string(rn)))
	})

	// Read from PTY and update terminal state
	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := manager.Read(buf)
			if err != nil {
				log.Printf("PTY Read Error: %v", err)
				done <- true
				return
			}
			if n > 0 {
				parser.Process(buf[:n])
			}
		}
	}()

	// Wait for shell to exit
	go func() {
		<-done
		os.Exit(0)
	}()

	// Start Ebitengine event loop
	rend.ShowAndRun(term)
}
