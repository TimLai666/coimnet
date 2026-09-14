package learning

import (
	"context"
	"fmt"
	"reflect"

	"github.com/TimLai666/coimnet/dynamics"
)

// coreModel hides which reference core a Network drives. Exactly one core is
// configured, and the interface stays private so the SDK gains no new public
// abstraction: callers still select a core through Config.
type coreModel interface {
	nodes() int
	edges() int
	// forward returns the owning core's trace and the per-step values the
	// readout observes: activated outputs for the continuous core and the
	// synaptic trace x for the LIF core.
	forward(ctx context.Context, p Parameters, initial []float64, inputs [][]float64) (coreTrace, [][]float64, error)
	backward(ctx context.Context, tr coreTrace, upstream [][]float64, window int) (coreGradient, error)
	events(tr coreTrace) ([][]float64, error)
	// fill writes this core's owned configuration copy into c.
	fill(c *Config)
	// continuous is the persistent-individual core, nil for spiking models.
	continuous() *dynamics.Continuous
	// theta reports whether the core owns a trainable threshold group.
	theta() bool
}

// coreTrace is one forward history. Only the core that produced it interprets
// its contents.
type coreTrace interface{ FinalVoltage() []float64 }

// coreGradient carries the gradients shared by both cores plus the LIF base
// threshold gradient, which is empty for the continuous core.
type coreGradient struct {
	core     dynamics.Gradient
	thetaRaw []float64
}

type continuousCore struct {
	model                *dynamics.Continuous
	nodeCount, edgeCount int
}

func (c continuousCore) nodes() int { return c.nodeCount }
func (c continuousCore) edges() int { return c.edgeCount }

func (c continuousCore) forward(ctx context.Context, p Parameters, initial []float64, inputs [][]float64) (coreTrace, [][]float64, error) {
	if len(p.ThetaRaw) != 0 {
		return nil, nil, fmt.Errorf("theta_raw requires a LIF core")
	}
	trace, err := c.model.Forward(ctx, p.Core, initial, inputs)
	if err != nil {
		return nil, nil, err
	}
	return trace, trace.Outputs(), nil
}

func (c continuousCore) backward(ctx context.Context, tr coreTrace, upstream [][]float64, window int) (coreGradient, error) {
	trace, ok := tr.(*dynamics.Trace)
	if !ok {
		return coreGradient{}, fmt.Errorf("trace belongs to a different core")
	}
	g, err := c.model.Backward(ctx, trace, upstream, window)
	if err != nil {
		return coreGradient{}, err
	}
	return coreGradient{core: g}, nil
}

func (c continuousCore) events(coreTrace) ([][]float64, error) {
	return nil, fmt.Errorf("spike events require a LIF core")
}

func (c continuousCore) fill(config *Config) {
	config.Dynamics = c.model.Config()
	config.LIF = nil
}

func (c continuousCore) continuous() *dynamics.Continuous { return c.model }
func (c continuousCore) theta() bool                      { return false }

type lifCore struct {
	model                *dynamics.LIF
	nodeCount, edgeCount int
}

func (l lifCore) nodes() int { return l.nodeCount }
func (l lifCore) edges() int { return l.edgeCount }

func (l lifCore) forward(ctx context.Context, p Parameters, initial []float64, inputs [][]float64) (coreTrace, [][]float64, error) {
	if len(p.ThetaRaw) != l.nodeCount {
		return nil, nil, fmt.Errorf("theta_raw has %d values, want %d", len(p.ThetaRaw), l.nodeCount)
	}
	trace, err := l.model.Forward(ctx, dynamics.LIFParameters{
		Weights: p.Core.Weights, Bias: p.Core.Bias, LogTau: p.Core.LogTau, ThetaRaw: p.ThetaRaw,
	}, initial, inputs)
	if err != nil {
		return nil, nil, err
	}
	// The readout and every edge observe the decaying synaptic trace, not the
	// raw event.
	return trace, trace.Outputs(), nil
}

func (l lifCore) backward(ctx context.Context, tr coreTrace, upstream [][]float64, window int) (coreGradient, error) {
	trace, ok := tr.(*dynamics.LIFTrace)
	if !ok {
		return coreGradient{}, fmt.Errorf("trace belongs to a different core")
	}
	g, err := l.model.Backward(ctx, trace, upstream, window)
	if err != nil {
		return coreGradient{}, err
	}
	return coreGradient{
		core:     dynamics.Gradient{Weights: g.Weights, Bias: g.Bias, LogTau: g.LogTau, Inputs: g.Inputs, Initial: g.Initial},
		thetaRaw: g.ThetaRaw,
	}, nil
}

func (l lifCore) events(tr coreTrace) ([][]float64, error) {
	trace, ok := tr.(*dynamics.LIFTrace)
	if !ok {
		return nil, fmt.Errorf("trace belongs to a different core")
	}
	return trace.Spikes(), nil
}

func (l lifCore) fill(config *Config) {
	owned := l.model.Config()
	config.LIF = &owned
	config.Dynamics = dynamics.Config{}
}

func (l lifCore) continuous() *dynamics.Continuous { return nil }
func (l lifCore) theta() bool                      { return true }

// newCore selects exactly one core and replaces c's connectivity with the
// model's own copy, so the network never aliases caller-owned configuration.
func newCore(c *Config) (coreModel, error) {
	if c.LIF != nil {
		if !zeroDynamics(c.Dynamics) {
			return nil, fmt.Errorf("configure either dynamics or lif, not both")
		}
		model, err := dynamics.NewLIF(*c.LIF)
		if err != nil {
			return nil, err
		}
		owned := model.Config()
		c.LIF, c.Dynamics = &owned, dynamics.Config{}
		return lifCore{model, owned.Nodes, len(owned.Sources)}, nil
	}
	if zeroDynamics(c.Dynamics) {
		return nil, fmt.Errorf("configure exactly one core: dynamics or lif")
	}
	model, err := dynamics.NewContinuous(c.Dynamics)
	if err != nil {
		return nil, err
	}
	owned := model.Config()
	c.Dynamics = owned
	return continuousCore{model, owned.Nodes, len(owned.Sources)}, nil
}

// zeroDynamics reports whether the continuous configuration carries no
// declaration at all. Empty non-nil edge slices are normalized to nil first, so
// an unused zero value stays acceptable next to a LIF core while any populated
// field is an error.
func zeroDynamics(c dynamics.Config) bool {
	if len(c.Sources) != 0 || len(c.Targets) != 0 || len(c.Delays) != 0 {
		return false
	}
	c.Sources, c.Targets, c.Delays = nil, nil, nil
	return reflect.DeepEqual(c, dynamics.Config{})
}

// configNodes reads the node count of whichever core a configuration declares.
func configNodes(c Config) int {
	if c.LIF != nil {
		return c.LIF.Nodes
	}
	return c.Dynamics.Nodes
}

// configEdges reads the edge count of whichever core a configuration declares.
func configEdges(c Config) int {
	if c.LIF != nil {
		return len(c.LIF.Sources)
	}
	return len(c.Dynamics.Sources)
}

func copyLIF(c *dynamics.LIFConfig) *dynamics.LIFConfig {
	if c == nil {
		return nil
	}
	owned := *c
	owned.Sources = append([]int(nil), c.Sources...)
	owned.Targets = append([]int(nil), c.Targets...)
	owned.Delays = append([]int(nil), c.Delays...)
	return &owned
}
