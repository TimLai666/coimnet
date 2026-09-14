package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/simulate"
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
