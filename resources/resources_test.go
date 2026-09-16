package resources

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// itemBytes returns one named item, failing when the report does not carry it.
// Every Estimate result lists every item, including the ones that are zero, so
// a missing name is a contract failure rather than an absent contribution.
func itemBytes(t *testing.T, r Report, name string) uint64 {
	t.Helper()
	for _, item := range r.Items {
		if item.Name == name {
			return item.Bytes
		}
	}
	t.Fatalf("report has no item %q; items = %+v", name, r.Items)
	return 0
}

func itemFormula(t *testing.T, r Report, name string) string {
	t.Helper()
	for _, item := range r.Items {
		if item.Name == name {
			return item.Formula
		}
	}
	t.Fatalf("report has no item %q", name)
	return ""
}

// specificationPlan is the master specification's own 17.1 arithmetic example:
// fifteen million edges at float32 under AdamW.
func specificationPlan() Plan {
	return Plan{
		Nodes:        100000,
		Edges:        15000000,
		StateDim:     1,
		Individuals:  1,
		HistorySteps: 0,
		Precision:    PrecisionF32,
		Optimizer:    OptimizerAdamW,
	}
}

func TestEstimatePinsSpecificationParameterAndOptimizerArithmetic(t *testing.T) {
	report, err := Estimate(specificationPlan())
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	parameters := itemBytes(t, report, "base_parameters")
	optimizer := itemBytes(t, report, "optimizer")
	if parameters != 60000000 {
		t.Fatalf("base parameters = %d, want E*w = 15000000*4 = 60000000", parameters)
	}
	if optimizer != 180000000 {
		t.Fatalf("optimizer = %d, want 3*E*w = 180000000", optimizer)
	}
	// 16E is the specification's own shorthand for parameters plus AdamW state
	// at float32: 4 bytes times four copies of every edge parameter.
	if parameters+optimizer != 240000000 {
		t.Fatalf("parameters+optimizer = %d, want 16E = 240000000", parameters+optimizer)
	}
	if got := itemBytes(t, report, "backward_history"); got != 0 {
		t.Fatalf("backward history without a truncation window = %d, want 0", got)
	}
	if formula := itemFormula(t, report, "base_parameters"); !strings.Contains(formula, "15000000") || !strings.Contains(formula, "60000000") {
		t.Fatalf("base parameter formula %q does not substitute its numbers", formula)
	}
}

func TestEstimatePinsSpecificationBackwardHistoryArithmetic(t *testing.T) {
	plan := specificationPlan()
	plan.HistorySteps = 128
	plan.Individuals = 1
	report, err := Estimate(plan)
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	// 2*E*w*T*B is the specification's 8BE*T for float32.
	if got := itemBytes(t, report, "backward_history"); got != 15360000000 {
		t.Fatalf("backward history = %d, want 2*E*w*T*B = 2*15000000*4*128*1 = 15360000000", got)
	}
	if formula := itemFormula(t, report, "backward_history"); !strings.Contains(formula, "128") || !strings.Contains(formula, "15360000000") {
		t.Fatalf("backward history formula %q does not substitute its numbers", formula)
	}
}

func TestEstimateZeroPlanIsZero(t *testing.T) {
	report, err := Estimate(Plan{Precision: PrecisionF64, Optimizer: OptimizerNone})
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	if report.TotalBytes != 0 {
		t.Fatalf("total for a plan that declares nothing = %d, want 0", report.TotalBytes)
	}
	if len(report.Items) == 0 {
		t.Fatal("a zero plan still has to list every item")
	}
	for _, item := range report.Items {
		if item.Bytes != 0 {
			t.Fatalf("item %q = %d, want 0", item.Name, item.Bytes)
		}
	}
}

func TestEstimateRejectsInvalidDeclarations(t *testing.T) {
	cases := map[string]Plan{
		"empty precision":       {Optimizer: OptimizerNone},
		"unknown precision":     {Precision: "f16", Optimizer: OptimizerNone},
		"unknown optimizer":     {Precision: PrecisionF64, Optimizer: "sgd"},
		"negative nodes":        {Nodes: -1, Precision: PrecisionF64, Optimizer: OptimizerNone},
		"negative edges":        {Edges: -1, Precision: PrecisionF64, Optimizer: OptimizerNone},
		"edges without nodes":   {Edges: 4, Precision: PrecisionF64, Optimizer: OptimizerNone},
		"negative delay":        {Nodes: 1, MaxDelay: -1, Precision: PrecisionF64, Optimizer: OptimizerNone},
		"negative buffer":       {Precision: PrecisionF64, Optimizer: OptimizerNone, BufferFactor: -0.5},
		"non-finite buffer":     {Precision: PrecisionF64, Optimizer: OptimizerNone, BufferFactor: math.Inf(1)},
		"plastic beyond edges":  {Nodes: 2, Edges: 1, PlasticEdges: 2, Precision: PrecisionF64, Optimizer: OptimizerNone},
		"eligibility no edges":  {Nodes: 2, EligibilityEdges: 1, Precision: PrecisionF64, Optimizer: OptimizerNone},
		"negative replay bytes": {Precision: PrecisionF64, Optimizer: OptimizerNone, ReplayItems: 1, ReplayItemBytes: -1},
	}
	for name, plan := range cases {
		if _, err := Estimate(plan); err == nil {
			t.Fatalf("%s: Estimate accepted %+v", name, plan)
		}
	}
}

func TestEstimateRejectsOverflowInsteadOfWrapping(t *testing.T) {
	plan := Plan{
		Nodes:       2,
		Edges:       math.MaxInt64 / 4,
		StateDim:    1,
		Individuals: 1,
		Precision:   PrecisionF64,
		Optimizer:   OptimizerAdamW,
	}
	if _, err := Estimate(plan); err == nil {
		t.Fatal("Estimate wrapped an overflowing edge count instead of reporting it")
	} else if !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("overflow error = %v, want it to name the overflow", err)
	}
	buffered := Plan{
		Nodes:        1,
		Edges:        1,
		StateDim:     1,
		Individuals:  1,
		Precision:    PrecisionF64,
		Optimizer:    OptimizerNone,
		BufferFactor: math.MaxFloat64,
	}
	if _, err := Estimate(buffered); err == nil {
		t.Fatal("Estimate accepted a buffer factor whose product does not fit")
	}
}

func TestCheckRefusesOverLimitAndChangesNothing(t *testing.T) {
	plan := specificationPlan()
	plan.HistorySteps = 128
	before := plan
	report, err := Estimate(plan)
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	if !reflect.DeepEqual(plan, before) {
		t.Fatal("Estimate changed the plan it was given")
	}
	reportBefore := Report{Items: append([]Item(nil), report.Items...), TotalBytes: report.TotalBytes}
	err = Check(report, 1<<20)
	if err == nil {
		t.Fatal("Check accepted a report far over its limit")
	}
	for _, item := range report.Items {
		if !strings.Contains(err.Error(), item.Name) {
			t.Fatalf("refusal %q does not list item %q", err, item.Name)
		}
	}
	if !reflect.DeepEqual(report, reportBefore) {
		t.Fatal("Check changed the report it was given")
	}
	if !reflect.DeepEqual(plan, before) {
		t.Fatal("Check changed the plan behind the report")
	}
	if err := Check(report, report.TotalBytes); err != nil {
		t.Fatalf("Check refused a report exactly at its limit: %v", err)
	}
}

// maleFullPlan is the whole annotated MaleCNS v1.0 graph of evidence/NAT-01
// (165,122 nodes, 25,563,197 edges) under the declared training plan: float64,
// AdamW, one individual, no truncation window, no declared delay, no
// plasticity, no modulation, no replay, and no buffer allowance. The encoder
// and readout counts come from a four-input, one-output selection: four input
// channels projected onto four selected neurons, and one readout neuron
// projected onto one output channel.
func maleFullPlan() Plan {
	return Plan{
		Nodes:         165122,
		Edges:         25563197,
		StateDim:      1,
		Individuals:   1,
		HistorySteps:  0,
		Precision:     PrecisionF64,
		Optimizer:     OptimizerAdamW,
		MaxDelay:      0,
		EncoderValues: 16,
		ReadoutValues: 1,
	}
}

// TestMaleFullEstimateIsRecorded computes the whole annotated MaleCNS v1.0
// graph under the declared training plan. The numbers here are the ones
// evidence/OPS-06/verification.json reports; they are arithmetic over declared
// arrays and are not a claim about resident set size.
func TestMaleFullEstimateIsRecorded(t *testing.T) {
	report, err := Estimate(maleFullPlan())
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	want := map[string]uint64{
		"graph_index":      205826560,
		"base_parameters":  204505576,
		"optimizer":        613516728,
		"neural_state":     1320976,
		"fast_changes":     0,
		"eligibility":      0,
		"delays":           1320976,
		"backward_history": 0,
		"encoder":          128,
		"readout":          8,
		"modulation":       0,
		"replay":           0,
		"buffers":          0,
	}
	for name, bytes := range want {
		if got := itemBytes(t, report, name); got != bytes {
			t.Fatalf("male-full item %q = %d, want %d", name, got, bytes)
		}
	}
	if report.TotalBytes != 1026490952 {
		t.Fatalf("male-full total = %d, want 1026490952", report.TotalBytes)
	}
	for _, item := range report.Items {
		t.Logf("%-18s %16d  %s", item.Name, item.Bytes, item.Formula)
	}
	t.Logf("%-18s %16d", "total_bytes", report.TotalBytes)
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	t.Logf("report json:\n%s", encoded)
}
