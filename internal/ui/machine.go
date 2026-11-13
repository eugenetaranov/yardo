package ui

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/sys/unix"
)

// Machine is how the machine is doing, for the home's Machine panel: read
// from /proc every few seconds while yardo is open.
type Machine struct {
	// CPU is the busy share (0 to 1) of each reading since yardo opened,
	// oldest first, at most machineKeep of them.
	CPU                                    []float64
	Load                                   [3]float64
	Cores                                  int
	MemUsed, MemTotal, DiskUsed, DiskTotal uint64
	Up                                     time.Duration
	// Read: there's a /proc to read (not on a Mac, say).
	Read bool
}

// machineMsg is a reading: the machine, and the CPU's busy and total
// times so far (the next reading's share is the difference).
type machineMsg struct {
	m           Machine
	busy, total uint64
}

const (
	machineEvery = 3 * time.Second
	machineKeep  = 300 // 15 minutes of readings
)

// procRoot is where /proc is; tests point it at a directory of their own.
var procRoot = "/proc"

// machine is the readings so far. It outlives a Menu: coming back home
// keeps the CPU's history.
var machine struct {
	m           Machine
	busy, total uint64
}

func readMachine() tea.Msg {
	var r machineMsg
	r.m.Cores = runtime.NumCPU()
	if b, err := os.ReadFile(filepath.Join(procRoot, "loadavg")); err == nil {
		f := strings.Fields(string(b))
		for n := 0; n < 3 && n < len(f); n++ {
			r.m.Load[n], _ = strconv.ParseFloat(f[n], 64)
		}
		r.m.Read = true
	}
	if b, err := os.ReadFile(filepath.Join(procRoot, "uptime")); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			s, _ := strconv.ParseFloat(f[0], 64)
			r.m.Up = time.Duration(s) * time.Second
		}
	}
	if f, err := os.Open(filepath.Join(procRoot, "meminfo")); err == nil {
		kb := map[string]uint64{}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			k, v, _ := strings.Cut(sc.Text(), ":")
			n, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
			kb[k] = n
		}
		f.Close()
		r.m.MemTotal = kb["MemTotal"] * 1024
		r.m.MemUsed = (kb["MemTotal"] - min(kb["MemAvailable"], kb["MemTotal"])) * 1024
	}
	if b, err := os.ReadFile(filepath.Join(procRoot, "stat")); err == nil {
		line, _, _ := strings.Cut(string(b), "\n")
		// cpu user nice system idle iowait irq softirq steal …
		if f := strings.Fields(line); len(f) > 5 && f[0] == "cpu" {
			for n, s := range f[1:] {
				v, _ := strconv.ParseUint(s, 10, 64)
				if n < 8 {
					r.total += v
				}
				if n != 3 && n != 4 && n < 8 {
					r.busy += v
				}
			}
		}
	}
	var st unix.Statfs_t
	if unix.Statfs("/", &st) == nil {
		bs := uint64(st.Bsize)
		r.m.DiskTotal = uint64(st.Blocks) * bs
		r.m.DiskUsed = (uint64(st.Blocks) - uint64(st.Bfree)) * bs
	}
	return r
}

// took keeps a reading: the CPU's share since the last one joins its
// history.
func (r machineMsg) took() {
	cpu := machine.m.CPU
	if machine.total > 0 && r.total > machine.total {
		cpu = append(cpu, float64(r.busy-machine.busy)/float64(r.total-machine.total))
		if len(cpu) > machineKeep {
			cpu = cpu[len(cpu)-machineKeep:]
		}
	}
	machine.m, machine.busy, machine.total = r.m, r.busy, r.total
	machine.m.CPU = cpu
}

// tickMachine reads the machine again after d.
func tickMachine(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return readMachine() })
}
