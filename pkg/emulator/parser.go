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
	csiParams  string
	csiCommand rune
	oscParams  string
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
				} else {
					p.terminal.putChar(r)
				}
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

		// Handle other states (Escape, CSI, OSC) byte by byte for simplicity
		r := rune(data[i])
		i++

		switch p.state {
		case StateEscape:
			if r == '[' {
				p.state = StateCSI
				p.csiParams = ""
			} else if r == ']' {
				p.state = StateOSC
				p.oscParams = ""
			} else {
				p.state = StateNormal
			}
		case StateCSI:
			if r >= '0' && r <= '9' || r == ';' || r == '?' {
				p.csiParams += string(r)
			} else {
				p.handleCSI(r)
				p.state = StateNormal
			}
		case StateOSC:
			if r == '\a' || r == '\x1b' { // BEL or start of ESC (for ST)
				// OSC finished
				p.state = StateNormal
			} else {
				p.oscParams += string(r)
			}
		}
	}
}


func (p *Parser) handleCSI(cmd rune) {
	params := strings.Split(p.csiParams, ";")

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
	case 'J': // Erase in Display
		mode := 0
		if len(params) >= 1 && params[0] != "" {
			mode, _ = strconv.Atoi(params[0])
		}
		if mode == 2 {
			p.terminal.clear()
		}
	case 'K': // Erase in Line
		mode := 0
		if len(params) >= 1 && params[0] != "" {
			mode, _ = strconv.Atoi(params[0])
		}
		p.terminal.eraseInLine(mode)
	case 'm': // Select Graphic Rendition (SGR) - simplified

		// Initial color support could go here
	case 'h': // Set Mode (DECSET)
		// We can handle ?1049 (Alt Buffer) etc later
	case 'l': // Reset Mode (DECRST)
	case 'n': // Device Status Report
		if len(params) >= 1 && params[0] == "6" {
			// Report Cursor Position - could implement this later
		}
	case 'r': // Set Scrolling Region
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
	}
}


