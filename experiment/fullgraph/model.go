// Package fullgraph is the OPS-07 short-training protocol on the whole main
// graph: every node and edge of the imported graph becomes one trainable model,
// its memory is estimated before anything is allocated, and nothing is ever
// shrunk to fit.
package fullgraph

import (
	"fmt"
	"math"
	"slices"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/resources"
	"github.com/TimLai666/coimnet/simulate"
)

// modelOptions fixes the model around the graph: continuous core with tanh and
// step DT, one input channel fanned out to every input node through the
// encoder, one output read from the readout nodes, and which parameter groups
// train.
type modelOptions struct {
	DT         float64 `json:"dt"`            // > 0
	Truncation int     `json:"truncation"`    // ≥ 0, handed to learning.Options.Truncation
	Rate       float64 `json:"learning_rate"` // > 0
}

// modelReport records what the conversion did, so the evidence can show every
// edge and node took part.
type modelReport struct {
	Nodes         int `json:"nodes"`
	Edges         int `json:"edges"`
	PositiveEdges int `json:"positive_edges"`       // fixed sign +1
	NegativeEdges int `json:"negative_edges"`       // fixed sign -1
	FreeEdges     int `json:"free_edges"`           // sign 0 from the variant
	ZeroFreed     int `json:"zero_magnitude_freed"` // variant said ±1 but the strength was exactly 0: stored as a free edge with weight 0, because a fixed-sign edge cannot hold zero
	InputNodes    int `json:"input_nodes"`
	ReadoutNodes  int `json:"readout_nodes"`
	Parameters    int `json:"parameters"` // len of every learnable array together
}

// buildModel turns a simulate variant into a learning model over every node and
// every edge of the graph, in the variant's edge order:
// Dynamics{Nodes: nodes, Sources, Targets, DT: o.DT, Activation "tanh"};
// EdgeSigns[i] = sign of Params.Signs[i] (the variant's signs are already the
// fixed ones), Weights[i] = log(|w_i|) for a fixed-sign edge with w_i ≠ 0 and
// the raw w_i for a free edge (a fixed-sign edge whose w_i == 0 becomes free
// with weight 0 and is counted in ZeroFreed); Bias and LogTau copied from the
// variant; InputSize 1, InputNodes = inputs (sorted, distinct, in range),
// Encoder = one 1 per input node; OutputSize 1, ReadoutNodes = readouts
// (sorted, distinct, in range), Readout = 1/len(readouts) for each readout
// node. The returned options are DefaultOptions with LearningRate o.Rate,
// Truncation o.Truncation and Trainable Weights, Bias, LogTau, Encoder and
// Readout all true (every learnable group of a continuous core). The input
// arrays are never modified. Errors: nodes ≤ 0, a variant whose
// Sources/Targets/Weights/Signs lengths differ, an index out of range, empty
// inputs or readouts, a non-finite weight, o invalid.
func buildModel(v simulate.Variant, nodes int, inputs, readouts []int, o modelOptions) (learning.Config, learning.Parameters, learning.Options, modelReport, error) {
	var zero modelReport
	if err := validateOptions(o); err != nil {
		return learning.Config{}, learning.Parameters{}, learning.Options{}, zero, err
	}
	if nodes <= 0 {
		return learning.Config{}, learning.Parameters{}, learning.Options{}, zero, fmt.Errorf("fullgraph: nodes must be positive, got %d", nodes)
	}
	edges := len(v.Sources)
	if len(v.Targets) != edges || len(v.Params.Weights) != edges || len(v.Params.Signs) != edges {
		return learning.Config{}, learning.Parameters{}, learning.Options{}, zero,
			fmt.Errorf("fullgraph: variant has %d sources, %d targets, %d weights and %d signs; all four must match", edges, len(v.Targets), len(v.Params.Weights), len(v.Params.Signs))
	}
	if len(v.Params.Bias) != nodes || len(v.Params.LogTau) != nodes {
		return learning.Config{}, learning.Parameters{}, learning.Options{}, zero,
			fmt.Errorf("fullgraph: variant has %d bias and %d log_tau values, want %d of each (one per node)", len(v.Params.Bias), len(v.Params.LogTau), nodes)
	}
	inputNodes, err := sortedDistinct("input", inputs, nodes)
	if err != nil {
		return learning.Config{}, learning.Parameters{}, learning.Options{}, zero, err
	}
	readoutNodes, err := sortedDistinct("readout", readouts, nodes)
	if err != nil {
		return learning.Config{}, learning.Parameters{}, learning.Options{}, zero, err
	}
	edgeSigns := make([]int8, edges)
	weights := make([]float64, edges)
	report := modelReport{Nodes: nodes, Edges: edges, InputNodes: len(inputNodes), ReadoutNodes: len(readoutNodes)}
	for i := range edges {
		weight := v.Params.Weights[i]
		if !finite(weight) {
			return learning.Config{}, learning.Parameters{}, learning.Options{}, zero, fmt.Errorf("fullgraph: edge %d weight is not finite", i)
		}
		switch sign := v.Params.Signs[i]; sign {
		case 0:
			edgeSigns[i], weights[i], report.FreeEdges = 0, weight, report.FreeEdges+1
		case 1:
			if weight == 0 {
				edgeSigns[i], weights[i], report.FreeEdges, report.ZeroFreed = 0, 0, report.FreeEdges+1, report.ZeroFreed+1
				break
			}
			edgeSigns[i], weights[i], report.PositiveEdges = 1, math.Log(math.Abs(weight)), report.PositiveEdges+1
		case -1:
			if weight == 0 {
				edgeSigns[i], weights[i], report.FreeEdges, report.ZeroFreed = 0, 0, report.FreeEdges+1, report.ZeroFreed+1
				break
			}
			edgeSigns[i], weights[i], report.NegativeEdges = -1, math.Log(math.Abs(weight)), report.NegativeEdges+1
		default:
			return learning.Config{}, learning.Parameters{}, learning.Options{}, zero, fmt.Errorf("fullgraph: edge %d sign is %d, want -1, 0 or +1", i, sign)
		}
	}
	bias := append([]float64(nil), v.Params.Bias...)
	logTau := append([]float64(nil), v.Params.LogTau...)
	for name, group := range map[string][]float64{"bias": bias, "log_tau": logTau} {
		for i, value := range group {
			if !finite(value) {
				return learning.Config{}, learning.Parameters{}, learning.Options{}, zero, fmt.Errorf("fullgraph: %s[%d] is not finite", name, i)
			}
		}
	}
	encoder := make([]float64, len(inputNodes))
	for i := range encoder {
		encoder[i] = 1
	}
	readout := make([]float64, len(readoutNodes))
	for i := range readout {
		readout[i] = 1 / float64(len(readoutNodes))
	}
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      nodes,
			Sources:    append([]int(nil), v.Sources...),
			Targets:    append([]int(nil), v.Targets...),
			DT:         o.DT,
			Activation: "tanh",
		},
		InputSize:    1,
		OutputSize:   1,
		InputNodes:   inputNodes,
		ReadoutNodes: readoutNodes,
		EdgeSigns:    edgeSigns,
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: weights, Bias: bias, LogTau: logTau},
		Encoder: encoder,
		Readout: readout,
	}
	options := learning.DefaultOptions()
	options.LearningRate = o.Rate
	options.Truncation = o.Truncation
	report.Parameters = len(weights) + len(bias) + len(logTau) + len(parameters.ThetaRaw) + len(encoder) + len(readout)
	return config, parameters, options, report, nil
}

// validateOptions rejects a modelOptions value the continuous core, the BPTT
// window or the optimizer could not use.
func validateOptions(o modelOptions) error {
	if !finite(o.DT) || o.DT <= 0 {
		return fmt.Errorf("fullgraph: dt must be finite and above zero, got %v", o.DT)
	}
	if o.Truncation < 0 {
		return fmt.Errorf("fullgraph: truncation must not be negative, got %d", o.Truncation)
	}
	if !finite(o.Rate) || o.Rate <= 0 {
		return fmt.Errorf("fullgraph: learning rate must be finite and above zero, got %v", o.Rate)
	}
	return nil
}

// sortedDistinct sorts a copy of ids ascending and refuses an empty list, an
// index outside [0, nodes) or a duplicate, so the produced model never has to
// guess which of two claims on the same node is meant.
func sortedDistinct(kind string, ids []int, nodes int) ([]int, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("fullgraph: the %s node list is empty", kind)
	}
	out := slices.Clone(ids)
	slices.Sort(out)
	for i, id := range out {
		if id < 0 || id >= nodes {
			return nil, fmt.Errorf("fullgraph: %s node %d is out of range [0, %d)", kind, id, nodes)
		}
		if i > 0 && out[i-1] == id {
			return nil, fmt.Errorf("fullgraph: %s node %d appears more than once", kind, id)
		}
	}
	return out, nil
}

// memoryPlan is the resources.Plan of one short training run of this model:
// Nodes and Edges from the config, StateDim 1, Individuals 1, HistorySteps =
// steps, Precision f64, Optimizer adamw, EncoderValues and ReadoutValues from
// the parameters, PlasticEdges = plasticEdges (0 when plasticity is off),
// Regions and Channels = chemistry sizes. Read resources.Plan for the exact
// field names and set every field the plan declares; do not invent fields.
func memoryPlan(c learning.Config, p learning.Parameters, steps, plasticEdges, regions, channels int) resources.Plan {
	maxDelay := 0
	for _, delay := range c.Dynamics.Delays {
		if delay > maxDelay {
			maxDelay = delay
		}
	}
	return resources.Plan{
		Nodes:              c.Dynamics.Nodes,
		Edges:              len(c.Dynamics.Sources),
		StateDim:           1,
		Individuals:        1,
		HistorySteps:       steps,
		Precision:          resources.PrecisionF64,
		Optimizer:          resources.OptimizerAdamW,
		PlasticEdges:       plasticEdges,
		EligibilityEdges:   0,
		MaxDelay:           maxDelay,
		EncoderValues:      len(p.Encoder),
		ReadoutValues:      len(p.Readout),
		ModulationRegions:  regions,
		ModulationChannels: channels,
		Receptors:          0,
		ReplayItems:        0,
		ReplayItemBytes:    0,
		BufferFactor:       0,
	}
}

// checkMemory runs resources.Estimate and refuses (error naming both numbers
// in MiB) when the estimate exceeds limitMiB; it never shrinks the model. The
// report is returned alongside a refusal so the caller can record the estimate
// that was too large. limitMiB ≤ 0 is an error.
func checkMemory(plan resources.Plan, limitMiB int) (resources.Report, error) {
	if limitMiB <= 0 {
		return resources.Report{}, fmt.Errorf("fullgraph: memory limit must be a positive number of MiB, got %d", limitMiB)
	}
	report, err := resources.Estimate(plan)
	if err != nil {
		return resources.Report{}, err
	}
	const mib = uint64(1 << 20)
	limit := uint64(limitMiB)
	if limit > math.MaxUint64/mib {
		return report, nil // the limit exceeds any representable uint64 byte count
	}
	if report.TotalBytes <= limit*mib {
		return report, nil
	}
	need := report.TotalBytes / mib
	if report.TotalBytes%mib != 0 {
		need++
	}
	return report, fmt.Errorf("fullgraph: the plan estimates %d MiB, above the %d MiB limit; nothing was shrunk to make it fit", need, limitMiB)
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
