package emulator

import "sync"

// Cell represents a single character on the terminal grid with its styling.
type Cell struct {
	Char     rune
	FgColor  int
	BgColor  int
	Modified bool
}

// Terminal represents the state of the terminal emulator.
type Terminal struct {
	Width  int
	Height int
	Grid   [][]Cell
	Cursor Cursor
	mu     sync.Mutex
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
			grid[i][j] = Cell{Char: ' '}
		}
	}

	return &Terminal{
		Width:  width,
		Height: height,
		Grid:   grid,
		Cursor: Cursor{X: 0, Y: 0},
	}
}

// PutChar places a character at the current cursor position and advances the cursor.
func (t *Terminal) PutChar(r rune) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.putChar(r)
}

func (t *Terminal) putChar(r rune) {
	if t.Cursor.Y >= t.Height {
		t.scrollUp()
		t.Cursor.Y = t.Height - 1
	}

	if t.Cursor.X >= t.Width {
		t.Cursor.X = 0
		t.Cursor.Y++
		if t.Cursor.Y >= t.Height {
			t.scrollUp()
			t.Cursor.Y = t.Height - 1
		}
	}

	t.Grid[t.Cursor.Y][t.Cursor.X] = Cell{
		Char:     r,
		Modified: true,
	}
	t.Cursor.X++
}

// scrollUp scrolls the terminal grid up by one line.
func (t *Terminal) scrollUp() {
	copy(t.Grid[0:], t.Grid[1:])
	t.Grid[t.Height-1] = make([]Cell, t.Width)
	for j := range t.Grid[t.Height-1] {
		t.Grid[t.Height-1][j] = Cell{Char: ' '}
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
			t.Grid[i][j] = Cell{Char: ' '}
		}
	}
	t.Cursor = Cursor{X: 0, Y: 0}
}

// EraseInLine erases parts of the current line.
func (t *Terminal) EraseInLine(mode int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.eraseInLine(mode)
}

func (t *Terminal) eraseInLine(mode int) {
	switch mode {
	case 0: // Erase from cursor to end of line
		for x := t.Cursor.X; x < t.Width; x++ {
			t.Grid[t.Cursor.Y][x] = Cell{Char: ' '}
		}
	case 1: // Erase from start of line to cursor
		for x := 0; x <= t.Cursor.X; x++ {
			t.Grid[t.Cursor.Y][x] = Cell{Char: ' '}
		}
	case 2: // Erase entire line
		for x := 0; x < t.Width; x++ {
			t.Grid[t.Cursor.Y][x] = Cell{Char: ' '}
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
	if t.Cursor.X + n > t.Width {
		n = t.Width - t.Cursor.X
	}
	// Shift left
	copy(t.Grid[t.Cursor.Y][t.Cursor.X:], t.Grid[t.Cursor.Y][t.Cursor.X+n:])
	// Fill end with spaces
	for x := t.Width - n; x < t.Width; x++ {
		t.Grid[t.Cursor.Y][x] = Cell{Char: ' '}
	}
}

// EraseChars erases n characters at the cursor position.
func (t *Terminal) EraseChars(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.eraseChars(n)
}

func (t *Terminal) eraseChars(n int) {
	if t.Cursor.X + n > t.Width {
		n = t.Width - t.Cursor.X
	}
	for x := t.Cursor.X; x < t.Cursor.X+n; x++ {
		t.Grid[t.Cursor.Y][x] = Cell{Char: ' '}
	}
}

// InsertLine inserts n blank lines at the cursor's current line.
func (t *Terminal) InsertLine(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.insertLine(n)
}

func (t *Terminal) insertLine(n int) {
	if t.Cursor.Y + n > t.Height {
		n = t.Height - t.Cursor.Y
	}
	// Shift down
	for y := t.Height - 1; y >= t.Cursor.Y+n; y-- {
		t.Grid[y] = t.Grid[y-n]
	}
	// Fill with blank lines
	for y := t.Cursor.Y; y < t.Cursor.Y+n; y++ {
		t.Grid[y] = make([]Cell, t.Width)
		for x := range t.Grid[y] {
			t.Grid[y][x] = Cell{Char: ' '}
		}
	}
}

// DeleteLine deletes n lines starting from the cursor's current line.
func (t *Terminal) DeleteLine(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.deleteLine(n)
}

func (t *Terminal) deleteLine(n int) {
	if t.Cursor.Y + n > t.Height {
		n = t.Height - t.Cursor.Y
	}
	// Shift up
	for y := t.Cursor.Y; y < t.Height-n; y++ {
		t.Grid[y] = t.Grid[y+n]
	}
	// Fill end with blank lines
	for y := t.Height - n; y < t.Height; y++ {
		t.Grid[y] = make([]Cell, t.Width)
		for x := range t.Grid[y] {
			t.Grid[y][x] = Cell{Char: ' '}
		}
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
	t.Cursor.X = x
	t.Cursor.Y = y
}
// Resize reallocates the grid if dimensions change.
func (t *Terminal) Resize(width, height int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if width == t.Width && height == t.Height {
		return
	}

	newGrid := make([][]Cell, height)
	for i := range newGrid {
		newGrid[i] = make([]Cell, width)
		for j := range newGrid[i] {
			newGrid[i][j] = Cell{Char: ' '}
		}
	}

	// Copy old content
	copyMaxY := height
	if t.Height < copyMaxY {
		copyMaxY = t.Height
	}
	copyMaxX := width
	if t.Width < copyMaxX {
		copyMaxX = t.Width
	}

	for y := 0; y < copyMaxY; y++ {
		for x := 0; x < copyMaxX; x++ {
			newGrid[y][x] = t.Grid[y][x]
		}
	}

	t.Grid = newGrid
	t.Width = width
	t.Height = height

	// Clamp cursor
	if t.Cursor.X >= width {
		t.Cursor.X = width - 1
	}
	if t.Cursor.Y >= height {
		t.Cursor.Y = height - 1
	}
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
