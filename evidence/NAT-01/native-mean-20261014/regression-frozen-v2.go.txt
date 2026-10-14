package simulate

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/dynamics"
)

func meanRangeProtocol(dt, bias float64, probes []Probe) Protocol {
	return Protocol{SchemaVersion: ProtocolSchemaVersion, Core: CoreContinuous,
		Continuous: &dynamics.Config{DT: dt, Activation: "softplus"},
		Injections: []Injection{{Channel: 0, Node: 0, Gain: 0}}, Probes: probes,
		Stimulus:        StimulusSpec{Pulse: &Pulse{Channels: 1, Steps: 1, Channel: 0, Onset: 0, Duration: 1, Amplitude: 0}},
		ParameterSource: ParameterSourceUniform, Uniform: &UniformParameters{Gain: 0, Bias: bias, LogTau: 0}}
}

func meanRangeBuild(t *testing.T, g *connectome.Graph, p Protocol, voltage []float64) (*Runner, ParameterSet) {
	t.Helper()
	params := mustParameters(t, g, *p.Uniform)
	runner, err := Build(context.Background(), g, params, p, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := runner.State()
	if voltage != nil {
		copy(snapshot.Continuous.Voltage, voltage)
		latest := snapshot.Continuous.History[len(snapshot.Continuous.History)-1]
		for i, v := range voltage {
			latest[i] = math.Max(v, 0) + math.Log1p(math.Exp(-math.Abs(v)))
		}
	}
	if err := runner.RestoreState(snapshot); err != nil {
		t.Fatal(err)
	}
	return runner, params
}

func preciseMeanRange(values []float64) float64 {
	total := new(big.Float).SetPrec(256)
	for _, v := range values {
		total.Add(total, new(big.Float).SetPrec(256).SetFloat64(v))
	}
	total.Quo(total, new(big.Float).SetPrec(256).SetInt64(int64(len(values))))
	v, _ := total.Float64()
	return v
}

func TestNativeMeanRepresentableRange(t *testing.T) {
	g := fixtureGraph(t)
	cases := []struct {
		name     string
		dt, bias float64
		voltage  []float64
		nodes    []int
		selector *Selector
	}{
		{"large_equal", 1, math.MaxFloat64, nil, []int{0, 1, 2}, nil},
		{"upper_equal", 1000, math.MaxFloat64, nil, []int{0, 1, 2}, nil},
		{"large_heterogeneous", 1, math.MaxFloat64, []float64{0, math.MaxFloat64 / 4, math.MaxFloat64 / 2}, []int{0, 1, 2}, nil},
		{"ascending_subset", 1, math.MaxFloat64, nil, []int{1, 2}, nil},
		{"selector", 1, math.MaxFloat64, nil, nil, &Selector{Field: "class", Equals: "ALPN"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			controls := []Probe{{Name: "n0", Nodes: []int{0}, Reduce: ReduceMeanOutput}, {Name: "n1", Nodes: []int{1}, Reduce: ReduceMeanOutput}, {Name: "n2", Nodes: []int{2}, Reduce: ReduceMeanOutput}}
			p := meanRangeProtocol(tc.dt, tc.bias, controls)
			control, _ := meanRangeBuild(t, g, p, tc.voltage)
			reference, err := control.Run(context.Background(), control.Stimulus())
			if err != nil {
				t.Fatal(err)
			}
			ids := tc.nodes
			if tc.selector != nil {
				ids = []int{1, 2}
			}
			values := make([]float64, len(ids))
			for i, id := range ids {
				values[i] = reference.Probes[id].Series[0]
			}
			want := preciseMeanRange(values)
			if !finite(want) {
				t.Fatal("independent mean is not finite")
			}
			if tc.name == "upper_equal" && want != math.MaxFloat64 {
				t.Fatal("upper fixture did not reach MaxFloat64")
			}
			p.Probes = []Probe{{Name: "mean", Nodes: tc.nodes, Selector: tc.selector, Reduce: ReduceMeanOutput}}
			runner, params := meanRangeBuild(t, g, p, tc.voltage)
			inputs := nativeReceptorJSON(t, []any{p, params})
			report, err := runner.Run(context.Background(), runner.Stimulus())
			if err != nil {
				t.Fatalf("finite mean %g was refused: %v", want, err)
			}
			got := report.Probes[0].Series[0]
			if !finite(got) || math.Abs(got/want-1) > 2e-15 {
				t.Fatalf("mean=%g independent=%g", got, want)
			}
			if tc.name == "upper_equal" && got != math.MaxFloat64 {
				t.Fatalf("upper mean=%g", got)
			}
			if !bytes.Equal(nativeReceptorJSON(t, runner.State()), nativeReceptorJSON(t, control.State())) {
				t.Fatal("mean reduction altered neural dynamics")
			}
			if !bytes.Equal(inputs, nativeReceptorJSON(t, []any{p, params})) {
				t.Fatal("Run mutated caller inputs")
			}
			if _, err := json.Marshal(report); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNativeSumTrueOverflowDoesNotCommit(t *testing.T) {
	g := fixtureGraph(t)
	p := meanRangeProtocol(1, math.MaxFloat64, []Probe{{Name: "sum", Nodes: []int{0, 1, 2}, Reduce: ReduceSumOutput}})
	runner, params := meanRangeBuild(t, g, p, nil)
	before := nativeReceptorJSON(t, []any{runner.State(), runner.PlasticState(), runner.Measurements(), p, params})
	report, err := runner.Run(context.Background(), runner.Stimulus())
	if err == nil || !strings.Contains(err.Error(), "non-finite") || !reflect.DeepEqual(report, RunReport{}) {
		t.Fatalf("overflow report/error=%+v/%v", report, err)
	}
	if !bytes.Equal(before, nativeReceptorJSON(t, []any{runner.State(), runner.PlasticState(), runner.Measurements(), p, params})) {
		t.Fatal("refusal committed state or inputs")
	}
}

func TestNativeTrackedSumMustRemainEncodable(t *testing.T) {
	g := fixtureGraph(t)
	for _, core := range []string{CoreContinuous, CoreLIF} {
		t.Run(core, func(t *testing.T) {
			p := meanRangeProtocol(1, math.MaxFloat64, []Probe{{Name: "one", Nodes: []int{0}, Reduce: ReduceMeanOutput}})
			if core == CoreLIF {
				p = lifProtocol(1)
				p.Probes = []Probe{{Name: "one", Nodes: []int{0}, Reduce: ReduceMeanOutput}}
			}
			var initial []float64
			if core == CoreContinuous {
				initial = []float64{-math.MaxFloat64, -math.MaxFloat64, -math.MaxFloat64}
			}
			runner, params := meanRangeBuild(t, g, p, initial)
			sets, err := ResolveSets(context.Background(), g, metricSets())
			if err != nil {
				t.Fatal(err)
			}
			if err := runner.TrackSets(sets, [][2]int{{0, 1}}); err != nil {
				t.Fatal(err)
			}
			if core == CoreLIF {
				if _, err := runner.Run(context.Background(), runner.Stimulus()); err != nil {
					t.Fatal(err)
				}
				if _, err := json.Marshal(runner.Measurements()); err != nil {
					t.Fatal(err)
				}
				return
			}
			// The first finite-output run records an encodable true sum. Its
			// positive continuation makes the next tracked sum overflow.
			if _, err := runner.Run(context.Background(), runner.Stimulus()); err != nil {
				t.Fatal(err)
			}
			if _, err := json.Marshal(runner.Measurements()); err != nil {
				t.Fatal(err)
			}
			before := nativeReceptorJSON(t, []any{runner.State(), runner.PlasticState(), runner.Measurements(), p, params})
			report, err := runner.Run(context.Background(), runner.Stimulus())
			if err == nil || !strings.Contains(err.Error(), "output sum") || !reflect.DeepEqual(report, RunReport{}) {
				t.Fatalf("unencodable measurement report/error=%+v/%v", report, err)
			}
			if !bytes.Equal(before, nativeReceptorJSON(t, []any{runner.State(), runner.PlasticState(), runner.Measurements(), p, params})) {
				t.Fatal("late measurement refusal changed persistent values")
			}
		})
	}
}

func TestNativeMeanOrdinaryAndEmpty(t *testing.T) {
	g := fixtureGraph(t)
	for _, core := range []string{CoreContinuous, CoreLIF} {
		p := meanRangeProtocol(1, .25, []Probe{{Name: "empty", Selector: &Selector{Field: "class", Equals: "missing", AllowEmpty: true}, Reduce: ReduceMeanOutput}, {Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceMeanOutput}})
		if core == CoreLIF {
			p = lifProtocol(1)
			p.Probes = []Probe{{Name: "empty", Selector: &Selector{Field: "class", Equals: "missing", AllowEmpty: true}, Reduce: ReduceMeanOutput}, {Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceMeanOutput}}
		}
		t.Run(core, func(t *testing.T) {
			runner, _ := meanRangeBuild(t, g, p, nil)
			report, err := runner.Run(context.Background(), runner.Stimulus())
			if err != nil {
				t.Fatal(err)
			}
			if report.Probes[0].Series[0] != 0 || !finite(report.Probes[1].Series[0]) {
				t.Fatal("ordinary or empty mean drift")
			}
		})
	}
}
