// Package resources turns a declared run plan into a complete, itemised memory
// estimate before anything is allocated. It exists so that a run that does not
// fit is refused with the arithmetic that refused it, instead of being made to
// fit by quietly shrinking the graph, the batch or the history window.
//
// Every number here is arithmetic over the arrays a run declares. It is not a
// resident set size: Go allocator slack, decoded JSON, the runtime itself and
// anything a dependency allocates are deliberately outside the accounting, in
// the same way connectome.StoreLimits accounts for store arrays only.
package resources

import (
	"fmt"
	"math"
	"strings"
)

// The declared numeric precisions and optimizers. A plan names one of each;
// an unknown name is refused rather than mapped onto a neighbour.
const (
	PrecisionF32 = "f32"
	PrecisionF64 = "f64"

	OptimizerNone  = "none"
	OptimizerAdamW = "adamw"
)

// Plan is one declared run, counted in whole items rather than bytes.
//
// Nodes and Edges are the computed graph. StateDim (C) is how many values one
// node's current state holds and Individuals (B) how many trajectories advance
// together. HistorySteps (T) is the backward window that keeps two values per
// edge per step; zero means no backward history is kept. PlasticEdges and
// EligibilityEdges are the subsets carrying a fast change and a recent
// participation record. MaxDelay is the longest declared conduction delay, so
// a model keeps MaxDelay+1 output frames. EncoderValues and ReadoutValues are
// the peripheral parameter counts. ModulationRegions, ModulationChannels and
// Receptors size the modulation state. ReplayItems and ReplayItemBytes size
// the replay buffer, which is counted in its own bytes rather than in model
// values. BufferFactor is the working-buffer allowance applied to the sum of
// everything above; zero declares that no allowance is estimated.
type Plan struct {
	Nodes              int     `json:"nodes"`
	Edges              int     `json:"edges"`
	StateDim           int     `json:"state_dim"`
	Individuals        int     `json:"individuals"`
	HistorySteps       int     `json:"history_steps"`
	Precision          string  `json:"precision"`
	Optimizer          string  `json:"optimizer"`
	PlasticEdges       int     `json:"plastic_edges"`
	EligibilityEdges   int     `json:"eligibility_edges"`
	MaxDelay           int     `json:"max_delay"`
	EncoderValues      int     `json:"encoder_values"`
	ReadoutValues      int     `json:"readout_values"`
	ModulationRegions  int     `json:"modulation_regions"`
	ModulationChannels int     `json:"modulation_channels"`
	Receptors          int     `json:"receptors"`
	ReplayItems        int     `json:"replay_items"`
	ReplayItemBytes    int     `json:"replay_item_bytes"`
	BufferFactor       float64 `json:"buffer_factor"`
}

// Item is one accounted contribution. Formula carries the rule and the same
// rule with this plan's numbers substituted, so a reader can redo the
// arithmetic without the plan in front of them.
type Item struct {
	Name    string `json:"name"`
	Formula string `json:"formula"`
	Bytes   uint64 `json:"bytes"`
}

// Report lists every item, including the ones that contribute nothing, so that
// two reports of the same build are comparable line by line.
type Report struct {
	Items      []Item `json:"items"`
	TotalBytes uint64 `json:"total_bytes"`
}

// errOverflow names the item whose arithmetic did not fit in a uint64. The
// word "overflow" is part of the message because a caller that sees it has to
// treat the plan as unanswerable rather than as merely large.
func errOverflow(item, formula string) error {
	return fmt.Errorf("resources: %s overflows a 64-bit byte count: %s", item, formula)
}

// mul multiplies without wrapping.
func mul(factors ...uint64) (uint64, bool) {
	product := uint64(1)
	for _, factor := range factors {
		if factor == 0 {
			return 0, true
		}
		if product > math.MaxUint64/factor {
			return 0, false
		}
		product *= factor
	}
	return product, true
}

// add sums without wrapping.
func add(terms ...uint64) (uint64, bool) {
	var total uint64
	for _, term := range terms {
		if total > math.MaxUint64-term {
			return 0, false
		}
		total += term
	}
	return total, true
}

// Estimate returns the complete itemised estimate of one plan. It never
// changes the plan and never lowers a declared quantity: a plan whose
// arithmetic does not fit is an error, not a smaller plan.
func Estimate(p Plan) (Report, error) {
	width, err := validate(p)
	if err != nil {
		return Report{}, err
	}
	w := uint64(width)
	n := uint64(p.Nodes)
	e := uint64(p.Edges)
	c := uint64(p.StateDim)
	b := uint64(p.Individuals)
	steps := uint64(p.HistorySteps)

	items := make([]Item, 0, 13)
	appendItem := func(name, rule, substituted string, value uint64, ok bool) error {
		formula := rule + " = " + substituted
		if !ok {
			return errOverflow(name, formula)
		}
		items = append(items, Item{Name: name, Formula: fmt.Sprintf("%s = %d", formula, value), Bytes: value})
		return nil
	}
	// appendZero records an item whose rule does not apply to this plan at all,
	// so that the report still lists it without printing arithmetic nobody did.
	appendZero := func(name, reason string) {
		items = append(items, Item{Name: name, Formula: "0 (" + reason + ")", Bytes: 0})
	}

	// The graph index is the compressed row offsets plus one endpoint per edge,
	// both as 8-byte integers. A plan with no nodes declares no graph and
	// therefore no index; validate has already refused edges without nodes.
	var index uint64
	if p.Nodes == 0 {
		appendZero("graph_index", "no nodes, so there is no index")
	} else {
		value, ok := mul(n+1+e, 8)
		if err := appendItem("graph_index", "(N+1+E)*8", fmt.Sprintf("(%d+1+%d)*8", p.Nodes, p.Edges), value, ok); err != nil {
			return Report{}, err
		}
		index = value
	}

	parameters, ok := mul(e, w)
	if err := appendItem("base_parameters", "E*w", fmt.Sprintf("%d*%d", p.Edges, width), parameters, ok); err != nil {
		return Report{}, err
	}

	var optimizer uint64
	if p.Optimizer == OptimizerAdamW {
		value, ok := mul(3, e, w)
		if err := appendItem("optimizer", "3*E*w", fmt.Sprintf("3*%d*%d", p.Edges, width), value, ok); err != nil {
			return Report{}, err
		}
		optimizer = value
	} else {
		appendZero("optimizer", "optimizer "+OptimizerNone)
	}

	state, ok := mul(n, c, w, b)
	if err := appendItem("neural_state", "N*C*w*B", fmt.Sprintf("%d*%d*%d*%d", p.Nodes, p.StateDim, width, p.Individuals), state, ok); err != nil {
		return Report{}, err
	}

	fast, ok := mul(uint64(p.PlasticEdges), w, b)
	if err := appendItem("fast_changes", "PlasticEdges*w*B", fmt.Sprintf("%d*%d*%d", p.PlasticEdges, width, p.Individuals), fast, ok); err != nil {
		return Report{}, err
	}

	eligibility, ok := mul(uint64(p.EligibilityEdges), w, b)
	if err := appendItem("eligibility", "EligibilityEdges*w*B", fmt.Sprintf("%d*%d*%d", p.EligibilityEdges, width, p.Individuals), eligibility, ok); err != nil {
		return Report{}, err
	}

	delays, ok := mul(n, uint64(p.MaxDelay)+1, w, b)
	if err := appendItem("delays", "N*(MaxDelay+1)*w*B", fmt.Sprintf("%d*(%d+1)*%d*%d", p.Nodes, p.MaxDelay, width, p.Individuals), delays, ok); err != nil {
		return Report{}, err
	}

	history, ok := mul(2, e, w, steps, b)
	if err := appendItem("backward_history", "2*E*w*T*B", fmt.Sprintf("2*%d*%d*%d*%d", p.Edges, width, p.HistorySteps, p.Individuals), history, ok); err != nil {
		return Report{}, err
	}

	encoder, ok := mul(uint64(p.EncoderValues), w)
	if err := appendItem("encoder", "EncoderValues*w", fmt.Sprintf("%d*%d", p.EncoderValues, width), encoder, ok); err != nil {
		return Report{}, err
	}

	readout, ok := mul(uint64(p.ReadoutValues), w)
	if err := appendItem("readout", "ReadoutValues*w", fmt.Sprintf("%d*%d", p.ReadoutValues, width), readout, ok); err != nil {
		return Report{}, err
	}

	concentrations, ok := mul(uint64(p.ModulationRegions), uint64(p.ModulationChannels))
	if !ok {
		return Report{}, errOverflow("modulation", "(Regions*Channels + Receptors)*w")
	}
	occupancy, ok := add(concentrations, uint64(p.Receptors))
	if !ok {
		return Report{}, errOverflow("modulation", "(Regions*Channels + Receptors)*w")
	}
	modulation, ok := mul(occupancy, w)
	if err := appendItem("modulation", "(Regions*Channels + Receptors)*w",
		fmt.Sprintf("(%d*%d + %d)*%d", p.ModulationRegions, p.ModulationChannels, p.Receptors, width), modulation, ok); err != nil {
		return Report{}, err
	}

	replay, ok := mul(uint64(p.ReplayItems), uint64(p.ReplayItemBytes))
	if err := appendItem("replay", "ReplayItems*ReplayItemBytes", fmt.Sprintf("%d*%d", p.ReplayItems, p.ReplayItemBytes), replay, ok); err != nil {
		return Report{}, err
	}

	subtotal, ok := add(index, parameters, optimizer, state, fast, eligibility, delays, history, encoder, readout, modulation, replay)
	if !ok {
		return Report{}, errOverflow("subtotal", "sum of every accounted item")
	}
	buffers := float64(p.BufferFactor) * float64(subtotal)
	if math.IsNaN(buffers) || math.IsInf(buffers, 0) || buffers >= math.MaxUint64 {
		return Report{}, errOverflow("buffers", fmt.Sprintf("BufferFactor * subtotal = %v * %d", p.BufferFactor, subtotal))
	}
	if err := appendItem("buffers", "BufferFactor*subtotal",
		fmt.Sprintf("%v*%d", p.BufferFactor, subtotal), uint64(buffers), true); err != nil {
		return Report{}, err
	}

	total, ok := add(subtotal, uint64(buffers))
	if !ok {
		return Report{}, errOverflow("total", "subtotal + buffers")
	}
	return Report{Items: items, TotalBytes: total}, nil
}

// Check refuses a report that does not fit in limitBytes and lists every item
// it accounted for, so the reader can see which one to change. It reads its
// arguments and changes nothing: the caller's report, and the plan behind it,
// are the same values afterwards.
func Check(r Report, limitBytes uint64) error {
	if r.TotalBytes <= limitBytes {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "resources: the declared plan needs %d bytes, which is above the limit of %d bytes; nothing was reduced to make it fit. Accounted items:", r.TotalBytes, limitBytes)
	for _, item := range r.Items {
		fmt.Fprintf(&b, "\n  %s: %d bytes (%s)", item.Name, item.Bytes, item.Formula)
	}
	return fmt.Errorf("%s", b.String())
}

// validate refuses a plan that cannot be counted and returns the byte width of
// one value.
func validate(p Plan) (int, error) {
	counts := []struct {
		name  string
		value int
	}{
		{"nodes", p.Nodes}, {"edges", p.Edges}, {"state_dim", p.StateDim}, {"individuals", p.Individuals},
		{"history_steps", p.HistorySteps}, {"plastic_edges", p.PlasticEdges}, {"eligibility_edges", p.EligibilityEdges},
		{"max_delay", p.MaxDelay}, {"encoder_values", p.EncoderValues}, {"readout_values", p.ReadoutValues},
		{"modulation_regions", p.ModulationRegions}, {"modulation_channels", p.ModulationChannels},
		{"receptors", p.Receptors}, {"replay_items", p.ReplayItems}, {"replay_item_bytes", p.ReplayItemBytes},
	}
	for _, count := range counts {
		if count.value < 0 {
			return 0, fmt.Errorf("resources: plan field %s is %d, which is not a count", count.name, count.value)
		}
	}
	if p.Nodes == 0 && p.Edges > 0 {
		return 0, fmt.Errorf("resources: plan declares %d edges and no nodes", p.Edges)
	}
	if p.PlasticEdges > p.Edges {
		return 0, fmt.Errorf("resources: plan declares %d plastic edges of %d edges", p.PlasticEdges, p.Edges)
	}
	if p.EligibilityEdges > p.Edges {
		return 0, fmt.Errorf("resources: plan declares %d eligibility edges of %d edges", p.EligibilityEdges, p.Edges)
	}
	if math.IsNaN(p.BufferFactor) || math.IsInf(p.BufferFactor, 0) || p.BufferFactor < 0 {
		return 0, fmt.Errorf("resources: buffer factor %v must be a finite non-negative number", p.BufferFactor)
	}
	switch p.Optimizer {
	case OptimizerNone, OptimizerAdamW:
	default:
		return 0, fmt.Errorf("resources: unsupported optimizer %q; the declared optimizers are %q and %q", p.Optimizer, OptimizerNone, OptimizerAdamW)
	}
	switch p.Precision {
	case PrecisionF32:
		return 4, nil
	case PrecisionF64:
		return 8, nil
	}
	return 0, fmt.Errorf("resources: unsupported precision %q; the declared precisions are %q and %q", p.Precision, PrecisionF32, PrecisionF64)
}
