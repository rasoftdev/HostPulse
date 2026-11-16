package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"hostpulse/internal/sysinfo"
)

const procMinRows = 8

func (m *model) view(w, h int) []string {
	if w < 60 || h < 14 {
		msg := fmt.Sprintf("Terminal too small (%dx%d). Minimum 60x14.", w, h)
		lines := make([]string, h)
		lines[0] = msg
		return lines
	}
	s := m.snap
	body := h - 2 // header + footer
	var rows []string

	// Top panels: CPU on the left, memory/network/disk on the right (stacked when the terminal is narrow).
	wide := w >= 110
	cw, rw := w, w
	if wide {
		cw = w / 2
		rw = w - cw
	}
	cpuNeed := len(m.cpuLines(s, cw-4, 1000)) + 2
	rightNeed := len(m.memLines(s, rw-4)) + 2
	var top []string
	if wide {
		th := imin(imax(cpuNeed, rightNeed), body-procMinRows)
		th = imax(th, 8)
		l := box(m.cpuTitle(s), m.cpuLines(s, cw-4, th-2), cw, th)
		r := box("Memory · Network · I/O", m.memLines(s, rw-4), rw, th)
		for i := range l {
			top = append(top, l[i]+r[i])
		}
	} else {
		ch := imin(cpuNeed, body-procMinRows)
		if ch >= 8 {
			top = append(top, box(m.cpuTitle(s), m.cpuLines(s, cw-4, ch-2), cw, ch)...)
		}
		rh := imin(rightNeed, body-len(top)-procMinRows)
		if rh >= 6 {
			top = append(top, box("Memory · Network · I/O", m.memLines(s, rw-4), rw, rh)...)
		}
	}
	rows = append(rows, top...)

	if d := m.diskLines(s, w-4, 4); len(d) > 0 && body-len(rows)-(len(d)+2) >= procMinRows {
		rows = append(rows, box("Disks", d, w, len(d)+2)...)
	}

	ph := body - len(rows)
	rows = append(rows, m.procBox(s, w, ph)...)

	out := make([]string, 0, h)
	out = append(out, m.header(s, w))
	out = append(out, rows...)
	for len(out) < h-1 {
		out = append(out, "")
	}
	out = append(out[:h-1], m.footer(w))
	return out
}

func (m *model) header(s sysinfo.Snapshot, w int) string {
	hs := s.Host
	left := bold(paint(cAccent, "◆ HostPulse")) + paint(cDim, "  ") + paint(cText, hs.Hostname)
	if hs.PrimaryIP != "" {
		left += paint(cDim, " · "+hs.PrimaryIP)
	}
	left += paint(cDim, " · "+hs.OS+" · kernel "+hs.Kernel)
	right := paint(cDim, "up ") + paint(cText, fmtUptime(hs.UptimeSec)) + paint(cDim, " · ") + paint(cText, time.Now().Format("15:04:05"))
	if m.paused {
		right = bold(paint(cYellow, "⏸ PAUSED")) + paint(cDim, " · ") + right
	}
	gap := w - vlen(left) - vlen(right)
	if gap < 1 {
		return clip(left, w-vlen(right)-1) + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func key(k, desc string) string { return paint(cAccent, k) + paint(cDim, " "+desc+"  ") }

func (m *model) footer(w int) string {
	if m.filtering {
		return paint(cAccent, "Filter ") + paint(cText, m.filter) + paint(cAccent, "▏") + paint(cDim, "   Enter accept · Esc clear")
	}
	f := key("q", "quit") + key("↑↓", "move") + key("c", "CPU") + key("m", "memory") + key("p", "PID") + key("n", "name") +
		key("r", "reverse") + key("/", "filter") + key("space", "pause")
	return clip(f, w)
}

func (m *model) cpuTitle(s sysinfo.Snapshot) string {
	t := paint(cLabel, "CPU")
	if s.CPU.Model != "" {
		t += paint(cDim, " · "+s.CPU.Model)
	}
	return t
}

func (m *model) cpuLines(s sysinfo.Snapshot, inner, maxLines int) []string {
	c := s.CPU
	var ls []string
	bw := inner - 4 - 7
	ls = append(ls, paint(cLabel, "CPU ")+bar(c.UsagePct, bw)+" "+paint(grad(c.UsagePct), fmt.Sprintf("%5.1f%%", c.UsagePct)))
	ls = append(ls, spark(m.cpuH, inner, true))
	ls = append(ls, paint(cDim, "usr ")+fmt.Sprintf("%4.1f", c.User)+paint(cDim, "  sys ")+fmt.Sprintf("%4.1f", c.System)+
		paint(cDim, "  io ")+fmt.Sprintf("%4.1f", c.IOWait)+paint(cDim, "  steal ")+fmt.Sprintf("%4.1f", c.Steal))

	n := len(c.PerCore)
	avail := imax(0, maxLines-5)
	if n > 0 && avail > 0 {
		cols, nrows := 1, n
		for cols = 1; cols <= 4; cols++ {
			nrows = (n + cols - 1) / cols
			if nrows <= avail {
				break
			}
		}
		if cols > 4 {
			cols = 4
		}
		nrows = imin(nrows, avail)
		colw := inner / cols
		cbw := imax(3, colw-2-8)
		perCol := (n + cols - 1) / cols
		for r := 0; r < nrows; r++ {
			var sb strings.Builder
			for cc := 0; cc < cols; cc++ {
				i := cc*perCol + r
				if i >= n {
					break
				}
				p := c.PerCore[i]
				cell := paint(cDim, fmt.Sprintf("%-3d", i)) + bar(p, cbw) + paint(grad(p), fmt.Sprintf("%4.0f%%", p))
				sb.WriteString(pad(cell, colw))
			}
			ls = append(ls, sb.String())
		}
	}

	info := paint(cDim, "load ") + fmt.Sprintf("%.2f %.2f %.2f", c.LoadAvg[0], c.LoadAvg[1], c.LoadAvg[2]) +
		paint(cDim, fmt.Sprintf(" · %d cores", c.Cores))
	if c.FreqMHz > 0 {
		info += paint(cDim, fmt.Sprintf(" · %.1f GHz", c.FreqMHz/1000))
	}
	if c.TempC > 0 {
		info += paint(cDim, " · ") + paint(grad((c.TempC-40)/50*100), fmt.Sprintf("%.0f°C", c.TempC))
	}
	ls = append(ls, info)
	ls = append(ls, paint(cDim, "tasks ")+fmt.Sprint(c.Processes)+paint(cDim, " · threads ")+fmt.Sprint(c.Threads)+
		paint(cDim, " · running ")+fmt.Sprint(c.Running)+paint(cDim, " · zombies ")+fmt.Sprint(c.Zombies))
	return ls
}

func (m *model) memLines(s sysinfo.Snapshot, inner int) []string {
	me, n := s.Memory, s.Network
	var ls []string
	bw := inner - 5 - 7
	ls = append(ls, paint(cLabel, "RAM  ")+bar(me.UsedPct, bw)+" "+paint(grad(me.UsedPct), fmt.Sprintf("%5.1f%%", me.UsedPct)))
	ls = append(ls, paint(cDim, "used ")+fmtBytes(me.Used)+paint(cDim, " / ")+fmtBytes(me.Total)+
		paint(cDim, " · cached ")+fmtBytes(me.Cached)+paint(cDim, " · free ")+fmtBytes(me.Available))
	if me.SwapTotal > 0 {
		sp := float64(me.SwapUsed) / float64(me.SwapTotal) * 100
		ls = append(ls, paint(cLabel, "Swap ")+bar(sp, bw)+" "+paint(grad(sp), fmt.Sprintf("%5.1f%%", sp)))
	} else {
		ls = append(ls, paint(cLabel, "Swap ")+paint(cDim, "no swap"))
	}
	ls = append(ls, "")
	sw := imax(4, inner-22)
	ls = append(ls, paint(cGreen, "↓ ")+pad(paint(cText, fmtRate(n.RxBps)), 11)+paint(cDim, fmt.Sprintf("%7.0f p/s ", n.RxPps))+spark(m.rxH, sw, false))
	ls = append(ls, paint(cOrange, "↑ ")+pad(paint(cText, fmtRate(n.TxBps)), 11)+paint(cDim, fmt.Sprintf("%7.0f p/s ", n.TxPps))+spark(m.txH, sw, false))
	if n.Conns.Available {
		ls = append(ls, paint(cDim, "TCP ")+fmt.Sprintf("%d", n.Conns.Established)+paint(cDim, " est · ")+fmt.Sprintf("%d", n.Conns.TimeWait)+
			paint(cDim, " tw · ")+fmt.Sprintf("%d", n.Conns.Listen)+paint(cDim, " listen · errors ")+fmt.Sprint(n.Errors))
	} else {
		ls = append(ls, paint(cDim, "network errors ")+fmt.Sprint(n.Errors))
	}
	ls = append(ls, "")
	d := s.DiskIO
	ls = append(ls, paint(cLabel, "Disk ")+paint(cDim, "read ")+fmtRate(d.ReadBps)+paint(cDim, " · write ")+fmtRate(d.WriteBps)+
		paint(cDim, fmt.Sprintf(" · %.0f iops", d.IOPS)))
	return ls
}

func (m *model) diskLines(s sysinfo.Snapshot, inner, max int) []string {
	var ls []string
	for i, d := range s.Disks {
		if i >= max {
			break
		}
		bw := imax(6, inner-64)
		mount := d.Mount
		if len(mount) > 14 {
			mount = "…" + mount[len(mount)-13:]
		}
		line := paint(cText, pad(mount, 14)) + " " + paint(cDim, pad(d.FSType, 7)) + " " + bar(d.UsedPct, bw) + " " +
			paint(grad(d.UsedPct), fmt.Sprintf("%5.1f%%", d.UsedPct)) + "  " + fmtBytes(d.Used) + paint(cDim, " / ") + fmtBytes(d.Total)
		if d.InodePct > 0 {
			line += paint(cDim, fmt.Sprintf("  inodes %.0f%%", d.InodePct))
		}
		ls = append(ls, line)
	}
	return ls
}

func (m *model) procList(s sysinfo.Snapshot) []sysinfo.Proc {
	var list []sysinfo.Proc
	f := strings.ToLower(m.filter)
	for _, p := range s.Procs {
		if f != "" && !strings.Contains(strings.ToLower(p.Cmd+" "+p.User+" "+fmt.Sprint(p.PID)), f) {
			continue
		}
		list = append(list, p)
	}
	less := func(a, b sysinfo.Proc) bool {
		switch m.sortBy {
		case byMem:
			return a.RSS > b.RSS
		case byPID:
			return a.PID < b.PID
		case byName:
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		if a.CPUPct != b.CPUPct {
			return a.CPUPct > b.CPUPct
		}
		return a.RSS > b.RSS
	}
	sort.SliceStable(list, func(i, j int) bool {
		if m.rev {
			return less(list[j], list[i])
		}
		return less(list[i], list[j])
	})
	return list
}

func stateColor(st byte) rgb {
	switch st {
	case 'R':
		return cGreen
	case 'Z':
		return cRed
	case 'D':
		return cOrange
	}
	return cDim
}

func (m *model) procBox(s sysinfo.Snapshot, w, h int) []string {
	if h < 4 {
		return nil
	}
	list := m.procList(s)
	inner := w - 4
	m.rows = imax(1, h-3)

	if m.selPID != 0 {
		for i, p := range list {
			if p.PID == m.selPID {
				m.sel = i
				break
			}
		}
	}
	m.sel = imax(0, imin(m.sel, len(list)-1))
	if len(list) > 0 {
		m.selPID = list[m.sel].PID
	}
	if m.sel < m.top {
		m.top = m.sel
	}
	if m.sel >= m.top+m.rows {
		m.top = m.sel - m.rows + 1
	}
	m.top = imax(0, imin(m.top, imax(0, len(list)-m.rows)))

	hc := func(txt string, active bool) string {
		if active {
			return bold(paint(cAccent, txt))
		}
		return paint(cDim, txt)
	}
	head := hc(fmt.Sprintf("%7s", "PID"), m.sortBy == byPID) + " " + hc(fmt.Sprintf("%-10s", "USER"), false) + " " + hc("S", false) + " " +
		hc(fmt.Sprintf("%6s", "CPU%"), m.sortBy == byCPU) + " " + hc(fmt.Sprintf("%5s", "MEM%"), m.sortBy == byMem) + " " +
		hc(fmt.Sprintf("%8s", "RES"), m.sortBy == byMem) + " " + hc(fmt.Sprintf("%4s", "THR"), false) + "  " + hc("COMMAND", m.sortBy == byName)
	lines := []string{head}

	for i := m.top; i < len(list) && i < m.top+m.rows; i++ {
		p := list[i]
		user := p.User
		if len(user) > 10 {
			user = user[:9] + "+"
		}
		cmd := p.Cmd
		if i == m.sel {
			line := fmt.Sprintf("%7d %-10s %c %6.1f %5.1f %8s %4d  %s", p.PID, user, p.State, p.CPUPct, p.MemPct, fmtBytes(p.RSS), p.Threads, cmd)
			lines = append(lines, "\x1b[7m"+pad(line, inner)+"\x1b[0m")
			continue
		}
		lines = append(lines, paint(cDim, fmt.Sprintf("%7d", p.PID))+" "+paint(cText, fmt.Sprintf("%-10s", user))+" "+
			paint(stateColor(p.State), string(p.State))+" "+
			paint(grad(clamp(p.CPUPct, 0, 100)), fmt.Sprintf("%6.1f", p.CPUPct))+" "+
			paint(grad(clamp(p.MemPct*4, 0, 100)), fmt.Sprintf("%5.1f", p.MemPct))+" "+
			fmt.Sprintf("%8s", fmtBytes(p.RSS))+" "+paint(cDim, fmt.Sprintf("%4d", p.Threads))+"  "+paint(cText, cmd))
	}

	order := map[sortKey]string{byCPU: "CPU", byMem: "memory", byPID: "PID", byName: "name"}[m.sortBy]
	arrow := "▼"
	if m.rev {
		arrow = "▲"
	}
	if m.sortBy == byPID || m.sortBy == byName {
		arrow = map[bool]string{false: "▲", true: "▼"}[m.rev]
	}
	title := paint(cLabel, "Processes") + paint(cDim, fmt.Sprintf(" · %d · sort %s %s", len(list), order, arrow))
	if m.filter != "" {
		title += paint(cDim, " · filter ") + paint(cYellow, m.filter)
	}
	return box(title, lines, w, h)
}
