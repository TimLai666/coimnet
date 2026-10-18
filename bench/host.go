package bench

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// CurrentHost describes this machine. machine names it for the report; an
// empty value falls back to the host name. gpu is recorded as given, since
// the CPU path has no portable way to name an accelerator.
func CurrentHost(machine, gpu string) Host {
	if machine == "" {
		machine, _ = os.Hostname()
	}
	return Host{
		Machine: machine, OS: runtime.GOOS, Arch: runtime.GOARCH,
		CPU: cpuModel(), LogicalCPU: runtime.NumCPU(), MemoryB: totalMemory(),
		GPU: gpu, Runtime: runtime.Version(),
	}
}

func cpuModel() string {
	switch runtime.GOOS {
	case "linux":
		if v := procField("/proc/cpuinfo", "model name"); v != "" {
			return v
		}
	case "darwin":
		if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return runtime.GOARCH
}

func totalMemory() uint64 {
	switch runtime.GOOS {
	case "linux":
		// MemTotal is reported in kB.
		fields := strings.Fields(procField("/proc/meminfo", "MemTotal"))
		if len(fields) > 0 {
			if kb, err := strconv.ParseUint(fields[0], 10, 64); err == nil {
				return kb * 1024
			}
		}
	case "darwin":
		if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
			if b, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64); err == nil {
				return b
			}
		}
	}
	return 0
}

// procField returns the value after the first "key:" line of a /proc file.
func procField(path, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		name, value, ok := strings.Cut(s.Text(), ":")
		if ok && strings.TrimSpace(name) == key {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
