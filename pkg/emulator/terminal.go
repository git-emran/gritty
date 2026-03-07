package emulator

import (
	"sync"

	"github.com/mattn/go-runewidth"
)

// Color represents a 24-bit TrueColor or an ANSI color index.
type Color uint32

const (
	ColorDefault Color = 0xFF000000 // Special value for default color
)

// Cell represents a single character on the terminal grid with its styling.
type Cell struct {
	Char     rune
	FgColor  Color
	BgColor  Color
	Bold     bool
	Italic   bool
	Wide     bool // Whether this is the first half of a wide character
	WideCont bool // Whether this is the second half of a wide character
	Modified bool // Dirty flag — set when cell changes, cleared after render
}

// Terminal represents the state of the terminal emulator.
type Terminal struct {
	Width  int
	Height int
	Grid   [][]Cell
	Cursor Cursor

	Scrollback    [][]Cell
	MaxScrollback int
	ViewOffset    int // Lines scrolled up from bottom.

	// Current styling
	CurrentFg   Color
	CurrentBg   Color
	CurrentBold bool

	// Dirty tracking — DirtyRows[y] is true if any cell in row y changed.
	DirtyRows     []bool
	PendingScroll int
	ForceRedraw   bool
	UseAltScreen  bool

	mu sync.Mutex
}

// Cursor represents the current position and state of the cursor.
type Cursor struct {
	X int
	Y int
}

// NewTerminal creates a new Terminal with the specified dimensions.
func NewTerminal(width, height int) *Terminal {
	grid := make([][]Cell, height)
	for i := range grid {
		grid[i] = make([]Cell, width)
		for j := range grid[i] {
			grid[i][j] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault}
		}
	}

	dirtyRows := make([]bool, height)
	for i := range dirtyRows {
		dirtyRows[i] = true // All rows dirty initially so first render is full
	}

	return &Terminal{
		Width:         width,
		Height:        height,
		Grid:          grid,
		Cursor:        Cursor{X: 0, Y: 0},
		Scrollback:    make([][]Cell, 0, 5000),
		MaxScrollback: 5000,
		CurrentFg:     ColorDefault,
		CurrentBg:     ColorDefault,
		DirtyRows:     dirtyRows,
		ForceRedraw:   true,
	}
}

// ClearDirty resets all dirty flags after the renderer has consumed them.
func (t *Terminal) ClearDirty() {
	for i := range t.DirtyRows {
		t.DirtyRows[i] = false
	}
}

func (t *Terminal) markAllDirty() {
	for i := range t.DirtyRows {
		t.DirtyRows[i] = true
	}
	t.ForceRedraw = true
}

// MarkCursorDirty marks the cursor row dirty so cursor blink triggers a partial refresh.
func (t *Terminal) MarkCursorDirty() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.Cursor.Y >= 0 && t.Cursor.Y < t.Height {
		t.DirtyRows[t.Cursor.Y] = true
		if t.Cursor.X >= 0 && t.Cursor.X < t.Width {
			t.Grid[t.Cursor.Y][t.Cursor.X].Modified = true
		}
	}
}

// PutChar places a character at the current cursor position and advances the cursor.
func (t *Terminal) PutChar(r rune) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.putChar(r)
}

func (t *Terminal) putChar(r rune) {
	w := runewidth.RuneWidth(r)
	if w <= 0 {
		return
	}

	if t.Cursor.Y >= t.Height {
		t.scrollUp()
		t.Cursor.Y = t.Height - 1
	}

	// Wrap if at end of line or if wide char won't fit
	if t.Cursor.X+w > t.Width {
		t.Cursor.X = 0
		t.Cursor.Y++
		if t.Cursor.Y >= t.Height {
			t.scrollUp()
			t.Cursor.Y = t.Height - 1
		}
	}

	// Place the character
	t.Grid[t.Cursor.Y][t.Cursor.X] = Cell{
		Char:     r,
		FgColor:  t.CurrentFg,
		BgColor:  t.CurrentBg,
		Bold:     t.CurrentBold,
		Wide:     w > 1,
		Modified: true,
	}
	t.DirtyRows[t.Cursor.Y] = true

	if w > 1 {
		// Mark the next cell as a continuation
		if t.Cursor.X+1 < t.Width {
			t.Grid[t.Cursor.Y][t.Cursor.X+1] = Cell{
				Char:     ' ',
				FgColor:  t.CurrentFg,
				BgColor:  t.CurrentBg,
				WideCont: true,
				Modified: true,
			}
		}
	}

	t.Cursor.X += w
}

// scrollUp scrolls the terminal grid up by one line.
func (t *Terminal) scrollUp() {
	if t.Height > 0 && len(t.Grid) > 0 {
		line := make([]Cell, len(t.Grid[0]))
		copy(line, t.Grid[0])
		t.Scrollback = append(t.Scrollback, line)
		if len(t.Scrollback) > t.MaxScrollback {
			t.Scrollback = t.Scrollback[len(t.Scrollback)-t.MaxScrollback:]
		}
		if t.ViewOffset > 0 && t.ViewOffset < len(t.Scrollback) {
			t.ViewOffset++
		}
	}

	copy(t.Grid[0:], t.Grid[1:])
	copy(t.DirtyRows[0:], t.DirtyRows[1:])
	t.Grid[t.Height-1] = make([]Cell, t.Width)
	for j := range t.Grid[t.Height-1] {
		t.Grid[t.Height-1][j] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault, Modified: true}
	}
	t.DirtyRows[t.Height-1] = true
	if t.ViewOffset == 0 {
		t.PendingScroll++
	}
}

// Clear clears the terminal grid.
func (t *Terminal) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clear()
}

func (t *Terminal) clear() {
	for i := range t.Grid {
		for j := range t.Grid[i] {
			t.Grid[i][j] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault, Modified: true}
		}
		t.DirtyRows[i] = true
	}
	t.Cursor = Cursor{X: 0, Y: 0}
	t.ViewOffset = 0
	t.ForceRedraw = true
}

// EraseInLine erases parts of the current line.
func (t *Terminal) EraseInLine(mode int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.eraseInLine(mode)
}

func (t *Terminal) eraseInLine(mode int) {
	t.DirtyRows[t.Cursor.Y] = true
	switch mode {
	case 0: // Erase from cursor to end of line
		for x := t.Cursor.X; x < t.Width; x++ {
			t.Grid[t.Cursor.Y][x] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault, Modified: true}
		}
	case 1: // Erase from start of line to cursor
		for x := 0; x <= t.Cursor.X; x++ {
			t.Grid[t.Cursor.Y][x] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault, Modified: true}
		}
	case 2: // Erase entire line
		for x := 0; x < t.Width; x++ {
			t.Grid[t.Cursor.Y][x] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault, Modified: true}
		}
	}
}

// DeleteChars deletes n characters at the cursor position.
func (t *Terminal) DeleteChars(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.deleteChars(n)
}

func (t *Terminal) deleteChars(n int) {
	if t.Cursor.X+n > t.Width {
		n = t.Width - t.Cursor.X
	}
	copy(t.Grid[t.Cursor.Y][t.Cursor.X:], t.Grid[t.Cursor.Y][t.Cursor.X+n:])
	for x := t.Width - n; x < t.Width; x++ {
		t.Grid[t.Cursor.Y][x] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault, Modified: true}
	}
	t.DirtyRows[t.Cursor.Y] = true
}

// EraseChars erases n characters at the cursor position.
func (t *Terminal) EraseChars(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.eraseChars(n)
}

func (t *Terminal) eraseChars(n int) {
	if t.Cursor.X+n > t.Width {
		n = t.Width - t.Cursor.X
	}
	for x := t.Cursor.X; x < t.Cursor.X+n; x++ {
		t.Grid[t.Cursor.Y][x] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault, Modified: true}
	}
	t.DirtyRows[t.Cursor.Y] = true
}

// InsertLine inserts n blank lines at the cursor's current line.
func (t *Terminal) InsertLine(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.insertLine(n)
}

func (t *Terminal) insertLine(n int) {
	if t.Cursor.Y+n > t.Height {
		n = t.Height - t.Cursor.Y
	}
	for y := t.Height - 1; y >= t.Cursor.Y+n; y-- {
		t.Grid[y] = t.Grid[y-n]
	}
	for y := t.Cursor.Y; y < t.Cursor.Y+n; y++ {
		t.Grid[y] = make([]Cell, t.Width)
		for x := range t.Grid[y] {
			t.Grid[y][x] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault, Modified: true}
		}
	}
	// Mark all rows from cursor down as dirty
	for y := t.Cursor.Y; y < t.Height; y++ {
		t.DirtyRows[y] = true
	}
}

// DeleteLine deletes n lines starting from the cursor's current line.
func (t *Terminal) DeleteLine(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.deleteLine(n)
}

func (t *Terminal) deleteLine(n int) {
	if t.Cursor.Y+n > t.Height {
		n = t.Height - t.Cursor.Y
	}
	for y := t.Cursor.Y; y < t.Height-n; y++ {
		t.Grid[y] = t.Grid[y+n]
	}
	for y := t.Height - n; y < t.Height; y++ {
		t.Grid[y] = make([]Cell, t.Width)
		for x := range t.Grid[y] {
			t.Grid[y][x] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault, Modified: true}
		}
	}
	for y := t.Cursor.Y; y < t.Height; y++ {
		t.DirtyRows[y] = true
	}
}

// MoveCursor moves the cursor to the specified position.
func (t *Terminal) MoveCursor(x, y int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.moveCursor(x, y)
}

func (t *Terminal) moveCursor(x, y int) {
	if x < 0 {
		x = 0
	}
	if x >= t.Width {
		x = t.Width - 1
	}
	if y < 0 {
		y = 0
	}
	if y >= t.Height {
		y = t.Height - 1
	}
	// Mark old and new cursor rows dirty so cursor is redrawn
	if t.Cursor.Y >= 0 && t.Cursor.Y < t.Height {
		t.DirtyRows[t.Cursor.Y] = true
		if t.Cursor.X >= 0 && t.Cursor.X < t.Width {
			t.Grid[t.Cursor.Y][t.Cursor.X].Modified = true
		}
	}
	t.Cursor.X = x
	t.Cursor.Y = y
	if y >= 0 && y < t.Height {
		t.DirtyRows[y] = true
		if x >= 0 && x < t.Width {
			t.Grid[y][x].Modified = true
		}
	}
}

// Resize reallocates the grid if dimensions change.
func (t *Terminal) Resize(width, height int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if width == t.Width && height == t.Height {
		return
	}

	if height < t.Height {
		linesToPush := t.Height - height
		for i := 0; i < linesToPush; i++ {
			t.Scrollback = append(t.Scrollback, t.Grid[i])
		}
		if len(t.Scrollback) > t.MaxScrollback {
			t.Scrollback = t.Scrollback[len(t.Scrollback)-t.MaxScrollback:]
		}
		for y := 0; y < height; y++ {
			t.Grid[y] = t.Grid[y+linesToPush]
		}
		t.Height = height
		t.Cursor.Y -= linesToPush
		if t.Cursor.Y < 0 {
			t.Cursor.Y = 0
		}
	} else if height > t.Height {
		added := height - t.Height
		pull := added
		if pull > len(t.Scrollback) {
			pull = len(t.Scrollback)
		}
		for i := 0; i < added; i++ {
			t.Grid = append(t.Grid, nil)
		}
		for y := t.Height - 1; y >= 0; y-- {
			t.Grid[y+pull] = t.Grid[y]
		}
		for i := 0; i < pull; i++ {
			sbIdx := len(t.Scrollback) - pull + i
			t.Grid[i] = t.Scrollback[sbIdx]
		}
		t.Scrollback = t.Scrollback[:len(t.Scrollback)-pull]
		t.Height = height
		t.Cursor.Y += pull
	}

	for y := 0; y < t.Height; y++ {
		row := t.Grid[y]
		newRow := make([]Cell, width)
		for x := 0; x < width; x++ {
			if row != nil && x < len(row) {
				newRow[x] = row[x]
			} else {
				newRow[x] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault}
			}
		}
		t.Grid[y] = newRow
	}

	t.Width = width

	t.DirtyRows = make([]bool, t.Height)
	for i := range t.DirtyRows {
		t.DirtyRows[i] = true
	}

	if t.Cursor.X >= width {
		t.Cursor.X = width - 1
	}
	if t.Cursor.Y >= height {
		t.Cursor.Y = height - 1
	}
	if t.ViewOffset > len(t.Scrollback) {
		t.ViewOffset = len(t.Scrollback)
	}
	t.PendingScroll = 0
	t.ForceRedraw = true
}

func (t *Terminal) maxViewOffsetLocked() int {
	if len(t.Scrollback) == 0 {
		return 0
	}
	return len(t.Scrollback)
}

func (t *Terminal) ScrollUpLines(lines int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if lines <= 0 {
		return
	}
	t.ViewOffset += lines
	max := t.maxViewOffsetLocked()
	if t.ViewOffset > max {
		t.ViewOffset = max
	}
	t.ForceRedraw = true
}

func (t *Terminal) ScrollDownLines(lines int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if lines <= 0 {
		return
	}
	t.ViewOffset -= lines
	if t.ViewOffset < 0 {
		t.ViewOffset = 0
	}
	t.ForceRedraw = true
}

func (t *Terminal) ScrollPageUp() {
	step := t.Height - 1
	if step < 1 {
		step = 1
	}
	t.ScrollUpLines(step)
}

func (t *Terminal) ScrollPageDown() {
	step := t.Height - 1
	if step < 1 {
		step = 1
	}
	t.ScrollDownLines(step)
}

func (t *Terminal) ScrollToBottom() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ViewOffset == 0 {
		return
	}
	t.ViewOffset = 0
	t.ForceRedraw = true
}

func (t *Terminal) ScrollToTop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	max := t.maxViewOffsetLocked()
	if t.ViewOffset == max {
		return
	}
	t.ViewOffset = max
	t.ForceRedraw = true
}

func (t *Terminal) IsViewingScrollback() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ViewOffset > 0
}

func (t *Terminal) IsAltScreenActive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.UseAltScreen
}

// RequestFullRedraw marks the full viewport dirty for renderer updates.
func (t *Terminal) RequestFullRedraw() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ForceRedraw = true
}

// ViewRow returns the visible row at y in the current viewport.
// Caller must hold t lock.
func (t *Terminal) ViewRow(y int) []Cell {
	total := len(t.Scrollback) + t.Height
	if y < 0 || y >= t.Height || total == 0 {
		return nil
	}
	start := total - t.Height - t.ViewOffset
	if start < 0 {
		start = 0
	}
	idx := start + y
	if idx < 0 || idx >= total {
		return nil
	}
	if idx < len(t.Scrollback) {
		return t.Scrollback[idx]
	}
	gridY := idx - len(t.Scrollback)
	if gridY < 0 || gridY >= len(t.Grid) {
		return nil
	}
	return t.Grid[gridY]
}

// CursorViewPosition returns cursor coordinates in the current viewport.
// Caller must hold t lock.
func (t *Terminal) CursorViewPosition() (x int, y int, ok bool) {
	total := len(t.Scrollback) + t.Height
	if total == 0 {
		return 0, 0, false
	}
	start := total - t.Height - t.ViewOffset
	if start < 0 {
		start = 0
	}
	abs := len(t.Scrollback) + t.Cursor.Y
	viewY := abs - start
	if viewY < 0 || viewY >= t.Height {
		return 0, 0, false
	}
	return t.Cursor.X, viewY, true
}

// Lock locks the terminal for reading/writing.
func (t *Terminal) Lock() {
	t.mu.Lock()
}

// Unlock unlocks the terminal.
func (t *Terminal) Unlock() {
	t.mu.Unlock()
}

func (t *Terminal) String() string {
	res := ""
	for _, row := range t.Grid {
		for _, cell := range row {
			res += string(cell.Char)
		}
		res += "\n"
	}
	return res
}
