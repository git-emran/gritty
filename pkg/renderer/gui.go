package renderer

import (
	"image/color"
	"log"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/emranhossain/terminal_go/pkg/emulator"
)

// nerdFontTheme wraps the default Fyne theme and overrides the monospace font
// with a Nerd Font so that private-use-area glyphs (icons) render correctly.
type nerdFontTheme struct {
	fyne.Theme
	monoFont fyne.Resource
}

func (t *nerdFontTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Monospace && t.monoFont != nil {
		return t.monoFont
	}
	return t.Theme.Font(style)
}

// loadNerdFont searches common macOS font locations for a Nerd Font TTF.
func loadNerdFont() fyne.Resource {
	candidates := []string{
		"FiraCodeNerdFontMono-Regular.ttf",
		"FiraCodeNerdFont-Regular.ttf",
		"HackNerdFontMono-Regular.ttf",
		"HackNerdFont-Regular.ttf",
		"JetBrainsMonoNerdFontMono-Regular.ttf",
		"JetBrainsMonoNerdFont-Regular.ttf",
		"SymbolsNerdFontMono-Regular.ttf",
	}
	searchDirs := []string{
		filepath.Join(os.Getenv("HOME"), "Library", "Fonts"),
		"/Library/Fonts",
		"/System/Library/Fonts",
	}
	for _, dir := range searchDirs {
		for _, name := range candidates {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				res, err := fyne.LoadResourceFromPath(path)
				if err == nil {
					log.Printf("Loaded Nerd Font: %s", path)
					return res
				}
			}
		}
	}
	log.Println("No Nerd Font found; icons may not render")
	return nil
}

// GUIRenderer implements Renderer using Fyne.
type GUIRenderer struct {
	app           fyne.App
	win           fyne.Window
	grid          *widget.TextGrid
	keyHandler    func(ev *fyne.KeyEvent)
	runeHandler   func(r rune)
	cursorVisible bool
	stopCursor    chan bool
	renderCh      chan struct{} // signals that a render is needed
}

// NewGUIRenderer creates a new GUI renderer.
func NewGUIRenderer(title string, width, height int) (*GUIRenderer, error) {
	a := app.New()
	// Set Nerd Font theme before creating any windows
	a.Settings().SetTheme(&nerdFontTheme{
		Theme:    theme.DefaultTheme(),
		monoFont: loadNerdFont(),
	})
	w := a.NewWindow(title)

	g := widget.NewTextGrid()
	// Pre-fill with spaces so the grid has the right dimensions
	row := make([]widget.TextGridCell, width)
	for i := range row {
		row[i] = widget.TextGridCell{Rune: ' '}
	}
	rows := make([]widget.TextGridRow, height)
	for i := range rows {
		cells := make([]widget.TextGridCell, width)
		copy(cells, row)
		rows[i] = widget.TextGridRow{Cells: cells}
	}
	g.Rows = rows
	g.Refresh()

	w.SetContent(g)
	w.Resize(fyne.NewSize(float32(width)*9, float32(height)*18)) // approximate cell size

	r := &GUIRenderer{
		app:           a,
		win:           w,
		grid:          g,
		cursorVisible: true,
		stopCursor:    make(chan bool),
		renderCh:      make(chan struct{}, 1),
	}

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

	// Cursor blink goroutine — just toggles the flag and signals a render
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.cursorVisible = !r.cursorVisible
				// Non-blocking signal
				select {
				case r.renderCh <- struct{}{}:
				default:
				}
			case <-r.stopCursor:
				return
			}
		}
	}()

	return r, nil
}

var ansiPalette = []color.Color{
	color.RGBA{0, 0, 0, 255},       // 0: Black
	color.RGBA{170, 0, 0, 255},     // 1: Red
	color.RGBA{0, 170, 0, 255},     // 2: Green
	color.RGBA{170, 85, 0, 255},    // 3: Yellow
	color.RGBA{0, 0, 170, 255},     // 4: Blue
	color.RGBA{170, 0, 170, 255},   // 5: Magenta
	color.RGBA{0, 170, 170, 255},   // 6: Cyan
	color.RGBA{170, 170, 170, 255}, // 7: White
	color.RGBA{85, 85, 85, 255},    // 8: Bright Black
	color.RGBA{255, 85, 85, 255},   // 9: Bright Red
	color.RGBA{85, 255, 85, 255},   // 10: Bright Green
	color.RGBA{255, 255, 85, 255},  // 11: Bright Yellow
	color.RGBA{85, 85, 255, 255},   // 12: Bright Blue
	color.RGBA{255, 85, 255, 255},  // 13: Bright Magenta
	color.RGBA{85, 255, 255, 255},  // 14: Bright Cyan
	color.RGBA{255, 255, 255, 255}, // 15: Bright White
}

// Render draws only the dirty cells of the terminal state to the Fyne grid.
func (r *GUIRenderer) Render(t *emulator.Terminal) {
	t.Lock()

	anyDirty := false
	for _, d := range t.DirtyRows {
		if d {
			anyDirty = true
			break
		}
	}
	if !anyDirty {
		t.Unlock()
		return
	}

	// Ensure grid has enough rows
	for len(r.grid.Rows) < len(t.Grid) {
		r.grid.Rows = append(r.grid.Rows, widget.TextGridRow{})
	}

	for y, row := range t.Grid {
		if !t.DirtyRows[y] {
			continue
		}

		// Ensure row has enough cells
		for len(r.grid.Rows[y].Cells) < len(row) {
			r.grid.Rows[y].Cells = append(r.grid.Rows[y].Cells, widget.TextGridCell{Rune: ' '})
		}

		for x, cell := range row {
			if !cell.Modified {
				// Still check if cursor position needs update
				isCursor := (x == t.Cursor.X && y == t.Cursor.Y)
				if !isCursor {
					continue
				}
			}

			char := cell.Char
			if char == 0 {
				char = ' '
			}

			// Render cursor
			isCursor := (x == t.Cursor.X && y == t.Cursor.Y)
			if isCursor && r.cursorVisible {
				if char == ' ' {
					char = '█'
				}
			}

			if cell.WideCont {
				// Leave continuation cells as space
				r.grid.Rows[y].Cells[x] = widget.TextGridCell{Rune: ' '}
				continue
			}

			gridCell := widget.TextGridCell{Rune: char}

			hasFg := cell.FgColor != emulator.ColorDefault
			hasBg := cell.BgColor != emulator.ColorDefault

			if hasFg || hasBg || isCursor {
				style := &widget.CustomTextGridStyle{}
				if hasFg {
					style.FGColor = emulatorColorToFyne(cell.FgColor)
				}
				if hasBg {
					style.BGColor = emulatorColorToFyne(cell.BgColor)
				}
				if isCursor && r.cursorVisible {
					// Invert colors for cursor block
					if !hasFg {
						style.BGColor = color.RGBA{200, 200, 200, 255}
						style.FGColor = color.RGBA{0, 0, 0, 255}
					}
				}
				gridCell.Style = style
			}

			r.grid.Rows[y].Cells[x] = gridCell
		}
	}

	t.ClearDirty()
	t.Unlock()

	r.grid.Refresh()
}

func emulatorColorToFyne(c emulator.Color) color.Color {
	val := uint32(c)
	if val&0x01000000 != 0 {
		// TrueColor
		rv := uint8((val >> 16) & 0xFF)
		gv := uint8((val >> 8) & 0xFF)
		bv := uint8(val & 0xFF)
		return color.RGBA{rv, gv, bv, 255}
	}

	// Indexed color 0-15
	if val < 16 {
		return ansiPalette[val]
	}

	// Indexed color 16-231: 6x6x6 color cube
	if val >= 16 && val <= 231 {
		val -= 16
		cubeVals := []uint8{0, 95, 135, 175, 215, 255}
		return color.RGBA{cubeVals[val/36], cubeVals[(val/6)%6], cubeVals[val%6], 255}
	}

	// Indexed color 232-255: grayscale ramp
	if val >= 232 && val <= 255 {
		gray := uint8((val-232)*10 + 8)
		return color.Gray{Y: gray}
	}

	return color.White
}

// SetKeyHandler sets the function to call on key events.
func (r *GUIRenderer) SetKeyHandler(h func(ev *fyne.KeyEvent)) {
	r.keyHandler = h
}

// SetRuneHandler sets the function to call on rune events.
func (r *GUIRenderer) SetRuneHandler(h func(rn rune)) {
	r.runeHandler = h
}

// RenderCh returns the channel that signals a render is needed (e.g. cursor blink).
func (r *GUIRenderer) RenderCh() <-chan struct{} {
	return r.renderCh
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
	return 80, 24
}
