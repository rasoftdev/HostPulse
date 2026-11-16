package tui

import (
	"fmt"
	"math"
	"strings"
)

type rgb [3]int

var (
	useColor = true

	cText   = rgb{214, 220, 232}
	cDim    = rgb{108, 115, 135}
	cLabel  = rgb{128, 168, 255}
	cAccent = rgb{94, 204, 232}
	cBorder = rgb{70, 78, 102}
	cTrack  = rgb{40, 46, 66} // empty part of thin bars
	cGreen  = rgb{98, 208, 128}
	cYellow = rgb{240, 204, 90}
	cOrange = rgb{244, 150, 80}
	cRed    = rgb{242, 92, 100}
)

// SetColor enables or disables colors.
func SetColor(on bool) { useColor = on }

func paint(c rgb, s string) string {
	if !useColor || s == "" {
		return s
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", c[0], c[1], c[2], s)
}

func bold(s string) string {
	if !useColor || s == "" {
		return s
	}
	return "\x1b[1m" + s + "\x1b[0m"
}

func lerp(a, b rgb, t float64) rgb {
	var o rgb
	for i := range o {
		o[i] = int(float64(a[i]) + (float64(b[i])-float64(a[i]))*t)
	}
	return o
}

// grad returns green → yellow → red for a percentage (0–100).
func grad(p float64) rgb {
	switch {
	case p <= 0:
		return cGreen
	case p >= 100:
		return cRed
	case p < 55:
		return lerp(cGreen, cYellow, p/55)
	default:
		return lerp(cYellow, cRed, (p-55)/45)
	}
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func imin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func imax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// vlen counts visible characters (ignoring ANSI sequences).
func vlen(s string) int {
	n, in := 0, false
	for _, r := range s {
		switch {
		case in:
			if r == 'm' {
				in = false
			}
		case r == '\x1b':
			in = true
		default:
			n++
		}
	}
	return n
}

func stripANSI(s string) string {
	var sb strings.Builder
	in := false
	for _, r := range s {
		switch {
		case in:
			if r == 'm' {
				in = false
			}
		case r == '\x1b':
			in = true
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// clip truncates to w visible characters, keeping colors.
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if vlen(s) <= w {
		return s
	}
	var sb strings.Builder
	n, in, esc := 0, false, false
	for _, r := range s {
		if in {
			sb.WriteRune(r)
			if r == 'm' {
				in = false
			}
			continue
		}
		if r == '\x1b' {
			in, esc = true, true
			sb.WriteRune(r)
			continue
		}
		if n >= w {
			break
		}
		sb.WriteRune(r)
		n++
	}
	if esc {
		sb.WriteString("\x1b[0m")
	}
	return sb.String()
}

func pad(s string, w int) string {
	s = clip(s, w)
	if n := vlen(s); n < w {
		s += strings.Repeat(" ", w-n)
	}
	return s
}

// bar draws a horizontal bar with a color gradient.
func bar(p float64, w int) string {
	if w < 1 {
		return ""
	}
	n := int(math.Round(clamp(p, 0, 100) / 100 * float64(w)))
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString(paint(grad(float64(i)/float64(w)*100), "█"))
	}
	sb.WriteString(paint(cBorder, strings.Repeat("░", w-n)))
	return sb.String()
}

var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// spark draws a mini chart of the latest values. pct=true uses a 0–100 scale with per-value colors.
func spark(v []float64, w int, pct bool) string {
	if w < 1 {
		return ""
	}
	if len(v) > w {
		v = v[len(v)-w:]
	}
	max := 100.0
	if !pct {
		max = 1
		for _, x := range v {
			max = math.Max(max, x)
		}
	}
	var sb strings.Builder
	sb.WriteString(strings.Repeat(" ", w-len(v)))
	for _, x := range v {
		idx := int(clamp(x/max, 0, 1) * float64(len(sparkRunes)-1))
		col := cAccent
		if pct {
			col = grad(x)
		}
		sb.WriteString(paint(col, string(sparkRunes[idx])))
	}
	return sb.String()
}

// box draws a rounded-border panel of exactly w×h.
func box(title string, lines []string, w, h int) []string {
	if w < 6 || h < 2 {
		return nil
	}
	inner := w - 4
	t := clip(" "+title+" ", w-4)
	out := make([]string, 0, h)
	out = append(out, paint(cBorder, "╭─")+t+paint(cBorder, strings.Repeat("─", imax(0, w-3-vlen(t)))+"╮"))
	v := paint(cBorder, "│")
	for i := 0; i < h-2; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, v+" "+pad(l, inner)+" "+v)
	}
	out = append(out, paint(cBorder, "╰"+strings.Repeat("─", w-2)+"╯"))
	return out
}

func fmtBytes(v uint64) string {
	units := []string{"B", "K", "M", "G", "T", "P"}
	f, i := float64(v), 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	switch {
	case i == 0:
		return fmt.Sprintf("%dB", v)
	case f >= 100:
		return fmt.Sprintf("%.0f%s", f, units[i])
	default:
		return fmt.Sprintf("%.1f%s", f, units[i])
	}
}

func fmtRate(bps float64) string { return fmtBytes(uint64(math.Max(0, bps))) + "/s" }

func fmtUptime(sec float64) string {
	s := int(sec)
	d, h, m := s/86400, s%86400/3600, s%3600/60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		return fmt.Sprintf("%dm %ds", m, s%60)
	}
}

// thinBar is a half-height bar (lower half blocks). It leaves a small gap above and below instead of a whole blank
// row, so stacked bars and bars under a panel title do not touch.
func thinBar(p float64, w int) string {
	if w < 1 {
		return ""
	}
	n := int(math.Round(clamp(p, 0, 100) / 100 * float64(w)))
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString(paint(grad(float64(i)/float64(w)*100), "▄"))
	}
	sb.WriteString(paint(cTrack, strings.Repeat("▄", w-n)))
	return sb.String()
}
