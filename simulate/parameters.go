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
)

// parameterHashDomain separates this hash from any other SHA-256 in the
// project, so a parameter hash can never collide with a manifest or report one.
const parameterHashDomain = "coimnet-simulate-parameters/v1"

// ParameterSet is one complete assignment of dynamics parameters to a graph.
// Weights follow the canonical edge order of the graph; Bias, LogTau and
// ThetaRaw follow the node index order. ThetaRaw is empty for the continuous
// core, which has no threshold. Source names the derivation rule and Hash
// fingerprints the whole set.
type ParameterSet struct {
	Source   string    `json:"source"`
	Weights  []float64 `json:"weights"`
	Bias     []float64 `json:"bias"`
	LogTau   []float64 `json:"log_tau"`
	ThetaRaw []float64 `json:"theta_raw"`
	Hash     string    `json:"hash"`
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
	if p.Source != ParameterSourceUniform {
		return fmt.Errorf("simulate: parameter source %q is not available until ticket 13; this ticket only accepts %q", p.Source, ParameterSourceUniform)
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
