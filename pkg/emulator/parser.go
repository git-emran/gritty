package emulator

import (
	"unicode/utf8"
)

// ParserState represents the current state of the ANSI parser.
type ParserState int

const (
	StateNormal ParserState = iota
	StateEscape
	StateCSI
	StateOSC
)

// Parser handles parsing of ANSI escape sequences.
type Parser struct {
	state      ParserState
	terminal   *Terminal
	csiParams  [32]int
	csiCount   int
	csiPrivate bool
	utf8Buf    []byte
}

// NewParser creates a new ANSI parser associated with a terminal.
func NewParser(t *Terminal) *Parser {
	return &Parser{
		state:    StateNormal,
		terminal: t,
	}
}

// Process parses a stream of bytes and updates the terminal state.
func (p *Parser) Process(data []byte) {
	p.terminal.Lock()
	defer p.terminal.Unlock()

	// Incorporate any buffered bytes from previous call
	if len(p.utf8Buf) > 0 {
		data = append(p.utf8Buf, data...)
		p.utf8Buf = nil
	}

	for i := 0; i < len(data); {
		if p.state == StateNormal {
			// Fast path for ASCII in Normal state
			if data[i] < utf8.RuneSelf {
				r := rune(data[i])
				if r == '\x1b' {
					p.state = StateEscape
				} else if r == '\r' {
					p.terminal.Cursor.X = 0
				} else if r == '\b' {
					p.terminal.moveCursor(p.terminal.Cursor.X-1, p.terminal.Cursor.Y)
				} else if r == '\n' {
					p.terminal.normalizeScrollRegionLocked()
					if p.terminal.Cursor.Y == p.terminal.ScrollBottom {
						p.terminal.scrollUpRegionLocked(p.terminal.ScrollTop, p.terminal.ScrollBottom)
					} else {
						p.terminal.Cursor.Y++
						if p.terminal.Cursor.Y >= p.terminal.Height {
							p.terminal.Cursor.Y = p.terminal.Height - 1
						}
					}
				} else if r == '\t' {
					// Advance to next tab stop (every 8 columns)
					next := ((p.terminal.Cursor.X / 8) + 1) * 8
					if next >= p.terminal.Width {
						next = p.terminal.Width - 1
					}
					p.terminal.Cursor.X = next
				} else if r >= 0x20 { // Printable ASCII
					p.terminal.putChar(r)
				}
				// Skip control chars we don't handle (0x00-0x1f except above)
				i++
				continue
			}

			// Handle multibyte UTF-8
			r, size := utf8.DecodeRune(data[i:])
			if r == utf8.RuneError {
				if !utf8.FullRune(data[i:]) {
					// Incomplete rune, buffer it and return
					p.utf8Buf = make([]byte, len(data[i:]))
					copy(p.utf8Buf, data[i:])
					return
				}
				// Actual encoding error, treat as single byte or skip
				r = rune(data[i])
				size = 1
			}
			p.terminal.putChar(r)
			i += size
			continue
		}

		// Handle other states (Escape, CSI, OSC) byte by byte
		r := rune(data[i])
		i++

		switch p.state {
		case StateEscape:
			switch r {
			case '[':
				p.state = StateCSI
				p.csiCount = 0
				p.csiPrivate = false
				p.csiParams[0] = -1
			case ']':
				p.state = StateOSC
			case 'M': // Reverse Index — scroll down
				p.terminal.normalizeScrollRegionLocked()
				if p.terminal.Cursor.Y == p.terminal.ScrollTop {
					p.terminal.scrollDownRegionLocked(p.terminal.ScrollTop, p.terminal.ScrollBottom)
				} else if p.terminal.Cursor.Y > 0 {
					p.terminal.Cursor.Y--
				}
				p.state = StateNormal
			case '7': // DECSC — Save cursor
				p.terminal.savedCursor = cursorState{
					cursor:    p.terminal.Cursor,
					fg:        p.terminal.CurrentFg,
					bg:        p.terminal.CurrentBg,
					bold:      p.terminal.CurrentBold,
					italic:    p.terminal.CurrentItalic,
					underline: p.terminal.CurrentUnderline,
				}
				p.state = StateNormal
			case '8': // DECRC — Restore cursor
				p.terminal.restoreCursorLocked(p.terminal.savedCursor)
				p.state = StateNormal
			case '(', ')': // Character set designation — consume next byte
				if i < len(data) {
					i++ // skip the charset designator byte
				}
				p.state = StateNormal
			default:
				p.state = StateNormal
			}
		case StateCSI:
			if r >= '0' && r <= '9' {
				if p.csiCount < 32 {
					if p.csiParams[p.csiCount] == -1 {
						p.csiParams[p.csiCount] = int(r - '0')
					} else {
						p.csiParams[p.csiCount] = p.csiParams[p.csiCount]*10 + int(r - '0')
					}
				}
			} else if r == ';' {
				if p.csiCount < 31 {
					p.csiCount++
					p.csiParams[p.csiCount] = -1
				}
			} else if r == '?' || r == '>' {
				p.csiPrivate = true
			} else if r == '!' || r == ' ' {
				// Ignore spaces and exclamations
			} else {
				p.handleCSI(r)
				p.state = StateNormal
			}
		case StateOSC:
			if r == '\x07' {
				// OSC finished (BEL); ESC \ (7-bit ST) is handled below
				p.state = StateNormal
			} else if r == '\x1b' {
				// ESC ST — next byte should be '\' to complete ST; consume it
				if i < len(data) && data[i] == '\\' {
					i++
				}
				p.state = StateNormal
			}
		}
	}
}

func (p *Parser) handleCSI(cmd rune) {
	paramCount := p.csiCount + 1

	switch cmd {
	case 'H', 'f': // Cursor Position
		row, col := 1, 1
		if p.csiParams[0] != -1 {
			row = p.csiParams[0]
		}
		if p.csiCount >= 1 && p.csiParams[1] != -1 {
			col = p.csiParams[1]
		}
		p.terminal.moveCursor(col-1, row-1)

	case 'G': // Cursor Horizontal Absolute
		col := 1
		if p.csiParams[0] != -1 {
			col = p.csiParams[0]
		}
		p.terminal.moveCursor(col-1, p.terminal.Cursor.Y)

	case 'd': // Cursor Vertical Absolute
		row := 1
		if p.csiParams[0] != -1 {
			row = p.csiParams[0]
		}
		p.terminal.moveCursor(p.terminal.Cursor.X, row-1)

	case 'J': // Erase in Display
		mode := 0
		if p.csiParams[0] != -1 {
			mode = p.csiParams[0]
		}
		switch mode {
		case 0: // Erase from cursor to end of screen
			p.terminal.eraseInLine(0)
			for y := p.terminal.Cursor.Y + 1; y < p.terminal.Height; y++ {
				for x := 0; x < p.terminal.Width; x++ {
					p.terminal.Grid[y][x] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault, Modified: true}
				}
				p.terminal.DirtyRows[y] = true
			}
		case 1: // Erase from start to cursor
			p.terminal.eraseInLine(1)
			for y := 0; y < p.terminal.Cursor.Y; y++ {
				for x := 0; x < p.terminal.Width; x++ {
					p.terminal.Grid[y][x] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault, Modified: true}
				}
				p.terminal.DirtyRows[y] = true
			}
		case 2, 3:
			p.terminal.clear()
		}

	case 'K': // Erase in Line
		mode := 0
		if p.csiParams[0] != -1 {
			mode = p.csiParams[0]
		}
		p.terminal.eraseInLine(mode)

	case 'm': // Select Graphic Rendition (SGR)
		// Default: reset all styles if parameters are omitted
		if paramCount == 1 && p.csiParams[0] == -1 {
			p.terminal.CurrentFg = ColorDefault
			p.terminal.CurrentBg = ColorDefault
			p.terminal.CurrentBold = false
			p.terminal.CurrentItalic = false
			p.terminal.CurrentUnderline = false
			break
		}

		for i := 0; i < paramCount; i++ {
			code := p.csiParams[i]
			if code == -1 || code == 0 {
				p.terminal.CurrentFg = ColorDefault
				p.terminal.CurrentBg = ColorDefault
				p.terminal.CurrentBold = false
				p.terminal.CurrentItalic = false
				p.terminal.CurrentUnderline = false
				continue
			}

			switch {
			case code == 1:
				p.terminal.CurrentBold = true
			case code == 2: // Dim — ignore
			case code == 3: // Italic
				p.terminal.CurrentItalic = true
			case code == 4: // Underline
				p.terminal.CurrentUnderline = true
			case code == 5, code == 6: // Blink — ignore
			case code == 7: // Reverse video — swap fg/bg
				p.terminal.CurrentFg, p.terminal.CurrentBg = p.terminal.CurrentBg, p.terminal.CurrentFg
			case code == 22:
				p.terminal.CurrentBold = false
			case code == 23: // Italic off
				p.terminal.CurrentItalic = false
			case code == 24: // Underline off
				p.terminal.CurrentUnderline = false
			case code == 27: // Reverse off
			case code >= 30 && code <= 37:
				p.terminal.CurrentFg = Color(code - 30)
			case code == 39:
				p.terminal.CurrentFg = ColorDefault
			case code >= 40 && code <= 47:
				p.terminal.CurrentBg = Color(code - 40)
			case code == 49:
				p.terminal.CurrentBg = ColorDefault
			case code >= 90 && code <= 97:
				p.terminal.CurrentFg = Color(code - 90 + 8)
			case code >= 100 && code <= 107:
				p.terminal.CurrentBg = Color(code - 100 + 8)
			case code == 38 || code == 48:
				if i+1 < paramCount {
					mode := p.csiParams[i+1]
					if mode == 5 && i+2 < paramCount {
						// 256 color
						val := p.csiParams[i+2]
						if val >= 0 && val <= 255 {
							if code == 38 {
								p.terminal.CurrentFg = Color(val)
							} else {
								p.terminal.CurrentBg = Color(val)
							}
						}
						i += 2
					} else if mode == 2 && i+4 < paramCount {
						// TrueColor
						rv := p.csiParams[i+2]
						gv := p.csiParams[i+3]
						bv := p.csiParams[i+4]
						if rv >= 0 && gv >= 0 && bv >= 0 {
							trueColor := uint32(0x01000000 | ((rv & 0xFF) << 16) | ((gv & 0xFF) << 8) | (bv & 0xFF))
							if code == 38 {
								p.terminal.CurrentFg = Color(trueColor)
							} else {
								p.terminal.CurrentBg = Color(trueColor)
							}
						}
						i += 4
					}
				}
			}
		}

	case 'h': // Set Mode (DECSET)
		if p.csiPrivate {
			for i := 0; i < paramCount; i++ {
				p2 := p.csiParams[i]
				switch p2 {
				case 47, 1047, 1049: // Alt screen
					p.terminal.enterAltScreenLocked()
				case 25: // Show cursor — ignore
				}
			}
		}
	case 'l': // Reset Mode (DECRST)
		if p.csiPrivate {
			for i := 0; i < paramCount; i++ {
				p2 := p.csiParams[i]
				switch p2 {
				case 47, 1047, 1049: // Leave alt screen
					p.terminal.exitAltScreenLocked()
				case 25: // Hide cursor — ignore
				}
			}
		}

	case 'n': // Device Status Report
		// Could implement cursor position report later

	case 'r': // Set Scrolling Region (DECSTBM)
		top := 1
		bottom := p.terminal.Height
		if p.csiParams[0] != -1 {
			top = p.csiParams[0]
		}
		if p.csiCount >= 1 && p.csiParams[1] != -1 {
			bottom = p.csiParams[1]
		}
		p.terminal.setScrollRegionLocked(top-1, bottom-1)

	case 'L': // Insert Line
		num := 1
		if p.csiParams[0] != -1 {
			num = p.csiParams[0]
		}
		p.terminal.insertLine(num)

	case 'M': // Delete Line
		num := 1
		if p.csiParams[0] != -1 {
			num = p.csiParams[0]
		}
		p.terminal.deleteLine(num)

	case 'P': // Delete Character (DCH)
		num := 1
		if p.csiParams[0] != -1 {
			num = p.csiParams[0]
		}
		p.terminal.deleteChars(num)

	case 'X': // Erase Character (ECH)
		num := 1
		if p.csiParams[0] != -1 {
			num = p.csiParams[0]
		}
		p.terminal.eraseChars(num)

	case '@': // Insert Blank Characters
		num := 1
		if p.csiParams[0] != -1 {
			num = p.csiParams[0]
		}
		// Shift right, insert spaces
		y := p.terminal.Cursor.Y
		x := p.terminal.Cursor.X
		if x+num > p.terminal.Width {
			num = p.terminal.Width - x
		}
		copy(p.terminal.Grid[y][x+num:], p.terminal.Grid[y][x:p.terminal.Width-num])
		for i := x; i < x+num; i++ {
			p.terminal.Grid[y][i] = Cell{Char: ' ', FgColor: ColorDefault, BgColor: ColorDefault, Modified: true}
		}
		p.terminal.DirtyRows[y] = true

	case 'A', 'B', 'C', 'D': // Cursor Up, Down, Forward, Back
		dist := 1
		if p.csiParams[0] != -1 {
			dist = p.csiParams[0]
		}
		switch cmd {
		case 'A':
			p.terminal.moveCursor(p.terminal.Cursor.X, p.terminal.Cursor.Y-dist)
		case 'B':
			p.terminal.moveCursor(p.terminal.Cursor.X, p.terminal.Cursor.Y+dist)
		case 'C':
			p.terminal.moveCursor(p.terminal.Cursor.X+dist, p.terminal.Cursor.Y)
		case 'D':
			p.terminal.moveCursor(p.terminal.Cursor.X-dist, p.terminal.Cursor.Y)
		}

	case 'E': // Cursor Next Line
		dist := 1
		if p.csiParams[0] != -1 {
			dist = p.csiParams[0]
		}
		p.terminal.moveCursor(0, p.terminal.Cursor.Y+dist)

	case 'F': // Cursor Previous Line
		dist := 1
		if p.csiParams[0] != -1 {
			dist = p.csiParams[0]
		}
		p.terminal.moveCursor(0, p.terminal.Cursor.Y-dist)

	case 'S': // Scroll Up
		num := 1
		if p.csiParams[0] != -1 {
			num = p.csiParams[0]
		}
		p.terminal.normalizeScrollRegionLocked()
		for i := 0; i < num; i++ {
			p.terminal.scrollUpRegionLocked(p.terminal.ScrollTop, p.terminal.ScrollBottom)
		}

	case 'T': // Scroll Down — insert lines at top
		num := 1
		if p.csiParams[0] != -1 {
			num = p.csiParams[0]
		}
		p.terminal.normalizeScrollRegionLocked()
		for i := 0; i < num; i++ {
			p.terminal.scrollDownRegionLocked(p.terminal.ScrollTop, p.terminal.ScrollBottom)
		}
	}
}
