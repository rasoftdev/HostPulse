package sysinfo

import (
	"testing"
	"time"
)

func TestSampler(t *testing.T) {
	s := &Sampler{Interval: 200 * time.Millisecond}
	s.Start()
	time.Sleep(1500 * time.Millisecond)
	sn := s.Get()

	if sn.Host.Hostname == "" || sn.Host.OS == "" {
		t.Errorf("host incompleto: %+v", sn.Host)
	}
	if sn.CPU.Cores == 0 || len(sn.CPU.PerCore) == 0 {
		t.Errorf("cpu sin núcleos: %+v", sn.CPU)
	}
	if sn.Memory.Total == 0 || sn.Memory.UsedPct <= 0 || sn.Memory.UsedPct > 100 {
		t.Errorf("memoria inválida: %+v", sn.Memory)
	}
	if len(sn.Procs) == 0 {
		t.Fatal("sin procesos")
	}
	if len(sn.Disks) == 0 {
		t.Errorf("sin discos")
	}
	t.Logf("%s | %s | %d núcleos | %q", sn.Host.Hostname, sn.Host.OS, sn.CPU.Cores, sn.CPU.Model)
	t.Logf("cpu %.1f%% mem %.1f%% procs %d hilos %d disks %d temp %.0f freq %.0f tcp %+v",
		sn.CPU.UsagePct, sn.Memory.UsedPct, sn.CPU.Processes, sn.CPU.Threads, len(sn.Disks), sn.CPU.TempC, sn.CPU.FreqMHz, sn.Network.Conns)
	for i, p := range sn.Procs {
		if i == 3 {
			break
		}
		t.Logf("  %6d %-12s %c %5.1f%% %8d %s", p.PID, p.User, p.State, p.CPUPct, p.RSS, p.Cmd)
	}
	for _, d := range sn.Disks {
		t.Logf("  disco %s %s %.1f%%", d.Mount, d.FSType, d.UsedPct)
	}
}
