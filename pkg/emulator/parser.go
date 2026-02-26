package emulator

import (
	"strconv"
	"strings"
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
	csiBuilder strings.Builder // replaces string concat for csiParams
	oscBuilder strings.Builder // replaces string concat for oscParams
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
					p.terminal.Cursor.Y++
					if p.terminal.Cursor.Y >= p.terminal.Height {
						p.terminal.scrollUp()
						p.terminal.Cursor.Y = p.terminal.Height - 1
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
				p.csiBuilder.Reset()
			case ']':
				p.state = StateOSC
				p.oscBuilder.Reset()
			case 'M': // Reverse Index — scroll down
				if p.terminal.Cursor.Y > 0 {
					p.terminal.Cursor.Y--
				} else {
					// Insert line at top
					p.terminal.insertLine(1)
				}
				p.state = StateNormal
			case '7': // Save cursor
				p.state = StateNormal
			case '8': // Restore cursor
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
			if (r >= '0' && r <= '9') || r == ';' || r == '?' || r == '>' || r == '!' || r == ' ' {
				p.csiBuilder.WriteRune(r)
			} else {
				p.handleCSI(r, p.csiBuilder.String())
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
			} else {
				p.oscBuilder.WriteRune(r)
			}
		}
	}
}

func (p *Parser) handleCSI(cmd rune, rawParams string) {
	// Strip leading '?' or '>' private mode markers
	private := false
	if len(rawParams) > 0 && (rawParams[0] == '?' || rawParams[0] == '>') {
		private = true
		rawParams = rawParams[1:]
	}

	params := strings.Split(rawParams, ";")

	switch cmd {
	case 'H', 'f': // Cursor Position
		row, col := 1, 1
		if len(params) >= 1 && params[0] != "" {
			row, _ = strconv.Atoi(params[0])
		}
		if len(params) >= 2 && params[1] != "" {
			col, _ = strconv.Atoi(params[1])
		}
		p.terminal.moveCursor(col-1, row-1)

	case 'G': // Cursor Horizontal Absolute
		col := 1
		if len(params) >= 1 && params[0] != "" {
			col, _ = strconv.Atoi(params[0])
		}
		p.terminal.moveCursor(col-1, p.terminal.Cursor.Y)

	case 'd': // Cursor Vertical Absolute
		row := 1
		if len(params) >= 1 && params[0] != "" {
			row, _ = strconv.Atoi(params[0])
		}
		p.terminal.moveCursor(p.terminal.Cursor.X, row-1)

	case 'J': // Erase in Display
		mode := 0
		if len(params) >= 1 && params[0] != "" {
			mode, _ = strconv.Atoi(params[0])
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
		if len(params) >= 1 && params[0] != "" {
			mode, _ = strconv.Atoi(params[0])
		}
		p.terminal.eraseInLine(mode)

	case 'm': // Select Graphic Rendition (SGR)
		for i := 0; i < len(params); i++ {
			param := params[i]
			if param == "" || param == "0" {
				p.terminal.CurrentFg = ColorDefault
				p.terminal.CurrentBg = ColorDefault
				p.terminal.CurrentBold = false
				continue
			}

			code, _ := strconv.Atoi(param)
			switch {
			case code == 1:
				p.terminal.CurrentBold = true
			case code == 2: // Dim — ignore
			case code == 3: // Italic — ignore for now
			case code == 4: // Underline — ignore for now
			case code == 5, code == 6: // Blink — ignore
			case code == 7: // Reverse video — swap fg/bg
				p.terminal.CurrentFg, p.terminal.CurrentBg = p.terminal.CurrentBg, p.terminal.CurrentFg
			case code == 22:
				p.terminal.CurrentBold = false
			case code == 23: // Italic off
			case code == 24: // Underline off
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
				if i+1 < len(params) {
					mode := params[i+1]
					if mode == "5" && i+2 < len(params) {
						// 256 color
						val, _ := strconv.Atoi(params[i+2])
						if code == 38 {
							p.terminal.CurrentFg = Color(val)
						} else {
							p.terminal.CurrentBg = Color(val)
						}
						i += 2
					} else if mode == "2" && i+4 < len(params) {
						// TrueColor
						rv, _ := strconv.Atoi(params[i+2])
						gv, _ := strconv.Atoi(params[i+3])
						bv, _ := strconv.Atoi(params[i+4])
						trueColor := uint32(0x01000000 | (rv << 16) | (gv << 8) | bv)
						if code == 38 {
							p.terminal.CurrentFg = Color(trueColor)
						} else {
							p.terminal.CurrentBg = Color(trueColor)
						}
						i += 4
					}
				}
			}
		}

	case 'h': // Set Mode (DECSET)
		if private {
			// Handle common private modes
			for _, p2 := range params {
				switch p2 {
				case "1049": // Alt screen — clear for now
					p.terminal.clear()
					p.terminal.UseAltScreen = true
				case "25": // Show cursor — ignore
				}
			}
		}
	case 'l': // Reset Mode (DECRST)
		if private {
			for _, p2 := range params {
				switch p2 {
				case "1049": // Leave alt screen
					p.terminal.clear()
					p.terminal.UseAltScreen = false
				case "25": // Hide cursor — ignore
				}
			}
		}

	case 'n': // Device Status Report
		// Could implement cursor position report later

	case 'r': // Set Scrolling Region — store but don't enforce for now

	case 'L': // Insert Line
		num := 1
		if len(params) >= 1 && params[0] != "" {
			num, _ = strconv.Atoi(params[0])
		}
		p.terminal.insertLine(num)

	case 'M': // Delete Line
		num := 1
		if len(params) >= 1 && params[0] != "" {
			num, _ = strconv.Atoi(params[0])
		}
		p.terminal.deleteLine(num)

	case 'P': // Delete Character (DCH)
		num := 1
		if len(params) >= 1 && params[0] != "" {
			num, _ = strconv.Atoi(params[0])
		}
		p.terminal.deleteChars(num)

	case 'X': // Erase Character (ECH)
		num := 1
		if len(params) >= 1 && params[0] != "" {
			num, _ = strconv.Atoi(params[0])
		}
		p.terminal.eraseChars(num)

	case '@': // Insert Blank Characters
		num := 1
		if len(params) >= 1 && params[0] != "" {
			num, _ = strconv.Atoi(params[0])
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
		if len(params) >= 1 && params[0] != "" {
			dist, _ = strconv.Atoi(params[0])
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
		if len(params) >= 1 && params[0] != "" {
			dist, _ = strconv.Atoi(params[0])
		}
		p.terminal.moveCursor(0, p.terminal.Cursor.Y+dist)

	case 'F': // Cursor Previous Line
		dist := 1
		if len(params) >= 1 && params[0] != "" {
			dist, _ = strconv.Atoi(params[0])
		}
		p.terminal.moveCursor(0, p.terminal.Cursor.Y-dist)

	case 'S': // Scroll Up
		num := 1
		if len(params) >= 1 && params[0] != "" {
			num, _ = strconv.Atoi(params[0])
		}
		for i := 0; i < num; i++ {
			p.terminal.scrollUp()
		}

	case 'T': // Scroll Down — insert lines at top
		num := 1
		if len(params) >= 1 && params[0] != "" {
			num, _ = strconv.Atoi(params[0])
		}
		savedY := p.terminal.Cursor.Y
		p.terminal.Cursor.Y = 0
		p.terminal.insertLine(num)
		p.terminal.Cursor.Y = savedY
	}
}
