# Gritty

Gritty is a high-performance terminal emulator built in Go, designed for speed and visual excellence. It leverages the [Ebitengine](https://ebitengine.org/) game engine for GPU-accelerated rendering and features a custom-built terminal emulator and PTY manager.

![Gritty](https://github.com/user-attachments/assets/dae15339-65bb-49f6-8e0f-975287b9c9ac)

## Features

- **Blazing Fast Rendering**: Custom raster-based renderer with dirty-region tracking to minimize CPU usage.
- **GPU-Accelerated Glyph Cache**: Efficient rendering of text using a dynamic glyph cache.
- **Nerd Font Support**: Full support for Nerd Fonts and modern typography.
- **Adaptive Theming**: Automatically detects system dark mode and adjusts themes accordingly.
- **Smooth Scrolling**: Optimized scroll performance with alternate screen handling (e.g., for `vim`, `less`).
- **macOS Integration**: Easily packageable as a native `.app` bundle.

## Architecture

Gritty uses a layered terminal-emulator architecture. Input starts in the GUI layer, flows through the PTY bridge into the shell, and the shell output returns through the ANSI parser into the in-memory terminal model. The renderer then paints that model to the screen using GPU-backed buffers.

```mermaid
graph TD
    User([User Input]) --> Renderer[Renderer - pkg/renderer]
    Renderer --> |Input Events| PTY[PTY Manager - pkg/pty]
    PTY --> |Shell Stream| Parser[ANSI Parser - pkg/emulator]
    Parser --> |State Updates| Term[Terminal State - pkg/emulator]
    Term --> |Grid Snapshot| Renderer
    Renderer --> |Draw| Screen(OS Window)
```

### System Design

- `cmd/term` is the composition root. It creates the renderer, terminal model, parser, and PTY manager, then wires keyboard, mouse wheel, resize, and shell I/O together.
- `pkg/emulator` is the terminal state machine. It owns the screen grid, cursor, scrollback buffer, dirty-row tracking, alternate-screen mode, and ANSI/VT parsing.
- `pkg/renderer` is the presentation layer. It translates terminal cells into pixels, handles theme and color resolution, caches glyphs, and manages the Ebiten game loop.
- `pkg/pty` is the operating-system boundary. It starts the shell under a pseudo-terminal, resizes it when the viewport changes, and exposes read/write methods for streaming bytes.

### Folder Guide

- `cmd/term/main.go`: Desktop app entry point and dependency wiring.
- `pkg/emulator/parser.go`: ANSI/escape-sequence parser that mutates terminal state.
- `pkg/emulator/terminal.go`: Core terminal data model and viewport/scrollback behavior.
- `pkg/renderer/gui.go`: Ebiten-based GUI renderer, input polling, drawing, and color handling.
- `pkg/renderer/theme.go`: Theme definitions and system appearance detection.
- `pkg/renderer/renderer.go`: Secondary renderer abstraction for the tcell/TUI implementation.
- `pkg/pty/manager.go`: PTY lifecycle and shell process management.
- `verify_terminal.py`: Manual verification script for colors, glyphs, and rendering behavior.
- `assets/`: Static assets such as the application icon.
- `term`, `term_test`: Local build artifacts or helper binaries produced during development.

### Data Flow

1. The user types or scrolls in the window managed by `pkg/renderer`.
2. `cmd/term` maps that input either to PTY bytes or to scrollback navigation on the in-memory terminal model.
3. `pkg/pty` forwards bytes to the child shell process and reads shell output back.
4. `pkg/emulator/parser.go` decodes control sequences and updates `pkg/emulator/terminal.go`.
5. `pkg/renderer/gui.go` reads the terminal snapshot, redraws only changed regions, and presents the framebuffer.

### Why This Structure

- The emulator is isolated from the renderer, so terminal semantics can evolve without coupling them to drawing code.
- The PTY manager is isolated from the parser, so shell/process concerns stay separate from ANSI/state concerns.
- The renderer works from terminal snapshots and dirty flags, which keeps repaint cost predictable even when output is heavy.
- The entry point stays thin and orchestration-focused, which makes it easier to swap renderers or add startup configuration later.

## Getting Started

### Prerequisites

- **Go**: Version 1.25 or later.
- **Dependencies**:
  - `ebitengine/v2`
  - `creack/pty`
  - `golang.org/x/image`

### Running for Development

```bash
make run
```

### Building

To build a standalone binary:

```bash
make build
```

### Packaging for macOS

To create a native `Gritty.app` requires Ebitengine

```bash
make build-mac
```

## Verification & Testing

Gritty includes a Python-based verification script to test terminal capabilities like colors, Nerd Font icons, and emoji rendering.

```bash
python3 verify_terminal.py

```


## Troubleshooting

- **Fonts**: If icons don't appear correctly, ensure you have a [Nerd Font](https://www.nerdfonts.com/) installed and that it's being picked up by the system.
- **Performance**: High CPU usage may occur if the terminal is flooded with output. Dirty-region tracking helps, but extremely high-throughput streams are still being optimized.

## Contribution

Contributions should preserve the existing separation between orchestration, terminal semantics, rendering, and PTY integration. Before making changes, identify which layer owns the behavior you are touching and keep the fix inside that boundary unless there is a clear architectural reason not to.

### Where Changes Belong

- Put application wiring, startup defaults, and cross-package coordination in `cmd/term`.
- Put escape-sequence handling, cursor movement, scrollback, resize semantics, and screen-buffer logic in `pkg/emulator`.
- Put drawing, theme, font, viewport, and GPU/image concerns in `pkg/renderer`.
- Put process spawning, shell lifecycle, and PTY read/write behavior in `pkg/pty`.
- Put verification utilities and developer experiments outside `pkg/` unless they are reusable library code.

### Folder Responsibilities In Detail

- `cmd/term`
  Owns top-level dependency construction and event routing. This package should stay small. If `main.go` starts accumulating terminal behavior or rendering logic, that logic probably belongs in `pkg/emulator` or `pkg/renderer`.
- `pkg/emulator`
  Treat this as the source of truth for terminal behavior. Cursor rules, erase semantics, alternate-screen behavior, scrollback management, dirty tracking, and ANSI parsing should be implemented here, not inferred later in the renderer.
- `pkg/renderer`
  This package should convert terminal state into visual output, not invent terminal semantics. It may do presentation-only adjustments such as theme selection, glyph caching, contrast correction, cursor drawing, and viewport layout.
- `pkg/pty`
  Keep this package narrowly focused on the shell process and pseudo-terminal. Avoid leaking renderer or parser concepts into it.
- `assets`
  Store static resources only. Avoid mixing generated binaries or build outputs into this directory.

### Coding Conventions

- Follow standard Go formatting. Run `gofmt` on every touched Go file before submitting a change.
- Keep packages cohesive. Prefer adding methods to an existing type in the correct package over introducing cross-cutting helpers in the wrong layer.
- Prefer small, explicit functions over large multipurpose ones, especially in the parser and renderer hot paths.
- Preserve ASCII by default in source files unless a file already relies on non-ASCII content.
- Keep comments short and high-signal. Explain non-obvious behavior, invariants, or performance-sensitive choices; do not narrate trivial assignments.
- Use the existing naming style: exported types and methods in PascalCase, internal helpers in camelCase, short receiver names when idiomatic.
- Avoid hidden side effects. If a method mutates terminal state, it should be obvious from the method name and package.
- Be careful with locks in `pkg/emulator`. Hold the terminal mutex only for the minimum work required, and avoid introducing renderer or PTY calls while the lock is held.
- Preserve dirty-region behavior in the renderer. Full redraws are sometimes necessary, but they should be deliberate.
- When adding input behavior, handle alternate-screen applications carefully so programs like `vim`, `less`, or TUIs still receive the keys they expect.

### Performance Expectations

- This project is optimized around incremental rendering. Prefer dirty-row or dirty-cell updates over whole-frame redraws where possible.
- Avoid unnecessary allocations inside per-frame or per-cell rendering code.
- Keep scrollback and resize behavior efficient. These paths are hit frequently during real shell usage.
- If a change affects drawing or parsing throughput, explain the tradeoff in the pull request or commit message.

### Testing And Verification

- Run `go test ./...` for build and package verification.
- Use `python3 verify_terminal.py` when changing colors, fonts, glyph rendering, or ANSI behavior that is easier to validate visually.
- Manually test common shell flows after UI or input changes: typing, resizing, scrollback, `vim`, `less`, and colored output such as `ls --color`.
- If a change is platform-specific, call that out clearly in the README update, issue, or pull request description.

### Pull Request Guidance

- Keep pull requests focused. A renderer change and a parser refactor should usually be separate unless one depends directly on the other.
- Describe the user-visible behavior change first, then summarize the implementation.
- Include screenshots or short clips for visual changes when possible.
- Note any known limitations, untested paths, or follow-up work explicitly.

### Common Mistakes To Avoid

- Do not put terminal parsing logic in the renderer.
- Do not send shell bytes from low-level emulator code.
- Do not bypass the terminal model when adding scrollback or cursor behavior.
- Do not add unrelated formatting or broad refactors to a behavior-focused patch.

## License

Feel Free to use but Attribution required.

