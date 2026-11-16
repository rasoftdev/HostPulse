package tui

import (
	"os"

	"golang.org/x/term"
)

// size returns the terminal columns and rows.
func size() (w, h int) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}

// rawMode disables echo and line mode (Linux, macOS and Windows).
// It returns the function that restores the terminal.
func rawMode() (restore func(), err error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, errNotTTY
	}
	st, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	undo := prepareOutput()
	return func() {
		undo()
		term.Restore(fd, st)
	}, nil
}
