//go:build !linux

package sysinfo

import (
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	gnet "github.com/shirou/gopsutil/v4/net"
)

var (
	freqAt, connsAt time.Time
	freqCache       float64
	connsCache      Conns
)

// readFreq: gopsutil queries the system (sometimes via WMI/IOKit), so the result is cached for 15 s.
func readFreq() float64 {
	if time.Since(freqAt) < 15*time.Second {
		return freqCache
	}
	freqAt = time.Now()
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		freqCache = infos[0].Mhz
	}
	return freqCache
}

// readConns counts TCP connections; on macOS it may need sudo, in which case it is hidden.
func readConns() Conns {
	if time.Since(connsAt) < 10*time.Second {
		return connsCache
	}
	connsAt = time.Now()
	connsCache = Conns{}
	cs, err := gnet.ConnectionsWithoutUids("tcp")
	if err != nil && len(cs) == 0 {
		return connsCache
	}
	connsCache.Available = true
	for _, c := range cs {
		switch strings.ToUpper(c.Status) {
		case "ESTABLISHED":
			connsCache.Established++
		case "TIME_WAIT":
			connsCache.TimeWait++
		case "LISTEN":
			connsCache.Listen++
		}
	}
	return connsCache
}
