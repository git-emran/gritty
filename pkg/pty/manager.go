package pty

import (
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// PTY defines the interface for interacting with a pseudo-terminal.
type PTY interface {
	io.ReadWriteCloser
	Resize(rows, cols uint16) error
}

// Manager handles the lifecycle of the PTY and the command running within it.
type Manager struct {
	ptyFile *os.File
	cmd     *exec.Cmd
}

// NewManager creates a new PTY and starts the specified command (e.g., shell).
func NewManager(command string, args []string) (*Manager, error) {
	cmd := exec.Command(command, args...)

	f, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	return &Manager{
		ptyFile: f,
		cmd:     cmd,
	}, nil
}

// Read reads data from the PTY.
func (m *Manager) Read(p []byte) (n int, err error) {
	return m.ptyFile.Read(p)
}

// Write writes data to the PTY.
func (m *Manager) Write(p []byte) (n int, err error) {
	return m.ptyFile.Write(p)
}

// Close closes the PTY and kills the command.
func (m *Manager) Close() error {
	if m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}
	return m.ptyFile.Close()
}

// Resize resizes the PTY window dimensions.
func (m *Manager) Resize(rows, cols uint16) error {
	return pty.Setsize(m.ptyFile, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

// Wait waits for the command to finish.
func (m *Manager) Wait() error {
	return m.cmd.Wait()
}
