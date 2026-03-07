package renderer

import (
	"image"
	"image/color"
	"log"
	"math"
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

	viewportPadX int
	viewportPadY int

	face          font.Face
	glyphBaseline int

	prevBuffer [][]cellSnapshot

	cursorVisible     bool
	lastCursorVisible bool
	prevCursorX       int
	prevCursorY       int
	isDarkMode        bool
	theme             terminalTheme
	lastThemeCheck    time.Time

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
	bold bool
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func loadNerdFontFace(size float64, dpi float64) font.Face {
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
				DPI:     dpi,
				Hinting: font.HintingFull,
			})
			if err == nil {
				log.Printf("Loaded font: %s (DPI: %.1f)", path, dpi)
				return face
			}
		}
	}

	return nil
}

func measureCell(face font.Face) (cellW, cellH, baseline int) {
	if face == nil {
		return 10, 20, 15
	}

	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	descent := metrics.Descent.Ceil()
	lineGap := 0
	cellH = ascent + descent + lineGap

	advance := font.MeasureString(face, "M").Ceil()
	cellW = advance
	baseline = ascent
	return cellW, cellH, baseline
}

func NewGUIRenderer(title string, cols, rows int) (*GUIRenderer, error) {
	scale := ebiten.DeviceScaleFactor()
	face := loadNerdFontFace(12, 96*scale)
	cellW, cellH, baseline := measureCell(face)
	padX := maxInt(cellW, 12)
	padY := maxInt(cellH/2, 12)

	fb := ebiten.NewImage(cols*cellW, rows*cellH)
	bb := ebiten.NewImage(cols*cellW, rows*cellH)

	wp := ebiten.NewImage(1, 1)
	wp.Fill(color.White)

	isDark := detectDarkMode()
	r := &GUIRenderer{
		framebuffer:       fb,
		backbuffer:        bb,
		glyphCache:        make(map[rune]*ebiten.Image),
		whitePixel:        wp,
		width:             cols,
		height:            rows,
		cellW:             cellW,
		cellH:             cellH,
		viewportPadX:      padX,
		viewportPadY:      padY,
		face:              face,
		glyphBaseline:     baseline,
		renderCh:          make(chan struct{}, 1),
		cursorVisible:     true,
		lastCursorVisible: true,
		prevCursorX:       -1,
		prevCursorY:       -1,
		isDarkMode:        isDark,
		theme:             activeTheme(isDark),
		lastThemeCheck:    time.Now(),
	}

	r.prevBuffer = make([][]cellSnapshot, rows)
	for y := range r.prevBuffer {
		r.prevBuffer[y] = make([]cellSnapshot, cols)
	}

	go r.cursorBlink()

	ebiten.SetWindowTitle(title)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetScreenFilterEnabled(false)
	ebiten.SetWindowSize(
		int(float64(cols*cellW+2*padX)/scale),
		int(float64(rows*cellH+2*padY)/scale),
	)
	// Window limits in logical pixels
	ebiten.SetWindowSizeLimits(
		int(float64(40*cellW+2*padX)/scale),
		int(float64(12*cellH+2*padY)/scale),
		-1,
		-1,
	)

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
	// Poll system appearance periodically so light/dark changes apply at runtime.
	if time.Since(r.lastThemeCheck) >= 2*time.Second {
		r.lastThemeCheck = time.Now()
		if isDark := detectDarkMode(); isDark != r.isDarkMode {
			r.applyTheme(isDark)
		}
	}

	if r.wheelHandler != nil {
		dx, dy := ebiten.Wheel()
		if dx != 0 || dy != 0 {
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

	// Dynamic scaling and resize handling
	scale := ebiten.DeviceScaleFactor()
	w, h := ebiten.WindowSize()
	// logical width * scale = physical width
	pW := int(float64(w) * scale)
	pH := int(float64(h) * scale)

	availableW := maxInt(pW-2*r.viewportPadX, r.cellW)
	availableH := maxInt(pH-2*r.viewportPadY, r.cellH)

	newCols := availableW / r.cellW
	newRows := availableH / r.cellH

	if newCols != r.width || newRows != r.height {
		if newCols > 0 && newRows > 0 {
			r.width = newCols
			r.height = newRows

			// Re-measure if scale changed significantly
			// (Ebitengine scale factor might change when moving between monitors)
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

	bg := r.resolveColor(emulator.ColorDefault, false, false)
	screen.Fill(bg)

	// Center the framebuffer to avoid uneven empty space (slack) on resize
	w, h := ebiten.WindowSize()
	scale := ebiten.DeviceScaleFactor()
	pW := int(float64(w) * scale)
	pH := int(float64(h) * scale)

	fbW := r.width * r.cellW
	fbH := r.height * r.cellH
	availableW := maxInt(pW-2*r.viewportPadX, fbW)
	availableH := maxInt(pH-2*r.viewportPadY, fbH)

	opts := &ebiten.DrawImageOptions{}
	offsetX := float64(r.viewportPadX + (availableW-fbW)/2)
	offsetY := float64(r.viewportPadY + (availableH-fbH)/2)
	opts.GeoM.Translate(offsetX, offsetY)

	screen.DrawImage(r.framebuffer, opts)
}

func (r *GUIRenderer) Layout(outsideWidth, outsideHeight int) (int, int) {
	s := ebiten.DeviceScaleFactor()
	return int(float64(outsideWidth) * s), int(float64(outsideHeight) * s)
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
		d.Dot = fixed.P(0, r.glyphBaseline)
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

			snap := cellSnapshot{char: char, fg: cell.FgColor, bg: cell.BgColor, bold: cell.Bold}
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

func (r *GUIRenderer) drawCellGPU(x, y int, snap cellSnapshot) {
	px := float64(x * r.cellW)
	py := float64(y * r.cellH)

	// Background
	bg := r.resolveColor(snap.bg, false, false)
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
	fg := r.resolveColor(snap.fg, true, snap.bold)
	fg = r.ensureReadableForeground(fg, bg)
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
	opts.ColorScale.Scale(float32(r.theme.cursor[0])/255.0, float32(r.theme.cursor[1])/255.0, float32(r.theme.cursor[2])/255.0, 1)
	r.framebuffer.DrawImage(r.whitePixel, opts)
}

func (r *GUIRenderer) applyTheme(isDark bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.isDarkMode = isDark
	r.theme = activeTheme(isDark)
	r.glyphCache = make(map[rune]*ebiten.Image) // Clear cache for new colors
	if r.term != nil {
		r.term.RequestFullRedraw()
	}
}

func (r *GUIRenderer) resolveColor(c emulator.Color, isFg bool, bold bool) color.RGBA {
	if c == emulator.ColorDefault {
		if isFg {
			return color.RGBA{R: r.theme.foreground[0], G: r.theme.foreground[1], B: r.theme.foreground[2], A: 255}
		}
		return color.RGBA{R: r.theme.background[0], G: r.theme.background[1], B: r.theme.background[2], A: 255}
	}

	val := uint32(c)
	hi := uint8(val >> 24)
	if hi == 0x01 {
		return color.RGBA{
			R: uint8((val >> 16) & 0xFF),
			G: uint8((val >> 8) & 0xFF),
			B: uint8(val & 0xFF),
			A: 255,
		}
	}

	if val <= 255 {
		idx := uint8(val)
		if isFg && bold && idx < 8 {
			idx += 8
		}
		return r.ansi256Color(idx)
	}

	return color.RGBA{
		R: uint8((val >> 16) & 0xFF),
		G: uint8((val >> 8) & 0xFF),
		B: uint8(val & 0xFF),
		A: 255,
	}
}

func (r *GUIRenderer) ansi256Color(idx uint8) color.RGBA {
	if idx < 16 {
		rgb := r.theme.ansi16[idx]
		return color.RGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 255}
	}

	if idx >= 16 && idx <= 231 {
		i := int(idx - 16)
		r := i / 36
		g := (i % 36) / 6
		b := i % 6
		scale := [6]uint8{0, 95, 135, 175, 215, 255}
		return color.RGBA{R: scale[r], G: scale[g], B: scale[b], A: 255}
	}

	gray := uint8(8 + (idx-232)*10)
	return color.RGBA{R: gray, G: gray, B: gray, A: 255}
}

func srgbToLinear(c uint8) float64 {
	v := float64(c) / 255.0
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func relativeLuminance(c color.RGBA) float64 {
	r := srgbToLinear(c.R)
	g := srgbToLinear(c.G)
	b := srgbToLinear(c.B)
	return 0.2126*r + 0.7152*g + 0.0722*b
}

func contrastRatio(a, b color.RGBA) float64 {
	la := relativeLuminance(a)
	lb := relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func blendColor(a, b color.RGBA, mix float64) color.RGBA {
	if mix <= 0 {
		return a
	}
	if mix >= 1 {
		return b
	}
	inv := 1 - mix
	return color.RGBA{
		R: uint8(float64(a.R)*inv + float64(b.R)*mix),
		G: uint8(float64(a.G)*inv + float64(b.G)*mix),
		B: uint8(float64(a.B)*inv + float64(b.B)*mix),
		A: 255,
	}
}

func (r *GUIRenderer) ensureReadableForeground(fg, bg color.RGBA) color.RGBA {
	if contrastRatio(fg, bg) >= 4.5 {
		return fg
	}

	target := color.RGBA{
		R: r.theme.foreground[0],
		G: r.theme.foreground[1],
		B: r.theme.foreground[2],
		A: 255,
	}
	if contrastRatio(target, bg) < 4.5 {
		light := color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
		dark := color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
		if contrastRatio(light, bg) >= contrastRatio(dark, bg) {
			target = light
		} else {
			target = dark
		}
	}

	for step := 1; step <= 6; step++ {
		candidate := blendColor(fg, target, float64(step)/6.0)
		if contrastRatio(candidate, bg) >= 4.5 {
			return candidate
		}
	}
	return target
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
