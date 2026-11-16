package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"hostpulse/internal/sysinfo"
	"hostpulse/internal/tui"
	"hostpulse/internal/version"
)

func main() {
	os.Setenv("LC_ALL", "C")
	fs := flag.NewFlagSet("hostpulse", flag.ExitOnError)
	interval := fs.Float64("d", 1, "seconds between updates")
	noColor := fs.Bool("no-color", false, "disable colors")
	ver := fs.Bool("v", false, "print the version")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "HostPulse %s — system monitor for the terminal\n\nUsage: hostpulse [options]\n\n", version.Version)
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nKeys: q quit · ↑↓ move · c/m/p/n sort · r reverse · / filter · space pause")
	}
	fs.Parse(os.Args[1:])

	if *ver {
		fmt.Println("hostpulse", version.Version)
		return
	}
	if *interval < 0.2 {
		*interval = 0.2
	}
	tui.SetColor(!*noColor && os.Getenv("NO_COLOR") == "")

	sam := &sysinfo.Sampler{Interval: time.Duration(*interval * float64(time.Second))}
	sam.Start()
	if err := tui.Run(sam); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
