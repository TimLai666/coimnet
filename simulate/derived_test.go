package simulate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/params"
)

// fixtureSetSHA256 stands in for the SHA-256 of the parameter set file. The
// tests never write the set to disk; the value only has to reach the hash.
const fixtureSetSHA256 = "1111111111111111111111111111111111111111111111111111111111111111"

// derivedProtocol is the fixture protocol for the derived parameter source.
// The uniform block carries only the node scalars: gain must stay zero,
// because the edge strengths come from the parameter set and the weight scale.
func derivedProtocol(core, policy string, scale float64, steps int) Protocol {
	p := Protocol{
		SchemaVersion: ProtocolSchemaVersion,
		Injections:    []Injection{{Channel: 0, Node: 0, Gain: 1}},
		Probes: []Probe{
			{Name: "all", Nodes: []int{0, 1, 2, 3}, Reduce: ReduceSumOutput},
		},
		Stimulus:        StimulusSpec{Pulse: &Pulse{Channels: 1, Steps: steps, Channel: 0, Onset: 0, Duration: 1, Amplitude: 2}},
		ParameterSource: ParameterSourceDerived,
		Uniform:         &UniformParameters{Gain: 0, Bias: 0, LogTau: 0, ThetaRaw: 0},
		Derived:         &DerivedParameters{UnknownSign: policy, WeightScale: scale},
	}
	switch core {
	case CoreLIF:
		p.Core = CoreLIF
		p.LIF = &dynamics.LIFConfig{
			DT: 1, TauSyn: 1, ThetaMin: .5, ThetaMax: 1.5, VReset: -.5,
			Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		}
		p.Probes = append(p.Probes, Probe{Name: "spikes", Nodes: []int{0, 1, 2, 3}, Reduce: ReduceSpikeFraction})
	default:
		p.Core = CoreContinuous
		p.Continuous = &dynamics.Config{DT: 1, Activation: "tanh"}
	}
	return p
}

// The derivation fixture of ticket 13, written out again from the ticket
// table. With gain 2 and the post_total normalizer the six edges are:
//
//	edge 0 (0,1) acetylcholine +1, 2*4/4 = 2
//	edge 1 (0,2) gaba          -1, 2*2/6
//	edge 2 (0,3) dopamine       0, post_total 0 so the weight is 0
//	edge 3 (1,2) glutamate     -1, 2*3/6 = 1
//	edge 4 (2,3) no match       0, post_total 0 so the weight is 0
//	edge 5 (3,0) no match       0, 2*8/7
//
// The weight scale is a power of two, so every expected product below is exact
// in binary floating point and does not depend on the order of multiplication.
const fixtureWeightScale = 4.0

var (
	fixtureSigns   = []int8{1, -1, 0, -1, 0, 0}
	fixtureWeights = []float64{2, 4.0 / 6.0, 0, 1, 0, 16.0 / 7.0}

	fixtureExclude    = []float64{fixtureWeightScale * 2, -fixtureWeightScale * (4.0 / 6.0), 0, -fixtureWeightScale * 1, 0, 0}
	fixtureExcitatory = []float64{fixtureWeightScale * 2, -fixtureWeightScale * (4.0 / 6.0), 0, -fixtureWeightScale * 1, 0, fixtureWeightScale * (16.0 / 7.0)}
	fixtureInhibitory = []float64{fixtureWeightScale * 2, -fixtureWeightScale * (4.0 / 6.0), 0, -fixtureWeightScale * 1, 0, -fixtureWeightScale * (16.0 / 7.0)}
)

func TestDerivedFixtureSetMatchesTheHandCalculatedTable(t *testing.T) {
	set, _ := derivedFixtureSet(t)
	if set.Source != params.SetSource {
		t.Fatalf("derived set source = %q", set.Source)
	}
	if len(set.EdgeSign) != len(fixtureSigns) {
		t.Fatalf("derived set has %d edges, want %d", len(set.EdgeSign), len(fixtureSigns))
	}
	for i, want := range fixtureSigns {
		if set.EdgeSign[i] != want {
			t.Fatalf("edge %d sign = %d, want %d", i, set.EdgeSign[i], want)
		}
	}
	for i, want := range fixtureWeights {
		if set.EdgeWeight[i] != want {
			t.Fatalf("edge %d derived weight = %v, want %v", i, set.EdgeWeight[i], want)
		}
	}
}

func TestFromDerivedAppliesEveryUnknownSignPolicy(t *testing.T) {
	set, _ := derivedFixtureSet(t)
	for _, policy := range []struct {
		name string
		want []float64
	}{
		{UnknownSignExclude, fixtureExclude},
		{UnknownSignExcitatory, fixtureExcitatory},
		{UnknownSignInhibitory, fixtureInhibitory},
	} {
		protocol := derivedProtocol(CoreLIF, policy.name, fixtureWeightScale, 2)
		parameters, summary, err := FromDerived(set, fixtureSetSHA256, protocol)
		if err != nil {
			t.Fatalf("%s: %v", policy.name, err)
		}
		if !equalSeries(parameters.Weights, policy.want) {
			t.Fatalf("%s weights = %v, want %v", policy.name, parameters.Weights, policy.want)
		}
		for i, weight := range parameters.Weights {
			// A zero weight must be positive zero, whatever the policy did.
			if weight == 0 && strings.HasPrefix(formatFloat(weight), "-") {
				t.Fatalf("%s weight %d is negative zero", policy.name, i)
			}
		}
		if parameters.Source != ParameterSourceDerived {
			t.Fatalf("%s source = %q", policy.name, parameters.Source)
		}
		if len(parameters.Bias) != 4 || len(parameters.LogTau) != 4 || len(parameters.ThetaRaw) != 4 {
			t.Fatalf("%s node scalars = %d/%d/%d, want 4 each", policy.name, len(parameters.Bias), len(parameters.LogTau), len(parameters.ThetaRaw))
		}
		if summary.PositiveEdges != 1 || summary.NegativeEdges != 2 || summary.UnknownSignEdges != 3 {
			t.Fatalf("%s summary = %+v; the fixture has one +1, two -1 and three unknown edges", policy.name, summary)
		}
		if summary.UnknownSignPolicy != policy.name || summary.WeightScale != fixtureWeightScale ||
			summary.ParameterSetSHA256 != fixtureSetSHA256 || summary.RulesHash != set.RulesHash {
			t.Fatalf("%s summary provenance = %+v", policy.name, summary)
		}
		if parameters.Derived == nil || *parameters.Derived != summary {
			t.Fatalf("%s parameter set does not carry its summary: %+v", policy.name, parameters.Derived)
		}
		if len(parameters.Hash) != 64 {
			t.Fatalf("%s hash = %q", policy.name, parameters.Hash)
		}
	}
}

// TestDerivedParameterHashCoversItsProvenance changes one input at a time and
// requires the hash to move, so a report can never claim the wrong file, rule
// set, policy or scale.
func TestDerivedParameterHashCoversItsProvenance(t *testing.T) {
	set, _ := derivedFixtureSet(t)
	base, _, err := FromDerived(set, fixtureSetSHA256, derivedProtocol(CoreLIF, UnknownSignExclude, fixtureWeightScale, 2))
	if err != nil {
		t.Fatal(err)
	}
	same, _, err := FromDerived(set, fixtureSetSHA256, derivedProtocol(CoreLIF, UnknownSignExclude, fixtureWeightScale, 2))
	if err != nil {
		t.Fatal(err)
	}
	if same.Hash != base.Hash {
		t.Fatal("the same inputs produced two hashes")
	}
	otherFile, _, err := FromDerived(set, strings.Repeat("2", 64), derivedProtocol(CoreLIF, UnknownSignExclude, fixtureWeightScale, 2))
	if err != nil {
		t.Fatal(err)
	}
	otherPolicy, _, err := FromDerived(set, fixtureSetSHA256, derivedProtocol(CoreLIF, UnknownSignExcitatory, fixtureWeightScale, 2))
	if err != nil {
		t.Fatal(err)
	}
	otherScale, _, err := FromDerived(set, fixtureSetSHA256, derivedProtocol(CoreLIF, UnknownSignExclude, 2*fixtureWeightScale, 2))
	if err != nil {
		t.Fatal(err)
	}
	for name, hash := range map[string]string{
		"parameter set file": otherFile.Hash,
		"unknown policy":     otherPolicy.Hash,
		"weight scale":       otherScale.Hash,
	} {
		if hash == base.Hash {
			t.Fatalf("changing the %s did not change the parameter hash", name)
		}
	}
	// A uniform set writes no provenance into its hash, so the encoding of
	// the source that already exists is unchanged and NAT-01's recorded
	// parameter hash stays reproducible.
	graph := fixtureGraph(t)
	uniform, err := UniformPositive(context.Background(), graph, UniformParameters{Gain: 2})
	if err != nil {
		t.Fatal(err)
	}
	if uniform.Derived != nil {
		t.Fatal("a uniform parameter set carries a derived summary")
	}
	if uniform.Hash != uniformFixtureHash {
		t.Fatalf("uniform hash = %q, want the value recorded before this ticket %q", uniform.Hash, uniformFixtureHash)
	}
}

// uniformFixtureHash is the parameter hash of UniformPositive over the
// three-neuron fixture with gain 2, recorded from the committed stage one code
// before the derived source existed.
const uniformFixtureHash = "854600dbf6196eb088c39b7fbed5fee6ebc0eb797b144533870f0e465863768d"

func TestDerivedParametersRunOnBothCores(t *testing.T) {
	set, graph := derivedFixtureSet(t)
	for _, core := range []string{CoreLIF, CoreContinuous} {
		for _, policy := range []string{UnknownSignExclude, UnknownSignExcitatory, UnknownSignInhibitory} {
			protocol := derivedProtocol(core, policy, fixtureWeightScale, 4)
			parameters, summary, err := FromDerived(set, fixtureSetSHA256, protocol)
			if err != nil {
				t.Fatalf("%s/%s: %v", core, policy, err)
			}
			runner, err := Build(context.Background(), graph, parameters, protocol, testLimits())
			if err != nil {
				t.Fatalf("%s/%s build: %v", core, policy, err)
			}
			report, err := runner.Run(context.Background(), runner.Stimulus())
			if err != nil {
				t.Fatalf("%s/%s run: %v", core, policy, err)
			}
			if report.ParameterSource != ParameterSourceDerived || report.ParameterHash != parameters.Hash {
				t.Fatalf("%s/%s report identity = %+v", core, policy, report)
			}
			if report.ParameterSetSHA256 != fixtureSetSHA256 || report.RulesHash != set.RulesHash ||
				report.UnknownSignPolicy != policy || report.WeightScale != fixtureWeightScale {
				t.Fatalf("%s/%s report provenance = %+v", core, policy, report)
			}
			if report.UnknownSignEdges == nil || *report.UnknownSignEdges != summary.UnknownSignEdges {
				t.Fatalf("%s/%s report unknown edges = %v", core, policy, report.UnknownSignEdges)
			}
			assumptions := strings.Join(report.Assumptions, " ")
			for _, phrase := range []string{"transmitter", "unknown", policy, "bias", "log_tau", "theta_raw"} {
				if !strings.Contains(assumptions, phrase) {
					t.Fatalf("%s/%s assumptions do not mention %q: %v", core, policy, phrase, report.Assumptions)
				}
			}
			if strings.Contains(assumptions, ParameterSourceUniform) {
				t.Fatalf("%s/%s assumptions still claim the uniform source: %v", core, policy, report.Assumptions)
			}
		}
	}
}

func TestDerivedRunsAreDeterministic(t *testing.T) {
	set, graph := derivedFixtureSet(t)
	protocol := derivedProtocol(CoreLIF, UnknownSignInhibitory, fixtureWeightScale, 6)
	encode := func() string {
		parameters, _, err := FromDerived(set, fixtureSetSHA256, protocol)
		if err != nil {
			t.Fatal(err)
		}
		runner, err := Build(context.Background(), graph, parameters, protocol, testLimits())
		if err != nil {
			t.Fatal(err)
		}
		report, err := runner.Run(context.Background(), runner.Stimulus())
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	if first, second := encode(), encode(); first != second {
		t.Fatalf("two identical derived runs differ:\n%s\n%s", first, second)
	}
}

func TestDerivedRunContinuesFromASavedState(t *testing.T) {
	set, graph := derivedFixtureSet(t)
	protocol := derivedProtocol(CoreLIF, UnknownSignExcitatory, fixtureWeightScale, 6)
	parameters, _, err := FromDerived(set, fixtureSetSHA256, protocol)
	if err != nil {
		t.Fatal(err)
	}
	whole, err := Build(context.Background(), graph, parameters, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	stimulus := whole.Stimulus()
	single, err := whole.Run(context.Background(), stimulus)
	if err != nil {
		t.Fatal(err)
	}

	split, err := Build(context.Background(), graph, parameters, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	partA, err := split.Run(context.Background(), stimulus[:2])
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(split.State())
	if err != nil {
		t.Fatal(err)
	}
	restored, err := DecodeState(strings.NewReader(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	continued, err := Build(context.Background(), graph, parameters, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := continued.RestoreState(restored); err != nil {
		t.Fatal(err)
	}
	partB, err := continued.Run(context.Background(), stimulus[2:])
	if err != nil {
		t.Fatal(err)
	}
	if partB.StepsBefore != 2 || partB.StepsAfter != 6 {
		t.Fatalf("continued steps %d..%d", partB.StepsBefore, partB.StepsAfter)
	}
	joined := append(append([]float64(nil), probeSeries(t, partA, "all")...), probeSeries(t, partB, "all")...)
	if !equalSeries(joined, probeSeries(t, single, "all")) {
		t.Fatalf("split derived run = %v, single = %v", joined, probeSeries(t, single, "all"))
	}
}

func TestDerivedSetIsRefusedForAnotherGraph(t *testing.T) {
	set, graph := derivedFixtureSet(t)
	if err := set.CheckGraph(graph); err != nil {
		t.Fatalf("the set was refused for its own graph: %v", err)
	}
	other := fixtureGraph(t)
	if err := set.CheckGraph(other); err == nil {
		t.Fatal("a parameter set derived for different wiring was accepted")
	}
	protocol := derivedProtocol(CoreLIF, UnknownSignExclude, fixtureWeightScale, 2)
	parameters, _, err := FromDerived(set, fixtureSetSHA256, protocol)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(context.Background(), other, parameters, protocol, testLimits()); err == nil {
		t.Fatal("a six-weight parameter set was built onto a two-edge graph")
	}
}

func TestDerivedProtocolValidationRejectsUnsupportedConfigurations(t *testing.T) {
	for name, mutate := range map[string]func(*Protocol){
		"no derived block": func(p *Protocol) { p.Derived = nil },
		"no uniform block": func(p *Protocol) { p.Uniform = nil },
		"uniform gain set": func(p *Protocol) { p.Uniform.Gain = 0.025 },
		"unknown policy":   func(p *Protocol) { p.Derived.UnknownSign = "average" },
		"empty policy":     func(p *Protocol) { p.Derived.UnknownSign = "" },
		"zero scale":       func(p *Protocol) { p.Derived.WeightScale = 0 },
		"negative scale":   func(p *Protocol) { p.Derived.WeightScale = -1 },
		"unknown source":   func(p *Protocol) { p.ParameterSource = "derived_from_release" },
	} {
		protocol := derivedProtocol(CoreLIF, UnknownSignExclude, fixtureWeightScale, 2)
		mutate(&protocol)
		if err := protocol.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	// The uniform source must refuse a derived block instead of ignoring it.
	uniform := lifProtocol(2)
	uniform.Derived = &DerivedParameters{UnknownSign: UnknownSignExclude, WeightScale: 1}
	if err := uniform.Validate(); err == nil {
		t.Error("the uniform source accepted a derived block")
	}
	// A derived protocol with a valid uniform block and no gain is accepted.
	if err := derivedProtocol(CoreContinuous, UnknownSignExcitatory, 0.5, 2).Validate(); err != nil {
		t.Errorf("a valid derived protocol was refused: %v", err)
	}
}

func TestFromDerivedRejectsMismatchedInput(t *testing.T) {
	set, _ := derivedFixtureSet(t)
	valid := derivedProtocol(CoreLIF, UnknownSignExclude, fixtureWeightScale, 2)
	if _, _, err := FromDerived(nil, fixtureSetSHA256, valid); err == nil {
		t.Error("a nil parameter set was accepted")
	}
	if _, _, err := FromDerived(set, "not-a-hash", valid); err == nil {
		t.Error("a malformed parameter set fingerprint was accepted")
	}
	uniform := lifProtocol(2)
	if _, _, err := FromDerived(set, fixtureSetSHA256, uniform); err == nil {
		t.Error("a uniform protocol was turned into derived parameters")
	}
	short := *set
	short.EdgeSign = short.EdgeSign[:1]
	if _, _, err := FromDerived(&short, fixtureSetSHA256, valid); err == nil {
		t.Error("a parameter set whose arrays disagree was accepted")
	}
}

// TestUniformProtocolStillRefusesDerivedParameters keeps the two sources from
// being mixed once both exist.
func TestUniformProtocolStillRefusesDerivedParameters(t *testing.T) {
	set, graph := derivedFixtureSet(t)
	derived := derivedProtocol(CoreLIF, UnknownSignExclude, fixtureWeightScale, 2)
	parameters, _, err := FromDerived(set, fixtureSetSHA256, derived)
	if err != nil {
		t.Fatal(err)
	}
	uniformProtocol := derived
	uniformProtocol.ParameterSource = ParameterSourceUniform
	uniformProtocol.Derived = nil
	uniformProtocol.Uniform = &UniformParameters{Gain: 2}
	if _, err := Build(context.Background(), graph, parameters, uniformProtocol, testLimits()); err == nil {
		t.Fatal("derived parameters were accepted by a uniform protocol")
	}
}

func formatFloat(v float64) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(encoded)
}
