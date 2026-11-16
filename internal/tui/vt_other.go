//go:build !windows

package tui

import "errors"

var errNotTTY = errors.New("hostpulse necesita una terminal interactiva")

func prepareOutput() func() { return func() {} }
