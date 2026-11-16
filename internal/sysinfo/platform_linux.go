//go:build linux

package sysinfo

import (
	"os"
	"sort"
	"strconv"
	"strings"
)

const clockTicks = 100 // USER_HZ en Linux

// readFreq devuelve la frecuencia media actual de los núcleos (MHz).
func readFreq() float64 {
	var sum float64
	var n int
	for _, l := range strings.Split(readFile("/proc/cpuinfo"), "\n") {
		if strings.HasPrefix(l, "cpu MHz") {
			if i := strings.Index(l, ":"); i > 0 {
				if v, err := strconv.ParseFloat(strings.TrimSpace(l[i+1:]), 64); err == nil {
					sum += v
					n++
				}
			}
		}
	}
	if n == 0 {
		return 0
	}
	return round1(sum / float64(n))
}

// readConns cuenta conexiones TCP leyendo /proc/net (mucho más barato que recorrer los descriptores de cada proceso).
func readConns() (c Conns) {
	c.Available = true
	for _, p := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		lines := strings.Split(readFile(p), "\n")
		for k := 1; k < len(lines); k++ {
			f := strings.Fields(lines[k])
			if len(f) < 4 {
				continue
			}
			switch f[3] {
			case "01":
				c.Established++
			case "06":
				c.TimeWait++
			case "0A":
				c.Listen++
			}
		}
	}
	return
}

func (s *Sampler) loadPasswd() {
	s.passwd = map[string]string{}
	for _, l := range strings.Split(readFile("/etc/passwd"), "\n") {
		f := strings.Split(l, ":")
		if len(f) > 2 {
			s.passwd[f[2]] = f[0]
		}
	}
}

func (s *Sampler) userFor(pid int) string {
	if u, ok := s.userOf[pid]; ok {
		return u
	}
	uid := "?"
	for _, l := range strings.Split(readFile("/proc/"+strconv.Itoa(pid)+"/status"), "\n") {
		if strings.HasPrefix(l, "Uid:") {
			if f := strings.Fields(l); len(f) > 1 {
				uid = f[1]
			}
			break
		}
	}
	if s.passwd == nil {
		s.loadPasswd()
	}
	name, ok := s.passwd[uid]
	if !ok {
		name = uid
	}
	s.userOf[pid] = name
	return name
}

func (s *Sampler) cmdFor(pid int, comm string) string {
	if c, ok := s.cmdOf[pid]; ok {
		return c
	}
	c := strings.TrimSpace(strings.ReplaceAll(readFile("/proc/"+strconv.Itoa(pid)+"/cmdline"), "\x00", " "))
	if c == "" {
		c = "[" + comm + "]" // hilo del kernel
	}
	s.cmdOf[pid] = c
	return c
}

// readProcs lee /proc/<pid>/stat de todos los procesos y calcula la CPU instantánea entre muestras.
func (s *Sampler) readProcs(dt float64, memTotal uint64) ([]Proc, int) {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil, 0
	}
	if s.prevProc == nil {
		s.prevProc, s.cmdOf, s.userOf = map[int]float64{}, map[int]string{}, map[int]string{}
	}
	cur := make(map[int]float64, len(ents))
	out := make([]Proc, 0, len(ents))
	zombies := 0
	page := uint64(os.Getpagesize())
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue // el proceso terminó mientras leíamos
		}
		str := string(b)
		lp, rp := strings.IndexByte(str, '('), strings.LastIndexByte(str, ')')
		if lp < 0 || rp < lp || rp+2 > len(str) {
			continue
		}
		comm := str[lp+1 : rp]
		f := strings.Fields(str[rp+2:])
		if len(f) < 22 {
			continue
		}
		secs := float64(u64(f[11])+u64(f[12])) / clockTicks
		cur[pid] = secs
		cpu := 0.0
		if prev, ok := s.prevProc[pid]; ok && dt > 0 && secs >= prev {
			cpu = (secs - prev) / dt * 100
		}
		rss := u64(f[21]) * page
		mem := 0.0
		if memTotal > 0 {
			mem = float64(rss) / float64(memTotal) * 100
		}
		st := f[0][0]
		if st == 'Z' {
			zombies++
		}
		th, _ := strconv.Atoi(f[17])
		out = append(out, Proc{
			PID: pid, User: s.userFor(pid), State: st, Threads: th,
			CPUPct: round1(cpu), MemPct: round1(mem), RSS: rss,
			Name: comm, Cmd: s.cmdFor(pid, comm),
		})
	}
	s.prevProc = cur
	for pid := range s.cmdOf {
		if _, ok := cur[pid]; !ok {
			delete(s.cmdOf, pid)
			delete(s.userOf, pid)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CPUPct > out[j].CPUPct })
	return out, zombies
}
