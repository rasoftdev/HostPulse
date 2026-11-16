//go:build !windows

package tui

import "errors"

var errNotTTY = errors.New("hostpulse needs an interactive terminal")

func prepareOutput() func() { return func() {} }
