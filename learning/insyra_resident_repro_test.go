package learning

import (
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/HazelnutParadise/insyra/accel"
	"github.com/HazelnutParadise/insyra/nn"
)

// The two tests below isolate a wrong encoder gradient seen on Vulkan
// (ubuntu-1, RTX 4070) and not on Metal: the first device use of a process
// returned values such as 2.24e-44 where the host gave -1.295. Each variant
// runs in a fresh process, so every run is a first device use. They use only
// Insyra, so a failing variant is a reproduction to report upstream.
// Run with COIMNET_INSYRA_REPRO=1.

var residentReproVariants = []string{"clone", "clone-sync", "backward", "backward-sync"}

func TestInsyraResidentRepro(t *testing.T) {
	if os.Getenv("COIMNET_INSYRA_REPRO") == "" {
		t.Skip("set COIMNET_INSYRA_REPRO=1 to run")
	}
	if _, err := accel.ResidentDevice(); err != nil {
		t.Skipf("no resident device: %v", err)
	}
	failed := false
	for _, variant := range residentReproVariants {
		for run := range 3 {
			cmd := exec.Command(os.Args[0], "-test.run=^TestInsyraResidentReproChild$", "-test.count=1", "-test.v")
			cmd.Env = append(os.Environ(), "COIMNET_INSYRA_REPRO_VARIANT="+variant)
			out, err := cmd.CombinedOutput()
			verdict := "ok"
			if err != nil {
				verdict, failed = "FAIL", true
			}
			detail := ""
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "repro:") {
					detail = strings.TrimSpace(line)
				}
			}
			t.Logf("%-14s run %d: %s %s", variant, run+1, verdict, detail)
		}
	}
	if failed {
		t.Fail()
	}
}

func TestInsyraResidentReproChild(t *testing.T) {
	variant := os.Getenv("COIMNET_INSYRA_REPRO_VARIANT")
	if variant == "" {
		t.Skip("child of TestInsyraResidentRepro")
	}
	device, err := accel.ResidentDevice()
	if err != nil {
		t.Fatalf("repro: no resident device: %v", err)
	}
	const m, k, n = 20, 12, 16
	fill := func(count, salt int) []float32 {
		v := make([]float32, count)
		for i := range v {
			v[i] = float32((i*7+salt)%11-5) / 8
		}
		return v
	}
	x, _ := nn.NewTensor([]int{m, k}, fill(m*k, 1))
	e, _ := nn.NewTensor([]int{k, n}, fill(k*n, 2))
	seed, _ := nn.NewTensor([]int{m, n}, fill(m*n, 3))
	sync := strings.HasSuffix(variant, "-sync")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("repro: %v", err)
		}
	}
	data := func(t2 *nn.Tensor) []float32 {
		t.Helper()
		v, err := t2.Float32Data()
		must(err)
		return v
	}
	compare := func(what string, want, got []float32) {
		t.Helper()
		for i := range want {
			if math.Float32bits(want[i]) != math.Float32bits(got[i]) {
				t.Fatalf("repro: %s[%d] = %v (bits %#x), want %v", what, i, got[i], math.Float32bits(got[i]), want[i])
			}
		}
		t.Logf("repro: %s matches (%d values)", what, len(want))
	}
	switch strings.TrimSuffix(variant, "-sync") {
	case "clone":
		up, err := seed.To(device)
		must(err)
		defer up.Release()
		if sync {
			must(accel.SynchronizeResident())
		}
		clone, err := up.To(device)
		must(err)
		defer clone.Release()
		host, err := clone.ToHost()
		must(err)
		compare("clone after upload", data(seed), data(host))
	case "backward":
		gradient := func(on accel.Device) []float32 {
			tape := nn.NewTape()
			defer tape.Close()
			must(tape.SetPrecision(accel.PrecisionFloat32Rounded))
			xd, ed, sd := x, e, seed
			if on.ID != "" {
				var err error
				xd, err = x.To(on)
				must(err)
				defer xd.Release()
				ed, err = e.To(on)
				must(err)
				defer ed.Release()
			}
			_, err := tape.Param(ed)
			must(err)
			y, err := tape.MatMul(xd, ed)
			must(err)
			if on.ID != "" {
				sd, err = seed.To(on)
				must(err)
				defer sd.Release()
				if sync {
					must(accel.SynchronizeResident())
				}
			}
			must(tape.BackwardFrom(y, sd))
			g, err := tape.Grad(ed)
			must(err)
			defer g.Release()
			host, err := g.ToHost()
			must(err)
			return data(host)
		}
		want := gradient(accel.Device{})
		got := gradient(device)
		compare("encoder gradient", want, got)
	default:
		t.Fatalf("repro: unknown variant %s", variant)
	}
}
