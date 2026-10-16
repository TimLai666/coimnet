package learning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

func chemicalStateCases(t *testing.T) []mixedReferenceCase {
	t.Helper()
	var cases []mixedReferenceCase
	for _, core := range []string{"mixed", "lif"} {
		for _, source := range []string{"controller", "neural"} {
			for _, mix := range []string{modulation.MixSum, modulation.MixMax} {
				for _, profile := range []string{"open-positive", "open-negative", "gain-upper", "threshold-upper", "floor"} {
					f := chemicalMaxReferenceFixture(t, core, source, mix, profile)
					chemicalHomeostasisSet(&f, true)
					f.config.Chemistry.InitialActivity = nil
					f.config.Chemistry.SourceStep = 0
					cases = append(cases, f)
				}
			}
		}
	}
	if len(cases) != 40 {
		t.Fatalf("state cases=%d want40", len(cases))
	}
	return cases
}

func chemicalStateReference(t *testing.T, f mixedReferenceCase) mixedReferenceForward {
	t.Helper()
	warmup := chemicalStateWarmup(f)
	if warmup != 0 {
		f.chemicalWarmup = true
		f.input = append([][]float64{{0, 0}}, f.input...)
	}
	const weightBase, biasBase, logTauBase, thetaBase = 0, 4, 6, 8
	initialBase := thetaBase + len(f.parameters.ThetaRaw)
	inputBase := initialBase + mixedReferenceNodes
	n := inputBase + len(f.input)*mixedReferenceNodes
	encoded, _, _ := referenceEncodedInputs(f.input, f.parameters.Encoder, inputBase)
	r := referenceForward(f, encoded, n, weightBase, biasBase, logTauBase, thetaBase, initialBase, inputBase)
	for label, n := range map[string]int{"voltage": len(r.voltage), "adaptation": len(r.adaptation), "refractory": len(r.refractory), "concentration": len(r.concentration)} {
		if n != 5+warmup {
			t.Fatalf("state reference %s has%d rows want%d", label, n, 5+warmup)
		}
	}
	return r
}

func chemicalStateWarmup(f mixedReferenceCase) int {
	if f.source == "neural" {
		return 1
	}
	return 0
}

func chemicalStateValues(row []dual, nodes []int) []float64 {
	out := make([]float64, len(nodes))
	for k, id := range nodes {
		out[k] = row[id].v
	}
	return out
}

func chemicalStateRows(t *testing.T, label string, got, want [][]float64, f32 bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s rows%d want%d", label, len(got), len(want))
	}
	for row, w := range want {
		if len(got[row]) != len(w) {
			t.Fatalf("%s row%d width%d want%d", label, row, len(got[row]), len(w))
		}
		for col, v := range w {
			tol := 1e-12 * (1 + math.Abs(v))
			if f32 {
				tol = 1e-5 + 1e-4*math.Abs(v)
			}
			if !finiteReference(v) || !finiteReference(got[row][col]) || math.Abs(got[row][col]-v) > tol {
				t.Fatalf("%s[%d][%d]=%.17g want%.17g", label, row, col, got[row][col], v)
			}
		}
	}
}

func chemicalStatePrediction(f mixedReferenceCase, ref mixedReferenceForward) [][]float64 {
	h := make([][]float32, len(ref.readout))
	for step, row := range ref.readout {
		h[step] = []float32{float32(row[0]), float32(row[1])}
	}
	p := f.parameters.Readout
	y := f32MatMul(h, [][]float32{{float32(p[0]), float32(p[1])}, {float32(p[2]), float32(p[3])}})
	out := make([][]float64, len(y))
	for step, row := range y {
		out[step] = []float64{float64(row[0]), float64(row[1])}
	}
	return out[chemicalStateWarmup(f):]
}

func chemicalStateIndividual(t *testing.T, f mixedReferenceCase) *learning.Individual {
	t.Helper()
	return chemicalStateIndividualWithWarmup(t, f, true)
}

func chemicalStateIndividualWithWarmup(t *testing.T, f mixedReferenceCase, warm bool) *learning.Individual {
	t.Helper()
	base := f.config
	part := base.Chemistry.Part
	base.Chemistry = nil
	a, err := learning.NewIndividual(base, f.parameters, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if warm && chemicalStateWarmup(f) != 0 {
		chemicalStateAdvance(t, a, [][]float64{{0, 0}}, false)
	}
	if err = a.EnableChemistry(part.Config); err != nil {
		t.Fatal(err)
	}
	if part.Expression != nil {
		if err = a.SetExpressionGain(*part.Expression); err != nil {
			t.Fatal(err)
		}
	}
	s := a.Snapshot()
	s.Chemical.State = part.State
	a, err = learning.RestoreIndividual(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func chemicalStateAdvance(t *testing.T, a *learning.Individual, input [][]float64, gated bool) [][]float64 {
	t.Helper()
	if gated {
		out, report, err := a.AdvanceGated(context.Background(), input, make([]float64, len(input)))
		if err != nil {
			t.Fatal(err)
		}
		if report != (learning.PlasticReport{}) {
			t.Fatalf("disabled plasticity report=%+v", report)
		}
		return out
	}
	out, err := a.Advance(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func chemicalStateSnapshot(t *testing.T, f mixedReferenceCase, r mixedReferenceForward, at int, s learning.IndividualSnapshot) {
	t.Helper()
	chemicalAt := at
	at += chemicalStateWarmup(f)
	if !reflect.DeepEqual(s.Parameters, f.parameters) || s.Optimizer.Updates != 0 || s.Optimizer.Episodes != 0 || s.Optimizer.Accumulator != nil {
		t.Fatal("inference changed parameters or optimizer clock")
	}
	var lif *dynamics.LIFState
	var continuous *dynamics.State
	var lifNodes, continuousNodes, delays []int
	if f.config.LIF != nil {
		if s.Neural.Core != learning.NeuralCoreLIF || s.Neural.LIF == nil || s.Neural.Mixed != nil || s.Neural.Continuous != nil {
			t.Fatal("wrong LIF state union")
		}
		lif = s.Neural.LIF
		lifNodes = []int{0, 1}
		delays = f.config.LIF.Delays
	} else {
		if s.Neural.Core != learning.NeuralCoreMixed || s.Neural.Mixed == nil || s.Neural.LIF != nil || s.Neural.Continuous != nil {
			t.Fatal("wrong mixed state union")
		}
		mixed := s.Neural.Mixed
		for id, rule := range f.config.Mixed.NodeRule {
			if rule == dynamics.RuleLIF {
				lifNodes = append(lifNodes, id)
			} else {
				continuousNodes = append(continuousNodes, id)
			}
		}
		if mixed.Steps != uint64(at) || !reflect.DeepEqual(mixed.Index.LIFNodes, lifNodes) || !reflect.DeepEqual(mixed.Index.ContinuousNodes, continuousNodes) {
			t.Fatal("mixed clock or node map differs")
		}
		lif = &mixed.LIF
		continuous = &mixed.Continuous
		delays = f.config.Mixed.Delays
	}
	if lif.Steps != uint64(at) {
		t.Fatalf("LIF steps=%d want%d", lif.Steps, at)
	}
	chemicalStateRows(t, "voltage", [][]float64{lif.Voltage}, [][]float64{chemicalStateValues(r.voltage[at], lifNodes)}, false)
	chemicalStateRows(t, "adaptation", [][]float64{lif.Adaptation}, [][]float64{chemicalStateValues(r.adaptation[at], lifNodes)}, false)
	rate, h := make([]float64, len(lifNodes)), make([]float64, len(lifNodes))
	refractory := make([]int, len(lifNodes))
	for k, id := range lifNodes {
		rate[k] = r.rate[at][id]
		h[k] = r.homeostasis[at][id]
		refractory[k] = r.refractory[at][id]
	}
	chemicalStateRows(t, "rate", [][]float64{lif.Rate}, [][]float64{rate}, false)
	chemicalStateRows(t, "homeostasis", [][]float64{lif.Homeostasis}, [][]float64{h}, false)
	if !reflect.DeepEqual(lif.Refractory, refractory) {
		t.Fatalf("refractory=%v want%v", lif.Refractory, refractory)
	}
	maxDelay := 0
	for _, d := range delays {
		if d > maxDelay {
			maxDelay = d
		}
	}
	history := func(nodes []int) [][]float64 {
		var out [][]float64
		for step := max(0, at-maxDelay); step <= at; step++ {
			row := make([]float64, len(nodes))
			if step > 0 {
				row = chemicalStateValues(r.rawOutput[step-1], nodes)
			}
			out = append(out, row)
		}
		return out
	}
	chemicalStateRows(t, "LIF history", lif.History, history(lifNodes), false)
	if continuous != nil {
		if continuous.Steps != uint64(at) {
			t.Fatal("continuous clock differs")
		}
		chemicalStateRows(t, "continuous voltage", [][]float64{continuous.Voltage}, [][]float64{chemicalStateValues(r.voltage[at], continuousNodes)}, false)
		chemicalStateRows(t, "continuous history", continuous.History, history(continuousNodes), false)
	}
	if s.Chemical == nil || s.Chemical.State.Steps != f.config.Chemistry.Part.State.Steps+uint64(chemicalAt) {
		t.Fatal("chemical clock differs")
	}
	concentration := make([][]float64, len(r.concentration[at]))
	for region, row := range r.concentration[at] {
		for _, v := range row {
			concentration[region] = append(concentration[region], v.v)
		}
	}
	chemicalStateRows(t, "concentration", s.Chemical.State.Concentration, concentration, false)
	if f.source == "controller" {
		controller := f.config.Chemistry.Part.Config.Sources[0].Controller
		width := controller.Inputs.SummaryWindow
		window := make([][]float64, width)
		for i := range window {
			window[i] = []float64{0, 0}
		}
		for step := 0; step < at; step++ {
			copy(window, window[1:])
			row := []float64{0, 0}
			if step > 0 {
				row = chemicalStateValues(r.rawOutput[step-1], []int{0, 1})
			}
			window[width-1] = row
		}
		if len(s.Chemical.SourceStates) != 1 || s.Chemical.SourceStates[0].Controller == nil {
			t.Fatal("controller boundary missing")
		}
		state := s.Chemical.SourceStates[0].Controller
		if state.Filled != min(at, width) {
			t.Fatalf("controller filled=%d want%d", state.Filled, min(at, width))
		}
		chemicalStateRows(t, "controller window", state.Window, window, false)
	} else if s.Chemical.SourceStates != nil {
		t.Fatal("stateless neural source gained controller state")
	}
}

func TestChemicalHomeostasisStateReferenceShape(t *testing.T) {
	f := chemicalStateCases(t)[0]
	chemicalStateReference(t, f)
}

func TestChemicalHomeostasisStateHand(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("enabled%v", enabled), func(t *testing.T) {
			f := mixedReferenceFixture("neural", "normal")
			f.source = "external"
			m := f.config.Mixed
			f.config.Mixed = nil
			f.config.LIF = &dynamics.LIFConfig{Nodes: 2, Sources: m.Sources, Targets: m.Targets, Delays: m.Delays, DT: math.Ln2, TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1, Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}, Homeostasis: &dynamics.LIFHomeostasis{Enabled: enabled, TauRate: 2 * math.Ln2, TargetRate: .25, Eta: 1 / math.Ln2, HMax: .4}}
			f.parameters = learning.Parameters{Core: dynamics.Parameters{Weights: []float64{0, 0, 0, 0}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}}, ThetaRaw: []float64{0, 0}, Encoder: []float64{1, 0, 0, 1}, Readout: []float64{1, 0, 0, 1}}
			f.config.Chemistry = &learning.ChemicalEpisode{Part: learning.ChemicalPart{Config: modulation.ChemistryConfig{
				Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: math.Ln2, Tau: []float64{1}},
				Sources:   []modulation.SourceSpec{{Kind: modulation.SourceExternalTimeline, Channel: 0, Timeline: &modulation.ExternalTimeline{ChannelCount: 1}}},
				Receptors: modulation.Receptors{Records: []modulation.Receptor{{Cells: []int{1}, Signal: "hand", Channel: 0, Status: modulation.StatusHypothesized, Kd: .5, N: 1, Evidence: "hand formula", MeasurementKind: "declared", MappingVersion: "state-hand/v1"}}},
				Effects:   []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, BetaScale: .5, GammaMin: .5, GammaMax: 2, BetaAbsMax: 1}}, Regions: modulation.RegionAssignment{NodeRegion: []int{0, 0}},
			}, State: modulation.ChemistryState{Concentration: [][]float64{{1}}}, Expression: &learning.ExpressionGain{Nodes: []int{1}, Receptor: 0, Scale: 1, Min: .5, Max: 2}}}
			a := chemicalStateIndividual(t, f)
			x := []float64{1, .5, .25, .125}
			v := []float64{-1, 29.0 / 24, 157.0 / 240, 511.0 / 1440}
			if !enabled {
				x = []float64{1, 1.5, .75, .375}
				v = []float64{-1, -1, -9.0 / 20, -71.0 / 360}
			}
			rate, h := []float64{.5, .25, .125, .0625}, []float64{.25, .25, .125, 0}
			inputs := []float64{4, 13.0 / 4, 0, 0}
			c := 1.0
			for step, input := range inputs {
				got := chemicalStateAdvance(t, a, [][]float64{{0, input}}, true)
				c *= .5
				occ := c / (.5 + c)
				chemicalStateRows(t, "hand readout", got, [][]float64{{0, float64(float32(x[step] * (1 + occ)))}}, false)
				s := a.Snapshot()
				st := s.Neural.LIF
				if st == nil || st.Steps != uint64(step+1) || s.Chemical.State.Steps != uint64(step+1) {
					t.Fatal("hand state clock differs")
				}
				chemicalStateRows(t, "hand voltage", [][]float64{st.Voltage}, [][]float64{{0, v[step]}}, false)
				chemicalStateRows(t, "hand concentration", s.Chemical.State.Concentration, [][]float64{{c}}, false)
				previous := 0.0
				if step > 0 {
					previous = x[step-1]
				}
				chemicalStateRows(t, "hand raw history", st.History, [][]float64{{0, previous}, {0, x[step]}}, false)
				if enabled {
					chemicalStateRows(t, "hand rate", [][]float64{st.Rate}, [][]float64{{0, rate[step]}}, false)
					chemicalStateRows(t, "hand homeostasis", [][]float64{st.Homeostasis}, [][]float64{{0, h[step]}}, false)
				} else if st.Rate != nil || st.Homeostasis != nil {
					t.Fatal("disabled hand state has stabiliser arrays")
				}
			}
		})
	}
}

func chemicalStatePartition(mask int) []int {
	var parts []int
	start := 0
	for step := 1; step <= 4; step++ {
		if step == 4 || mask&(1<<(step-1)) != 0 {
			parts = append(parts, step-start)
			start = step
		}
	}
	return parts
}

func TestChemicalHomeostasisStatePublic(t *testing.T) {
	routes, controls := 0, 0
	for _, f := range chemicalStateCases(t) {
		t.Run(f.name, func(t *testing.T) {
			ref := chemicalStateReference(t, f)
			prediction := chemicalStatePrediction(f, ref)
			var on learning.IndividualSnapshot
			for _, gated := range []bool{false, true} {
				for mask := 0; mask < 8; mask++ {
					routes++
					t.Run(fmt.Sprintf("gated%v/partition%d", gated, mask), func(t *testing.T) {
						live, twin := chemicalStateIndividual(t, f), chemicalStateIndividual(t, f)
						optimizer := live.Snapshot().Optimizer
						at := 0
						for _, length := range chemicalStatePartition(mask) {
							end := at + length
							x := chemicalStateAdvance(t, live, f.input[at:end], gated)
							y := chemicalStateAdvance(t, twin, f.input[at:end], gated)
							chemicalStateRows(t, "persistent readout", x, prediction[at:end], true)
							if !reflect.DeepEqual(x, y) {
								t.Fatal("restore twin readout differs")
							}
							s, other := live.Snapshot(), twin.Snapshot()
							chemicalStateSnapshot(t, f, ref, end, s)
							chemicalStateSnapshot(t, f, ref, end, other)
							if !reflect.DeepEqual(s, other) || !reflect.DeepEqual(s.Optimizer, optimizer) {
								t.Fatal("restore twin or optimizer differs")
							}
							var err error
							twin, err = learning.RestoreIndividual(other)
							if err != nil {
								t.Fatal(err)
							}
							if !reflect.DeepEqual(twin.Snapshot(), other) {
								t.Fatal("restore changed snapshot")
							}
							at = end
						}
						if at != 4 {
							t.Fatal("partition did not cover four steps")
						}
						if !gated && mask == 0 {
							on = live.Snapshot()
						}
					})
				}
			}
			if t.Failed() {
				return
			}
			if strings.HasSuffix(f.name, "open-positive") {
				controls++
				off := f
				episode := *f.config.Chemistry
				episode.Part.Expression = nil
				off.config.Chemistry = &episode
				a := chemicalStateIndividual(t, off)
				out := chemicalStateAdvance(t, a, f.input, false)
				expected := on
				part := *on.Chemical
				part.Expression = nil
				expected.Chemical = &part
				if !reflect.DeepEqual(a.Snapshot(), expected) {
					t.Fatal("expression changed persistent state")
				}
				difference := 0.0
				for step, row := range out {
					for node, value := range row {
						difference = math.Max(difference, math.Abs(value-prediction[step][node]))
					}
				}
				if !(difference > 1e-6) {
					t.Fatal("expression state control was unobservable")
				}
				t.Logf("expression state control max_readout_abs=%g", difference)
			}
		})
	}
	if routes != 640 || controls != 8 {
		t.Fatalf("routes/controls=%d/%d want640/8", routes, controls)
	}
}

// A fresh neural source has no previous observed row. Preserve the documented
// refusal; the successful matrix warms the core before enabling chemistry.
func TestChemicalHomeostasisStateFreshNeuralRefusal(t *testing.T) {
	for _, f := range chemicalStateCases(t) {
		if f.source != "neural" {
			continue
		}
		t.Run(f.name, func(t *testing.T) {
			for _, gated := range []bool{false, true} {
				t.Run(fmt.Sprintf("gated%v", gated), func(t *testing.T) {
					a := chemicalStateIndividualWithWarmup(t, f, false)
					before, report := a.Snapshot(), a.ChemistryReport()
					var out [][]float64
					var err error
					if gated {
						var plastic learning.PlasticReport
						out, plastic, err = a.AdvanceGated(context.Background(), f.input[:1], []float64{0})
						if plastic != (learning.PlasticReport{}) {
							t.Fatal("refused fresh source changed plastic report")
						}
					} else {
						out, err = a.Advance(context.Background(), f.input[:1])
					}
					if err == nil || !strings.Contains(err.Error(), "neural source has no activity for step 0") || out != nil {
						t.Fatalf("fresh neural source refusal: out=%v err=%v", out, err)
					}
					if !reflect.DeepEqual(a.Snapshot(), before) || !reflect.DeepEqual(a.ChemistryReport(), report) {
						t.Fatal("refused fresh neural source committed state/report")
					}
				})
			}
		})
	}
}

func TestChemicalHomeostasisStateAtomic(t *testing.T) {
	for _, f := range chemicalStateCases(t) {
		if !strings.HasSuffix(f.name, "open-positive") {
			continue
		}
		t.Run(f.name, func(t *testing.T) {
			ref := chemicalStateReference(t, f)
			prediction := chemicalStatePrediction(f, ref)
			a := chemicalStateIndividual(t, f)
			chemicalStateAdvance(t, a, f.input[:2], true)
			before, report := a.Snapshot(), a.ChemistryReport()
			cancelled, cancel := context.WithCancel(context.Background())
			cancel()
			cases := []struct {
				name  string
				ctx   context.Context
				input [][]float64
				gate  []float64
			}{
				{"empty", context.Background(), nil, []float64{}},
				{"gate-width", context.Background(), f.input[2:], []float64{0}},
				{"open-gate", context.Background(), f.input[2:], []float64{0, 1}},
				{"non-finite", context.Background(), [][]float64{{math.NaN(), 0}, {0, 0}}, []float64{0, 0}},
				{"cancelled", cancelled, f.input[2:], []float64{0, 0}},
			}
			for _, c := range cases {
				out, plastic, err := a.AdvanceGated(c.ctx, c.input, c.gate)
				if err == nil || out != nil || plastic != (learning.PlasticReport{}) {
					t.Fatalf("%s did not refuse atomically: %v %v %v", c.name, out, plastic, err)
				}
				if !reflect.DeepEqual(a.Snapshot(), before) || !reflect.DeepEqual(a.ChemistryReport(), report) {
					t.Fatalf("%s changed state/report", c.name)
				}
			}
			out := chemicalStateAdvance(t, a, f.input[2:], true)
			chemicalStateRows(t, "post-refusal continuation", out, prediction[2:], true)
			chemicalStateSnapshot(t, f, ref, 4, a.Snapshot())
		})
	}
}

const chemicalStateWorkerEnv = "COIMNET_HOMEOSTASIS_STATE_WORKER"

func TestChemicalHomeostasisStateProcessWorker(t *testing.T) {
	if os.Getenv(chemicalStateWorkerEnv) != "1" {
		t.Skip("subprocess entrypoint; exercised by FreshProcess")
	}
	index, err := strconv.Atoi(os.Getenv("COIMNET_HOMEOSTASIS_STATE_CASE"))
	if err != nil {
		t.Fatal(err)
	}
	split, err := strconv.Atoi(os.Getenv("COIMNET_HOMEOSTASIS_STATE_SPLIT"))
	if err != nil {
		t.Fatal(err)
	}
	cases := chemicalStateCases(t)
	if index < 0 || index >= len(cases) || (split != 1 && split != 3) {
		t.Fatal("bad subprocess case or split")
	}
	f := cases[index]
	ref := chemicalStateReference(t, f)
	prediction := chemicalStatePrediction(f, ref)
	s, err := checkpoint.LoadIndividual(context.Background(), os.Getenv("COIMNET_HOMEOSTASIS_STATE_INPUT"))
	if err != nil {
		t.Fatal(err)
	}
	chemicalStateSnapshot(t, f, ref, split, s)
	a, err := learning.RestoreIndividual(s)
	if err != nil {
		t.Fatal(err)
	}
	out := chemicalStateAdvance(t, a, f.input[split:], true)
	chemicalStateRows(t, "process readout", out, prediction[split:], true)
	chemicalStateSnapshot(t, f, ref, 4, a.Snapshot())
	if err = checkpoint.SaveIndividual(context.Background(), os.Getenv("COIMNET_HOMEOSTASIS_STATE_OUTPUT"), a.Snapshot()); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(os.Getenv("COIMNET_HOMEOSTASIS_STATE_READOUT"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.NewEncoder(file).Encode(out); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestChemicalHomeostasisStateFreshProcess(t *testing.T) {
	count := 0
	for index, f := range chemicalStateCases(t) {
		if !strings.HasSuffix(f.name, "open-positive") {
			continue
		}
		for _, split := range []int{1, 3} {
			count++
			t.Run(fmt.Sprintf("%s/split%d", f.name, split), func(t *testing.T) {
				ref := chemicalStateReference(t, f)
				prediction := chemicalStatePrediction(f, ref)
				a := chemicalStateIndividual(t, f)
				chemicalStateAdvance(t, a, f.input[:split], true)
				chemicalStateSnapshot(t, f, ref, split, a.Snapshot())
				dir := t.TempDir()
				input := filepath.Join(dir, "input.json")
				output := filepath.Join(dir, "output.json")
				readout := filepath.Join(dir, "readout.json")
				if err := checkpoint.SaveIndividual(context.Background(), input, a.Snapshot()); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestChemicalHomeostasisStateProcessWorker$", "-test.v")
				cmd.Env = append(os.Environ(), chemicalStateWorkerEnv+"=1", "COIMNET_HOMEOSTASIS_STATE_CASE="+strconv.Itoa(index), "COIMNET_HOMEOSTASIS_STATE_SPLIT="+strconv.Itoa(split), "COIMNET_HOMEOSTASIS_STATE_INPUT="+input, "COIMNET_HOMEOSTASIS_STATE_OUTPUT="+output, "COIMNET_HOMEOSTASIS_STATE_READOUT="+readout)
				if log, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("fresh process: %v\n%s", err, log)
				}
				s, err := checkpoint.LoadIndividual(context.Background(), output)
				if err != nil {
					t.Fatal(err)
				}
				chemicalStateSnapshot(t, f, ref, 4, s)
				same := chemicalStateAdvance(t, a, f.input[split:], true)
				if !reflect.DeepEqual(a.Snapshot(), s) {
					t.Fatal("fresh process snapshot differs from uninterrupted continuation")
				}
				raw, err := os.ReadFile(readout)
				if err != nil {
					t.Fatal(err)
				}
				var out [][]float64
				if err = json.Unmarshal(raw, &out); err != nil {
					t.Fatal(err)
				}
				chemicalStateRows(t, "saved process readout", out, prediction[split:], true)
				if !reflect.DeepEqual(same, out) {
					t.Fatal("fresh process readout differs from uninterrupted continuation")
				}
				t.Logf("fresh-process case=%s split=%d steps=4", f.name, split)
			})
		}
	}
	if count != 16 {
		t.Fatalf("fresh processes%d want16", count)
	}
}
