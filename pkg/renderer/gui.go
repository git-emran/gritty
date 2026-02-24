package renderer

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/emranhossain/terminal_go/pkg/emulator"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type GUIRenderer struct {
	img *image.RGBA
	screen *ebiten.Image

	width  int
	height int

	cellW int
	cellH int

	face font.Face

	prevBuffer [][]cellSnapshot

	cursorVisible bool
	prevCursorX   int
	prevCursorY   int

	renderCh   chan struct{}
	
	mu sync.Mutex

	// Handlers
	resizeHandler func(cols, rows int)
	keyHandler    func(key ebiten.Key)
	runeHandler   func(rn rune)

	term *emulator.Terminal
}

type cellSnapshot struct {
	char rune
	fg   emulator.Color
	bg   emulator.Color
}

func loadNerdFontFace(size float64) font.Face {
	candidates := []string{
		"JetBrainsMonoNerdFontMono-Regular.ttf",
		"FiraCodeNerdFontMono-Regular.ttf",
		"HackNerdFontMono-Regular.ttf",
	}

	searchDirs := []string{
		filepath.Join(os.Getenv("HOME"), "Library", "Fonts"),
		"/Library/Fonts",
	}

	for _, dir := range searchDirs {
		for _, name := range candidates {
			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			ft, err := opentype.Parse(data)
			if err != nil {
				continue
			}
			face, err := opentype.NewFace(ft, &opentype.FaceOptions{
				Size:    size,
				DPI:     72,
				Hinting: font.HintingFull,
			})
			if err == nil {
				log.Println("Loaded font:", path)
				return face
			}
		}
	}

	// If no Nerd Font found, we'll try a basic font or at least return something valid
	// For now, let's assume one of these exists. If not, this might panic or return nil.
	// In a real app we'd bundle a font.
	return nil 
}

func NewGUIRenderer(title string, cols, rows int) (*GUIRenderer, error) {
	cellW := 9
	cellH := 18

	img := image.NewRGBA(image.Rect(0, 0, cols*cellW, rows*cellH))

	r := &GUIRenderer{
		img:          img,
		width:        cols,
		height:       rows,
		cellW:        cellW,
		cellH:        cellH,
		face:         loadNerdFontFace(14),
		renderCh:     make(chan struct{}, 1),
		cursorVisible: true,
	}

	r.prevBuffer = make([][]cellSnapshot, rows)
	for y := range r.prevBuffer {
		r.prevBuffer[y] = make([]cellSnapshot, cols)
	}

	go r.cursorBlink()

	ebiten.SetWindowTitle(title)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	return r, nil
}

func (r *GUIRenderer) cursorBlink() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.mu.Lock()
			r.cursorVisible = !r.cursorVisible
			r.mu.Unlock()
			select {
			case r.renderCh <- struct{}{}:
			default:
			}
		}
	}
}

// Update implements ebiten.Game
func (r *GUIRenderer) Update() error {
	// Handle input
	if r.keyHandler != nil {
		// This is a bit simplified, but Ebitengine handles repeats too.
		// For a terminal, we need character events and special keys.
		
		// Runes
		var runes []rune
		runes = ebiten.AppendInputChars(runes[:0])
		for _, rn := range runes {
			if r.runeHandler != nil {
				r.runeHandler(rn)
			}
		}

		// Special keys
		keys := []ebiten.Key{
			ebiten.KeyEnter, ebiten.KeyBackspace, ebiten.KeyTab,
			ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyLeft, ebiten.KeyRight,
			ebiten.KeyHome, ebiten.KeyEnd, ebiten.KeyPageUp, ebiten.KeyPageDown,
			ebiten.KeyDelete, ebiten.KeyEscape,
		}

		for _, k := range keys {
			if inpututil.IsKeyJustPressed(k) || inpututil.KeyPressDuration(k) > 30 && inpututil.KeyPressDuration(k)%2 == 0 {
				r.keyHandler(k)
			}
		}
	}

	// Resize handling
	w, h := ebiten.WindowSize()
	newCols := w / r.cellW
	newRows := h / r.cellH
	if newCols != r.width || newRows != r.height {
		if newCols > 0 && newRows > 0 {
			r.width = newCols
			r.height = newRows
			r.img = image.NewRGBA(image.Rect(0, 0, r.width*r.cellW, r.height*r.cellH))
			
			r.prevBuffer = make([][]cellSnapshot, r.height)
			for y := range r.prevBuffer {
				r.prevBuffer[y] = make([]cellSnapshot, r.width)
			}

			if r.resizeHandler != nil {
				r.resizeHandler(newCols, newRows)
			}
		}
	}

	return nil
}

// Draw implements ebiten.Game
func (r *GUIRenderer) Draw(screen *ebiten.Image) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.term != nil {
		r.renderTerm(r.term)
	}

	if r.screen == nil || r.screen.Bounds().Dx() != r.img.Bounds().Dx() || r.screen.Bounds().Dy() != r.img.Bounds().Dy() {
		r.screen = ebiten.NewImageFromImage(r.img)
	} else {
		r.screen.WritePixels(r.img.Pix)
	}

	screen.DrawImage(r.screen, nil)
}

// Layout implements ebiten.Game
func (r *GUIRenderer) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func (r *GUIRenderer) renderTerm(t *emulator.Terminal) {
	t.Lock()
	defer t.Unlock()

	// Ensure prevBuffer is the right size (in case terminal resized but we didn't update yet)
	if len(r.prevBuffer) != t.Height || (len(r.prevBuffer) > 0 && len(r.prevBuffer[0]) != t.Width) {
		r.prevBuffer = make([][]cellSnapshot, t.Height)
		for y := range r.prevBuffer {
			r.prevBuffer[y] = make([]cellSnapshot, t.Width)
		}
	}

	for y := 0; y < t.Height; y++ {
		for x := 0; x < t.Width; x++ {
			if y >= len(t.Grid) || x >= len(t.Grid[y]) {
				continue
			}
			cell := t.Grid[y][x]

			char := cell.Char
			if char == 0 || cell.WideCont {
				char = ' '
			}

			snap := cellSnapshot{
				char: char,
				fg:   cell.FgColor,
				bg:   cell.BgColor,
			}

			if y < len(r.prevBuffer) && x < len(r.prevBuffer[y]) && snap != r.prevBuffer[y][x] {
				r.prevBuffer[y][x] = snap
				r.drawCell(x, y, snap)
			}
		}
	}

	// cursor
	cx := t.Cursor.X
	cy := t.Cursor.Y

	if r.prevCursorX != cx || r.prevCursorY != cy {
		r.redrawFromBuffer(r.prevCursorX, r.prevCursorY)
	}

	if r.cursorVisible {
		r.drawCursor(cx, cy)
	}

	r.prevCursorX = cx
	r.prevCursorY = cy
}

func (r *GUIRenderer) drawCell(x, y int, snap cellSnapshot) {
	px := x * r.cellW
	py := y * r.cellH

	bg := emulatorColorToRGBA(snap.bg)
	draw.Draw(r.img, image.Rect(px, py, px+r.cellW, py+r.cellH),
		&image.Uniform{bg}, image.Point{}, draw.Src)

	if snap.char == ' ' {
		return
	}

	fg := emulatorColorToRGBA(snap.fg)

	if r.face != nil {
		d := &font.Drawer{
			Dst:  r.img,
			Src:  image.NewUniform(fg),
			Face: r.face,
			Dot:  fixed.P(px, py+r.cellH-4),
		}
		d.DrawString(string(snap.char))
	}
}

func (r *GUIRenderer) redrawFromBuffer(x, y int) {
	if x < 0 || y < 0 || y >= r.height || x >= r.width {
		return
	}
	if y >= len(r.prevBuffer) || x >= len(r.prevBuffer[y]) {
		return
	}
	snap := r.prevBuffer[y][x]
	r.drawCell(x, y, snap)
}

func (r *GUIRenderer) drawCursor(x, y int) {
	px := x * r.cellW
	py := y * r.cellH

	draw.Draw(r.img,
		image.Rect(px, py, px+2, py+r.cellH),
		&image.Uniform{color.White},
		image.Point{},
		draw.Src)
}

func emulatorColorToRGBA(c emulator.Color) color.Color {
	if c == emulator.ColorDefault {
		return color.Black
	}
	val := uint32(c)
	r := uint8((val >> 16) & 0xFF)
	g := uint8((val >> 8) & 0xFF)
	b := uint8(val & 0xFF)
	return color.RGBA{r, g, b, 255}
}

func (r *GUIRenderer) RenderCh() <-chan struct{} {
	return r.renderCh
}

func (r *GUIRenderer) ShowAndRun(t *emulator.Terminal) {
	r.term = t
	if err := ebiten.RunGame(r); err != nil {
		log.Fatal(err)
	}
}

func (r *GUIRenderer) SetResizeHandler(h func(cols, rows int)) {
	r.resizeHandler = h
}

func (r *GUIRenderer) SetKeyHandler(h func(key ebiten.Key)) {
	r.keyHandler = h
}

func (r *GUIRenderer) SetRuneHandler(h func(rn rune)) {
	r.runeHandler = h
}

func (r *GUIRenderer) Close() {
	// Ebitengine doesn't have a direct Close() for the window like Fyne,
	// but we can signal the shell or just let the process exit.
}
