package main

import (
	"log"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"github.com/emranhossain/terminal_go/pkg/emulator"
	"github.com/emranhossain/terminal_go/pkg/pty"
	"github.com/emranhossain/terminal_go/pkg/renderer"
)

func main() {
	// Initialize logging
	f, err := os.OpenFile("term.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(f)
		defer f.Close()
	}
	log.Println("Starting GUI terminal...")

	// Fixed dimensions for now
	width, height := 80, 24

	// Initialize GUI Renderer
	rend, err := renderer.NewGUIRenderer("Go Terminal", width, height)
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
	// This is critical for full-screen apps like vim/nvim — they query the
	// PTY for its dimensions and draw exactly that many rows/columns.
	rend.SetResizeHandler(func(cols, rows int) {
		term.Resize(cols, rows)
		manager.Resize(uint16(rows), uint16(cols))
		log.Printf("Resized terminal to %dx%d", cols, rows)
	})

	// Bridge GUI keys to PTY
	rend.SetKeyHandler(func(ev *fyne.KeyEvent) {
		switch ev.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			manager.Write([]byte{'\r'})
		case fyne.KeyBackspace:
			manager.Write([]byte{0x7f})
		case fyne.KeyTab:
			manager.Write([]byte{'\t'})
		case fyne.KeyUp:
			manager.Write([]byte("\x1b[A"))
		case fyne.KeyDown:
			manager.Write([]byte("\x1b[B"))
		case fyne.KeyRight:
			manager.Write([]byte("\x1b[C"))
		case fyne.KeyLeft:
			manager.Write([]byte("\x1b[D"))
		case fyne.KeyHome:
			manager.Write([]byte("\x1b[H"))
		case fyne.KeyEnd:
			manager.Write([]byte("\x1b[F"))
		case fyne.KeyPageUp:
			manager.Write([]byte("\x1b[5~"))
		case fyne.KeyPageDown:
			manager.Write([]byte("\x1b[6~"))
		case fyne.KeyDelete:
			manager.Write([]byte("\x1b[3~"))
		case fyne.KeyEscape:
			manager.Write([]byte{0x1b})
		}
	})

	rend.SetRuneHandler(func(rn rune) {
		manager.Write([]byte(string(rn)))
	})

	// Render trigger channel — batches rapid output into at most ~60fps renders
	renderTrigger := make(chan struct{}, 1)

	// Read from PTY and update terminal state
	go func() {
		// 32KB buffer — much larger than before to handle burst output efficiently
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
				// Signal render needed (non-blocking, coalesces multiple reads)
				select {
				case renderTrigger <- struct{}{}:
				default:
				}
			}
		}
	}()

	// Render loop — drains renderTrigger at most every ~16ms (≈60fps)
	go func() {
		ticker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Check if there's a pending render signal
				select {
				case <-renderTrigger:
					rend.Render(term)
				default:
				}
			case <-rend.RenderCh():
				// Cursor blink triggered a render
				term.MarkCursorDirty()
				rend.Render(term)
			case <-done:
				return
			}
		}
	}()

	// Wait for shell to exit
	go func() {
		<-done
		rend.Close()
	}()

	// Start Fyne event loop
	rend.ShowAndRun()
}
