package learning

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/dynamics"
)

// vectorIndividualProfile is the persistent-inference profile of the vector
// core: the same solver, precision and lifecycles as the continuous profile,
// extended to per-node component rows.
const vectorIndividualProfile = "vector-f64-insyra-f32-persistent-inference-episode-learning/v1"

// vectorCore drives a *dynamics.VectorContinuous whose node rows are N*C values
// wide. Unlike the other three cores it owns its full configuration copy here:
// VectorContinuous publishes no Config, so the copy made at construction time is
// the one Config publishes through fill. Everything else mirrors the scalar
// continuous core, only the vectors are wider.
type vectorCore struct {
	model                *dynamics.VectorContinuous
	config               dynamics.Config
	layout               dynamics.VectorLayout
	nodeCount, edgeCount int
}

// newVectorCore validates the declaration, builds the model, and keeps the deep
// configuration copy and its layout that fill publishes.
func newVectorCore(c dynamics.Config) (*vectorCore, error) {
	model, err := dynamics.NewVectorContinuous(c)
	if err != nil {
		return nil, err
	}
	owned := copyVectorConfig(c)
	layout, err := dynamics.NewVectorLayout(owned.Nodes, len(owned.Sources), owned.StateDimension, owned.EdgeShape)
	if err != nil {
		return nil, err
	}
	return &vectorCore{model: model, config: owned, layout: layout, nodeCount: owned.Nodes, edgeCount: len(owned.Sources)}, nil
}

func (v vectorCore) nodes() int { return v.nodeCount }
func (v vectorCore) edges() int { return v.edgeCount }
func (v vectorCore) weightCount() int {
	return v.layout.WeightValues()
}
func (v vectorCore) biasCount() int    { return v.layout.NodeValues() }
func (v vectorCore) stateDim() int     { return v.layout.C }
func (v vectorCore) matrixEdges() bool { return v.layout.Matrix }

func (v vectorCore) forward(ctx context.Context, p Parameters, initial []float64, inputs [][]float64) (coreTrace, [][]float64, error) {
	if len(p.ThetaRaw) != 0 {
		return nil, nil, fmt.Errorf("theta_raw requires a LIF core")
	}
	trace, err := v.model.Forward(ctx, dynamics.VectorParameters{
		Weights: p.Core.Weights, Bias: p.Core.Bias, LogTau: p.Core.LogTau,
	}, initial, inputs)
	if err != nil {
		return nil, nil, err
	}
	// The vector trace publishes its initial output row too; the episode
	// readout only observes post-step rows, exactly as the scalar trace does.
	outs := trace.Outputs()
	if len(outs) > 0 {
		outs = outs[1:]
	}
	return trace, outs, nil
}

func (v vectorCore) backward(ctx context.Context, tr coreTrace, upstream [][]float64, window int) (coreGradient, error) {
	trace, ok := tr.(*dynamics.VectorTrace)
	if !ok {
		return coreGradient{}, fmt.Errorf("trace belongs to a different core")
	}
	g, err := v.model.Backward(ctx, trace, upstream, window)
	if err != nil {
		return coreGradient{}, err
	}
	return coreGradient{
		core: dynamics.Gradient{Weights: g.Weights, Bias: g.Bias, LogTau: g.LogTau, Inputs: g.Inputs, Initial: g.Initial},
	}, nil
}

func (v vectorCore) events(coreTrace) ([][]float64, error) { return nil, nil }

func (v vectorCore) fill(config *Config) {
	config.Dynamics = copyVectorConfig(v.config)
	config.LIF = nil
	config.Mixed = nil
	config.LIFIndex = nil
}

func (v vectorCore) theta() bool       { return false }
func (v vectorCore) thetaCount() int   { return 0 }
func (v vectorCore) thetaNodes() []int { return nil }
func (v vectorCore) profile() string   { return vectorIndividualProfile }

func (v vectorCore) newState(initial []float64) (NeuralState, error) {
	return NeuralState{}, vectorStateDeferred()
}

func (v vectorCore) validateState(s NeuralState) error {
	return vectorStateDeferred()
}

func (v vectorCore) advance(ctx context.Context, p Parameters, s NeuralState, inputs [][]float64) (NeuralState, [][]float64, [][]float64, error) {
	return NeuralState{}, nil, nil, vectorStateDeferred()
}

func (v vectorCore) advanceModulated(ctx context.Context, p Parameters, s NeuralState, inputs [][]float64, mod *dynamics.Modulation) (NeuralState, [][]float64, [][]float64, error) {
	return NeuralState{}, nil, nil, vectorStateDeferred()
}

// vectorStateDeferred is the single refusal every persistent-state entry point
// of the vector core returns until the next stage wires it. It is named so a
// caller can recognise the wall this ticket deliberately left standing.
func vectorStateDeferred() error {
	return fmt.Errorf("vector core persistent state arrives with the next ticket")
}

// copyVectorConfig deep copies the vector declaration exactly as
// NewVectorContinuous normalises it internally, so fill publishes the same
// configuration the running model owns.
func copyVectorConfig(c dynamics.Config) dynamics.Config {
	owned := c
	owned.Sources = append([]int(nil), c.Sources...)
	owned.Targets = append([]int(nil), c.Targets...)
	if len(c.Delays) == 0 {
		owned.Delays = make([]int, len(c.Sources))
	} else {
		owned.Delays = append([]int(nil), c.Delays...)
	}
	return owned
}
