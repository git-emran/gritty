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

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"github.com/emranhossain/terminal_go/pkg/emulator"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type GUIRenderer struct {
	app fyne.App
	win fyne.Window

	raster *canvas.Raster
	img    *image.RGBA

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
	stopCursor chan bool

	mu sync.Mutex
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

	log.Println("Using default theme font")
	return theme.TextFont()
}

func NewGUIRenderer(title string, cols, rows int) (*GUIRenderer, error) {
	a := app.New()
	w := a.NewWindow(title)

	cellW := 9
	cellH := 18

	img := image.NewRGBA(image.Rect(0, 0, cols*cellW, rows*cellH))

	r := &GUIRenderer{
		app:          a,
		win:          w,
		img:          img,
		width:        cols,
		height:       rows,
		cellW:        cellW,
		cellH:        cellH,
		face:         loadNerdFontFace(14),
		renderCh:     make(chan struct{}, 1),
		stopCursor:   make(chan bool),
		cursorVisible: true,
	}

	r.prevBuffer = make([][]cellSnapshot, rows)
	for y := range r.prevBuffer {
		r.prevBuffer[y] = make([]cellSnapshot, cols)
	}

	r.raster = canvas.NewRasterFromImage(img)
	w.SetContent(r.raster)
	w.Resize(fyne.NewSize(float32(cols*cellW), float32(rows*cellH)))

	go r.cursorBlink()

	return r, nil
}

func (r *GUIRenderer) cursorBlink() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.cursorVisible = !r.cursorVisible
			select {
			case r.renderCh <- struct{}{}:
			default:
			}
		case <-r.stopCursor:
			return
		}
	}
}

func (r *GUIRenderer) Render(t *emulator.Terminal) {
	r.mu.Lock()
	defer r.mu.Unlock()

	t.Lock()
	defer t.Unlock()

	for y := 0; y < r.height; y++ {
		for x := 0; x < r.width; x++ {
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

			if snap != r.prevBuffer[y][x] {
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

	r.raster.Refresh()
}

func (r *GUIRenderer) drawCell(x, y int, snap cellSnapshot) {
	px := x * r.cellW
	py := y * r.cellH

	bg := emulatorColorToRGBA(snap.bg)
	draw.Draw(r.img, image.Rect(px, py, px+r.cellW, py+r.cellH),
		&image.Uniform{bg}, image.Point{}, draw.Src)

	fg := emulatorColorToRGBA(snap.fg)

	d := &font.Drawer{
		Dst:  r.img,
		Src:  image.NewUniform(fg),
		Face: r.face,
		Dot:  fixed.P(px, py+r.cellH-4),
	}
	d.DrawString(string(snap.char))
}

func (r *GUIRenderer) redrawFromBuffer(x, y int) {
	if x < 0 || y < 0 || y >= r.height || x >= r.width {
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

func (r *GUIRenderer) ShowAndRun() {
	r.win.ShowAndRun()
}

func (r *GUIRenderer) Close() {
	close(r.stopCursor)
	r.win.Close()
}
