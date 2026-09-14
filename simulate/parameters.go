package simulate

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/params"
)

// parameterHashDomain separates this hash from any other SHA-256 in the
// project, so a parameter hash can never collide with a manifest or report one.
const parameterHashDomain = "coimnet-simulate-parameters/v1"

// ParameterSet is one complete assignment of dynamics parameters to a graph.
// Weights follow the canonical edge order of the graph; Bias, LogTau and
// ThetaRaw follow the node index order. ThetaRaw is empty for the continuous
// core, which has no threshold. Source names the derivation rule and Hash
// fingerprints the whole set. Derived is set only for the derived source and
// records which file, rules, policy and scale produced these weights.
type ParameterSet struct {
	Source   string          `json:"source"`
	Weights  []float64       `json:"weights"`
	Bias     []float64       `json:"bias"`
	LogTau   []float64       `json:"log_tau"`
	ThetaRaw []float64       `json:"theta_raw"`
	Derived  *DerivedSummary `json:"derived,omitempty"`
	Hash     string          `json:"hash"`
}

// DerivedSummary records where a derived parameter set came from and what the
// unknown sign policy actually did. The three counts are over the edges of the
// parameter set, in canonical edge order: an edge counts as unknown when the
// derivation left its sign unknown, whatever the policy then applied to it.
type DerivedSummary struct {
	ParameterSetSHA256 string  `json:"parameter_set_sha256"`
	RulesHash          string  `json:"rules_hash"`
	UnknownSignPolicy  string  `json:"unknown_sign_policy"`
	WeightScale        float64 `json:"weight_scale"`
	PositiveEdges      uint64  `json:"positive_edges"`
	NegativeEdges      uint64  `json:"negative_edges"`
	UnknownSignEdges   uint64  `json:"unknown_sign_edges"`
}

// UniformPositive derives the engineering_uniform_positive parameter set from a
// graph. Every edge weight is u.Gain times its raw source weight, which makes
// every connection excitatory, and every neuron receives the same bias, log_tau
// and theta_raw. A null raw weight is an error: the connectome carries no
// synaptic strength for such an edge and substituting zero would invent one.
//
// The context bounds the edge stream, which is 25.5 million records on the full
// MaleCNS graph; the ticket's sketch of this function had no context parameter.
//
// Hash is the SHA-256 of, in order: the domain string
// "coimnet-simulate-parameters/v1", a zero byte, Source, a zero byte, then for
// weights, bias, log_tau and theta_raw in that order the element count as a
// little-endian uint64 followed by each value's IEEE-754 bits as a
// little-endian uint64.
func UniformPositive(ctx context.Context, g *connectome.Graph, u UniformParameters) (ParameterSet, error) {
	if ctx == nil {
		return ParameterSet{}, errors.New("simulate: nil context")
	}
	if err := ctx.Err(); err != nil {
		return ParameterSet{}, fmt.Errorf("simulate: %w", err)
	}
	if g == nil || g.NodeCount() == 0 {
		return ParameterSet{}, errors.New("simulate: nil or empty graph")
	}
	for name, value := range map[string]float64{"gain": u.Gain, "bias": u.Bias, "log_tau": u.LogTau, "theta_raw": u.ThetaRaw} {
		if !finite(value) {
			return ParameterSet{}, fmt.Errorf("simulate: uniform %s is not finite", name)
		}
	}
	nodes, edges := int(g.NodeCount()), int(g.EdgeCount())
	weights := make([]float64, 0, edges)
	err := g.StreamAnnotatedEdges(ctx, func(edge connectome.EdgeRecord) error {
		if !edge.Weight.Valid {
			return fmt.Errorf("edge %d->%d at absolute row %d has a null raw weight; %s cannot invent one", edge.Source, edge.Target, edge.Position.AbsoluteRow, ParameterSourceUniform)
		}
		weight := u.Gain * float64(edge.Weight.Value)
		if !finite(weight) {
			return fmt.Errorf("edge %d->%d weight %g*%d is not representable", edge.Source, edge.Target, u.Gain, edge.Weight.Value)
		}
		weights = append(weights, weight)
		return nil
	})
	if err != nil {
		return ParameterSet{}, err
	}
	if len(weights) != edges {
		return ParameterSet{}, fmt.Errorf("simulate: streamed %d edges, the graph declares %d", len(weights), edges)
	}
	set := ParameterSet{
		Source:   ParameterSourceUniform,
		Weights:  weights,
		Bias:     fill(nodes, u.Bias),
		LogTau:   fill(nodes, u.LogTau),
		ThetaRaw: fill(nodes, u.ThetaRaw),
	}
	set.Hash = set.fingerprint()
	return set, nil
}

// FromDerived turns a derived parameter set into the runner's parameters:
//
//	weight[i] = weight_scale * sign[i] * set.EdgeWeight[i]
//
// for a sign of +1 or -1. An edge whose sign the rules left unknown (sign 0)
// follows the protocol's declared policy: exclude gives 0, excitatory gives
// +|weight| and inhibitory gives -|weight|. No sign is guessed: the policy is
// recorded in the returned summary, in the parameter hash and in the run
// report. A product that is zero is stored as positive zero, so the hash and
// the report never distinguish -0 from 0.
//
// Bias, log_tau and theta_raw still come from the protocol's uniform block:
// the release carries no per-neuron time constant or threshold, and nothing
// here invents one. setSHA256 is the SHA-256 of the parameter set file the set
// was read from; it enters the hash so a report names the exact file. The
// ticket sketch of this function had no such parameter, but the same sentence
// requires the file fingerprint in the hash and a *params.Set does not carry
// it, so the caller passes it in.
//
// The hash extends the encoding documented on UniformPositive: after the
// source and its zero byte, a derived set writes setSHA256, a zero byte, the
// rules hash, a zero byte, the policy, a zero byte and the weight scale's
// IEEE-754 bits as a little-endian uint64; then the four value groups follow
// exactly as before. A uniform set writes nothing between the source and the
// groups, so hashes recorded before this addition are unchanged.
func FromDerived(set *params.Set, setSHA256 string, protocol Protocol) (ParameterSet, DerivedSummary, error) {
	var (
		empty   ParameterSet
		summary DerivedSummary
	)
	if set == nil {
		return empty, summary, errors.New("simulate: nil derived parameter set")
	}
	if protocol.ParameterSource != ParameterSourceDerived {
		return empty, summary, fmt.Errorf("simulate: the protocol declares parameter source %q, FromDerived produces %q", protocol.ParameterSource, ParameterSourceDerived)
	}
	if err := protocol.validateParameterSource(); err != nil {
		return empty, summary, err
	}
	if set.Source != params.SetSource {
		return empty, summary, fmt.Errorf("simulate: parameter set source %q, want %q", set.Source, params.SetSource)
	}
	if !isLowerHex(setSHA256) {
		return empty, summary, fmt.Errorf("simulate: parameter set fingerprint %q is not 64 lowercase hexadecimal characters", setSHA256)
	}
	if !isLowerHex(set.RulesHash) {
		return empty, summary, fmt.Errorf("simulate: parameter set rules hash %q is not 64 lowercase hexadecimal characters", set.RulesHash)
	}
	if len(set.EdgeSign) != len(set.EdgeWeight) {
		return empty, summary, fmt.Errorf("simulate: parameter set has %d weights and %d signs", len(set.EdgeWeight), len(set.EdgeSign))
	}
	nodes := len(set.NodePreTotal)
	if nodes == 0 || len(set.NodePostTotal) != nodes {
		return empty, summary, fmt.Errorf("simulate: parameter set declares %d nodes", nodes)
	}
	policy, scale := protocol.Derived.UnknownSign, protocol.Derived.WeightScale
	weights := make([]float64, len(set.EdgeWeight))
	for i, derived := range set.EdgeWeight {
		if !finite(derived) {
			return empty, summary, fmt.Errorf("simulate: derived weight %d is not finite", i)
		}
		sign := 0.0
		switch set.EdgeSign[i] {
		case 1:
			summary.PositiveEdges++
			sign = 1
		case -1:
			summary.NegativeEdges++
			sign = -1
		case 0:
			summary.UnknownSignEdges++
			switch policy {
			case UnknownSignExcitatory:
				sign = 1
			case UnknownSignInhibitory:
				sign = -1
			default: // UnknownSignExclude
				sign = 0
			}
			derived = math.Abs(derived)
		default:
			return empty, summary, fmt.Errorf("simulate: derived sign %d at edge %d is not -1, 0 or +1", set.EdgeSign[i], i)
		}
		weight := scale * sign * derived
		if !finite(weight) {
			return empty, summary, fmt.Errorf("simulate: edge %d weight %g*%g*%g is not representable", i, scale, sign, derived)
		}
		if weight == 0 {
			weight = 0 // never store negative zero
		}
		weights[i] = weight
	}
	summary.ParameterSetSHA256 = setSHA256
	summary.RulesHash = set.RulesHash
	summary.UnknownSignPolicy = policy
	summary.WeightScale = scale
	parameters := ParameterSet{
		Source:   ParameterSourceDerived,
		Weights:  weights,
		Bias:     fill(nodes, protocol.Uniform.Bias),
		LogTau:   fill(nodes, protocol.Uniform.LogTau),
		ThetaRaw: fill(nodes, protocol.Uniform.ThetaRaw),
		Derived:  &DerivedSummary{},
	}
	*parameters.Derived = summary
	parameters.Hash = parameters.fingerprint()
	return parameters, summary, nil
}

func isLowerHex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func fill(n int, value float64) []float64 {
	values := make([]float64, n)
	for i := range values {
		values[i] = value
	}
	return values
}

// fingerprint implements the encoding documented on UniformPositive.
func (p ParameterSet) fingerprint() string {
	digest := sha256.New()
	digest.Write([]byte(parameterHashDomain))
	digest.Write([]byte{0})
	digest.Write([]byte(p.Source))
	digest.Write([]byte{0})
	buffer := make([]byte, 8)
	if p.Derived != nil {
		digest.Write([]byte(p.Derived.ParameterSetSHA256))
		digest.Write([]byte{0})
		digest.Write([]byte(p.Derived.RulesHash))
		digest.Write([]byte{0})
		digest.Write([]byte(p.Derived.UnknownSignPolicy))
		digest.Write([]byte{0})
		binary.LittleEndian.PutUint64(buffer, math.Float64bits(p.Derived.WeightScale))
		digest.Write(buffer)
	}
	for _, group := range [][]float64{p.Weights, p.Bias, p.LogTau, p.ThetaRaw} {
		binary.LittleEndian.PutUint64(buffer, uint64(len(group)))
		digest.Write(buffer)
		for _, value := range group {
			binary.LittleEndian.PutUint64(buffer, math.Float64bits(value))
			digest.Write(buffer)
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// validate checks a caller supplied parameter set against the graph shape and
// the core that will consume it.
func (p ParameterSet) validate(nodes, edges int, core string) error {
	switch p.Source {
	case ParameterSourceUniform:
		if p.Derived != nil {
			return fmt.Errorf("simulate: parameter source %q carries a derived summary", ParameterSourceUniform)
		}
	case ParameterSourceDerived:
		if p.Derived == nil {
			return fmt.Errorf("simulate: parameter source %q carries no derived summary", ParameterSourceDerived)
		}
		if !isLowerHex(p.Derived.ParameterSetSHA256) || !isLowerHex(p.Derived.RulesHash) {
			return errors.New("simulate: the derived summary must name the parameter set and rules hashes")
		}
		switch p.Derived.UnknownSignPolicy {
		case UnknownSignExclude, UnknownSignExcitatory, UnknownSignInhibitory:
		default:
			return fmt.Errorf("simulate: derived unknown sign policy %q is not declared", p.Derived.UnknownSignPolicy)
		}
		if !finite(p.Derived.WeightScale) || p.Derived.WeightScale <= 0 {
			return fmt.Errorf("simulate: derived weight scale is %v, want a finite value above zero", p.Derived.WeightScale)
		}
	default:
		return fmt.Errorf("simulate: unsupported parameter source %q; choose %q or %q", p.Source, ParameterSourceUniform, ParameterSourceDerived)
	}
	if len(p.Weights) != edges {
		return fmt.Errorf("simulate: parameter set has %d weights, the graph has %d edges", len(p.Weights), edges)
	}
	for name, group := range map[string][]float64{"bias": p.Bias, "log_tau": p.LogTau} {
		if len(group) != nodes {
			return fmt.Errorf("simulate: parameter set has %d %s values, the graph has %d nodes", len(group), name, nodes)
		}
	}
	if core == CoreLIF && len(p.ThetaRaw) != nodes {
		return fmt.Errorf("simulate: parameter set has %d theta_raw values, the spiking core needs %d", len(p.ThetaRaw), nodes)
	}
	for name, group := range map[string][]float64{"weights": p.Weights, "bias": p.Bias, "log_tau": p.LogTau, "theta_raw": p.ThetaRaw} {
		for i, value := range group {
			if !finite(value) {
				return fmt.Errorf("simulate: parameter %s[%d] is not finite", name, i)
			}
		}
	}
	if p.Hash != p.fingerprint() {
		return errors.New("simulate: parameter set hash does not match its values")
	}
	return nil
}
