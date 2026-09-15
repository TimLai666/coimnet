package learning

import (
	"errors"
	"fmt"
	"math"
)

// UpdateMasks selects individual edges and nodes inside a trainable group. The
// effective mask is the group flag of Options.Trainable AND the per-item entry
// here: Edges applies to the weight group and Nodes applies to bias, log_tau
// and theta_raw together. A nil UpdateMasks, or a nil half of one, means every
// item of the affected groups follows its group flag alone. A masked parameter
// keeps its value, its Adam moments and its step count unchanged, weight decay
// and range projection included.
type UpdateMasks struct {
	Edges []bool `json:"edges,omitempty"`
	Nodes []bool `json:"nodes,omitempty"`
}

// ParameterRanges bounds parameter values. Zero means unlimited in every
// field, so an empty ParameterRanges constrains nothing. The bounds are
// applied by projection after the AdamW update, which is a declared rule on
// parameters: it never changes optimizer moments or step counts, and it never
// touches a masked parameter. WeightMagnitudeMax bounds the weight the core
// integrates, so on a fixed-sign edge it becomes a cap on the log magnitude at
// log(WeightMagnitudeMax).
type ParameterRanges struct {
	WeightMagnitudeMax float64 `json:"weight_magnitude_max,omitempty"`
	BiasAbsMax         float64 `json:"bias_abs_max,omitempty"`
	LogTauMin          float64 `json:"log_tau_min,omitempty"`
	LogTauMax          float64 `json:"log_tau_max,omitempty"`
	ThetaRawAbsMax     float64 `json:"theta_raw_abs_max,omitempty"`
}

// The unknown-sign policies a derived parameter set can be read under. The
// three names match the ones simulate declares for the same question, except
// that a trainer replaces simulate's "exclude" with "free": excluding an edge
// would mean a permanent zero weight, while a trainer can simply leave that
// edge's sign unconstrained and learn it.
const (
	UnknownSignFree       = "free"
	UnknownSignExcitatory = "excitatory"
	UnknownSignInhibitory = "inhibitory"
)

// defaultMinLogMagnitude is the floor Config.MinLogMagnitude selects when it is
// left at zero. exp(-20) is about 2.06e-9, far above the smallest normal
// float64 and far below any weight the derivation produces.
const defaultMinLogMagnitude = -20

// maxLogMagnitude is the largest log magnitude with a finite exponential.
var maxLogMagnitude = math.Log(math.MaxFloat64)

// SignSummary reports what a derived parameter set contributed and which
// policy the unknown edges were read under.
type SignSummary struct {
	Policy        string `json:"unknown_sign_policy"`
	PositiveEdges uint64 `json:"positive_edges"`
	NegativeEdges uint64 `json:"negative_edges"`
	UnknownEdges  uint64 `json:"unknown_sign_edges"`
}

// DerivedSetSource is the declared source label of a parameter set produced by
// the params package. It is repeated here as a string constant rather than
// imported, for the reason DerivedSigns documents.
const DerivedSetSource = "derived_release/v1"

// DerivedSigns carries the three fields of a derived parameter set that the
// sign mapping reads. The ticket sketched this function as taking a
// *params.Set; it takes the fields instead because params imports connectome
// and connectome's own tests are in package connectome and import learning, so
// importing params here closes an import cycle in connectome's test binary.
// Build one at the call site with
//
//	learning.DerivedSigns{Source: set.Source, EdgeSigns: set.EdgeSign, Edges: set.Edges()}
type DerivedSigns struct {
	Source    string
	EdgeSigns []int8
	Edges     int
}

// SignsFromParameterSet turns the per-edge signs of a derived parameter set
// into a Config.EdgeSigns array. A +1 or -1 stays as it is; an edge the
// derivation left unknown follows unknownPolicy: "free" leaves it learnable
// (0), "excitatory" fixes it to +1 and "inhibitory" fixes it to -1. No sign is
// guessed silently: the policy and the three counts come back in the summary.
// The returned array follows the canonical edge order of the graph the set was
// derived for, which is the order a Config must declare its edges in.
func SignsFromParameterSet(set DerivedSigns, unknownPolicy string) ([]int8, SignSummary, error) {
	var summary SignSummary
	if set.Source != DerivedSetSource {
		return nil, summary, fmt.Errorf("parameter set source %q, want %q", set.Source, DerivedSetSource)
	}
	var unknown int8
	switch unknownPolicy {
	case UnknownSignFree:
		unknown = 0
	case UnknownSignExcitatory:
		unknown = 1
	case UnknownSignInhibitory:
		unknown = -1
	default:
		return nil, summary, fmt.Errorf("unknown sign policy %q, want %q, %q or %q",
			unknownPolicy, UnknownSignFree, UnknownSignExcitatory, UnknownSignInhibitory)
	}
	if set.Edges < 0 {
		return nil, summary, errors.New("parameter set declares a negative edge count")
	}
	if len(set.EdgeSigns) != set.Edges {
		return nil, summary, fmt.Errorf("parameter set has %d signs and %d edges", len(set.EdgeSigns), set.Edges)
	}
	signs := make([]int8, len(set.EdgeSigns))
	for i, sign := range set.EdgeSigns {
		switch sign {
		case 1:
			summary.PositiveEdges++
			signs[i] = 1
		case -1:
			summary.NegativeEdges++
			signs[i] = -1
		case 0:
			summary.UnknownEdges++
			signs[i] = unknown
		default:
			return nil, SignSummary{}, fmt.Errorf("parameter set edge %d sign is %d, want -1, 0 or +1", i, sign)
		}
	}
	summary.Policy = unknownPolicy
	return signs, summary, nil
}

// EffectiveWeights returns the weights the core actually integrates.
// Parameters.Core.Weights stores raw values: a free edge stores its weight and
// a fixed-sign edge stores the log magnitude rho, whose effective weight is
// sign * exp(rho). A zero magnitude is therefore not representable and a
// fixed-sign edge can never cross zero. The returned slice is owned by the
// caller. A Config that declares no sign returns a copy of the raw weights.
func EffectiveWeights(c Config, p Parameters) ([]float64, error) {
	edges := configEdges(c)
	if err := validateSignConfig(c, edges); err != nil {
		return nil, err
	}
	if len(p.Core.Weights) != edges {
		return nil, fmt.Errorf("weights have %d values, the core has %d edges", len(p.Core.Weights), edges)
	}
	out := make([]float64, edges)
	copy(out, p.Core.Weights)
	for i, sign := range c.EdgeSigns {
		if sign == 0 {
			continue
		}
		magnitude := math.Exp(out[i])
		if !finite(magnitude) || magnitude == 0 {
			return nil, fmt.Errorf("edge %d log magnitude %g has no representable positive magnitude", i, out[i])
		}
		out[i] = float64(sign) * magnitude
	}
	return out, nil
}

// validateSignConfig checks the sign declaration and the log-magnitude floor
// against a known edge count.
func validateSignConfig(c Config, edges int) error {
	if len(c.EdgeSigns) != 0 {
		if len(c.EdgeSigns) != edges {
			return fmt.Errorf("edge_signs has %d entries, the core has %d edges", len(c.EdgeSigns), edges)
		}
		for i, sign := range c.EdgeSigns {
			if sign < -1 || sign > 1 {
				return fmt.Errorf("edge_signs[%d] is %d, want -1, 0 or +1", i, sign)
			}
		}
	}
	_, err := minLogMagnitude(c)
	return err
}

// minLogMagnitude resolves the declared floor, rejecting a value that has no
// finite exponential. Zero selects the documented default.
func minLogMagnitude(c Config) (float64, error) {
	if !finite(c.MinLogMagnitude) {
		return 0, fmt.Errorf("min_log_magnitude is not finite")
	}
	if c.MinLogMagnitude >= maxLogMagnitude {
		return 0, fmt.Errorf("min_log_magnitude %g has no finite exponential", c.MinLogMagnitude)
	}
	if c.MinLogMagnitude == 0 {
		return defaultMinLogMagnitude, nil
	}
	return c.MinLogMagnitude, nil
}

// hasFixedSigns reports whether any edge left the free parametrization. An
// all-zero declaration is the same model as no declaration at all.
func hasFixedSigns(signs []int8) bool {
	for _, sign := range signs {
		if sign != 0 {
			return true
		}
	}
	return false
}

// validateMasks checks the per-item masks against the topology. An absent half
// means that group is open, so only a present half has to match.
func validateMasks(m *UpdateMasks, nodes, edges int) error {
	if m == nil {
		return nil
	}
	if len(m.Edges) != 0 && len(m.Edges) != edges {
		return fmt.Errorf("update mask has %d edge entries, the core has %d edges", len(m.Edges), edges)
	}
	if len(m.Nodes) != 0 && len(m.Nodes) != nodes {
		return fmt.Errorf("update mask has %d node entries, the core has %d nodes", len(m.Nodes), nodes)
	}
	return nil
}

// validateRanges rejects a bound that cannot describe an interval.
func validateRanges(r *ParameterRanges) error {
	if r == nil {
		return nil
	}
	for _, bound := range []struct {
		name  string
		value float64
	}{
		{"weight_magnitude_max", r.WeightMagnitudeMax}, {"bias_abs_max", r.BiasAbsMax},
		{"log_tau_min", r.LogTauMin}, {"log_tau_max", r.LogTauMax}, {"theta_raw_abs_max", r.ThetaRawAbsMax},
	} {
		if !finite(bound.value) {
			return fmt.Errorf("parameter range %s is not finite", bound.name)
		}
	}
	for _, bound := range []struct {
		name  string
		value float64
	}{
		{"weight_magnitude_max", r.WeightMagnitudeMax}, {"bias_abs_max", r.BiasAbsMax}, {"theta_raw_abs_max", r.ThetaRawAbsMax},
	} {
		if bound.value < 0 {
			return fmt.Errorf("parameter range %s is %g; a magnitude limit cannot be negative", bound.name, bound.value)
		}
	}
	if r.LogTauMin != 0 && r.LogTauMax != 0 && r.LogTauMin > r.LogTauMax {
		return fmt.Errorf("parameter range log_tau_min %g is above log_tau_max %g", r.LogTauMin, r.LogTauMax)
	}
	return nil
}

// validateRangesAgainstSigns rejects a weight cap a fixed-sign edge could
// never satisfy, because the log-magnitude floor keeps it larger.
func validateRangesAgainstSigns(c Config, r *ParameterRanges) error {
	if r == nil || r.WeightMagnitudeMax <= 0 || !hasFixedSigns(c.EdgeSigns) {
		return nil
	}
	floor, err := minLogMagnitude(c)
	if err != nil {
		return err
	}
	if math.Log(r.WeightMagnitudeMax) < floor {
		return fmt.Errorf("weight_magnitude_max %g is below the log-magnitude floor exp(%g)", r.WeightMagnitudeMax, floor)
	}
	return nil
}

// parameterGroup is one contiguous run of the flat parameter vector. The order
// is the one AdamState documents: weights, bias, log_tau, theta_raw, encoder,
// readout.
type parameterGroup struct {
	name         string
	offset, size int
}

func parameterGroups(p Parameters) [6]parameterGroup {
	sizes := [6]struct {
		name string
		size int
	}{
		{"weights", len(p.Core.Weights)}, {"bias", len(p.Core.Bias)}, {"log_tau", len(p.Core.LogTau)},
		{"theta_raw", len(p.ThetaRaw)}, {"encoder", len(p.Encoder)}, {"readout", len(p.Readout)},
	}
	var groups [6]parameterGroup
	offset := 0
	for i, g := range sizes {
		groups[i] = parameterGroup{g.name, offset, g.size}
		offset += g.size
	}
	return groups
}

// constrained reports whether a step has any projection to do at all. When it
// is false the update path allocates nothing extra and behaves exactly as it
// did before this ticket.
func constrained(fixedSigns bool, r *ParameterRanges) bool {
	if fixedSigns {
		return true
	}
	if r == nil {
		return false
	}
	return *r != ParameterRanges{}
}

// doProject applies the declared range rules and the fixed-sign log-magnitude
// floor to the flat parameter vector in place. Only updatable parameters are
// projected, so a masked parameter stays bit-identical; optimizer moments and
// step counts are not arguments here and are never touched. It returns how
// many values each rule moved, or nil when it moved none.
func doProject(c Config, r *ParameterRanges, flat []float64, mask []bool, shape Parameters) (map[string]int, error) {
	floor, err := minLogMagnitude(c)
	if err != nil {
		return nil, err
	}
	weightCap, logWeightCap := 0.0, 0.0
	if r != nil && r.WeightMagnitudeMax > 0 {
		weightCap = r.WeightMagnitudeMax
		logWeightCap = math.Log(weightCap)
	}
	counts := map[string]int{}
	for _, g := range parameterGroups(shape) {
		if g.size == 0 || g.name == "encoder" || g.name == "readout" {
			continue
		}
		for item := range g.size {
			index := g.offset + item
			if !mask[index] {
				continue
			}
			value := flat[index]
			switch g.name {
			case "weights":
				if item < len(c.EdgeSigns) && c.EdgeSigns[item] != 0 {
					if weightCap > 0 && value > logWeightCap {
						value = logWeightCap
						counts["weights"]++
					}
					if value < floor {
						value = floor
						counts["min_log_magnitude"]++
					}
					break
				}
				if weightCap > 0 {
					if value > weightCap {
						value = weightCap
						counts["weights"]++
					} else if value < -weightCap {
						value = -weightCap
						counts["weights"]++
					}
				}
			case "bias":
				if r != nil && r.BiasAbsMax > 0 {
					if value > r.BiasAbsMax {
						value = r.BiasAbsMax
						counts["bias"]++
					} else if value < -r.BiasAbsMax {
						value = -r.BiasAbsMax
						counts["bias"]++
					}
				}
			case "log_tau":
				if r != nil && r.LogTauMax != 0 && value > r.LogTauMax {
					value = r.LogTauMax
					counts["log_tau"]++
				} else if r != nil && r.LogTauMin != 0 && value < r.LogTauMin {
					value = r.LogTauMin
					counts["log_tau"]++
				}
			case "theta_raw":
				if r != nil && r.ThetaRawAbsMax > 0 {
					if value > r.ThetaRawAbsMax {
						value = r.ThetaRawAbsMax
						counts["theta_raw"]++
					} else if value < -r.ThetaRawAbsMax {
						value = -r.ThetaRawAbsMax
						counts["theta_raw"]++
					}
				}
			}
			flat[index] = value
		}
	}
	if len(counts) == 0 {
		return nil, nil
	}
	return counts, nil
}

// copyOptions deep copies the two optional pointers so a trainer never aliases
// caller-owned masks or ranges and a snapshot never aliases the trainer.
func copyOptions(o Options) Options {
	if o.Masks != nil {
		owned := UpdateMasks{Edges: append([]bool(nil), o.Masks.Edges...), Nodes: append([]bool(nil), o.Masks.Nodes...)}
		o.Masks = &owned
	}
	if o.Ranges != nil {
		owned := *o.Ranges
		o.Ranges = &owned
	}
	return o
}
