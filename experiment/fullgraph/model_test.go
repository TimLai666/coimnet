package fullgraph

import (
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/resources"
	"github.com/TimLai666/coimnet/simulate"
)

// fixtureVariant is the hand-written 4-node, 5-edge variant the contract fixes:
// sources {0,1,2,3,0}, targets {1,2,3,0,2}, weights {0.5,-0.25,0,0.8,0.1},
// signs {1,-1,1,0,0}, zero bias and log_tau = log(2) on every node.
func fixtureVariant() simulate.Variant {
	logTau := make([]float64, 4)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	return simulate.Variant{
		Name:    simulate.VariantOriginal,
		Sources: []int{0, 1, 2, 3, 0},
		Targets: []int{1, 2, 3, 0, 2},
		Params: simulate.ParameterSet{
			Source:  simulate.ParameterSourceDerived,
			Weights: []float64{0.5, -0.25, 0, 0.8, 0.1},
			Signs:   []int8{1, -1, 1, 0, 0},
			Bias:    make([]float64, 4),
			LogTau:  logTau,
		},
	}
}

func fixtureOptions() modelOptions {
	return modelOptions{DT: 0.1, Truncation: 8, Rate: 0.01}
}

func TestBuildModelConvertsSignsAndMagnitudes(t *testing.T) {
	config, parameters, options, report, err := buildModel(fixtureVariant(), 4, []int{0}, []int{2, 3}, fixtureOptions())
	if err != nil {
		t.Fatalf("buildModel: %v", err)
	}
	wantSigns := []int8{1, -1, 0, 0, 0}
	if !slices.Equal(config.EdgeSigns, wantSigns) {
		t.Errorf("EdgeSigns = %v, want %v", config.EdgeSigns, wantSigns)
	}
	wantWeights := []float64{math.Log(0.5), math.Log(0.25), 0, 0.8, 0.1}
	if len(parameters.Core.Weights) != len(wantWeights) {
		t.Fatalf("stored %d weights, want %d", len(parameters.Core.Weights), len(wantWeights))
	}
	for i, want := range wantWeights {
		if math.Abs(parameters.Core.Weights[i]-want) > 1e-15 {
			t.Errorf("stored weight %d = %v, want %v (within 1e-15)", i, parameters.Core.Weights[i], want)
		}
	}
	if report.ZeroFreed != 1 {
		t.Errorf("ZeroFreed = %d, want 1", report.ZeroFreed)
	}
	if report.PositiveEdges != 1 {
		t.Errorf("PositiveEdges = %d, want 1", report.PositiveEdges)
	}
	if report.NegativeEdges != 1 {
		t.Errorf("NegativeEdges = %d, want 1", report.NegativeEdges)
	}
	if report.FreeEdges != 3 {
		t.Errorf("FreeEdges = %d, want 3", report.FreeEdges)
	}
	effective, err := learning.EffectiveWeights(config, parameters)
	if err != nil {
		t.Fatalf("EffectiveWeights: %v", err)
	}
	original := []float64{0.5, -0.25, 0, 0.8, 0.1}
	for i, want := range original {
		if math.Abs(effective[i]-want) > 1e-12 {
			t.Errorf("effective weight %d = %v, want %v (within 1e-12)", i, effective[i], want)
		}
	}
	wantEncoder := []float64{1}
	if !slices.Equal(parameters.Encoder, wantEncoder) {
		t.Errorf("Encoder = %v, want %v", parameters.Encoder, wantEncoder)
	}
	wantReadout := []float64{0.5, 0.5}
	if !slices.Equal(parameters.Readout, wantReadout) {
		t.Errorf("Readout = %v, want %v", parameters.Readout, wantReadout)
	}
	if !options.Trainable.Weights || !options.Trainable.Bias || !options.Trainable.Tau || !options.Trainable.Encoder || !options.Trainable.Readout {
		t.Errorf("Trainable = %+v, want the five continuous-core groups all true", options.Trainable)
	}
	if options.Trainable.Theta {
		t.Errorf("Trainable.Theta = true, the continuous core owns no threshold group")
	}
	if options.Truncation != 8 {
		t.Errorf("Truncation = %d, want 8", options.Truncation)
	}
	if options.LearningRate != 0.01 {
		t.Errorf("LearningRate = %v, want 0.01", options.LearningRate)
	}
	if config.InputSize != 1 || config.OutputSize != 1 {
		t.Errorf("InputSize/OutputSize = %d/%d, want 1/1", config.InputSize, config.OutputSize)
	}
	if config.Dynamics.Nodes != 4 || config.Dynamics.DT != 0.1 || config.Dynamics.Activation != "tanh" {
		t.Errorf("core = nodes %d dt %v activation %q, want 4/0.1/tanh", config.Dynamics.Nodes, config.Dynamics.DT, config.Dynamics.Activation)
	}
	if config.Dynamics.StateDimension != 0 || config.Dynamics.Workers != 0 {
		t.Errorf("core = state_dimension %d workers %d, want the scalar single-threaded core", config.Dynamics.StateDimension, config.Dynamics.Workers)
	}
}

func TestBuildModelKeepsEveryNodeAndEdge(t *testing.T) {
	variant := fixtureVariant()
	before := deepCopyVariant(variant)
	inputs, readouts := []int{0}, []int{2, 3}
	config, parameters, options, report, err := buildModel(variant, 4, inputs, readouts, fixtureOptions())
	if err != nil {
		t.Fatalf("buildModel: %v", err)
	}
	if report.Nodes != 4 {
		t.Errorf("Nodes = %d, want 4", report.Nodes)
	}
	if report.Edges != 5 {
		t.Errorf("Edges = %d, want 5", report.Edges)
	}
	wantParameters := 5 + 4 + 4 + 1 + 2
	if report.Parameters != wantParameters {
		t.Errorf("Parameters = %d, want %d", report.Parameters, wantParameters)
	}
	if report.InputNodes != 1 {
		t.Errorf("InputNodes = %d, want 1", report.InputNodes)
	}
	if report.ReadoutNodes != 2 {
		t.Errorf("ReadoutNodes = %d, want 2", report.ReadoutNodes)
	}
	if !variantEqual(variant, before) {
		t.Errorf("variant slices changed during buildModel:\nbefore = %+v\nafter  = %+v", before, variant)
	}
	if !reflect.DeepEqual(inputs, []int{0}) || !reflect.DeepEqual(readouts, []int{2, 3}) {
		t.Errorf("input arrays were modified: inputs = %v, readouts = %v", inputs, readouts)
	}
	if _, err := learning.NewTrainer(config, parameters, options); err != nil {
		t.Errorf("NewTrainer: %v", err)
	}
}

func TestBuildModelRejects(t *testing.T) {
	cases := []struct {
		name     string
		variant  simulate.Variant
		nodes    int
		inputs   []int
		readouts []int
		options  modelOptions
	}{
		{name: "no nodes", nodes: 0, inputs: []int{0}, readouts: []int{2, 3}},
		{name: "targets one short", nodes: 4, inputs: []int{0}, readouts: []int{2, 3}},
		{name: "input out of range", nodes: 4, inputs: []int{4}, readouts: []int{2, 3}},
		{name: "duplicate readout", nodes: 4, inputs: []int{0}, readouts: []int{2, 2}},
		{name: "empty readouts", nodes: 4, inputs: []int{0}, readouts: nil},
		{name: "NaN weight", nodes: 4, inputs: []int{0}, readouts: []int{2, 3}},
		{name: "zero dt", nodes: 4, inputs: []int{0}, readouts: []int{2, 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			variant := fixtureVariant()
			options := fixtureOptions()
			switch tc.name {
			case "targets one short":
				variant.Targets = []int{1, 2, 3, 0}
			case "NaN weight":
				variant.Params.Weights = []float64{0.5, -0.25, math.NaN(), 0.8, 0.1}
			case "zero dt":
				options = modelOptions{DT: 0, Truncation: 8, Rate: 0.01}
			}
			if _, _, _, _, err := buildModel(variant, tc.nodes, tc.inputs, tc.readouts, options); err == nil {
				t.Errorf("buildModel succeeded, want an error")
			}
		})
	}
}

func TestMemoryPlanAndCheck(t *testing.T) {
	config, parameters, _, _, err := buildModel(fixtureVariant(), 4, []int{0}, []int{2, 3}, fixtureOptions())
	if err != nil {
		t.Fatalf("buildModel: %v", err)
	}
	plan := memoryPlan(config, parameters, 32, 0, 1, 1)
	if plan.Nodes != 4 || plan.Edges != 5 || plan.HistorySteps != 32 {
		t.Errorf("plan = nodes %d edges %d history %d, want 4/5/32", plan.Nodes, plan.Edges, plan.HistorySteps)
	}
	if plan.StateDim != 1 || plan.Individuals != 1 || plan.Precision != resources.PrecisionF64 || plan.Optimizer != resources.OptimizerAdamW {
		t.Errorf("plan = state_dim %d individuals %d precision %q optimizer %q, want 1/1/f64/adamw", plan.StateDim, plan.Individuals, plan.Precision, plan.Optimizer)
	}
	if plan.EncoderValues != 1 || plan.ReadoutValues != 2 {
		t.Errorf("plan = encoder %d readout %d values, want 1 and 2", plan.EncoderValues, plan.ReadoutValues)
	}
	if plan.PlasticEdges != 0 || plan.EligibilityEdges != 0 || plan.MaxDelay != 0 {
		t.Errorf("plan = plastic %d eligibility %d max delay %d, want 0/0/0", plan.PlasticEdges, plan.EligibilityEdges, plan.MaxDelay)
	}
	if plan.ModulationRegions != 1 || plan.ModulationChannels != 1 || plan.Receptors != 0 {
		t.Errorf("plan = regions %d channels %d receptors %d, want 1/1/0", plan.ModulationRegions, plan.ModulationChannels, plan.Receptors)
	}
	if plan.ReplayItems != 0 || plan.ReplayItemBytes != 0 || plan.BufferFactor != 0 {
		t.Errorf("plan = replay %d items %d bytes factor %v, want 0/0/0", plan.ReplayItems, plan.ReplayItemBytes, plan.BufferFactor)
	}

	roomy := 1 << 20
	report, err := checkMemory(plan, roomy)
	if err != nil {
		t.Fatalf("checkMemory with a roomy limit: %v", err)
	}
	direct, err := resources.Estimate(plan)
	if err != nil {
		t.Fatalf("resources.Estimate: %v", err)
	}
	if report.TotalBytes != direct.TotalBytes {
		t.Errorf("reported %d bytes, direct estimate %d bytes", report.TotalBytes, direct.TotalBytes)
	}
	if report.TotalBytes == 0 {
		t.Errorf("the estimated total must be above zero for a 4-node, 5-edge plan")
	}

	if _, err := checkMemory(plan, 0); err == nil {
		t.Errorf("limit 0 MiB accepted, want an error")
	}
	if _, err := checkMemory(plan, -1); err == nil {
		t.Errorf("negative limit accepted, want an error")
	}

	big := plan
	big.Edges = 1_000_000_000
	bigReport, bigErr := checkMemory(big, 1024)
	if bigErr == nil {
		t.Fatalf("a 1e9-edge plan under 1024 MiB accepted, want a refusal")
	}
	if bigReport.TotalBytes == 0 {
		t.Errorf("refusal should still report the estimated total, got 0")
	}
	expected, err := resources.Estimate(big)
	if err != nil {
		t.Fatalf("resources.Estimate of the big plan: %v", err)
	}
	const mib = uint64(1 << 20)
	needMiB := expected.TotalBytes / mib
	if expected.TotalBytes%mib != 0 {
		needMiB++
	}
	msg := bigErr.Error()
	if !strings.Contains(msg, "1024") || !strings.Contains(msg, strconv.FormatUint(needMiB, 10)) {
		t.Errorf("refusal %q must name both MiB numbers (%d estimate, 1024 limit)", msg, needMiB)
	}
}

func deepCopyVariant(v simulate.Variant) simulate.Variant {
	return simulate.Variant{
		Name:    v.Name,
		Sources: slices.Clone(v.Sources),
		Targets: slices.Clone(v.Targets),
		Params: simulate.ParameterSet{
			Source:   v.Params.Source,
			Weights:  slices.Clone(v.Params.Weights),
			Signs:    slices.Clone(v.Params.Signs),
			Bias:     slices.Clone(v.Params.Bias),
			LogTau:   slices.Clone(v.Params.LogTau),
			ThetaRaw: slices.Clone(v.Params.ThetaRaw),
			Hash:     v.Params.Hash,
		},
	}
}

// variantEqual reports whether two variants carry identical slices, for proving
// that a call never touched the caller's arrays.
func variantEqual(a, b simulate.Variant) bool {
	return slices.Equal(a.Sources, b.Sources) &&
		slices.Equal(a.Targets, b.Targets) &&
		slices.Equal(a.Params.Weights, b.Params.Weights) &&
		slices.Equal(a.Params.Signs, b.Params.Signs) &&
		slices.Equal(a.Params.Bias, b.Params.Bias) &&
		slices.Equal(a.Params.LogTau, b.Params.LogTau) &&
		slices.Equal(a.Params.ThetaRaw, b.Params.ThetaRaw)
}
