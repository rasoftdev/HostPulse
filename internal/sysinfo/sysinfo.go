// Package sysinfo recoge métricas del sistema en Linux, macOS y Windows.
package sysinfo

import (
	"context"
	"net"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
)

type Host struct {
	Hostname  string
	OS        string
	Kernel    string
	Arch      string
	UptimeSec float64
	BootTime  string
	PrimaryIP string
	IPs       []string
}

type CPU struct {
	Model     string
	Cores     int
	UsagePct  float64
	User      float64
	System    float64
	IOWait    float64
	Steal     float64
	PerCore   []float64
	LoadAvg   [3]float64
	TempC     float64 // 0 = sin sensor
	FreqMHz   float64 // 0 = desconocida
	Processes int
	Threads   int
	Running   int
	Zombies   int
}

type Memory struct {
	Total     uint64
	Used      uint64
	Available uint64
	Cached    uint64
	UsedPct   float64
	SwapTotal uint64
	SwapUsed  uint64
}

type Disk struct {
	Mount    string
	Device   string
	FSType   string
	Total    uint64
	Used     uint64
	Free     uint64
	UsedPct  float64
	InodePct float64 // 0 = no aplica (p. ej. Windows)
}

type DiskIO struct {
	ReadBps  float64
	WriteBps float64
	IOPS     float64
}

type Iface struct {
	Name    string
	RxBytes uint64
	TxBytes uint64
}

type Conns struct {
	Available   bool // false si el sistema no deja contar conexiones (p. ej. macOS sin sudo)
	Established int
	TimeWait    int
	Listen      int
}

type Network struct {
	RxBps      float64
	TxBps      float64
	RxPps      float64
	TxPps      float64
	Errors     uint64
	Interfaces []Iface
	Conns      Conns
}

type Proc struct {
	PID     int
	User    string
	State   byte // R, S, D, Z, T...
	Threads int
	CPUPct  float64 // instantáneo; 100 = un núcleo completo (como htop)
	MemPct  float64
	RSS     uint64
	Name    string // nombre corto
	Cmd     string // línea de comandos completa
}

// Snapshot es una lectura completa del sistema.
type Snapshot struct {
	Host      Host
	CPU       CPU
	Memory    Memory
	Disks     []Disk
	DiskIO    DiskIO
	Network   Network
	Procs     []Proc
	SampledAt string
}

type Sampler struct {
	// Interval entre muestras (por defecto 1 s).
	Interval time.Duration

	mu   sync.RWMutex
	snap Snapshot

	prevAll   cpu.TimesStat
	prevCores []cpu.TimesStat
	prevNet   [4]uint64 // rx bytes, tx bytes, rx pkts, tx pkts
	prevDisk  [3]uint64 // ops, bytes leídos, bytes escritos
	prevT     time.Time

	// Estado de la lectura de procesos (ver procs_*.go)
	prevProc map[int]float64 // pid -> segundos de CPU acumulados
	cmdOf    map[int]string
	nameOf   map[int]string
	userOf   map[int]string
	passwd   map[string]string
	thrOf    map[int]int // hilos por PID (solo fuera de Linux; se consulta de forma escalonada)
}

func (s *Sampler) Start() {
	s.static()
	s.tick(0)
	if s.Interval <= 0 {
		s.Interval = time.Second
	}
	go func() {
		t := time.NewTicker(s.Interval)
		for i := 1; ; i++ {
			<-t.C
			s.tick(i)
		}
	}()
}

func (s *Sampler) Get() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

func readFile(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}

func u64(s string) uint64 { v, _ := strconv.ParseUint(s, 10, 64); return v }

func round1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }

func prettyOS(i *host.InfoStat) string {
	for _, l := range strings.Split(readFile("/etc/os-release"), "\n") {
		if strings.HasPrefix(l, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(l, "PRETTY_NAME="), `"`)
		}
	}
	switch runtime.GOOS {
	case "darwin":
		return "macOS " + i.PlatformVersion
	case "windows":
		return strings.TrimPrefix(i.Platform, "Microsoft ")
	}
	return strings.TrimSpace(i.Platform + " " + i.PlatformVersion)
}

func (s *Sampler) static() {
	h := &s.snap.Host
	h.Hostname, _ = os.Hostname()
	h.OS, h.Arch = runtime.GOOS, runtime.GOARCH
	if i, err := host.Info(); err == nil {
		if i.Hostname != "" {
			h.Hostname = i.Hostname
		}
		h.OS, h.Kernel = prettyOS(i), i.KernelVersion
	}
	c := &s.snap.CPU
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		c.Model = strings.Join(strings.Fields(infos[0].ModelName), " ")
	}
	c.Cores, _ = cpu.Counts(true)
	if c.Cores == 0 {
		c.Cores = runtime.NumCPU()
	}
}

func cpuTotal(t cpu.TimesStat) float64 {
	return t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
}

// usage calcula porcentajes entre dos lecturas acumuladas de tiempos de CPU.
func usage(cur, prev cpu.TimesStat) (pct, user, sys, iow, steal float64) {
	pt := cpuTotal(prev)
	dt := cpuTotal(cur) - pt
	if pt == 0 || dt <= 0 {
		return
	}
	d := func(c, p float64) float64 {
		if c < p {
			return 0
		}
		return (c - p) / dt * 100
	}
	idle := d(cur.Idle+cur.Iowait, prev.Idle+prev.Iowait)
	pct = clamp100(100 - idle)
	return pct, d(cur.User+cur.Nice, prev.User+prev.Nice), d(cur.System+cur.Irq+cur.Softirq, prev.System+prev.Irq+prev.Softirq),
		d(cur.Iowait, prev.Iowait), d(cur.Steal, prev.Steal)
}

func clamp100(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

var partRe = regexp.MustCompile(`^((sd|vd|xvd|hd)[a-z]+\d+|(nvme\d+n\d+|mmcblk\d+)p\d+|disk\d+s\d+)$`)

func physDisk(n string) bool {
	l := strings.ToLower(n)
	for _, p := range []string{"loop", "ram", "zram", "dm-", "md", "sr", "fd"} {
		if strings.HasPrefix(l, p) {
			return false
		}
	}
	return !partRe.MatchString(l)
}

func virtualIface(name string) bool {
	l := strings.ToLower(name)
	if l == "lo" || l == "lo0" || strings.Contains(l, "loopback") {
		return true
	}
	for _, p := range []string{"docker", "br-", "veth", "virbr", "cni", "flannel", "kube", "lxc", "lxd", "cali", "vxlan", "weave", "podman",
		"cilium", "vmnet", "vboxnet", "vethernet", "isatap", "teredo", "utun", "awdl", "llw", "bridge", "gif", "stf", "anpi"} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

func (s *Sampler) tick(i int) {
	now := time.Now()
	first := s.prevT.IsZero()
	dt := now.Sub(s.prevT).Seconds()

	// CPU
	var c CPU
	if all, err := cpu.Times(false); err == nil && len(all) > 0 {
		c.UsagePct, c.User, c.System, c.IOWait, c.Steal = usage(all[0], s.prevAll)
		s.prevAll = all[0]
	}
	cores, _ := cpu.Times(true)
	for k, v := range cores {
		p := 0.0
		if k < len(s.prevCores) {
			p, _, _, _, _ = usage(v, s.prevCores[k])
		}
		c.PerCore = append(c.PerCore, round1(p))
	}
	s.prevCores = cores

	// Red
	var rx, tx, rp, tp, errs uint64
	var ifaces []Iface
	if ios, err := gnet.IOCounters(true); err == nil {
		for _, io := range ios {
			if virtualIface(io.Name) {
				continue
			}
			rx, tx, rp, tp = rx+io.BytesRecv, tx+io.BytesSent, rp+io.PacketsRecv, tp+io.PacketsSent
			errs += io.Errin + io.Errout
			ifaces = append(ifaces, Iface{io.Name, io.BytesRecv, io.BytesSent})
		}
	}
	var n Network
	if !first && dt > 0 && rx >= s.prevNet[0] && tx >= s.prevNet[1] {
		n.RxBps, n.TxBps = float64(rx-s.prevNet[0])/dt, float64(tx-s.prevNet[1])/dt
		if rp >= s.prevNet[2] && tp >= s.prevNet[3] {
			n.RxPps, n.TxPps = float64(rp-s.prevNet[2])/dt, float64(tp-s.prevNet[3])/dt
		}
	}
	s.prevNet = [4]uint64{rx, tx, rp, tp}
	n.Errors, n.Interfaces = errs, ifaces

	// E/S de disco (solo dispositivos físicos para no contar las particiones dos veces)
	var ops, rb, wb uint64
	if ios, err := disk.IOCounters(); err == nil {
		for name, io := range ios {
			if physDisk(name) {
				ops += io.ReadCount + io.WriteCount
				rb += io.ReadBytes
				wb += io.WriteBytes
			}
		}
	}
	var dio DiskIO
	if !first && dt > 0 && rb >= s.prevDisk[1] && wb >= s.prevDisk[2] {
		dio = DiskIO{
			ReadBps:  float64(rb-s.prevDisk[1]) / dt,
			WriteBps: float64(wb-s.prevDisk[2]) / dt,
			IOPS:     float64(ops-s.prevDisk[0]) / dt,
		}
	}
	s.prevDisk = [3]uint64{ops, rb, wb}
	s.prevT = now

	// Memoria
	var m Memory
	if vm, err := mem.VirtualMemory(); err == nil {
		m.Total, m.Available = vm.Total, vm.Available
		if vm.Total > vm.Available {
			m.Used = vm.Total - vm.Available
		}
		if vm.Total > 0 {
			m.UsedPct = round1(float64(m.Used) / float64(vm.Total) * 100)
		}
		m.Cached = vm.Cached + vm.Buffers
	}
	if sw, err := mem.SwapMemory(); err == nil {
		m.SwapTotal, m.SwapUsed = sw.Total, sw.Used
	}

	if la, err := load.Avg(); err == nil {
		c.LoadAvg = [3]float64{la.Load1, la.Load5, la.Load15}
	}
	up, _ := host.Uptime()
	procs, zombies := s.readProcs(dt, m.Total)

	s.mu.Lock()
	defer s.mu.Unlock()
	sn := &s.snap
	sn.CPU.UsagePct, sn.CPU.User, sn.CPU.System, sn.CPU.IOWait, sn.CPU.Steal = round1(c.UsagePct), round1(c.User), round1(c.System), round1(c.IOWait), round1(c.Steal)
	sn.CPU.PerCore, sn.CPU.LoadAvg = c.PerCore, c.LoadAvg
	sn.Memory, sn.DiskIO = m, dio
	sn.Host.UptimeSec = float64(up)
	sn.Host.BootTime = now.Add(-time.Duration(up) * time.Second).UTC().Format(time.RFC3339)
	prevConns := sn.Network.Conns
	sn.Network = n
	sn.Network.Conns = prevConns

	sn.Procs, sn.CPU.Zombies = procs, zombies
	sn.CPU.Processes, sn.CPU.Threads, sn.CPU.Running = len(procs), 0, 0
	for _, p := range procs {
		sn.CPU.Threads += p.Threads
		if p.State == 'R' {
			sn.CPU.Running++
		}
	}
	if i%3 == 0 {
		sn.Network.Conns = readConns()
		sn.CPU.TempC = readTemp()
		sn.CPU.FreqMHz = readFreq()
	}
	if i%15 == 0 {
		sn.Disks = readDisks()
		sn.Host.IPs = localIPs()
		sn.Host.PrimaryIP = PrimaryIP()
	}
	sn.SampledAt = now.UTC().Format(time.RFC3339)
}

// readTemp devuelve la temperatura de la CPU (o la mayor disponible); 0 si el sistema no expone sensores.
func readTemp() float64 {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ts, _ := sensors.TemperaturesWithContext(ctx)
	var cpuBest, anyBest float64
	for _, t := range ts {
		v := t.Temperature
		if v <= 1 || v >= 130 {
			continue
		}
		if v > anyBest {
			anyBest = v
		}
		k := strings.ToLower(t.SensorKey)
		for _, w := range []string{"coretemp", "k10temp", "cpu", "package", "tctl", "soc"} {
			if strings.Contains(k, w) && v > cpuBest {
				cpuBest = v
			}
		}
	}
	if cpuBest > 0 {
		return cpuBest
	}
	return anyBest
}

var skipFS = map[string]bool{"squashfs": true, "iso9660": true, "udf": true, "devfs": true, "autofs": true, "overlay": true, "tmpfs": true}

func skipMount(p disk.PartitionStat) bool {
	if skipFS[strings.ToLower(p.Fstype)] {
		return true
	}
	for _, pre := range []string{"/snap/", "/var/lib/docker", "/var/lib/snapd", "/usr/src/linux-headers", "/private/var/vm",
		"/Library/Developer", "/Volumes/Recovery"} {
		if strings.HasPrefix(p.Mountpoint, pre) {
			return true
		}
	}
	return strings.HasPrefix(p.Mountpoint, "/System/Volumes/") && p.Mountpoint != "/System/Volumes/Data"
}

func readDisks() []Disk {
	parts, err := disk.Partitions(false)
	if err != nil && len(parts) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []Disk
	for _, p := range parts {
		if skipMount(p) || seen[p.Device] {
			continue
		}
		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		seen[p.Device] = true
		out = append(out, Disk{
			Mount: p.Mountpoint, Device: p.Device, FSType: p.Fstype,
			Total: u.Total, Used: u.Used, Free: u.Free,
			UsedPct: round1(u.UsedPercent), InodePct: round1(u.InodesUsedPercent),
		})
	}
	return out
}

func localIPs() []string {
	ips := []string{}
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 || virtualIface(ifc.Name) {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLinkLocalUnicast() {
				ips = append(ips, n.IP.String())
			}
		}
	}
	return ips
}

// PrimaryIP devuelve la IP de salida por defecto (un socket UDP «conectado» no envía tráfico).
func PrimaryIP() string {
	if c, err := net.Dial("udp", "192.0.2.1:9"); err == nil {
		defer c.Close()
		if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
			return a.IP.String()
		}
	}
	if ips := localIPs(); len(ips) > 0 {
		return ips[0]
	}
	return "127.0.0.1"
}
