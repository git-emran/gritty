package renderer

import (
	"github.com/emranhossain/terminal_go/pkg/emulator"
	"github.com/gdamore/tcell/v2"
)

// Renderer interface for terminal visualization.
type Renderer interface {
	Render(t *emulator.Terminal)
	Close()
}

// TUIRenderer implements Renderer using tcell.
type TUIRenderer struct {
	screen tcell.Screen
}

// NewTUIRenderer creates a new TUI renderer.
func NewTUIRenderer() (*TUIRenderer, error) {
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	if err := s.Init(); err != nil {
		return nil, err
	}
	s.SetStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))
	s.Clear()


	return &TUIRenderer{screen: s}, nil
}

// Render draws the terminal state to the screen.
func (r *TUIRenderer) Render(t *emulator.Terminal) {
	t.Lock()
	defer t.Unlock()

	r.screen.Clear()
	for y, row := range t.Grid {
		for x, cell := range row {
			st := tcell.StyleDefault
			r.screen.SetContent(x, y, cell.Char, nil, st)
		}
	}
	r.screen.ShowCursor(t.Cursor.X, t.Cursor.Y)
	r.screen.Show()
}


// Close closes the screen.
func (r *TUIRenderer) Close() {
	r.screen.Fini()
}

// Screen returns the underlying tcell screen.
func (r *TUIRenderer) Screen() tcell.Screen {
	return r.screen
}
