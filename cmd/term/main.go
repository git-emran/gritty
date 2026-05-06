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
	rend, err := renderer.NewGUIRenderer("Gritty", width, height)
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
		shiftPressed := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
		ctrlPressed := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)
		altPressed := ebiten.IsKeyPressed(ebiten.KeyAlt) || ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight)
		scrollbackActive := term.IsViewingScrollback()
		altScreenActive := term.IsAltScreenActive()
		send := func(data []byte) {
			term.ScrollToBottom()
			manager.Write(data)
		}

		// Ctrl key chords (critical for Neovim and TUIs).
		if ctrlPressed {
			if key >= ebiten.KeyA && key <= ebiten.KeyZ {
				send([]byte{byte(key - ebiten.KeyA + 1)})
				return
			}
			if key == ebiten.KeySpace {
				send([]byte{0x00})
				return
			}
		}

		// Alt/Meta key chords for non-text keys (basic support).
		prefixAlt := func(seq []byte) []byte {
			if !altPressed {
				return seq
			}
			out := make([]byte, 0, len(seq)+1)
			out = append(out, 0x1b)
			out = append(out, seq...)
			return out
		}

		switch key {
		case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
			send(prefixAlt([]byte{'\r'}))
		case ebiten.KeyBackspace:
			send(prefixAlt([]byte{0x7f}))
		case ebiten.KeyTab:
			send(prefixAlt([]byte{'\t'}))
		case ebiten.KeyUp:
			if !altScreenActive && (shiftPressed || scrollbackActive) {
				term.ScrollUpLines(1)
				return
			}
			send(prefixAlt([]byte("\x1b[A")))
		case ebiten.KeyDown:
			if !altScreenActive && (shiftPressed || scrollbackActive) {
				term.ScrollDownLines(1)
				return
			}
			send(prefixAlt([]byte("\x1b[B")))
		case ebiten.KeyRight:
			send(prefixAlt([]byte("\x1b[C")))
		case ebiten.KeyLeft:
			send(prefixAlt([]byte("\x1b[D")))
		case ebiten.KeyHome:
			if !altScreenActive && scrollbackActive {
				term.ScrollToTop()
				return
			}
			send(prefixAlt([]byte("\x1b[H")))
		case ebiten.KeyEnd:
			if !altScreenActive && scrollbackActive {
				term.ScrollToBottom()
				return
			}
			send(prefixAlt([]byte("\x1b[F")))
		case ebiten.KeyPageUp:
			if !altScreenActive {
				term.ScrollPageUp()
				return
			}
			send(prefixAlt([]byte("\x1b[5~")))
		case ebiten.KeyPageDown:
			if !altScreenActive {
				term.ScrollPageDown()
				return
			}
			send(prefixAlt([]byte("\x1b[6~")))
		case ebiten.KeyInsert:
			send(prefixAlt([]byte("\x1b[2~")))
		case ebiten.KeyDelete:
			send(prefixAlt([]byte("\x1b[3~")))
		case ebiten.KeyEscape:
			term.ScrollToBottom()
			manager.Write([]byte{0x1b})
		case ebiten.KeyF1:
			send(prefixAlt([]byte("\x1bOP")))
		case ebiten.KeyF2:
			send(prefixAlt([]byte("\x1bOQ")))
		case ebiten.KeyF3:
			send(prefixAlt([]byte("\x1bOR")))
		case ebiten.KeyF4:
			send(prefixAlt([]byte("\x1bOS")))
		case ebiten.KeyF5:
			send(prefixAlt([]byte("\x1b[15~")))
		case ebiten.KeyF6:
			send(prefixAlt([]byte("\x1b[17~")))
		case ebiten.KeyF7:
			send(prefixAlt([]byte("\x1b[18~")))
		case ebiten.KeyF8:
			send(prefixAlt([]byte("\x1b[19~")))
		case ebiten.KeyF9:
			send(prefixAlt([]byte("\x1b[20~")))
		case ebiten.KeyF10:
			send(prefixAlt([]byte("\x1b[21~")))
		case ebiten.KeyF11:
			send(prefixAlt([]byte("\x1b[23~")))
		case ebiten.KeyF12:
			send(prefixAlt([]byte("\x1b[24~")))
		}
	})

	rend.SetRuneHandler(func(rn rune) {
		term.ScrollToBottom()
		manager.Write([]byte(string(rn)))
	})

	var scrollAccum float64
	rend.SetWheelHandler(func(dx, dy float64) {
		isAlt := term.IsAltScreenActive()

		scrollAccum += dy
		if scrollAccum >= 1.0 {
			lines := int(scrollAccum)
			scrollAccum -= float64(lines)
			if isAlt {
				for i := 0; i < lines; i++ {
					manager.Write([]byte("\x1b[A")) // Arrow Up
				}
			} else {
				term.ScrollUpLines(lines)
			}
		} else if scrollAccum <= -1.0 {
			lines := int(-scrollAccum)
			scrollAccum += float64(lines)
			if isAlt {
				for i := 0; i < lines; i++ {
					manager.Write([]byte("\x1b[B")) // Arrow Down
				}
			} else {
				term.ScrollDownLines(lines)
			}
		}
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
