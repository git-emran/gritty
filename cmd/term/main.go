package main

import (
	"log"
	"os"

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
		shell = "/bin/zsh" // Prefer zsh on Mac
	}

	// Set environment for the shell
	os.Setenv("TERM", "xterm-256color")
	os.Setenv("LANG", "en_US.UTF-8")
	os.Setenv("LC_ALL", "en_US.UTF-8")
	
	manager, err := pty.NewManager(shell, []string{"-l"}) // Login shell

	if err != nil {
		log.Fatalf("failed to start pty: %v", err)
	}
	defer manager.Close()


	manager.Resize(uint16(height), uint16(width))

	// Channel for terminal output
	done := make(chan bool)

	// Bridge GUI keys to PTY

	rend.SetKeyHandler(func(ev *fyne.KeyEvent) {
		log.Printf("Key pressed: %v", ev.Name)
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
		}
	})



	rend.SetRuneHandler(func(rn rune) {
		log.Printf("Rune typed: %c", rn)
		manager.Write([]byte(string(rn)))
	})


	// Read from PTY and update terminal state
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := manager.Read(buf)
			if err != nil {
				log.Printf("PTY Read Error: %v", err)
				done <- true
				return
			}
			parser.Process(buf[:n])
			rend.Render(term)
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

