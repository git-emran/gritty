package renderer

import (
	"image"
	"image/color"
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
	framebuffer *ebiten.Image
	backbuffer  *ebiten.Image
	glyphCache  map[rune]*ebiten.Image
	whitePixel  *ebiten.Image

	width  int
	height int

	cellW int
	cellH int

	face font.Face

	prevBuffer [][]cellSnapshot

	cursorVisible     bool
	lastCursorVisible bool
	prevCursorX       int
	prevCursorY       int

	renderCh chan struct{}

	mu sync.Mutex

	// Handlers
	resizeHandler func(cols, rows int)
	keyHandler    func(key ebiten.Key)
	runeHandler   func(rn rune)
	wheelHandler  func(dx, dy float64)

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

	return nil
}

func NewGUIRenderer(title string, cols, rows int) (*GUIRenderer, error) {
	cellW := 9
	cellH := 18

	fb := ebiten.NewImage(cols*cellW, rows*cellH)
	bb := ebiten.NewImage(cols*cellW, rows*cellH)

	wp := ebiten.NewImage(1, 1)
	wp.Fill(color.White)

	r := &GUIRenderer{
		framebuffer:       fb,
		backbuffer:        bb,
		glyphCache:        make(map[rune]*ebiten.Image),
		whitePixel:        wp,
		width:             cols,
		height:            rows,
		cellW:             cellW,
		cellH:             cellH,
		face:              loadNerdFontFace(14),
		renderCh:          make(chan struct{}, 1),
		cursorVisible:     true,
		lastCursorVisible: true,
		prevCursorX:       -1,
		prevCursorY:       -1,
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
	for range ticker.C {
		r.mu.Lock()
		r.cursorVisible = !r.cursorVisible
		r.mu.Unlock()
		select {
		case r.renderCh <- struct{}{}:
		default:
		}
	}
}

func (r *GUIRenderer) Update() error {
	if r.wheelHandler != nil {
		dx, dy := ebiten.Wheel()
		if dx != 0 || dy != 0 {
			log.Printf("Wheel Event: dx=%f, dy=%f\n", dx, dy)
			r.wheelHandler(dx, dy)
		}
	}

	// Handle input
	if r.keyHandler != nil {
		runes := ebiten.AppendInputChars(nil)
		for _, rn := range runes {
			if r.runeHandler != nil {
				r.runeHandler(rn)
			}
		}

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
			r.framebuffer = ebiten.NewImage(r.width*r.cellW, r.height*r.cellH)
			r.backbuffer = ebiten.NewImage(r.width*r.cellW, r.height*r.cellH)

			r.prevBuffer = make([][]cellSnapshot, r.height)
			for y := range r.prevBuffer {
				r.prevBuffer[y] = make([]cellSnapshot, r.width)
			}

			r.prevCursorX = -1
			r.prevCursorY = -1

			if r.resizeHandler != nil {
				r.resizeHandler(newCols, newRows)
			}
		}
	}

	return nil
}

func (r *GUIRenderer) Draw(screen *ebiten.Image) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.renderTerm(r.term)

	bg := emulatorColorToRGBA(emulator.ColorDefault, false)
	screen.Fill(bg)

	screen.DrawImage(r.framebuffer, nil)
}

func (r *GUIRenderer) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func (r *GUIRenderer) getGlyph(char rune) *ebiten.Image {
	if img, ok := r.glyphCache[char]; ok {
		return img
	}

	img := image.NewRGBA(image.Rect(0, 0, r.cellW, r.cellH))

	if r.face != nil {
		d := font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(color.White),
			Face: r.face,
		}
		d.Dot = fixed.P(0, r.cellH-4)
		d.DrawString(string(char))
	}

	eImg := ebiten.NewImageFromImage(img)
	r.glyphCache[char] = eImg
	return eImg
}

func (r *GUIRenderer) renderTerm(t *emulator.Terminal) bool {
	if t == nil {
		return false
	}

	t.Lock()
	if t.Width <= 0 || t.Height <= 0 {
		t.Unlock()
		return false
	}

	width := t.Width
	height := t.Height
	shift := t.PendingScroll
	t.PendingScroll = 0

	cx, cy, cursorInView := t.CursorViewPosition()

	forceFull := false
	if t.ForceRedraw {
		forceFull = true
		t.ForceRedraw = false
	}

	if len(r.prevBuffer) != height || (len(r.prevBuffer) > 0 && len(r.prevBuffer[0]) != width) {
		r.prevBuffer = make([][]cellSnapshot, height)
		for y := range r.prevBuffer {
			r.prevBuffer[y] = make([]cellSnapshot, width)
		}
		forceFull = true
	}

	dirtyRows := make([]bool, len(t.DirtyRows))
	copy(dirtyRows, t.DirtyRows)
	for y := 0; y < len(t.DirtyRows); y++ {
		t.DirtyRows[y] = false
	}

	viewRows := make([][]emulator.Cell, height)
	for y := 0; y < height; y++ {
		if forceFull || shift > 0 || (y < len(dirtyRows) && dirtyRows[y]) {
			row := t.ViewRow(y)
			if row != nil {
				viewRows[y] = make([]emulator.Cell, len(row))
				copy(viewRows[y], row)
			}
		}
	}
	t.Unlock()

	cursorChanged := r.prevCursorX != cx || r.prevCursorY != cy
	cursorVisibilityChanged := r.cursorVisible != r.lastCursorVisible

	hasDirtyRows := forceFull || shift > 0
	if !hasDirtyRows {
		for y := 0; y < len(dirtyRows); y++ {
			if dirtyRows[y] {
				hasDirtyRows = true
				break
			}
		}
	}

	if !hasDirtyRows && !cursorChanged && !cursorVisibilityChanged {
		return false
	}

	// Process hardware scrolling
	if shift > 0 && !forceFull {
		r.backbuffer.Clear()
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(0, float64(-shift*r.cellH))
		r.backbuffer.DrawImage(r.framebuffer, opts)
		r.framebuffer, r.backbuffer = r.backbuffer, r.framebuffer

		// Shift prevBuffer so we don't redraw everything
		if shift < height {
			copy(r.prevBuffer[0:], r.prevBuffer[shift:])
			for y := height - shift; y < height; y++ {
				for x := range r.prevBuffer[y] {
					r.prevBuffer[y][x] = cellSnapshot{}
				}
			}
		} else {
			for y := 0; y < height; y++ {
				for x := range r.prevBuffer[y] {
					r.prevBuffer[y][x] = cellSnapshot{}
				}
			}
		}
	}

	changed := shift > 0
	for y := 0; y < height; y++ {
		if !forceFull && (y >= len(dirtyRows) || !dirtyRows[y]) {
			continue
		}
		row := viewRows[y]
		for x := 0; x < width; x++ {
			cell := emulator.Cell{Char: ' ', FgColor: emulator.ColorDefault, BgColor: emulator.ColorDefault}
			if row != nil && x < len(row) {
				cell = row[x]
			}
			char := cell.Char
			if char == 0 || cell.WideCont {
				char = ' '
			}

			snap := cellSnapshot{char: char, fg: cell.FgColor, bg: cell.BgColor}
			if forceFull || snap != r.prevBuffer[y][x] {
				r.prevBuffer[y][x] = snap
				r.drawCellGPU(x, y, snap)
				changed = true
			}
		}
	}

	if cursorChanged || cursorVisibilityChanged {
		if r.redrawFromBuffer(r.prevCursorX, r.prevCursorY) {
			changed = true
		}
	}

	if cursorInView && r.cursorVisible {
		r.drawCursorGPU(cx, cy)
		changed = true
	}

	r.prevCursorX = cx
	r.prevCursorY = cy
	r.lastCursorVisible = r.cursorVisible

	return changed
}

func emulatorColorToRGBA(c emulator.Color, isFg bool) color.RGBA {
	if c == emulator.ColorDefault {
		if isFg {
			return color.RGBA{R: 220, G: 220, B: 220, A: 255}
		}
		return color.RGBA{R: 30, G: 30, B: 30, A: 255}
	}
	val := uint32(c)
	rv := uint8((val >> 16) & 0xFF)
	gv := uint8((val >> 8) & 0xFF)
	bv := uint8(val & 0xFF)
	return color.RGBA{R: rv, G: gv, B: bv, A: 255}
}

func (r *GUIRenderer) drawCellGPU(x, y int, snap cellSnapshot) {
	px := float64(x * r.cellW)
	py := float64(y * r.cellH)

	// Background
	bg := emulatorColorToRGBA(snap.bg, false)
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(float64(r.cellW), float64(r.cellH))
	opts.GeoM.Translate(px, py)
	opts.ColorScale.ScaleWithColor(bg)
	opts.CompositeMode = ebiten.CompositeModeCopy
	r.framebuffer.DrawImage(r.whitePixel, opts)

	if snap.char == ' ' || r.face == nil {
		return
	}

	// Foreground
	fg := emulatorColorToRGBA(snap.fg, true)
	glyph := r.getGlyph(snap.char)
	
	opts = &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(px, py)
	opts.ColorScale.ScaleWithColor(fg)
	r.framebuffer.DrawImage(glyph, opts)
}

func (r *GUIRenderer) redrawFromBuffer(x, y int) bool {
	if y < 0 || x < 0 || y >= len(r.prevBuffer) || x >= len(r.prevBuffer[y]) {
		return false
	}
	r.drawCellGPU(x, y, r.prevBuffer[y][x])
	return true
}

func (r *GUIRenderer) drawCursorGPU(x, y int) {
	if y < 0 || x < 0 || y >= len(r.prevBuffer) || x >= len(r.prevBuffer[y]) {
		return
	}

	px := float64(x * r.cellW)
	py := float64(y * r.cellH)

	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(2, float64(r.cellH))
	opts.GeoM.Translate(px, py)
	r.framebuffer.DrawImage(r.whitePixel, opts)
}

func (r *GUIRenderer) RenderCh() <-chan struct{} {
	return r.renderCh
}

func (r *GUIRenderer) ShowAndRun(t *emulator.Terminal) {
	r.term = t
	ebiten.SetScreenClearedEveryFrame(false)
	ebiten.SetRunnableOnUnfocused(true)
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

func (r *GUIRenderer) SetWheelHandler(h func(dx, dy float64)) {
	r.wheelHandler = h
}

func (r *GUIRenderer) Close() {
}
