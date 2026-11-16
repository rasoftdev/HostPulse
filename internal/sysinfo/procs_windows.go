//go:build windows

package sysinfo

import (
	"sort"
	"strings"
	"unsafe"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
)

// detailBudget limits how many new PIDs get their command line / user resolved per sample (both are slow).
const detailBudget = 30

// queryProcs returns the raw SystemProcessInformation buffer (one syscall for every process).
func queryProcs(buf []byte) ([]byte, error) {
	for {
		var need uint32
		var p unsafe.Pointer
		if len(buf) > 0 {
			p = unsafe.Pointer(&buf[0])
		}
		err := windows.NtQuerySystemInformation(windows.SystemProcessInformation, p, uint32(len(buf)), &need)
		if err == nil {
			return buf, nil
		}
		if err != windows.STATUS_INFO_LENGTH_MISMATCH && err != windows.STATUS_BUFFER_TOO_SMALL {
			return nil, err
		}
		buf = make([]byte, int(need)+64<<10)
	}
}

// readProcs reads every process with a single NtQuerySystemInformation call (what Task Manager does) instead of
// opening one handle per process, which burned a lot of CPU. Command line and user are resolved once per PID.
func (s *Sampler) readProcs(dt float64, memTotal uint64) ([]Proc, int) {
	buf, err := queryProcs(s.procBuf)
	if err != nil {
		return nil, 0
	}
	s.procBuf = buf
	if s.prevProc == nil {
		s.prevProc, s.cmdOf, s.nameOf, s.userOf, s.thrOf = map[int]float64{}, map[int]string{}, map[int]string{}, map[int]string{}, map[int]int{}
	}
	cur := make(map[int]float64, len(s.prevProc)+16)
	alive := make(map[int]bool, len(s.prevProc)+16)
	out := make([]Proc, 0, len(s.prevProc)+16)
	budget := detailBudget

	for off := 0; off < len(buf); {
		e := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buf[off]))
		pid := int(e.UniqueProcessID)
		if pid != 0 { // skip "System Idle Process"
			alive[pid] = true
			name, ok := s.nameOf[pid]
			if !ok {
				name = e.ImageName.String()
				if name == "" {
					name = "?"
				}
				s.nameOf[pid] = name
				s.cmdOf[pid] = name
			}
			if _, ok := s.userOf[pid]; !ok {
				if budget > 0 && pid > 4 {
					budget--
					s.userOf[pid], s.cmdOf[pid] = procDetails(pid, name)
				} else {
					s.userOf[pid] = "-"
					if pid <= 4 {
						s.userOf[pid] = "SYSTEM"
					}
				}
			}
			secs := float64(e.UserTime+e.KernelTime) * 1e-7
			cur[pid] = secs
			cpuPct := 0.0
			if prev, ok := s.prevProc[pid]; ok && dt > 0 && secs >= prev {
				cpuPct = (secs - prev) / dt * 100
			}
			rss := uint64(e.WorkingSetSize)
			memPct := 0.0
			if memTotal > 0 {
				memPct = float64(rss) / float64(memTotal) * 100
			}
			st := byte('S')
			if cpuPct >= 0.1 {
				st = 'R'
			}
			out = append(out, Proc{
				PID: pid, User: s.userOf[pid], State: st, Threads: int(e.NumberOfThreads),
				CPUPct: round1(cpuPct), MemPct: round1(memPct), RSS: rss,
				Name: name, Cmd: s.cmdOf[pid],
			})
		}
		if e.NextEntryOffset == 0 {
			break
		}
		off += int(e.NextEntryOffset)
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
	return out, 0
}

// procDetails resolves user and command line for one PID (failures fall back to "-" and the image name).
func procDetails(pid int, name string) (user, cmd string) {
	user, cmd = "-", name
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return
	}
	if c, _ := p.Cmdline(); strings.TrimSpace(c) != "" {
		cmd = strings.TrimSpace(c)
	}
	if u, _ := p.Username(); u != "" {
		if i := strings.LastIndexAny(u, `\`); i >= 0 {
			u = u[i+1:]
		}
		user = u
	}
	return
}
