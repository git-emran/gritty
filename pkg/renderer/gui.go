package renderer

import (
	"image"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/emranhossain/terminal_go/pkg/emulator"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type GUIRenderer struct {
	framebuffer *ebiten.Image
	backbuffer  *ebiten.Image
	glyphCache  map[glyphKey]*ebiten.Image
	whitePixel  *ebiten.Image

	width  int
	height int

	cellW int
	cellH int

	viewportPadX int
	viewportPadY int

	faceRegular    font.Face
	faceBold       font.Face
	faceItalic     font.Face
	faceBoldItalic font.Face
	glyphBaseline  int

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

	changedCells []drawJob

	term *emulator.Terminal
}

type cellSnapshot struct {
	char      rune
	fg        emulator.Color
	bg        emulator.Color
	bold      bool
	italic    bool
	underline bool
}

type drawJob struct {
	x    int
	y    int
	snap cellSnapshot
}

type glyphKey struct {
	char  rune
	style uint8
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type fontFaces struct {
	regular    font.Face
	bold       font.Face
	italic     font.Face
	boldItalic font.Face
}

// systemFontDirs returns platform-specific font search directories.
// Covers macOS, common Linux distros, and Windows.
func systemFontDirs() []string {
	home := os.Getenv("HOME")
	switch runtime.GOOS {
	case "darwin":
		return []string{
			filepath.Join(home, "Library", "Fonts"),
			"/Library/Fonts",
			"/System/Library/Fonts",
			"/System/Library/Fonts/Supplemental",
		}
	case "linux":
		return []string{
			filepath.Join(home, ".local", "share", "fonts"),
			filepath.Join(home, ".fonts"),
			"/usr/share/fonts/truetype",
			"/usr/share/fonts/opentype",
			"/usr/share/fonts/TTF",
			"/usr/share/fonts",
			"/usr/local/share/fonts",
		}
	case "windows":
		windir := os.Getenv("WINDIR")
		if windir == "" {
			windir = `C:\Windows`
		}
		local := os.Getenv("LOCALAPPDATA")
		dirs := []string{filepath.Join(windir, "Fonts")}
		if local != "" {
			dirs = append(dirs, filepath.Join(local, "Microsoft", "Windows", "Fonts"))
		}
		return dirs
	default:
		return []string{"/usr/share/fonts", "/usr/local/share/fonts"}
	}
}

// loadFontFace searches dirs for the first matching candidate filename and
// returns a parsed font.Face. Returns nil if nothing is found.
func loadFontFace(searchDirs []string, candidates []string, size float64, dpi float64) font.Face {
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

// loadEmbeddedFace parses one of the compiled-in Go gofont TTF byte slices.
// This is the guaranteed last-resort — no files required on disk.
func loadEmbeddedFace(ttfData []byte, size float64, dpi float64) font.Face {
	ft, err := opentype.Parse(ttfData)
	if err != nil {
		return nil
	}
	face, err := opentype.NewFace(ft, &opentype.FaceOptions{
		Size:    size,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	return face
}

// loadFontFaces builds font faces using a 3-tier priority:
//  1. Nerd Fonts (user / system installed) — best glyph coverage
//  2. System monospace fonts for the current OS — reliable fallback
//  3. Compiled-in Go gomono — absolute last resort, always works
func loadFontFaces(size float64, dpi float64) fontFaces {
	dirs := systemFontDirs()

	// ── Tier 1: Nerd Fonts ───────────────────────────────────────────────────
	nerdRegular := []string{
		"JetBrainsMonoNerdFontMono-Regular.ttf",
		"FiraCodeNerdFontMono-Regular.ttf",
		"HackNerdFontMono-Regular.ttf",
		"HasklugNerdFontMono-Regular.otf",
		"JetBrainsMonoNLNerdFont-Regular.ttf",
	}
	nerdBold := []string{
		"JetBrainsMonoNerdFontMono-Bold.ttf",
		"FiraCodeNerdFontMono-Bold.ttf",
		"HackNerdFontMono-Bold.ttf",
		"HasklugNerdFontMono-Bold.otf",
		"JetBrainsMonoNLNerdFont-Bold.ttf",
	}
	nerdItalic := []string{
		"JetBrainsMonoNerdFontMono-Italic.ttf",
		"FiraCodeNerdFontMono-Italic.ttf",
		"HackNerdFontMono-Italic.ttf",
		"HasklugNerdFontMono-Italic.otf",
		"JetBrainsMonoNLNerdFont-Italic.ttf",
	}
	nerdBoldItalic := []string{
		"JetBrainsMonoNerdFontMono-BoldItalic.ttf",
		"FiraCodeNerdFontMono-BoldItalic.ttf",
		"HackNerdFontMono-BoldItalic.ttf",
		"HasklugNerdFontMono-BoldItalic.otf",
		"JetBrainsMonoNLNerdFont-BoldItalic.ttf",
	}

	// ── Tier 2: System monospace fonts (cross-platform) ──────────────────────
	// macOS: SF Mono, Menlo, Monaco, Courier New
	// Linux: DejaVu Sans Mono, Liberation Mono, Ubuntu Mono, Noto Mono
	// Windows: Cascadia Code, Consolas, Courier New, Lucida Console
	sysRegular := []string{
		// macOS
		"SFNSMono.ttf",
		"Menlo.ttc",
		"Monaco.ttf",
		"Courier New.ttf",
		"Andale Mono.ttf",
		// Linux
		"DejaVuSansMono.ttf",
		"dejavu/DejaVuSansMono.ttf",
		"truetype/dejavu/DejaVuSansMono.ttf",
		"LiberationMono-Regular.ttf",
		"liberation/LiberationMono-Regular.ttf",
		"UbuntuMono-Regular.ttf",
		"ubuntu/UbuntuMono-Regular.ttf",
		"NotoMono-Regular.ttf",
		"noto/NotoMono-Regular.ttf",
		// Windows
		"CascadiaCode.ttf",
		"CascadiaMono.ttf",
		"consola.ttf",
		"lucon.ttf",
		"cour.ttf",
	}
	sysBold := []string{
		// macOS — SF Mono has no separate bold file (weight embedded)
		"CourierNewBold.ttf",
		"Courier New Bold.ttf",
		// Linux
		"DejaVuSansMono-Bold.ttf",
		"dejavu/DejaVuSansMono-Bold.ttf",
		"truetype/dejavu/DejaVuSansMono-Bold.ttf",
		"LiberationMono-Bold.ttf",
		"liberation/LiberationMono-Bold.ttf",
		"UbuntuMono-Bold.ttf",
		"ubuntu/UbuntuMono-Bold.ttf",
		// Windows
		"CascadiaCode.ttf",
		"consolab.ttf",
		"courbd.ttf",
	}
	sysItalic := []string{
		// macOS
		"SFNSMonoItalic.ttf",
		"CourierNewItalic.ttf",
		"Courier New Italic.ttf",
		// Linux
		"DejaVuSansMono-Oblique.ttf",
		"dejavu/DejaVuSansMono-Oblique.ttf",
		"truetype/dejavu/DejaVuSansMono-Oblique.ttf",
		"LiberationMono-Italic.ttf",
		"liberation/LiberationMono-Italic.ttf",
		"UbuntuMono-Italic.ttf",
		"ubuntu/UbuntuMono-Italic.ttf",
		// Windows
		"couri.ttf",
	}
	sysBoldItalic := []string{
		"Courier New Bold Italic.ttf",
		"DejaVuSansMono-BoldOblique.ttf",
		"dejavu/DejaVuSansMono-BoldOblique.ttf",
		"truetype/dejavu/DejaVuSansMono-BoldOblique.ttf",
		"LiberationMono-BoldItalic.ttf",
		"liberation/LiberationMono-BoldItalic.ttf",
		"UbuntuMono-BoldItalic.ttf",
		"ubuntu/UbuntuMono-BoldItalic.ttf",
		"courbi.ttf",
	}

	// Try Nerd Font first, then system font
	resolveFace := func(nerd, sys []string, embeddedData []byte) font.Face {
		if f := loadFontFace(dirs, nerd, size, dpi); f != nil {
			return f
		}
		if f := loadFontFace(dirs, sys, size, dpi); f != nil {
			return f
		}
		// ── Tier 3: embedded Go gomono — always succeeds ──────────────────
		log.Printf("Font: using embedded Go gomono fallback (DPI: %.1f)", dpi)
		return loadEmbeddedFace(embeddedData, size, dpi)
	}

	return fontFaces{
		regular:    resolveFace(nerdRegular, sysRegular, gomono.TTF),
		bold:       resolveFace(nerdBold, sysBold, gomonobold.TTF),
		italic:     resolveFace(nerdItalic, sysItalic, gomonoitalic.TTF),
		boldItalic: resolveFace(nerdBoldItalic, sysBoldItalic, gomonobolditalic.TTF),
	}
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
	faces := loadFontFaces(12, 96*scale)
	cellW, cellH, baseline := measureCell(faces.regular)
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
		glyphCache:        make(map[glyphKey]*ebiten.Image),
		whitePixel:        wp,
		width:             cols,
		height:            rows,
		cellW:             cellW,
		cellH:             cellH,
		viewportPadX:      padX,
		viewportPadY:      padY,
		faceRegular:       faces.regular,
		faceBold:          faces.bold,
		faceItalic:        faces.italic,
		faceBoldItalic:    faces.boldItalic,
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
			ebiten.KeyInsert, ebiten.KeyDelete, ebiten.KeyEscape,
			ebiten.KeyF1, ebiten.KeyF2, ebiten.KeyF3, ebiten.KeyF4, ebiten.KeyF5, ebiten.KeyF6,
			ebiten.KeyF7, ebiten.KeyF8, ebiten.KeyF9, ebiten.KeyF10, ebiten.KeyF11, ebiten.KeyF12,
		}

		ctrlPressed := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)
		if ctrlPressed {
			keys = append(keys,
				ebiten.KeyA, ebiten.KeyB, ebiten.KeyC, ebiten.KeyD, ebiten.KeyE, ebiten.KeyF, ebiten.KeyG,
				ebiten.KeyH, ebiten.KeyI, ebiten.KeyJ, ebiten.KeyK, ebiten.KeyL, ebiten.KeyM, ebiten.KeyN,
				ebiten.KeyO, ebiten.KeyP, ebiten.KeyQ, ebiten.KeyR, ebiten.KeyS, ebiten.KeyT, ebiten.KeyU,
				ebiten.KeyV, ebiten.KeyW, ebiten.KeyX, ebiten.KeyY, ebiten.KeyZ,
				ebiten.KeySpace,
			)
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

func (r *GUIRenderer) glyphFace(style uint8) font.Face {
	isBold := (style & 0x1) != 0
	isItalic := (style & 0x2) != 0

	if isBold && isItalic && r.faceBoldItalic != nil {
		return r.faceBoldItalic
	}
	if isBold && r.faceBold != nil {
		return r.faceBold
	}
	if isItalic && r.faceItalic != nil {
		return r.faceItalic
	}
	return r.faceRegular
}

func (r *GUIRenderer) getGlyph(char rune, style uint8) *ebiten.Image {
	key := glyphKey{char: char, style: style}
	if img, ok := r.glyphCache[key]; ok {
		return img
	}

	img := image.NewRGBA(image.Rect(0, 0, r.cellW, r.cellH))

	face := r.glyphFace(style)
	if face != nil {
		d := font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(color.White),
			Face: face,
		}
		d.Dot = fixed.P(0, r.glyphBaseline)
		d.DrawString(string(char))
	}

	eImg := ebiten.NewImageFromImage(img)
	r.glyphCache[key] = eImg
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

	if cap(r.changedCells) < width*height {
		r.changedCells = make([]drawJob, 0, width*height)
	}
	r.changedCells = r.changedCells[:0]

	// Process shift in prevBuffer under lock to keep it in sync with the Grid
	if shift > 0 && !forceFull {
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

	// Capture modified cells directly under the lock (extremely fast struct comparison)
	for y := 0; y < height; y++ {
		isRowDirty := forceFull || shift > 0 || (y < len(t.DirtyRows) && t.DirtyRows[y])
		if y < len(t.DirtyRows) {
			t.DirtyRows[y] = false
		}
		if !isRowDirty {
			continue
		}

		row := t.ViewRow(y)
		if row == nil {
			continue
		}

		for x := 0; x < width; x++ {
			cell := row[x]
			char := cell.Char
			if char == 0 || cell.WideCont {
				char = ' '
			}

			snap := cellSnapshot{
				char:      char,
				fg:        cell.FgColor,
				bg:        cell.BgColor,
				bold:      cell.Bold,
				italic:    cell.Italic,
				underline: cell.Underline,
			}

			if forceFull || snap != r.prevBuffer[y][x] {
				r.prevBuffer[y][x] = snap
				r.changedCells = append(r.changedCells, drawJob{x: x, y: y, snap: snap})
			}
		}
	}
	t.Unlock()

	cursorChanged := r.prevCursorX != cx || r.prevCursorY != cy
	cursorVisibilityChanged := r.cursorVisible != r.lastCursorVisible

	changed := shift > 0 || len(r.changedCells) > 0 || cursorChanged || cursorVisibilityChanged

	if !changed {
		return false
	}

	// Process hardware scrolling on GPU (outside the lock)
	if shift > 0 && !forceFull {
		r.backbuffer.Clear()
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(0, float64(-shift*r.cellH))
		r.backbuffer.DrawImage(r.framebuffer, opts)
		r.framebuffer, r.backbuffer = r.backbuffer, r.framebuffer
	}

	// Draw changed cells to GPU (outside the lock)
	for _, job := range r.changedCells {
		r.drawCellGPU(job.x, job.y, job.snap)
	}

	if cursorChanged || cursorVisibilityChanged {
		r.redrawFromBuffer(r.prevCursorX, r.prevCursorY)
	}

	if cursorInView && r.cursorVisible {
		r.drawCursorGPU(cx, cy)
	}

	r.prevCursorX = cx
	r.prevCursorY = cy
	r.lastCursorVisible = r.cursorVisible

	return true
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

	if snap.char == ' ' || r.faceRegular == nil {
		return
	}

	// Foreground
	fg := r.resolveColor(snap.fg, true, snap.bold)
	style := uint8(0)
	if snap.bold {
		style |= 0x1
	}
	if snap.italic {
		style |= 0x2
	}
	glyph := r.getGlyph(snap.char, style)

	opts = &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(px, py)
	opts.ColorScale.ScaleWithColor(fg)
	r.framebuffer.DrawImage(glyph, opts)

	if snap.underline {
		lineY := py + float64(r.cellH-2)
		if lineY < py {
			lineY = py
		}
		uOpts := &ebiten.DrawImageOptions{}
		uOpts.GeoM.Scale(float64(r.cellW), 1)
		uOpts.GeoM.Translate(px, lineY)
		uOpts.ColorScale.ScaleWithColor(fg)
		uOpts.CompositeMode = ebiten.CompositeModeSourceOver
		r.framebuffer.DrawImage(r.whitePixel, uOpts)
	}
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
	r.glyphCache = make(map[glyphKey]*ebiten.Image)
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
