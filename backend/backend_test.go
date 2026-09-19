package backend_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/backend"
)

type fake struct {
	name string
	caps backend.Capabilities
}

func (f fake) Name() string                       { return f.name }
func (f fake) Capabilities() backend.Capabilities { return f.caps }

func TestCPUDeclaresEverything(t *testing.T) {
	cpu := backend.CPU{}
	if got := cpu.Name(); got != "cpu" {
		t.Errorf("CPU{}.Name() = %q, want %q", got, "cpu")
	}
	caps := cpu.Capabilities()
	if !caps.ContinuousForward ||
		!caps.SpikingForward ||
		!caps.SparseBackward ||
		!caps.VariableWeights ||
		!caps.TeacherLoss ||
		!caps.DeviceStateSave ||
		!caps.Deterministic {
		t.Errorf("CPU{}.Capabilities() = %+v, want every field true", caps)
	}
}

func TestMissingAndCheck(t *testing.T) {
	lacksSpikingAndDeterministic := fake{
		name: "gpu",
		caps: backend.Capabilities{
			ContinuousForward: true,
			SparseBackward:    true,
			VariableWeights:   true,
			TeacherLoss:       true,
			DeviceStateSave:   true,
		},
	}
	all := backend.Requirement{
		ContinuousForward: true,
		SpikingForward:    true,
		SparseBackward:    true,
		VariableWeights:   true,
		TeacherLoss:       true,
		DeviceStateSave:   true,
		Deterministic:     true,
	}
	if got := backend.Missing(lacksSpikingAndDeterministic.caps, all); !equalStrings(got, []string{"deterministic", "spiking_forward"}) {
		t.Errorf("Missing(...) = %q, want [deterministic spiking_forward]", got)
	}
	err := backend.Check(lacksSpikingAndDeterministic, all)
	if err == nil {
		t.Fatal("Check(...) = nil, want error naming both missing capabilities")
	}
	if !strings.Contains(err.Error(), "deterministic") || !strings.Contains(err.Error(), "spiking_forward") {
		t.Errorf("Check error %q does not name both missing capabilities", err)
	}

	onlyForward := backend.Requirement{ContinuousForward: true}
	if got := backend.Missing(lacksSpikingAndDeterministic.caps, onlyForward); got != nil {
		t.Errorf("Missing(...) = %q, want nil", got)
	}
	if err := backend.Check(lacksSpikingAndDeterministic, onlyForward); err != nil {
		t.Errorf("Check(...) = %v, want nil", err)
	}
}

func TestSelectFallsBackExplicitly(t *testing.T) {
	gpu := fake{
		name: "gpu",
		caps: backend.Capabilities{
			ContinuousForward: true,
			SpikingForward:    true,
			VariableWeights:   true,
			TeacherLoss:       true,
			DeviceStateSave:   true,
			Deterministic:     true,
		},
	}
	needSparse := backend.Requirement{SparseBackward: true}
	available := []backend.Backend{gpu}

	b, rep, err := backend.Select("gpu", needSparse, available, true)
	if err != nil {
		t.Fatalf("Select(gpu, fallback=true): %v", err)
	}
	if b.Name() != "cpu" {
		t.Errorf("fallback backend = %q, want %q", b.Name(), "cpu")
	}
	if rep.Requested != "gpu" || rep.Used != "cpu" || !rep.FellBack {
		t.Errorf("report = %+v, want Requested=gpu Used=cpu FellBack=true", rep)
	}
	if !strings.Contains(rep.Reason, "sparse_backward") {
		t.Errorf("Reason = %q, want it to contain %q", rep.Reason, "sparse_backward")
	}
	if !equalStrings(rep.Missing, []string{"sparse_backward"}) {
		t.Errorf("Missing = %q, want [sparse_backward]", rep.Missing)
	}

	if _, rep, err := backend.Select("gpu", needSparse, available, false); err == nil {
		t.Errorf("Select(gpu, fallback=false) = (%+v, %v), want error", rep, err)
	} else if !strings.Contains(err.Error(), "gpu") {
		t.Errorf("error %q does not name requested backend", err)
	}

	if b, rep, err := backend.Select("tpu", needSparse, available, true); err != nil {
		t.Fatalf("Select(tpu, fallback=true): %v", err)
	} else if b.Name() != "cpu" || !rep.FellBack || rep.Reason != "not registered" {
		t.Errorf("tpu fallback = (%q, %+v), want cpu/FellBack=true/Reason=not registered", b.Name(), rep)
	}

	if b, rep, err := backend.Select("", backend.Requirement{}, available, true); err != nil {
		t.Fatalf("Select(\"\", fallback=true): %v", err)
	} else if b.Name() != "cpu" || rep.Used != "cpu" || rep.FellBack {
		t.Errorf("empty request = (%q, %+v), want Used=cpu and FellBack=false", b.Name(), rep)
	}

	duplicates := []backend.Backend{gpu, fake{name: "gpu", caps: gpu.caps}}
	if _, _, err := backend.Select("gpu", needSparse, duplicates, true); err == nil {
		t.Error("Select with duplicate \"gpu\" registrations = nil, want error")
	} else if !strings.Contains(err.Error(), "gpu") {
		t.Errorf("duplicate error %q does not name backend", err)
	}
}

func TestLedgerNeverReappliesHalfAnUpdate(t *testing.T) {
	l := new(backend.Ledger)

	if seq, err := l.Begin(); err != nil {
		t.Fatalf("first Begin: %v", err)
	} else if seq != 1 {
		t.Errorf("first Begin = %d, want 1", seq)
	}
	if _, err := l.Begin(); err == nil {
		t.Error("second Begin while in flight = nil error, want error")
	}

	if err := l.Abort(1); err != nil {
		t.Fatalf("Abort(1): %v", err)
	}
	if got := l.Applied(); got != 0 {
		t.Errorf("Applied after Abort = %d, want 0", got)
	}
	if seq, err := l.Begin(); err != nil {
		t.Fatalf("Begin after Abort: %v", err)
	} else if seq != 1 {
		t.Errorf("Begin after Abort = %d, want 1 (same number re-reserved)", seq)
	}
	if err := l.Commit(2); err == nil {
		t.Error("Commit(2) with update 1 in flight = nil error, want error")
	}
	if err := l.Commit(1); err != nil {
		t.Fatalf("Commit(1): %v", err)
	}
	if got := l.Applied(); got != 1 {
		t.Errorf("Applied after Commit = %d, want 1", got)
	}
	if err := l.Commit(1); err == nil {
		t.Error("Commit(1) with nothing in flight = nil error, want error")
	}
	if seq, err := l.Begin(); err != nil {
		t.Fatalf("Begin after Commit: %v", err)
	} else if seq != 2 {
		t.Errorf("Begin after Commit = %d, want 2", seq)
	}
}

func TestLedgerConcurrentBeginsSerialize(t *testing.T) {
	l := new(backend.Ledger)
	const n = 10
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				seq, err := l.Begin()
				if err != nil {
					continue
				}
				if err := l.Commit(seq); err != nil {
					t.Errorf("Commit(%d): %v", seq, err)
					return
				}
				return
			}
		}()
	}
	wg.Wait()
	if got := l.Applied(); got != n {
		t.Errorf("Applied = %d, want %d", got, n)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
