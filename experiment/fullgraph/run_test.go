package fullgraph

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/resources"
)

func TestDefaultOptions(t *testing.T) {
	got := DefaultOptions()
	want := Options{
		InputSet:     "alin",
		ReadoutSet:   "descending_neuron",
		Steps:        16,
		Truncation:   8,
		ContinueRows: 4,
		PlasticEdges: 4096,
		Chemistry:    true,
		LearningRate: 0.001,
		MaxMemoryMiB: 12288,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DefaultOptions() = %+v, want %+v", got, want)
	}
}

func TestDefaultOptionsFitTheDefaultBudget(t *testing.T) {
	o := DefaultOptions()
	plan := resources.Plan{
		Nodes:              165_122,
		Edges:              25_563_197,
		StateDim:           1,
		Individuals:        1,
		HistorySteps:       o.Steps,
		Precision:          resources.PrecisionF64,
		Optimizer:          resources.OptimizerAdamW,
		PlasticEdges:       4096,
		ModulationRegions:  1,
		ModulationChannels: 1,
	}
	within, err := resources.Estimate(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.HistorySteps = 32
	tooLarge, err := resources.Estimate(plan)
	if err != nil {
		t.Fatal(err)
	}
	limitBytes := uint64(o.MaxMemoryMiB) << 20
	t.Logf("%d-step estimate: %.1f MiB; 32-step estimate: %.1f MiB; default budget: %d MiB",
		o.Steps, float64(within.TotalBytes)/(1<<20), float64(tooLarge.TotalBytes)/(1<<20), o.MaxMemoryMiB)
	if within.TotalBytes > limitBytes {
		t.Errorf("default %d-step estimate %d bytes exceeds %d MiB", o.Steps, within.TotalBytes, o.MaxMemoryMiB)
	}
	if tooLarge.TotalBytes <= limitBytes {
		t.Errorf("32-step estimate %d bytes fits within %d MiB, want it to exceed budget", tooLarge.TotalBytes, o.MaxMemoryMiB)
	}
}

func TestOptionsValidate(t *testing.T) {
	valid := DefaultOptions()
	valid.Store, valid.Params, valid.Protocol, valid.OutDir = "store", "params", "protocol", "out"
	cases := []struct {
		name   string
		mutate func(*Options)
	}{
		{"missing store", func(o *Options) { o.Store = "" }},
		{"missing params", func(o *Options) { o.Params = "" }},
		{"missing protocol", func(o *Options) { o.Protocol = "" }},
		{"missing out dir", func(o *Options) { o.OutDir = "" }},
		{"steps below two", func(o *Options) { o.Steps = 1 }},
		{"truncation negative", func(o *Options) { o.Truncation = -1 }},
		{"continue rows zero", func(o *Options) { o.ContinueRows = 0 }},
		{"plastic edges negative", func(o *Options) { o.PlasticEdges = -1 }},
		{"learning rate zero", func(o *Options) { o.LearningRate = 0 }},
		{"memory limit zero", func(o *Options) { o.MaxMemoryMiB = 0 }},
		{"input equals readout", func(o *Options) { o.InputSet, o.ReadoutSet = "same", "same" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := valid
			tc.mutate(&o)
			if err := o.Validate(); err == nil {
				t.Errorf("Validate() accepted %s", tc.name)
			}
		})
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("the valid options must pass Validate: %v", err)
	}
}

// TestMechanismsShowPlasticityAndChemistryActed runs the same fixture twice,
// once with the plastic and chemical layers on and once with both off, and
// requires mechanismsOf to report that the on run actually moved fast state
// and concentration and the off run reports every one of its six fields as
// zero.
func TestMechanismsShowPlasticityAndChemistryActed(t *testing.T) {
	ctx := context.Background()
	c, p, o := buildFixtureConfig(t)
	on, rep, err := shortTraining(ctx, c, p, o, runOptions{Steps: 8, ContinueRows: 2, PlasticEdges: 2, Chemistry: true, MaxCells: 0}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := mechanismsOf(on, rep)
	t.Logf("on: plastic_edges=%d plastic_steps=%d fast_nonzero=%d fast_max_abs=%g chemistry_steps=%d concentration_max=%g",
		m.PlasticEdges, m.PlasticSteps, m.FastNonzero, m.FastMaxAbs, m.ChemistrySteps, m.ConcentrationMax)
	if m.FastNonzero <= 0 {
		t.Errorf("FastNonzero = %d, want > 0 with 2 plastic edges", m.FastNonzero)
	}
	if m.FastMaxAbs <= 0 {
		t.Errorf("FastMaxAbs = %v, want > 0 with 2 plastic edges", m.FastMaxAbs)
	}
	if m.ConcentrationMax <= 0 {
		t.Errorf("ConcentrationMax = %v, want > 0 with chemistry on", m.ConcentrationMax)
	}
	if m.PlasticSteps <= 0 {
		t.Errorf("PlasticSteps = %d, want > 0 with 2 plastic edges", m.PlasticSteps)
	}
	if m.ChemistrySteps <= 0 {
		t.Errorf("ChemistrySteps = %d, want > 0 with chemistry on", m.ChemistrySteps)
	}

	off, repOff, err := shortTraining(ctx, c, p, o, runOptions{Steps: 8, ContinueRows: 2, PlasticEdges: 0, Chemistry: false, MaxCells: 0}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mOff := mechanismsOf(off, repOff)
	t.Logf("off: plastic_edges=%d plastic_steps=%d fast_nonzero=%d fast_max_abs=%g chemistry_steps=%d concentration_max=%g",
		mOff.PlasticEdges, mOff.PlasticSteps, mOff.FastNonzero, mOff.FastMaxAbs, mOff.ChemistrySteps, mOff.ConcentrationMax)
	if mOff != (Mechanisms{}) {
		t.Errorf("off mechanisms = %+v, want every field zero", mOff)
	}
}

// TestRunRefusesExistingOutDirBeforeLoading proves that Run checks the output
// directory before anything is read: the store path names a file that does not
// exist, so any error that mentions the out directory must have come from the
// up-front refusal rather than from loading the store.
func TestRunRefusesExistingOutDirBeforeLoading(t *testing.T) {
	ctx := context.Background()
	o := DefaultOptions()
	o.Store = filepath.Join(t.TempDir(), "missing-store.coimnet")
	o.Params = filepath.Join(t.TempDir(), "missing-params.json")
	o.Protocol = filepath.Join(t.TempDir(), "missing-protocol.json")
	o.OutDir = t.TempDir()
	_, err := Run(ctx, o)
	if err == nil {
		t.Fatal("Run accepted an existing output directory")
	}
	if got := err.Error(); !strings.Contains(got, o.OutDir) || !strings.Contains(got, "already exists") {
		t.Errorf("refusal %q must name the existing output directory %q", got, o.OutDir)
	}
}

// TestContinuationDigest verifies that continuationDigest produces deterministic,
// distinct SHA-256 digests over float64 output rows.
func TestContinuationDigest(t *testing.T) {
	rows1 := [][]float64{{1.0, 2.0}, {3.0, 4.0}}
	rows2 := [][]float64{{1.0, 2.0}, {3.0, 4.0}}
	rows3 := [][]float64{{1.0, 2.0}, {3.0, 5.0}}
	d1 := continuationDigest(rows1)
	d2 := continuationDigest(rows2)
	d3 := continuationDigest(rows3)
	if d1 == "" {
		t.Fatal("continuationDigest returned empty string")
	}
	if d1 != d2 {
		t.Errorf("digest of identical rows differed: %s vs %s", d1, d2)
	}
	if d1 == d3 {
		t.Errorf("digest of different rows agreed: %s", d1)
	}
}

// TestRunHonoursContextCancellation verifies that Run aborts when given a canceled context.
func TestRunHonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o := DefaultOptions()
	o.Store = "store"
	o.Params = "params"
	o.Protocol = "protocol"
	o.OutDir = filepath.Join(t.TempDir(), "nonexistent")
	_, err := Run(ctx, o)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run with canceled context returned %v, want %v", err, context.Canceled)
	}
}
