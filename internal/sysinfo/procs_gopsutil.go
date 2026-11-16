//go:build !linux && !windows

package sysinfo

import (
	"sort"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

// readProcs lists processes with gopsutil; name, command line and user are fetched only once per PID.
func (s *Sampler) readProcs(dt float64, memTotal uint64) ([]Proc, int) {
	ps, err := process.Processes()
	if err != nil && len(ps) == 0 {
		return nil, 0
	}
	if s.prevProc == nil {
		s.prevProc, s.cmdOf, s.nameOf, s.userOf, s.thrOf = map[int]float64{}, map[int]string{}, map[int]string{}, map[int]string{}, map[int]int{}
	}
	cur := make(map[int]float64, len(ps))
	out := make([]Proc, 0, len(ps))
	zombies := 0
	alive := make(map[int]bool, len(ps))
	thrBudget := 25
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
		th, ok := s.thrOf[pid]
		if !ok && thrBudget > 0 {
			// NumThreads is very slow on Windows (~6 ms per process): it is queried only for a few new PIDs per sample.
			thrBudget--
			n, _ := p.NumThreads()
			th = int(n)
			s.thrOf[pid] = th
		}

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
			PID: pid, User: s.userOf[pid], State: st, Threads: th,
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
			delete(s.thrOf, pid)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CPUPct > out[j].CPUPct })
	return out, zombies
}
