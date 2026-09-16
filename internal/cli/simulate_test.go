package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/plasticity"
	"github.com/TimLai666/coimnet/simulate"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
)

// simulateStore builds the shared two-neuron import fixture into a graph store.
// Node IDs 1 and 2 become indices 0 and 1; the edges are 0->1 with raw weight 5
// and 1->0 with raw weight 6.
func simulateStore(t *testing.T) (dir, store string) {
	t.Helper()
	dir = importFixture(t)
	store = filepath.Join(dir, "graph.coimgraph")
	var out, errout bytes.Buffer
	args := []string{"data", "import", "--manifest", filepath.Join(dir, "manifest.json"), "--temp-dir", t.TempDir(), "--out-store", store}
	if err := Run(context.Background(), args, &out, &errout); err != nil {
		t.Fatalf("data import: %v; stderr=%s", err, errout.String())
	}
	return dir, store
}

func lifSimulateProtocol(steps int) simulate.Protocol {
	inline := make([][]float64, steps)
	for t := range inline {
		inline[t] = []float64{0}
	}
	inline[0][0] = 2
	return simulate.Protocol{
		SchemaVersion: simulate.ProtocolSchemaVersion,
		Core:          simulate.CoreLIF,
		LIF: &dynamics.LIFConfig{
			DT: 1, TauSyn: 1, ThetaMin: .5, ThetaMax: 1.5, VReset: -.5,
			Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		},
		Injections: []simulate.Injection{{Channel: 0, Node: 0, Gain: 1}},
		Probes: []simulate.Probe{
			{Name: "driven", Nodes: []int{0}, Reduce: simulate.ReduceSpikeFraction},
			{Name: "population", Nodes: []int{0, 1}, Reduce: simulate.ReduceMeanOutput},
		},
		Stimulus:        simulate.StimulusSpec{Inline: inline},
		ParameterSource: simulate.ParameterSourceUniform,
		Uniform:         &simulate.UniformParameters{Gain: .2},
	}
}

func writeProtocol(t *testing.T, dir, name string, protocol simulate.Protocol) string {
	t.Helper()
	encoded, err := json.MarshalIndent(protocol, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type simulateOutput struct {
	SchemaVersion   string `json:"schema_version"`
	Core            string `json:"core"`
	CoreConfigHash  string `json:"core_config_hash"`
	ParameterSource string `json:"parameter_source"`
	ParameterHash   string `json:"parameter_hash"`
	// The five derived fields are absent from a uniform report.
	ParameterSetSHA256 string  `json:"parameter_set_sha256"`
	RulesHash          string  `json:"rules_hash"`
	UnknownSignPolicy  string  `json:"unknown_sign_policy"`
	UnknownSignEdges   *uint64 `json:"unknown_sign_edges"`
	WeightScale        float64 `json:"weight_scale"`
	ProtocolHash       string  `json:"protocol_hash"`
	Steps              int     `json:"steps"`
	StepsBefore        uint64  `json:"steps_before"`
	StepsAfter         uint64  `json:"steps_after"`
	Nodes              int     `json:"nodes"`
	Edges              int     `json:"edges"`
	GraphHashes        struct {
		NodeIndex string `json:"node_index"`
		EdgeOrder string `json:"edge_order"`
	} `json:"graph_hashes"`
	Injections []struct {
		Channel   int     `json:"channel"`
		Gain      float64 `json:"gain"`
		NodeCount int     `json:"node_count"`
		Selector  *struct {
			Field      string `json:"field"`
			Equals     string `json:"equals"`
			Count      int    `json:"count"`
			FirstIndex int    `json:"first_index"`
		} `json:"selector"`
	} `json:"injections"`
	Probes []struct {
		Name      string    `json:"name"`
		Reduce    string    `json:"reduce"`
		NodeCount int       `json:"node_count"`
		Series    []float64 `json:"series"`
		Selector  *struct {
			Field string `json:"field"`
			Count int    `json:"count"`
		} `json:"selector"`
	} `json:"probes"`
	Monitors struct {
		SilentFraction        float64    `json:"silent_fraction"`
		PopulationRatePerStep []float64  `json:"population_rate_per_step"`
		RateQuantiles         [5]float64 `json:"rate_quantiles"`
		NonFinite             bool       `json:"non_finite"`
	} `json:"monitors"`
	StabilityFlags []string `json:"stability_flags"`
	Assumptions    []string `json:"assumptions"`
	Plasticity     *struct {
		Rule                 string  `json:"rule"`
		EnabledEdges         int     `json:"enabled_edges"`
		GateChannel          int     `json:"gate_channel"`
		Frozen               bool    `json:"frozen"`
		ClampedByWMin        uint64  `json:"clamped_by_w_min"`
		ClampedByPlasticMax  uint64  `json:"clamped_by_plastic_max"`
		PlasticL2Before      float64 `json:"plastic_l2_before"`
		PlasticL2After       float64 `json:"plastic_l2_after"`
		ChunkSize            int     `json:"chunk_size"`
		WallClockPenaltyNote string  `json:"wall_clock_penalty_note"`
	} `json:"plasticity"`
}

func runSimulate(t *testing.T, args ...string) (simulateOutput, string) {
	t.Helper()
	var out, errout bytes.Buffer
	if err := Run(context.Background(), args, &out, &errout); err != nil {
		t.Fatalf("%v: %v; stderr=%s", args, err, errout.String())
	}
	var decoded simulateOutput
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid run report JSON: %v\n%s", err, out.String())
	}
	return decoded, out.String()
}

func probeByName(t *testing.T, report simulateOutput, name string) []float64 {
	t.Helper()
	for _, probe := range report.Probes {
		if probe.Name == name {
			return probe.Series
		}
	}
	t.Fatalf("report has no probe %q", name)
	return nil
}

func TestSimulateRunReportsTheSpikingCoreOnAStore(t *testing.T) {
	dir, store := simulateStore(t)
	protocol := writeProtocol(t, dir, "lif.json", lifSimulateProtocol(4))
	report, raw := runSimulate(t, "simulate", "run", "--store", store, "--protocol", protocol)
	if report.SchemaVersion != "coimnet-simulate-run/v1" || report.Core != "lif" {
		t.Fatalf("report identity = %s", raw)
	}
	if report.Steps != 4 || report.StepsBefore != 0 || report.StepsAfter != 4 || report.Nodes != 2 || report.Edges != 2 {
		t.Fatalf("report shape = %s", raw)
	}
	if report.ParameterSource != "engineering_uniform_positive" || len(report.ParameterHash) != 64 || len(report.ProtocolHash) != 64 || len(report.CoreConfigHash) != 64 {
		t.Fatalf("report identity = %s", raw)
	}
	if len(report.GraphHashes.NodeIndex) != 64 || len(report.GraphHashes.EdgeOrder) != 64 {
		t.Fatalf("graph hashes = %+v", report.GraphHashes)
	}
	if len(report.Injections) != 1 || report.Injections[0].NodeCount != 1 || report.Injections[0].Gain != 1 {
		t.Fatalf("injections = %+v", report.Injections)
	}
	// The pulse drives node 0 above the threshold exactly once, at step 0.
	if want := []float64{1, 0, 0, 0}; !sameSeries(probeByName(t, report, "driven"), want) {
		t.Fatalf("driven probe = %v, want %v", probeByName(t, report, "driven"), want)
	}
	if len(report.Monitors.PopulationRatePerStep) != 4 || report.Monitors.NonFinite {
		t.Fatalf("monitors = %+v", report.Monitors)
	}
	if report.Monitors.SilentFraction != .5 {
		t.Fatalf("silent fraction = %v; only node 0 spikes", report.Monitors.SilentFraction)
	}
	if len(report.StabilityFlags) != 0 {
		t.Fatalf("stability flags = %v", report.StabilityFlags)
	}
	if !strings.Contains(strings.Join(report.Assumptions, " "), "not a biological parameter set") {
		t.Fatalf("assumptions = %v", report.Assumptions)
	}

	_, again := runSimulate(t, "simulate", "run", "--store", store, "--protocol", protocol)
	if again != raw {
		t.Fatalf("two identical runs produced different output:\n%s\n%s", raw, again)
	}
}

func TestSimulateRunReportsTheContinuousCore(t *testing.T) {
	dir, store := simulateStore(t)
	p := lifSimulateProtocol(4)
	p.Core = simulate.CoreContinuous
	p.LIF = nil
	p.Continuous = &dynamics.Config{DT: 1, Activation: "tanh"}
	p.Probes = []simulate.Probe{{Name: "population", Nodes: []int{0, 1}, Reduce: simulate.ReduceSumOutput}}
	protocol := writeProtocol(t, dir, "continuous.json", p)
	report, raw := runSimulate(t, "simulate", "run", "--store", store, "--protocol", protocol)
	if report.Core != "continuous" || report.Steps != 4 {
		t.Fatalf("report = %s", raw)
	}
	if len(report.Monitors.PopulationRatePerStep) != 0 || report.Monitors.RateQuantiles != [5]float64{} {
		t.Fatalf("continuous monitors = %+v", report.Monitors)
	}
	if len(probeByName(t, report, "population")) != 4 {
		t.Fatalf("continuous probe = %s", raw)
	}
}

func TestSimulateRunSavesAndContinuesState(t *testing.T) {
	dir, store := simulateStore(t)
	whole := writeProtocol(t, dir, "whole.json", lifSimulateProtocol(4))
	single, _ := runSimulate(t, "simulate", "run", "--store", store, "--protocol", whole)

	firstHalf := lifSimulateProtocol(2)
	secondHalf := lifSimulateProtocol(2)
	secondHalf.Stimulus = simulate.StimulusSpec{Inline: [][]float64{{0}, {0}}}
	firstPath := writeProtocol(t, dir, "first.json", firstHalf)
	secondPath := writeProtocol(t, dir, "second.json", secondHalf)
	statePath := filepath.Join(dir, "state.json")

	partA, _ := runSimulate(t, "simulate", "run", "--store", store, "--protocol", firstPath, "--state-out", statePath)
	if partA.StepsAfter != 2 {
		t.Fatalf("first half steps = %d", partA.StepsAfter)
	}
	stateBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		SchemaVersion string `json:"schema_version"`
		Core          string `json:"core"`
		LIF           *struct {
			Steps uint64 `json:"steps"`
		} `json:"lif"`
	}
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		t.Fatalf("invalid state JSON: %v\n%s", err, stateBytes)
	}
	if state.SchemaVersion != "coimnet-simulate-state/v1" || state.Core != "lif" || state.LIF == nil || state.LIF.Steps != 2 {
		t.Fatalf("state = %s", stateBytes)
	}

	partB, _ := runSimulate(t, "simulate", "run", "--store", store, "--protocol", secondPath, "--state-in", statePath)
	if partB.StepsBefore != 2 || partB.StepsAfter != 4 {
		t.Fatalf("continued steps = %d..%d", partB.StepsBefore, partB.StepsAfter)
	}
	for _, name := range []string{"driven", "population"} {
		joined := append(append([]float64(nil), probeByName(t, partA, name)...), probeByName(t, partB, name)...)
		if want := probeByName(t, single, name); !sameSeries(joined, want) {
			t.Fatalf("probe %q split = %v, single = %v", name, joined, want)
		}
	}
	joinedRate := append(append([]float64(nil), partA.Monitors.PopulationRatePerStep...), partB.Monitors.PopulationRatePerStep...)
	if !sameSeries(joinedRate, single.Monitors.PopulationRatePerStep) {
		t.Fatalf("split population rate = %v, single = %v", joinedRate, single.Monitors.PopulationRatePerStep)
	}
	// An existing --state-out path must never be overwritten.
	var out, errout bytes.Buffer
	if err := Run(context.Background(), []string{"simulate", "run", "--store", store, "--protocol", firstPath, "--state-out", statePath}, &out, &errout); err == nil {
		t.Fatal("an existing state file was overwritten")
	}
}

func TestSimulateRunResolvesSelectorsAndReportsThem(t *testing.T) {
	dir, store := simulateStore(t)
	// The import fixture maps no class column, so every annotation field is
	// null. An allow_empty selector must still be resolved and reported.
	p := lifSimulateProtocol(2)
	p.Probes = append(p.Probes, simulate.Probe{
		Name:     "unlabeled",
		Selector: &simulate.Selector{Field: "class", Equals: "ALIN", AllowEmpty: true},
		Reduce:   simulate.ReduceSpikeFraction,
	})
	protocol := writeProtocol(t, dir, "selector.json", p)
	report, raw := runSimulate(t, "simulate", "run", "--store", store, "--protocol", protocol)
	var found bool
	for _, probe := range report.Probes {
		if probe.Name != "unlabeled" {
			continue
		}
		found = true
		if probe.NodeCount != 0 || probe.Selector == nil || probe.Selector.Field != "class" || probe.Selector.Count != 0 {
			t.Fatalf("selector probe = %+v", probe)
		}
	}
	if !found {
		t.Fatalf("selector probe missing from %s", raw)
	}

	strict := lifSimulateProtocol(2)
	strict.Probes = append(strict.Probes, simulate.Probe{
		Name: "typo", Selector: &simulate.Selector{Field: "hemilineage", Equals: "x"}, Reduce: simulate.ReduceSpikeFraction,
	})
	strictPath := writeProtocol(t, dir, "strict-selector.json", strict)
	var out, errout bytes.Buffer
	if err := Run(context.Background(), []string{"simulate", "run", "--store", store, "--protocol", strictPath}, &out, &errout); err == nil {
		t.Fatal("an unknown selector field was accepted")
	}
}

func TestSimulateRunFailuresAndHelp(t *testing.T) {
	dir, store := simulateStore(t)
	valid := writeProtocol(t, dir, "valid.json", lifSimulateProtocol(2))

	foreignSource := lifSimulateProtocol(2)
	foreignSource.ParameterSource = "derived_from_release"
	foreign := writeProtocol(t, dir, "foreign.json", foreignSource)

	continuousSpikes := lifSimulateProtocol(2)
	continuousSpikes.Core = simulate.CoreContinuous
	continuousSpikes.LIF = nil
	continuousSpikes.Continuous = &dynamics.Config{DT: 1, Activation: "tanh"}
	spikeProbe := writeProtocol(t, dir, "continuous-spikes.json", continuousSpikes)

	absurd := lifSimulateProtocol(2)
	absurd.Injections = []simulate.Injection{{Channel: 0, Node: 0, Gain: 1e308}}
	absurdPath := writeProtocol(t, dir, "absurd.json", absurd)

	malformed := filepath.Join(dir, "malformed.json")
	if err := os.WriteFile(malformed, []byte(`{"schema_version":"coimnet-simulate-protocol/v1",}`), 0o600); err != nil {
		t.Fatal(err)
	}
	unknownField := filepath.Join(dir, "unknown-field.json")
	if err := os.WriteFile(unknownField, []byte(`{"schema_version":"coimnet-simulate-protocol/v1","core":"lif","surprise":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	existingState := filepath.Join(dir, "existing-state.json")
	if err := os.WriteFile(existingState, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	for name, args := range map[string][]string{
		"no flags":             {"simulate", "run"},
		"no store":             {"simulate", "run", "--protocol", valid},
		"no protocol":          {"simulate", "run", "--store", store},
		"missing store":        {"simulate", "run", "--store", filepath.Join(dir, "absent.coimgraph"), "--protocol", valid},
		"missing protocol":     {"simulate", "run", "--store", store, "--protocol", filepath.Join(dir, "absent.json")},
		"malformed protocol":   {"simulate", "run", "--store", store, "--protocol", malformed},
		"unknown field":        {"simulate", "run", "--store", store, "--protocol", unknownField},
		"foreign source":       {"simulate", "run", "--store", store, "--protocol", foreign},
		"spikes on continuous": {"simulate", "run", "--store", store, "--protocol", spikeProbe},
		"absurd gain":          {"simulate", "run", "--store", store, "--protocol", absurdPath},
		"tiny memory":          {"simulate", "run", "--store", store, "--protocol", valid, "--max-memory-bytes", "16"},
		"tiny store limit":     {"simulate", "run", "--store", store, "--protocol", valid, "--max-store-bytes", "16"},
		"existing state out":   {"simulate", "run", "--store", store, "--protocol", valid, "--state-out", existingState},
		"missing state in":     {"simulate", "run", "--store", store, "--protocol", valid, "--state-in", filepath.Join(dir, "absent-state.json")},
		"positional argument":  {"simulate", "run", "--store", store, "--protocol", valid, "extra"},
		"unknown flag":         {"simulate", "run", "--unknown"},
		"unknown subcommand":   {"simulate", "sweep"},
	} {
		var out, errout bytes.Buffer
		if err := Run(context.Background(), args, &out, &errout); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	// A state file written for a different core must be refused, not coerced.
	statePath := filepath.Join(dir, "lif-state.json")
	runSimulate(t, "simulate", "run", "--store", store, "--protocol", valid, "--state-out", statePath)
	continuous := lifSimulateProtocol(2)
	continuous.Core = simulate.CoreContinuous
	continuous.LIF = nil
	continuous.Continuous = &dynamics.Config{DT: 1, Activation: "tanh"}
	continuous.Probes = []simulate.Probe{{Name: "population", Nodes: []int{0, 1}, Reduce: simulate.ReduceSumOutput}}
	continuousPath := writeProtocol(t, dir, "continuous-restore.json", continuous)
	var out, errout bytes.Buffer
	if err := Run(context.Background(), []string{"simulate", "run", "--store", store, "--protocol", continuousPath, "--state-in", statePath}, &out, &errout); err == nil {
		t.Fatal("a spiking state was restored into the continuous core")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	out.Reset()
	if err := Run(canceled, []string{"simulate", "run", "--store", store, "--protocol", valid}, &out, &errout); err == nil {
		t.Fatal("a canceled context produced a report")
	}

	out.Reset()
	if err := Run(context.Background(), []string{"simulate", "run", "--help"}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"--store", "--protocol", "state-in", "state-out", "max-memory-bytes", "max-store-bytes", "max-footer-bytes", "Example:", "Errors:", "Options:", "engineering_uniform_positive"} {
		if !strings.Contains(out.String(), word) {
			t.Fatalf("simulate run help missing %s:\n%s", word, out.String())
		}
	}
	out.Reset()
	if err := Run(context.Background(), []string{"simulate"}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "simulate run") {
		t.Fatalf("simulate usage = %s", out.String())
	}
	out.Reset()
	if err := Run(context.Background(), []string{"--help"}, &out, &errout); err != nil || !strings.Contains(out.String(), "simulate run") {
		t.Fatalf("overview missing simulate run: %v\n%s", err, out.String())
	}
}

func TestSimulateRunReportsStabilityFlagsWithoutFailing(t *testing.T) {
	dir, store := simulateStore(t)
	p := lifSimulateProtocol(4)
	p.Thresholds = simulate.Thresholds{MaxPopulationRate: .1, MinActiveFraction: .9}
	protocol := writeProtocol(t, dir, "flagged.json", p)
	report, raw := runSimulate(t, "simulate", "run", "--store", store, "--protocol", protocol)
	flags := strings.Join(report.StabilityFlags, ",")
	if !strings.Contains(flags, "max_population_rate_exceeded") || !strings.Contains(flags, "min_active_fraction_below_threshold") {
		t.Fatalf("stability flags = %v in %s", report.StabilityFlags, raw)
	}
}

func sameSeries(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// derivedSimulateProtocol drives the same two-neuron store from the derived
// parameter source. The uniform block carries only the node scalars: its gain
// must stay zero, because the edge strengths come from the parameter set.
func derivedSimulateProtocol(steps int, policy string, scale float64) simulate.Protocol {
	p := lifSimulateProtocol(steps)
	p.ParameterSource = simulate.ParameterSourceDerived
	p.Uniform = &simulate.UniformParameters{}
	p.Derived = &simulate.DerivedParameters{UnknownSign: policy, WeightScale: scale}
	return p
}

// deriveParameterSet runs data derive over the derivation fixture and returns
// the parameter set path with its SHA-256.
func deriveParameterSet(t *testing.T, dir, store, rules, name string) (string, string) {
	t.Helper()
	out := filepath.Join(dir, name)
	var stdout, stderr bytes.Buffer
	args := []string{"data", "derive", "--store", store, "--rules", rules, "--out", out, "--temp-dir", t.TempDir()}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("data derive: %v; stderr=%s", err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return out, hex.EncodeToString(sum[:])
}

func TestSimulateRunAppliesADerivedParameterSet(t *testing.T) {
	dir, store, rules := deriveFixture(t)
	params, sum := deriveParameterSet(t, dir, store, rules, "params.coimparams")
	protocol := writeProtocol(t, dir, "derived.json", derivedSimulateProtocol(4, "exclude", 2))
	report, raw := runSimulate(t, "simulate", "run", "--store", store, "--protocol", protocol, "--params", params)

	if report.ParameterSource != "derived_release/v1" || len(report.ParameterHash) != 64 {
		t.Fatalf("report identity = %s", raw)
	}
	if report.ParameterSetSHA256 != sum || len(report.RulesHash) != 64 {
		t.Fatalf("report provenance = %s", raw)
	}
	if report.UnknownSignPolicy != "exclude" || report.WeightScale != 2 {
		t.Fatalf("report policy = %s", raw)
	}
	// Both fixture edges carry a derived sign, so the count is present and zero.
	if report.UnknownSignEdges == nil || *report.UnknownSignEdges != 0 {
		t.Fatalf("report unknown_sign_edges = %v in %s", report.UnknownSignEdges, raw)
	}
	assumptions := strings.Join(report.Assumptions, " ")
	for _, phrase := range []string{"transmitter", "exclude", "bias", "log_tau", "theta_raw"} {
		if !strings.Contains(assumptions, phrase) {
			t.Fatalf("assumptions do not mention %q: %v", phrase, report.Assumptions)
		}
	}
	if strings.Contains(assumptions, "engineering_uniform_positive") {
		t.Fatalf("a derived run still claims the uniform source: %v", report.Assumptions)
	}

	_, again := runSimulate(t, "simulate", "run", "--store", store, "--protocol", protocol, "--params", params)
	if again != raw {
		t.Fatalf("two identical derived runs produced different output:\n%s\n%s", raw, again)
	}

	// A uniform run over the same store must differ: the derived set makes the
	// second edge inhibitory, which the uniform source cannot express.
	uniform := writeProtocol(t, dir, "uniform-compare.json", lifSimulateProtocol(4))
	uniformReport, _ := runSimulate(t, "simulate", "run", "--store", store, "--protocol", uniform)
	if uniformReport.ParameterHash == report.ParameterHash {
		t.Fatal("the derived and uniform parameter sets share a hash")
	}
	if uniformReport.ParameterSetSHA256 != "" || uniformReport.RulesHash != "" ||
		uniformReport.UnknownSignPolicy != "" || uniformReport.WeightScale != 0 || uniformReport.UnknownSignEdges != nil {
		t.Fatalf("a uniform report carries derived fields: %+v", uniformReport)
	}
}

func TestSimulateRunContinuesADerivedRunFromAState(t *testing.T) {
	dir, store, rules := deriveFixture(t)
	params, _ := deriveParameterSet(t, dir, store, rules, "params.coimparams")
	whole := writeProtocol(t, dir, "derived-whole.json", derivedSimulateProtocol(4, "excitatory", 1.5))
	single, _ := runSimulate(t, "simulate", "run", "--store", store, "--protocol", whole, "--params", params)

	firstHalf := derivedSimulateProtocol(2, "excitatory", 1.5)
	secondHalf := derivedSimulateProtocol(2, "excitatory", 1.5)
	secondHalf.Stimulus = simulate.StimulusSpec{Inline: [][]float64{{0}, {0}}}
	firstPath := writeProtocol(t, dir, "derived-first.json", firstHalf)
	secondPath := writeProtocol(t, dir, "derived-second.json", secondHalf)
	statePath := filepath.Join(dir, "derived-state.json")

	partA, _ := runSimulate(t, "simulate", "run", "--store", store, "--protocol", firstPath, "--params", params, "--state-out", statePath)
	partB, _ := runSimulate(t, "simulate", "run", "--store", store, "--protocol", secondPath, "--params", params, "--state-in", statePath)
	if partA.StepsAfter != 2 || partB.StepsBefore != 2 || partB.StepsAfter != 4 {
		t.Fatalf("split derived steps = %d, %d..%d", partA.StepsAfter, partB.StepsBefore, partB.StepsAfter)
	}
	for _, name := range []string{"driven", "population"} {
		joined := append(append([]float64(nil), probeByName(t, partA, name)...), probeByName(t, partB, name)...)
		if want := probeByName(t, single, name); !sameSeries(joined, want) {
			t.Fatalf("derived probe %q split = %v, single = %v", name, joined, want)
		}
	}
}

func TestSimulateRunDerivedParameterFailuresAndHelp(t *testing.T) {
	dir, store, rules := deriveFixture(t)
	params, _ := deriveParameterSet(t, dir, store, rules, "params.coimparams")
	derived := writeProtocol(t, dir, "derived-fail.json", derivedSimulateProtocol(2, "exclude", 2))
	uniform := writeProtocol(t, dir, "uniform-fail.json", lifSimulateProtocol(2))

	// A derived block on the uniform source is refused by the protocol itself.
	mixed := lifSimulateProtocol(2)
	mixed.Derived = &simulate.DerivedParameters{UnknownSign: "exclude", WeightScale: 1}
	mixedPath := writeProtocol(t, dir, "mixed.json", mixed)

	noPolicy := derivedSimulateProtocol(2, "", 2)
	noPolicyPath := writeProtocol(t, dir, "no-policy.json", noPolicy)

	gainSet := derivedSimulateProtocol(2, "exclude", 2)
	gainSet.Uniform = &simulate.UniformParameters{Gain: .2}
	gainPath := writeProtocol(t, dir, "derived-gain.json", gainSet)

	for name, args := range map[string][]string{
		"derived without params":   {"simulate", "run", "--store", store, "--protocol", derived},
		"uniform with params":      {"simulate", "run", "--store", store, "--protocol", uniform, "--params", params},
		"missing params file":      {"simulate", "run", "--store", store, "--protocol", derived, "--params", filepath.Join(dir, "absent.coimparams")},
		"params is not a set":      {"simulate", "run", "--store", store, "--protocol", derived, "--params", store},
		"derived block on uniform": {"simulate", "run", "--store", store, "--protocol", mixedPath, "--params", params},
		"no unknown policy":        {"simulate", "run", "--store", store, "--protocol", noPolicyPath, "--params", params},
		"uniform gain on derived":  {"simulate", "run", "--store", store, "--protocol", gainPath, "--params", params},
		// --max-store-bytes bounds both the store and the parameter set file.
		"tiny file limit": {"simulate", "run", "--store", store, "--protocol", derived, "--params", params, "--max-store-bytes", "64"},
	} {
		var out, errout bytes.Buffer
		if err := Run(context.Background(), args, &out, &errout); err == nil {
			t.Errorf("%s accepted", name)
		}
	}

	var out, errout bytes.Buffer
	if err := Run(context.Background(), []string{"simulate", "run", "--help"}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"--params", "derived_release/v1", "unknown_sign"} {
		if !strings.Contains(out.String(), word) {
			t.Fatalf("simulate run help missing %s:\n%s", word, out.String())
		}
	}
}

// compareStore builds a four neuron store whose annotations carry a class
// column, so a compare protocol can resolve named sets. Bodies 1..4 become
// node indices 0..3 with classes A, A, B and B; the six edges are
// 0->1, 0->2, 1->2, 1->3, 2->3 and 3->0 with raw weights 5, 6, 4, 1, 3 and 2.
func compareStore(t *testing.T) (dir, store string) {
	t.Helper()
	dir = t.TempDir()
	writeImportFeather(t, filepath.Join(dir, "annotations.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "status", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "class", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{1, 2, 3, 4}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"Traced", "Traced", "Traced", "Traced"}, nil)
		b.Field(2).(*array.StringBuilder).AppendValues([]string{"A", "A", "B", "B"}, nil)
	})
	writeImportFeather(t, filepath.Join(dir, "weights.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "weight", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{1, 1, 2, 2, 3, 4}, nil)
		b.Field(1).(*array.Int64Builder).AppendValues([]int64{2, 3, 3, 4, 4, 1}, nil)
		b.Field(2).(*array.Int64Builder).AppendValues([]int64{5, 6, 4, 1, 3, 2}, nil)
	})
	writeImportFeather(t, filepath.Join(dir, "nt.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "consensus_nt", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{1}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"acetylcholine"}, nil)
	})
	weights := fingerprintJSON(t, dir, "weights.feather")
	weights["role"] = "weights"
	annotations := fingerprintJSON(t, dir, "annotations.feather")
	annotations["role"] = "annotations"
	nt := fingerprintJSON(t, dir, "nt.feather")
	nt["role"] = "neurotransmitters"
	manifest := map[string]any{
		"schema_version": "coimnet-dataset-manifest/v1",
		"dataset":        "compare-fixture",
		"namespace":      "compare-fixture-v1",
		"source_version": "v1",
		"license":        map[string]any{"name": "CC-BY-4.0", "url": "https://creativecommons.org/licenses/by/4.0/"},
		"acquired_at":    "2026-09-15T00:00:00Z",
		"files":          []any{weights, annotations, nt},
		"field_mapping": map[string]any{
			"weights":           map[string]any{"source": "body_pre", "target": "body_post", "value": "weight"},
			"annotations":       map[string]any{"id": "bodyId", "status": "status", "class": "class"},
			"neurotransmitters": map[string]any{"id": "body", "consensus": "consensus_nt"},
		},
		"identity": map[string]any{
			"weights_endpoints_are_annotation_ids":    true,
			"neurotransmitter_ids_are_annotation_ids": true,
			"evidence": "fixture",
		},
		"selection":           map[string]any{"source": "annotations", "field": "status", "equals": "Traced", "label": "engineering selection"},
		"duplicate_semantics": "unknown",
		"coordinate_unit":     "unverified",
		"transform_history":   []any{map[string]any{"step": "generate", "description": "fixture", "version": "test"}},
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	store = filepath.Join(dir, "graph.coimgraph")
	var out, errout bytes.Buffer
	if err := Run(context.Background(), []string{"data", "import", "--manifest", filepath.Join(dir, "manifest.json"), "--temp-dir", t.TempDir(), "--out-store", store}, &out, &errout); err != nil {
		t.Fatalf("data import: %v; stderr=%s", err, errout.String())
	}
	return dir, store
}

// uniformCompareProtocol drives the four neuron compare store with the uniform
// engineering source and the two null models that source supports.
func uniformCompareProtocol(steps int) simulate.CompareProtocol {
	run := lifSimulateProtocol(steps)
	run.Uniform = &simulate.UniformParameters{Gain: .5}
	run.Probes = []simulate.Probe{{Name: "population", Nodes: []int{0, 1, 2, 3}, Reduce: simulate.ReduceSpikeFraction}}
	return simulate.CompareProtocol{
		SchemaVersion: simulate.CompareProtocolSchemaVersion,
		Run:           run,
		Sets: []simulate.NamedSet{
			{Name: "class_a", Selectors: []simulate.Selector{{Field: "class", Equals: "A"}}},
			{Name: "class_b", Selectors: []simulate.Selector{{Field: "class", Equals: "B"}}},
		},
		Metrics: []simulate.Metric{
			{Name: "a_fraction", Kind: simulate.MetricSpikeFraction, Set: "class_a", Window: [2]int{0, steps}},
			{Name: "b_rate", Kind: simulate.MetricMeanRate, Set: "class_b", Window: [2]int{0, steps}},
		},
		Thresholds: []simulate.Threshold{
			{Metric: "a_fraction", Op: simulate.OpAtLeast, Value: 0.5},
			{Metric: "b_rate", Op: simulate.OpAbove, Value: 10},
		},
		NullModels: []simulate.NullModelSpec{
			{Kind: simulate.NullDegreePreservingRewire, SwapFactor: 2},
			{Kind: simulate.NullWeightShuffle},
		},
		Seeds: []uint64{1, 2},
	}
}

func writeCompareProtocol(t *testing.T, dir, name string, cp simulate.CompareProtocol) string {
	t.Helper()
	encoded, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type compareOutput struct {
	SchemaVersion      string `json:"schema_version"`
	ProtocolHash       string `json:"protocol_hash"`
	ParameterSource    string `json:"parameter_source"`
	ParameterSetSHA256 string `json:"parameter_set_sha256"`
	Reproduction       string `json:"reproduction"`
	Sets               []struct {
		Name     string `json:"name"`
		Count    int    `json:"count"`
		NodeHash string `json:"node_hash"`
	} `json:"sets"`
	Cells []struct {
		Index   int    `json:"index"`
		Variant string `json:"variant"`
		Kind    string `json:"kind"`
		Seed    uint64 `json:"seed"`
		Null    *struct {
			Kind         string `json:"kind"`
			Seed         uint64 `json:"seed"`
			PRNG         string `json:"prng"`
			Attempts     uint64 `json:"attempts"`
			Applied      uint64 `json:"applied"`
			TopologyHash string `json:"topology_hash"`
		} `json:"null_model"`
		Run struct {
			SchemaVersion string `json:"schema_version"`
			TopologyHash  string `json:"topology_hash"`
			Plasticity    *struct {
				Rule           string  `json:"rule"`
				EnabledEdges   int     `json:"enabled_edges"`
				Frozen         bool    `json:"frozen"`
				PlasticL2After float64 `json:"plastic_l2_after"`
			} `json:"plasticity"`
		} `json:"run"`
		Deltas []struct {
			Metric  string  `json:"metric"`
			Value   float64 `json:"value"`
			Defined bool    `json:"defined"`
		} `json:"deltas_from_original"`
		Metrics []struct {
			Name    string  `json:"name"`
			Value   float64 `json:"value"`
			Defined bool    `json:"defined"`
		} `json:"metrics"`
		Thresholds []struct {
			Metric string `json:"metric"`
			Passed bool   `json:"passed"`
			Reason string `json:"reason"`
		} `json:"thresholds"`
		WallSeconds float64 `json:"wall_seconds"`
	} `json:"cells"`
	Summary []struct {
		Metric  string `json:"metric"`
		PerKind []struct {
			Kind      string     `json:"kind"`
			Seeds     int        `json:"seeds"`
			Defined   int        `json:"defined"`
			Quantiles [5]float64 `json:"quantiles"`
		} `json:"per_kind"`
	} `json:"summary"`
}

func runCompare(t *testing.T, args ...string) (compareOutput, string) {
	t.Helper()
	var out, errout bytes.Buffer
	if err := Run(context.Background(), args, &out, &errout); err != nil {
		t.Fatalf("%v: %v; stderr=%s", args, err, errout.String())
	}
	var decoded compareOutput
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid compare report JSON: %v\n%s", err, out.String())
	}
	return decoded, out.String()
}

// maskWallSeconds removes the only field two identical compares may differ in.
func maskWallSeconds(t *testing.T, raw string) string {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		t.Fatal(err)
	}
	cells, _ := document["cells"].([]any)
	for _, cell := range cells {
		if object, ok := cell.(map[string]any); ok {
			delete(object, "wall_seconds")
		}
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestSimulateCompareRunsTheMatrixAndWritesEveryCell(t *testing.T) {
	dir, store := compareStore(t)
	protocol := writeCompareProtocol(t, dir, "compare.json", uniformCompareProtocol(6))
	outDir := filepath.Join(dir, "cells")
	report, raw := runCompare(t, "simulate", "compare", "--store", store, "--protocol", protocol, "--out-dir", outDir)

	if report.SchemaVersion != "coimnet-simulate-compare-report/v1" || len(report.ProtocolHash) != 64 {
		t.Fatalf("report identity = %s", raw)
	}
	if report.ParameterSource != "engineering_uniform_positive" || report.ParameterSetSHA256 != "" {
		t.Fatalf("report provenance = %q %q", report.ParameterSource, report.ParameterSetSHA256)
	}
	if len(report.Cells) != 5 {
		t.Fatalf("got %d cells, want 1 original and 2 kinds x 2 seeds", len(report.Cells))
	}
	if report.Cells[0].Variant != "original" || report.Cells[0].Null != nil {
		t.Fatalf("cell 0 = %+v", report.Cells[0])
	}
	if len(report.Sets) != 2 || report.Sets[0].Count != 2 || report.Sets[1].Count != 2 {
		t.Fatalf("resolved sets = %+v", report.Sets)
	}
	for i, cell := range report.Cells[1:] {
		if cell.Null == nil || cell.Null.PRNG != "pcg" || cell.Null.Seed != cell.Seed {
			t.Fatalf("cell %d null model = %+v", i+1, cell.Null)
		}
		if cell.Run.TopologyHash != cell.Null.TopologyHash {
			t.Fatalf("cell %d topology hash %q does not match its null model %q", i+1, cell.Run.TopologyHash, cell.Null.TopologyHash)
		}
	}
	// The declared threshold on b_rate cannot pass, and that is not an error.
	for _, cell := range report.Cells {
		if len(cell.Thresholds) != 2 || cell.Thresholds[1].Passed {
			t.Fatalf("cell %d thresholds = %+v", cell.Index, cell.Thresholds)
		}
	}
	if len(report.Summary) != 2 || len(report.Summary[0].PerKind) != 2 || report.Summary[0].PerKind[0].Seeds != 2 {
		t.Fatalf("summary = %+v", report.Summary)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(report.Cells) {
		t.Fatalf("out-dir holds %d files for %d cells", len(entries), len(report.Cells))
	}
	for _, cell := range report.Cells {
		path := filepath.Join(outDir, fmt.Sprintf("cell-%d-%s.json", cell.Index, cell.Variant))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cell file %s: %v", path, err)
		}
		var run simulateOutput
		if err := json.Unmarshal(data, &run); err != nil {
			t.Fatalf("cell file %s is not a run report: %v", path, err)
		}
		if run.SchemaVersion != "coimnet-simulate-run/v1" || run.Steps != 6 {
			t.Fatalf("cell file %s = %s", path, data)
		}
	}

	_, again := runCompare(t, "simulate", "compare", "--store", store, "--protocol", protocol)
	if maskWallSeconds(t, raw) != maskWallSeconds(t, again) {
		t.Fatalf("two identical compares differ:\n%s\n%s", maskWallSeconds(t, raw), maskWallSeconds(t, again))
	}
}

// TestSimulateCompareRunsTheDerivedSource uses the two neuron derivation
// fixture, whose annotations carry no class column; both sets therefore
// declare allow_empty and resolve to zero nodes, which makes every metric
// undefined and every threshold fail with that reason. It is the end to end
// check that an undefined metric is reported, never silently zero.
func TestSimulateCompareRunsTheDerivedSource(t *testing.T) {
	dir, store, rules := deriveFixture(t)
	paramsPath, sum := deriveParameterSet(t, dir, store, rules, "compare-params.coimparams")
	cp := uniformCompareProtocol(4)
	cp.Run = derivedSimulateProtocol(4, "exclude", 2)
	cp.Run.Probes = []simulate.Probe{{Name: "population", Nodes: []int{0, 1}, Reduce: simulate.ReduceSpikeFraction}}
	cp.Sets = []simulate.NamedSet{
		{Name: "unlabeled", Selectors: []simulate.Selector{{Field: "class", Equals: "ALIN", AllowEmpty: true}}},
	}
	cp.Metrics = []simulate.Metric{{Name: "unlabeled_rate", Kind: simulate.MetricMeanRate, Set: "unlabeled", Window: [2]int{0, 4}}}
	cp.Thresholds = []simulate.Threshold{{Metric: "unlabeled_rate", Op: simulate.OpAtLeast, Value: 0}}
	cp.NullModels = []simulate.NullModelSpec{
		{Kind: simulate.NullDegreePreservingRewire, SwapFactor: 1},
		{Kind: simulate.NullSignShuffle},
		{Kind: simulate.NullWeightShuffle},
	}
	cp.Seeds = []uint64{4}
	protocol := writeCompareProtocol(t, dir, "derived-compare.json", cp)

	report, raw := runCompare(t, "simulate", "compare", "--store", store, "--protocol", protocol, "--params", paramsPath)
	if report.ParameterSource != "derived_release/v1" || report.ParameterSetSHA256 != sum {
		t.Fatalf("derived compare provenance = %s", raw)
	}
	if len(report.Cells) != 4 {
		t.Fatalf("got %d cells, want 1 original and 3 kinds x 1 seed", len(report.Cells))
	}
	for _, cell := range report.Cells {
		if cell.Metrics[0].Defined {
			t.Fatalf("cell %d reports a defined metric over an empty set: %+v", cell.Index, cell.Metrics)
		}
		if cell.Thresholds[0].Passed || cell.Thresholds[0].Reason != "undefined" {
			t.Fatalf("cell %d threshold = %+v", cell.Index, cell.Thresholds)
		}
	}
	for _, summary := range report.Summary {
		for _, perKind := range summary.PerKind {
			if perKind.Defined != 0 || perKind.Quantiles != [5]float64{} {
				t.Fatalf("summary %+v claims a distribution of undefined values", perKind)
			}
		}
	}
}

func TestSimulateCompareFailuresAndHelp(t *testing.T) {
	dir, store := compareStore(t)
	valid := writeCompareProtocol(t, dir, "valid-compare.json", uniformCompareProtocol(4))

	unknownKind := uniformCompareProtocol(4)
	unknownKind.NullModels = []simulate.NullModelSpec{{Kind: "rewire", SwapFactor: 1}}
	unknownKindPath := writeCompareProtocol(t, dir, "unknown-kind.json", unknownKind)

	duplicateSeed := uniformCompareProtocol(4)
	duplicateSeed.Seeds = []uint64{3, 3}
	duplicateSeedPath := writeCompareProtocol(t, dir, "duplicate-seed.json", duplicateSeed)

	strayThreshold := uniformCompareProtocol(4)
	strayThreshold.Thresholds = []simulate.Threshold{{Metric: "absent", Op: simulate.OpAtLeast, Value: 1}}
	strayThresholdPath := writeCompareProtocol(t, dir, "stray-threshold.json", strayThreshold)

	signOnUniform := uniformCompareProtocol(4)
	signOnUniform.NullModels = []simulate.NullModelSpec{{Kind: simulate.NullSignShuffle}}
	signOnUniformPath := writeCompareProtocol(t, dir, "sign-on-uniform.json", signOnUniform)

	runProtocol := writeProtocol(t, dir, "plain-run.json", lifSimulateProtocol(4))

	malformed := filepath.Join(dir, "malformed-compare.json")
	if err := os.WriteFile(malformed, []byte(`{"schema_version":"coimnet-simulate-compare/v1",}`), 0o600); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "existing-out")
	if err := os.Mkdir(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	derivedDir, derivedStore, rules := deriveFixture(t)
	paramsPath, _ := deriveParameterSet(t, derivedDir, derivedStore, rules, "fail-params.coimparams")

	for name, args := range map[string][]string{
		"no flags":            {"simulate", "compare"},
		"no store":            {"simulate", "compare", "--protocol", valid},
		"no protocol":         {"simulate", "compare", "--store", store},
		"missing protocol":    {"simulate", "compare", "--store", store, "--protocol", filepath.Join(dir, "absent.json")},
		"malformed protocol":  {"simulate", "compare", "--store", store, "--protocol", malformed},
		"run protocol":        {"simulate", "compare", "--store", store, "--protocol", runProtocol},
		"unknown kind":        {"simulate", "compare", "--store", store, "--protocol", unknownKindPath},
		"duplicate seed":      {"simulate", "compare", "--store", store, "--protocol", duplicateSeedPath},
		"threshold no metric": {"simulate", "compare", "--store", store, "--protocol", strayThresholdPath},
		"sign on uniform":     {"simulate", "compare", "--store", store, "--protocol", signOnUniformPath},
		"existing out-dir":    {"simulate", "compare", "--store", store, "--protocol", valid, "--out-dir", existing},
		"uniform with params": {"simulate", "compare", "--store", store, "--protocol", valid, "--params", paramsPath},
		"tiny memory":         {"simulate", "compare", "--store", store, "--protocol", valid, "--max-memory-bytes", "16"},
		"positional":          {"simulate", "compare", "--store", store, "--protocol", valid, "extra"},
		"unknown flag":        {"simulate", "compare", "--surprise"},
	} {
		var out, errout bytes.Buffer
		if err := Run(context.Background(), args, &out, &errout); err == nil {
			t.Errorf("%s accepted", name)
		}
	}

	// The derived source still requires its parameter set.
	derivedCompare := uniformCompareProtocol(4)
	derivedCompare.Run = derivedSimulateProtocol(4, "exclude", 2)
	derivedCompare.Run.Probes = []simulate.Probe{{Name: "population", Nodes: []int{0, 1}, Reduce: simulate.ReduceSpikeFraction}}
	derivedCompare.Sets = []simulate.NamedSet{{Name: "unlabeled", Selectors: []simulate.Selector{{Field: "class", Equals: "ALIN", AllowEmpty: true}}}}
	derivedCompare.Metrics = []simulate.Metric{{Name: "unlabeled_rate", Kind: simulate.MetricMeanRate, Set: "unlabeled", Window: [2]int{0, 4}}}
	derivedCompare.Thresholds = nil
	derivedCompare.NullModels = []simulate.NullModelSpec{{Kind: simulate.NullSignShuffle}}
	derivedCompare.Seeds = []uint64{1}
	derivedPath := writeCompareProtocol(t, derivedDir, "needs-params.json", derivedCompare)
	var out, errout bytes.Buffer
	if err := Run(context.Background(), []string{"simulate", "compare", "--store", derivedStore, "--protocol", derivedPath}, &out, &errout); err == nil {
		t.Error("a derived compare ran without --params")
	}

	out.Reset()
	if err := Run(context.Background(), []string{"simulate", "compare", "--help"}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"--store", "--protocol", "--params", "--out-dir", "Example:", "Errors:", "Options:",
		"degree_preserving_rewire", "sign_shuffle", "weight_shuffle"} {
		if !strings.Contains(out.String(), word) {
			t.Fatalf("simulate compare help missing %s:\n%s", word, out.String())
		}
	}
	out.Reset()
	if err := Run(context.Background(), []string{"simulate"}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "simulate compare") {
		t.Fatalf("simulate usage = %s", out.String())
	}
	out.Reset()
	if err := Run(context.Background(), []string{"--help"}, &out, &errout); err != nil || !strings.Contains(out.String(), "simulate compare") {
		t.Fatalf("overview missing simulate compare: %v\n%s", err, out.String())
	}
}

// plasticSimulateProtocol is lifSimulateProtocol with a second stimulus channel
// carrying a constant learning gate and the plasticity block that reads it. The
// gain is raised so node 1 reaches the threshold at step 1, which is what makes
// the first eligibility of edge 0 non-zero.
func plasticSimulateProtocol(steps int) simulate.Protocol {
	p := lifSimulateProtocol(steps)
	p.Uniform = &simulate.UniformParameters{Gain: .5}
	inline := make([][]float64, steps)
	for t := range inline {
		inline[t] = []float64{0, 1}
	}
	inline[0][0] = 2
	p.Stimulus = simulate.StimulusSpec{Inline: inline}
	p.Plasticity = &simulate.Plasticity{
		Rule: plasticity.Rule{
			Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625,
		},
		All: true, GateChannel: 1, GateScale: 1,
	}
	return p
}

// TestSimulateRunAcceptsThePlasticityBlock is the CLI half of the runner work:
// the block is read out of the protocol, no new flag exists, and the report
// carries the plasticity block. The same protocol without the block is the run
// it always was.
func TestSimulateRunAcceptsThePlasticityBlock(t *testing.T) {
	dir, store := simulateStore(t)
	protocol := writeProtocol(t, dir, "plastic.json", plasticSimulateProtocol(4))
	report, raw := runSimulate(t, "simulate", "run", "--store", store, "--protocol", protocol)

	if report.Plasticity == nil {
		t.Fatalf("the CLI report carries no plasticity block: %s", raw)
	}
	got := *report.Plasticity
	if got.Rule != plasticity.RuleHebbianRate || got.EnabledEdges != 2 || got.GateChannel != 1 || got.Frozen {
		t.Fatalf("plasticity block = %+v", got)
	}
	if got.ChunkSize != 1 || got.WallClockPenaltyNote == "" {
		t.Fatalf("plasticity block = %+v, want the forced chunk size and its note", got)
	}
	if got.PlasticL2Before != 0 || got.PlasticL2After <= 0 {
		t.Fatalf("plastic l2 = %v..%v, want a run that moved", got.PlasticL2Before, got.PlasticL2After)
	}
	if !strings.Contains(strings.Join(report.Assumptions, " "), "Local plasticity was enabled") {
		t.Fatalf("assumptions = %v", report.Assumptions)
	}
	plain := writeProtocol(t, dir, "plain.json", lifSimulateProtocol(4))
	base, _ := runSimulate(t, "simulate", "run", "--store", store, "--protocol", plain)
	if base.Plasticity != nil {
		t.Fatalf("a protocol without the block reported plasticity %+v", base.Plasticity)
	}

	// No flag was added: the block travels in the protocol only.
	var out, errout bytes.Buffer
	if err := Run(context.Background(), []string{"simulate", "run", "--plasticity", "x", "--store", store, "--protocol", protocol}, &out, &errout); err == nil {
		t.Fatal("simulate run accepted a --plasticity flag")
	}
	var help bytes.Buffer
	if err := Run(context.Background(), []string{"simulate", "run", "--help"}, &help, &errout); err != nil {
		t.Fatalf("simulate run --help: %v", err)
	}
	if !strings.Contains(help.String(), "plasticity block") || !strings.Contains(help.String(), "gate_channel") {
		t.Fatalf("simulate run --help does not describe the block:\n%s", help.String())
	}
}

// TestSimulateCompareAcceptsTheLearningVariants is the CLI half of the compare
// work: the three cells are declared in the protocol, written to --out-dir
// under their variant names, and the original cell carries no plasticity.
func TestSimulateCompareAcceptsTheLearningVariants(t *testing.T) {
	dir, store := compareStore(t)
	cp := uniformCompareProtocol(6)
	inline := make([][]float64, 6)
	for t := range inline {
		inline[t] = []float64{0, 1}
	}
	inline[0][0] = 2
	cp.Run.Stimulus = simulate.StimulusSpec{Inline: inline}
	cp.Run.Plasticity = &simulate.Plasticity{
		Rule: plasticity.Rule{
			Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625,
		},
		All: true, GateChannel: 1, GateScale: 1,
	}
	cp.LearningVariants = []string{"original", "plastic", "learned_then_frozen"}
	protocol := writeCompareProtocol(t, dir, "learning.json", cp)
	outDir := filepath.Join(dir, "learning-cells")
	report, raw := runCompare(t, "simulate", "compare", "--store", store, "--protocol", protocol, "--out-dir", outDir)

	if len(report.Cells) != 3+len(cp.NullModels)*len(cp.Seeds) {
		t.Fatalf("got %d cells: %s", len(report.Cells), raw)
	}
	for i, want := range []string{"original", "plastic", "learned_then_frozen"} {
		if report.Cells[i].Variant != want || report.Cells[i].Kind != "" {
			t.Fatalf("cell %d = %+v, want the %q variant", i, report.Cells[i], want)
		}
	}
	if report.Cells[0].Run.Plasticity != nil {
		t.Fatalf("the original cell reports plasticity %+v", report.Cells[0].Run.Plasticity)
	}
	if report.Cells[1].Run.Plasticity == nil || report.Cells[1].Run.Plasticity.Frozen {
		t.Fatalf("the plastic cell = %+v", report.Cells[1].Run.Plasticity)
	}
	frozen := report.Cells[2].Run.Plasticity
	if frozen == nil || !frozen.Frozen || frozen.PlasticL2After != report.Cells[1].Run.Plasticity.PlasticL2After {
		t.Fatalf("the frozen cell = %+v", frozen)
	}
	for _, cell := range report.Cells[3:] {
		if cell.Run.Plasticity != nil {
			t.Fatalf("null model cell %d reports plasticity %+v", cell.Index, cell.Run.Plasticity)
		}
	}
	for _, delta := range report.Cells[0].Deltas {
		if delta.Defined && delta.Value != 0 {
			t.Fatalf("the original cell has a non-zero delta %+v", delta)
		}
	}
	if !strings.Contains(report.Reproduction, "learned_then_frozen") {
		t.Fatalf("reproduction = %q", report.Reproduction)
	}
	for _, name := range []string{"cell-0-original.json", "cell-1-plastic.json", "cell-2-learned_then_frozen.json"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("out-dir is missing %s: %v", name, err)
		}
	}

	var out, errout bytes.Buffer
	if err := Run(context.Background(), []string{"simulate", "compare", "--learning-variants", "plastic", "--store", store, "--protocol", protocol}, &out, &errout); err == nil {
		t.Fatal("simulate compare accepted a --learning-variants flag")
	}
	var help bytes.Buffer
	if err := Run(context.Background(), []string{"simulate", "compare", "--help"}, &help, &errout); err != nil {
		t.Fatalf("simulate compare --help: %v", err)
	}
	if !strings.Contains(help.String(), "learning_variants") || !strings.Contains(help.String(), "learned_then_frozen") {
		t.Fatalf("simulate compare --help does not describe the variants:\n%s", help.String())
	}
}

// TestSimulateRunRefusesStateFlagsWithThePlasticityBlock pins the one
// combination the block cannot support: the state snapshot is the core's state
// and holds no fast changes, so a split run would restart them at zero.
func TestSimulateRunRefusesStateFlagsWithThePlasticityBlock(t *testing.T) {
	dir, store := simulateStore(t)
	protocol := writeProtocol(t, dir, "plastic-state.json", plasticSimulateProtocol(4))
	for _, flag := range []string{"--state-out", "--state-in"} {
		var out, errout bytes.Buffer
		args := []string{"simulate", "run", "--store", store, "--protocol", protocol, flag, filepath.Join(dir, "state.json")}
		err := Run(context.Background(), args, &out, &errout)
		if err == nil || !strings.Contains(err.Error(), "plasticity block") {
			t.Fatalf("%s with a plasticity block = %v", flag, err)
		}
	}
}
