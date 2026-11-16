//go:build windows

package sysinfo

import (
	"testing"
	"time"
)

func TestReadProcsWindows(t *testing.T) {
	s := &Sampler{}
	s.readProcs(1, 1<<34)
	start := time.Now()
	ps, _ := s.readProcs(1, 1<<34)
	t.Logf("%d procs in %v", len(ps), time.Since(start))
	if len(ps) < 10 {
		t.Fatal("too few processes")
	}
	t.Logf("%+v", ps[0])
}
