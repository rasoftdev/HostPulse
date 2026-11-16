package tui

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"hostpulse/internal/sysinfo"
)

const histLen = 240

type sortKey int

const (
	byCPU sortKey = iota
	byMem
	byPID
	byName
)

type model struct {
	snap   sysinfo.Snapshot
	paused bool

	sortBy sortKey
	rev    bool

	sel    int
	selPID int
	top    int

	filter    string
	filtering bool

	cpuH, memH, rxH, txH []float64
	lastSample           string
	rows                 int // filas visibles de la tabla de procesos (la fija la vista)
}

func push(h []float64, v float64) []float64 {
	h = append(h, v)
	if len(h) > histLen {
		h = h[len(h)-histLen:]
	}
	return h
}

func (m *model) feed(s sysinfo.Snapshot) {
	m.snap = s
	if s.SampledAt == m.lastSample {
		return
	}
	m.lastSample = s.SampledAt
	m.cpuH = push(m.cpuH, s.CPU.UsagePct)
	m.memH = push(m.memH, s.Memory.UsedPct)
	m.rxH = push(m.rxH, s.Network.RxBps)
	m.txH = push(m.txH, s.Network.TxBps)
}

// parseKeys convierte los bytes de la entrada en nombres de tecla.
func parseKeys(b []byte) []string {
	var out []string
	for i := 0; i < len(b); {
		c := b[i]
		if c == 0x1b {
			if i+2 < len(b) && b[i+1] == '[' {
				j := i + 2
				for j < len(b) && (b[j] < 0x40 || b[j] > 0x7e) {
					j++
				}
				if j < len(b) {
					switch string(b[i+2 : j+1]) {
					case "A":
						out = append(out, "up")
					case "B":
						out = append(out, "down")
					case "5~":
						out = append(out, "pgup")
					case "6~":
						out = append(out, "pgdn")
					case "H", "1~":
						out = append(out, "home")
					case "F", "4~":
						out = append(out, "end")
					}
					i = j + 1
					continue
				}
			}
			if i+2 < len(b) && b[i+1] == 'O' {
				switch b[i+2] {
				case 'A':
					out = append(out, "up")
				case 'B':
					out = append(out, "down")
				case 'H':
					out = append(out, "home")
				case 'F':
					out = append(out, "end")
				}
				i += 3
				continue
			}
			out = append(out, "esc")
			i++
			continue
		}
		switch {
		case c == 3:
			out = append(out, "ctrl-c")
		case c == 10 || c == 13:
			out = append(out, "enter")
		case c == 127 || c == 8:
			out = append(out, "backspace")
		case c >= 32:
			r, n := utf8.DecodeRune(b[i:])
			out = append(out, string(r))
			i += n
			continue
		}
		i++
	}
	return out
}

func (m *model) setSort(k sortKey) {
	if m.sortBy == k {
		m.rev = !m.rev
		return
	}
	m.sortBy, m.rev = k, false
}

// key procesa una tecla. Devuelve true si hay que salir.
func (m *model) key(k string) bool {
	if m.filtering {
		switch k {
		case "esc":
			m.filter, m.filtering = "", false
		case "enter":
			m.filtering = false
		case "backspace":
			if m.filter != "" {
				_, n := utf8.DecodeLastRuneInString(m.filter)
				m.filter = m.filter[:len(m.filter)-n]
			}
		case "ctrl-c":
			return true
		default:
			if utf8.RuneCountInString(k) == 1 {
				m.filter += k
			}
		}
		m.sel, m.selPID, m.top = 0, 0, 0
		return false
	}
	page := imax(1, m.rows-1)
	switch k {
	case "q", "Q", "ctrl-c":
		return true
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup":
		m.move(-page)
	case "pgdn":
		m.move(page)
	case "home", "g":
		m.move(-1 << 30)
	case "end", "G":
		m.move(1 << 30)
	case "c":
		m.setSort(byCPU)
	case "m":
		m.setSort(byMem)
	case "p":
		m.setSort(byPID)
	case "n":
		m.setSort(byName)
	case "r":
		m.rev = !m.rev
	case "/":
		m.filtering = true
	case "esc":
		m.filter = ""
	case " ":
		m.paused = !m.paused
	}
	return false
}

func (m *model) move(d int) {
	m.sel += d
	m.selPID = 0 // se recalcula en la vista según el índice
}

// Run abre la interfaz y bloquea hasta que el usuario sale.
func Run(sam *sysinfo.Sampler) error {
	restore, err := rawMode()
	if err != nil {
		return err
	}
	out := os.Stdout
	fmt.Fprint(out, "\x1b[?1049h\x1b[?25l\x1b[2J")
	cleanup := func() {
		fmt.Fprint(out, "\x1b[?25h\x1b[?1049l")
		restore()
	}
	defer cleanup()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	keys := make(chan []byte, 16)
	go func() {
		buf := make([]byte, 64)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				close(keys)
				return
			}
			keys <- append([]byte(nil), buf[:n]...)
		}
	}()

	m := &model{}
	m.feed(sam.Get())
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	dirty := true
	lw, lh := 0, 0

	for {
		w, h := size()
		if w != lw || h != lh {
			lw, lh, dirty = w, h, true
			fmt.Fprint(out, "\x1b[2J")
		}
		if dirty {
			frame := m.view(w, h)
			fmt.Fprint(out, "\x1b[H"+strings.Join(frame, "\x1b[K\r\n")+"\x1b[K\x1b[J")
			dirty = false
		}
		select {
		case <-sig:
			return nil
		case b, ok := <-keys:
			if !ok {
				return nil
			}
			for _, k := range parseKeys(b) {
				if m.key(k) {
					return nil
				}
			}
			dirty = true
		case <-tick.C:
			if !m.paused {
				if s := sam.Get(); s.SampledAt != m.lastSample {
					m.feed(s)
					dirty = true
				}
			}
		}
	}
}
