//go:build windows

package tui

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

var errNotTTY = errors.New("hostpulse necesita una terminal interactiva (cmd, PowerShell o Windows Terminal)")

// prepareOutput activa las secuencias ANSI y UTF-8 en la consola de Windows.
func prepareOutput() func() {
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	hasMode := windows.GetConsoleMode(h, &mode) == nil
	if hasMode {
		windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
	cp, _ := windows.GetConsoleOutputCP()
	windows.SetConsoleOutputCP(65001)
	return func() {
		if hasMode {
			windows.SetConsoleMode(h, mode)
		}
		if cp != 0 {
			windows.SetConsoleOutputCP(cp)
		}
	}
}
