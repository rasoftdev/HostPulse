//go:build !linux

package sysinfo

import (
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

var (
	freqAt, connsAt time.Time
	freqCache       float64
	connsCache      Conns
)

// readFreq: gopsutil consulta el sistema (a veces con WMI/IOKit), así que se cachea 15 s.
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

// readConns cuenta conexiones TCP; en macOS puede requerir sudo, y entonces se oculta.
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

// readProcs lista procesos con gopsutil; nombre, línea de comandos y usuario se piden una sola vez por PID.
func (s *Sampler) readProcs(dt float64, memTotal uint64) ([]Proc, int) {
	ps, err := process.Processes()
	if err != nil && len(ps) == 0 {
		return nil, 0
	}
	if s.prevProc == nil {
		s.prevProc, s.cmdOf, s.nameOf, s.userOf = map[int]float64{}, map[int]string{}, map[int]string{}, map[int]string{}
	}
	cur := make(map[int]float64, len(ps))
	out := make([]Proc, 0, len(ps))
	zombies := 0
	alive := make(map[int]bool, len(ps))
	for _, p := range ps {
		pid := int(p.Pid)
		alive[pid] = true
		name, ok := s.nameOf[pid]
		if !ok {
			name, _ = p.Name()
			if name == "" {
				name = "?"
			}
			s.nameOf[pid] = name
			cmd, _ := p.Cmdline()
			if cmd = strings.TrimSpace(cmd); cmd == "" {
				cmd = name
			}
			s.cmdOf[pid] = cmd
			user, _ := p.Username()
			if user == "" {
				user = "-"
			} else if i := strings.LastIndexAny(user, `\`); i >= 0 {
				user = user[i+1:] // DOMINIO\usuario → usuario
			}
			s.userOf[pid] = user
		}

		cpuPct := 0.0
		if t, err := p.Times(); err == nil {
			secs := t.User + t.System
			cur[pid] = secs
			if prev, ok := s.prevProc[pid]; ok && dt > 0 && secs >= prev {
				cpuPct = (secs - prev) / dt * 100
			}
		}
		var rss uint64
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			rss = mi.RSS
		}
		memPct := 0.0
		if memTotal > 0 {
			memPct = float64(rss) / float64(memTotal) * 100
		}
		th, _ := p.NumThreads()

		st := byte('S')
		if cpuPct >= 0.1 {
			st = 'R'
		}
		if sts, err := p.Status(); err == nil && len(sts) > 0 && sts[0] != "" {
			switch c := strings.ToUpper(sts[0])[0]; c {
			case 'Z', 'T', 'D':
				st = c
			}
		}
		if st == 'Z' {
			zombies++
		}
		out = append(out, Proc{
			PID: pid, User: s.userOf[pid], State: st, Threads: int(th),
			CPUPct: round1(cpuPct), MemPct: round1(memPct), RSS: rss,
			Name: name, Cmd: s.cmdOf[pid],
		})
	}
	s.prevProc = cur
	for pid := range s.nameOf {
		if !alive[pid] {
			delete(s.nameOf, pid)
			delete(s.cmdOf, pid)
			delete(s.userOf, pid)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CPUPct > out[j].CPUPct })
	return out, zombies
}
