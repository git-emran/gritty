package renderer

import (
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/widget"
	"github.com/emranhossain/terminal_go/pkg/emulator"
)

// GUIRenderer implements Renderer using Fyne.
type GUIRenderer struct {
	app        fyne.App
	win        fyne.Window
	grid        *widget.TextGrid
	keyHandler   func(ev *fyne.KeyEvent)
	runeHandler  func(r rune)
	cursorVisible bool
	stopCursor    chan bool
}


// NewGUIRenderer creates a new GUI renderer.
func NewGUIRenderer(title string, width, height int) (*GUIRenderer, error) {
	a := app.New()
	w := a.NewWindow(title)

	g := widget.NewTextGrid()
	// Set initial size safely using SetText
	blankLine := strings.Repeat(" ", width)
	var sb strings.Builder
	for i := 0; i < height; i++ {
		sb.WriteString(blankLine)
		if i < height-1 {
			sb.WriteString("\n")
		}
	}
	g.SetText(sb.String())


	w.SetContent(g)
	w.Resize(fyne.NewSize(800, 600))

	r := &GUIRenderer{
		app:           a,
		win:           w,
		grid:          g,
		cursorVisible: true,
		stopCursor:    make(chan bool),
	}

	// Start blinking cursor goroutine
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.cursorVisible = !r.cursorVisible
				// We don't have a direct handle to the terminal here to trigger a render,
				// but we can at least refresh the grid.
				r.grid.Refresh()
			case <-r.stopCursor:
				return
			}
		}
	}()


	w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if r.keyHandler != nil {
			r.keyHandler(ev)
		}
	})

	w.Canvas().SetOnTypedRune(func(rn rune) {
		if r.runeHandler != nil {
			r.runeHandler(rn)
		}
	})

	return r, nil
}


// Render draws the terminal state to the Fyne grid.
func (r *GUIRenderer) Render(t *emulator.Terminal) {
	t.Lock()
	defer t.Unlock()

	for y, row := range t.Grid {
		for x, cell := range row {
			char := cell.Char
			// Show cursor by swapping space with block or just rendering block
			if r.cursorVisible && x == t.Cursor.X && y == t.Cursor.Y {
				if char == ' ' {
					char = '█'
				}
			}
			r.grid.SetCell(y, x, widget.TextGridCell{Rune: char})
		}
	}
	r.grid.Refresh()
}

func tcellToFyneColor(c int, isFg bool) color.Color {
	// Simplified color mapping for now
	return nil // Use theme defaults
}



// SetKeyHandler sets the function to call on key events.
func (r *GUIRenderer) SetKeyHandler(h func(ev *fyne.KeyEvent)) {
	r.keyHandler = h
}

// SetRuneHandler sets the function to call on rune events.
func (r *GUIRenderer) SetRuneHandler(h func(rn rune)) {
	r.runeHandler = h
}


// ShowAndRun shows the window and starts the Fyne event loop.
func (r *GUIRenderer) ShowAndRun() {
	r.win.ShowAndRun()
}

// Close closes the renderer.
func (r *GUIRenderer) Close() {
	close(r.stopCursor)
	r.win.Close()
}


// Size returns the grid dimensions.
func (r *GUIRenderer) Size() (int, int) {
	// For simplicity, let's assume a fixed size or calculate from window
	return 80, 24 // Default terminal size
}
