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
	// weightCount is the number of stored weight values: edges on the scalar
	// and spiking cores, and the layout's per-edge capacity on the vector core
	// (E for scalar edges or E*C*C for matrix edges).
	weightCount() int
	// biasCount is the number of stored bias values: nodes on the scalar and
	// spiking cores, and N*C node-major values on the vector core.
	biasCount() int
	// stateDim is the per-node component count: 1 on every core that existed
	// before the vector core, and C on the vector core.
	stateDim() int
	// forward returns the owning core's trace and the per-step values the
	// readout observes: activated outputs for the continuous core and the
	// synaptic trace x for the LIF core.
	forward(ctx context.Context, p Parameters, initial []float64, inputs [][]float64) (coreTrace, [][]float64, error)
	backward(ctx context.Context, tr coreTrace, upstream [][]float64, window int) (coreGradient, error)
	events(tr coreTrace) ([][]float64, error)
	// fill writes this core's owned configuration copy into c.
	fill(c *Config)
	// theta reports whether the core owns a trainable threshold group.
	theta() bool
	// thetaCount is the length of that group: zero on the continuous core, one
	// per node on the spiking core and one per LIF node on the mixed core.
	thetaCount() int
	// thetaNodes maps each theta entry to its node, in ascending node order.
	// It is nil where that map is the identity, which is every core whose
	// threshold group covers all of its nodes.
	thetaNodes() []int
	// profile names the individual snapshot profile this core persists under.
	profile() string
	// newState starts a persistent trajectory at the given initial voltage.
	newState(initial []float64) (NeuralState, error)
	// validateState rejects a persistent state that does not belong here.
	validateState(s NeuralState) error
	// advance continues a persistent state and returns the values the readout
	// observes after each step: activated outputs for the continuous core and
	// the synaptic trace x for the LIF core, matching forward. The third result
	// is the 0/1 event of every neuron after each step, which only a spiking
	// core produces; the continuous core returns nil there. Local plasticity
	// reads both, so they leave the core together rather than staying inside it.
	advance(ctx context.Context, p Parameters, s NeuralState, inputs [][]float64) (NeuralState, [][]float64, [][]float64, error)
	// advanceModulated is advance with the per-step, per-node modulation a
	// chemical layer produced for exactly these steps. A nil modulation is
	// advance itself, bit for bit; the continuous core refuses a threshold row
	// that is not zero, because it has no threshold to move. Nothing here
	// reaches Parameters: the modulation is an argument of one call.
	advanceModulated(ctx context.Context, p Parameters, s NeuralState, inputs [][]float64, mod *dynamics.Modulation) (NeuralState, [][]float64, [][]float64, error)
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

func (c continuousCore) nodes() int       { return c.nodeCount }
func (c continuousCore) edges() int       { return c.edgeCount }
func (c continuousCore) weightCount() int { return c.edgeCount }
func (c continuousCore) biasCount() int   { return c.nodeCount }
func (c continuousCore) stateDim() int    { return 1 }

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
	config.Mixed = nil
	config.LIFIndex = nil
}

func (c continuousCore) theta() bool       { return false }
func (c continuousCore) thetaCount() int   { return 0 }
func (c continuousCore) thetaNodes() []int { return nil }
func (c continuousCore) profile() string   { return IndividualProfile }

func (c continuousCore) newState(initial []float64) (NeuralState, error) {
	state, err := c.model.NewState(initial)
	if err != nil {
		return NeuralState{}, err
	}
	return NeuralState{Core: NeuralCoreContinuous, Continuous: &state}, nil
}

func (c continuousCore) validateState(s NeuralState) error {
	if s.Core != NeuralCoreContinuous || s.Continuous == nil || s.LIF != nil || s.Mixed != nil {
		return fmt.Errorf("neural state declares core %q, want a single %q state", s.Core, NeuralCoreContinuous)
	}
	return c.model.ValidateState(*s.Continuous)
}

func (c continuousCore) advance(ctx context.Context, p Parameters, s NeuralState, inputs [][]float64) (NeuralState, [][]float64, [][]float64, error) {
	return c.advanceModulated(ctx, p, s, inputs, nil)
}

func (c continuousCore) advanceModulated(ctx context.Context, p Parameters, s NeuralState, inputs [][]float64, mod *dynamics.Modulation) (NeuralState, [][]float64, [][]float64, error) {
	if err := c.validateState(s); err != nil {
		return NeuralState{}, nil, nil, err
	}
	if len(p.ThetaRaw) != 0 {
		return NeuralState{}, nil, nil, fmt.Errorf("theta_raw requires a LIF core")
	}
	next, outputs, err := c.model.AdvanceModulated(ctx, p.Core, *s.Continuous, inputs, mod)
	if err != nil {
		return NeuralState{}, nil, nil, err
	}
	// A continuous core emits no events; local plasticity reads its output y
	// at both ends of an edge instead.
	return NeuralState{Core: NeuralCoreContinuous, Continuous: &next}, outputs, nil, nil
}

type lifCore struct {
	model                *dynamics.LIF
	nodeCount, edgeCount int
}

func (l lifCore) nodes() int       { return l.nodeCount }
func (l lifCore) edges() int       { return l.edgeCount }
func (l lifCore) weightCount() int { return l.edgeCount }
func (l lifCore) biasCount() int   { return l.nodeCount }
func (l lifCore) stateDim() int    { return 1 }

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
	config.Mixed = nil
	config.LIFIndex = nil
}

func (l lifCore) theta() bool       { return true }
func (l lifCore) thetaCount() int   { return l.nodeCount }
func (l lifCore) thetaNodes() []int { return nil }
func (l lifCore) profile() string   { return IndividualProfileLIF }

func (l lifCore) newState(initial []float64) (NeuralState, error) {
	state, err := l.model.NewState(initial)
	if err != nil {
		return NeuralState{}, err
	}
	return NeuralState{Core: NeuralCoreLIF, LIF: &state}, nil
}

func (l lifCore) validateState(s NeuralState) error {
	if s.Core != NeuralCoreLIF || s.LIF == nil || s.Continuous != nil || s.Mixed != nil {
		return fmt.Errorf("neural state declares core %q, want a single %q state", s.Core, NeuralCoreLIF)
	}
	return l.model.ValidateState(*s.LIF)
}

func (l lifCore) advance(ctx context.Context, p Parameters, s NeuralState, inputs [][]float64) (NeuralState, [][]float64, [][]float64, error) {
	return l.advanceModulated(ctx, p, s, inputs, nil)
}

func (l lifCore) advanceModulated(ctx context.Context, p Parameters, s NeuralState, inputs [][]float64, mod *dynamics.Modulation) (NeuralState, [][]float64, [][]float64, error) {
	if err := l.validateState(s); err != nil {
		return NeuralState{}, nil, nil, err
	}
	if len(p.ThetaRaw) != l.nodeCount {
		return NeuralState{}, nil, nil, fmt.Errorf("theta_raw has %d values, want %d", len(p.ThetaRaw), l.nodeCount)
	}
	// The persistent readout observes the same decaying synaptic trace as the
	// episode path. The 0/1 events come back alongside it because local
	// plasticity uses them as the post signal.
	next, outputs, spikes, err := l.model.AdvanceModulated(ctx, dynamics.LIFParameters{
		Weights: p.Core.Weights, Bias: p.Core.Bias, LogTau: p.Core.LogTau, ThetaRaw: p.ThetaRaw,
	}, *s.LIF, inputs, mod)
	if err != nil {
		return NeuralState{}, nil, nil, err
	}
	return NeuralState{Core: NeuralCoreLIF, LIF: &next}, outputs, spikes, nil
}

type mixedCore struct {
	model                *dynamics.Mixed
	nodeCount, edgeCount int
	// lifIndex is the ascending LIF node list. It is the map a reader needs to
	// tell which node each theta_raw entry belongs to, so the network publishes
	// it through Config.
	lifIndex []int
}

func (x mixedCore) nodes() int       { return x.nodeCount }
func (x mixedCore) edges() int       { return x.edgeCount }
func (x mixedCore) weightCount() int { return x.edgeCount }
func (x mixedCore) biasCount() int   { return x.nodeCount }
func (x mixedCore) stateDim() int    { return 1 }

func (x mixedCore) forward(ctx context.Context, p Parameters, initial []float64, inputs [][]float64) (coreTrace, [][]float64, error) {
	if len(p.ThetaRaw) != len(x.lifIndex) {
		return nil, nil, fmt.Errorf("theta_raw has %d values, want %d, one per LIF node", len(p.ThetaRaw), len(x.lifIndex))
	}
	trace, err := x.model.Forward(ctx, dynamics.MixedParameters{
		Weights: p.Core.Weights, Bias: p.Core.Bias, LogTau: p.Core.LogTau, ThetaRaw: p.ThetaRaw,
	}, initial, inputs)
	if err != nil {
		return nil, nil, err
	}
	// Every node has one output series: the activated output of a continuous
	// node and the synaptic trace of a LIF node.
	return trace, trace.Outputs(), nil
}

func (x mixedCore) backward(ctx context.Context, tr coreTrace, upstream [][]float64, window int) (coreGradient, error) {
	trace, ok := tr.(*dynamics.MixedTrace)
	if !ok {
		return coreGradient{}, fmt.Errorf("trace belongs to a different core")
	}
	g, err := x.model.Backward(ctx, trace, upstream, window)
	if err != nil {
		return coreGradient{}, err
	}
	return coreGradient{
		core:     dynamics.Gradient{Weights: g.Weights, Bias: g.Bias, LogTau: g.LogTau, Inputs: g.Inputs, Initial: g.Initial},
		thetaRaw: g.ThetaRaw,
	}, nil
}

func (x mixedCore) events(tr coreTrace) ([][]float64, error) {
	trace, ok := tr.(*dynamics.MixedTrace)
	if !ok {
		return nil, fmt.Errorf("trace belongs to a different core")
	}
	return trace.Spikes(), nil
}

func (x mixedCore) fill(config *Config) {
	owned := x.model.Config()
	config.Mixed = &owned
	config.LIFIndex = append([]int(nil), x.lifIndex...)
	config.Dynamics = dynamics.Config{}
	config.LIF = nil
}

func (x mixedCore) theta() bool       { return true }
func (x mixedCore) thetaCount() int   { return len(x.lifIndex) }
func (x mixedCore) thetaNodes() []int { return x.lifIndex }
func (x mixedCore) profile() string   { return IndividualProfileMixed }

func (x mixedCore) newState(initial []float64) (NeuralState, error) {
	state, err := x.model.NewState(initial)
	if err != nil {
		return NeuralState{}, err
	}
	return NeuralState{Core: NeuralCoreMixed, Mixed: &state}, nil
}

func (x mixedCore) validateState(s NeuralState) error {
	if s.Core != NeuralCoreMixed || s.Mixed == nil || s.Continuous != nil || s.LIF != nil {
		return fmt.Errorf("neural state declares core %q, want a single %q state", s.Core, NeuralCoreMixed)
	}
	return x.model.ValidateState(*s.Mixed)
}

func (x mixedCore) advance(ctx context.Context, p Parameters, s NeuralState, inputs [][]float64) (NeuralState, [][]float64, [][]float64, error) {
	return x.advanceModulated(ctx, p, s, inputs, nil)
}

func (x mixedCore) advanceModulated(ctx context.Context, p Parameters, s NeuralState, inputs [][]float64, mod *dynamics.Modulation) (NeuralState, [][]float64, [][]float64, error) {
	if err := x.validateState(s); err != nil {
		return NeuralState{}, nil, nil, err
	}
	if len(p.ThetaRaw) != len(x.lifIndex) {
		return NeuralState{}, nil, nil, fmt.Errorf("theta_raw has %d values, want %d, one per LIF node", len(p.ThetaRaw), len(x.lifIndex))
	}
	// The event rows come back at full node width; a continuous node owns no
	// event and its entry is always zero, which is why local plasticity reads
	// the rule assignment before it uses them as a post signal.
	next, outputs, spikes, err := x.model.AdvanceModulated(ctx, dynamics.MixedParameters{
		Weights: p.Core.Weights, Bias: p.Core.Bias, LogTau: p.Core.LogTau, ThetaRaw: p.ThetaRaw,
	}, *s.Mixed, inputs, mod)
	if err != nil {
		return NeuralState{}, nil, nil, err
	}
	return NeuralState{Core: NeuralCoreMixed, Mixed: &next}, outputs, spikes, nil
}

// newCore selects exactly one core and replaces c's connectivity with the
// model's own copy, so the network never aliases caller-owned configuration.
func newCore(c *Config) (coreModel, error) {
	declared := 0
	if !zeroDynamics(c.Dynamics) {
		declared++
	}
	if c.LIF != nil {
		declared++
	}
	if c.Mixed != nil {
		declared++
	}
	if declared != 1 {
		return nil, fmt.Errorf("configure exactly one core: dynamics, lif or mixed")
	}
	if c.Mixed != nil {
		model, err := dynamics.NewMixed(*c.Mixed)
		if err != nil {
			return nil, err
		}
		owned := model.Config()
		index := model.LIFNodes()
		if len(c.LIFIndex) != 0 && !sameIndex(c.LIFIndex, index) {
			return nil, fmt.Errorf("lif_index contradicts the rule assignment of the mixed core")
		}
		c.Mixed, c.Dynamics, c.LIF = &owned, dynamics.Config{}, nil
		c.LIFIndex = append([]int(nil), index...)
		return mixedCore{model, owned.Nodes, len(owned.Sources), index}, nil
	}
	// Only the mixed core owns a threshold group that covers part of the nodes,
	// so an index anywhere else would be a knob nothing reads.
	if len(c.LIFIndex) != 0 {
		return nil, fmt.Errorf("lif_index belongs to the mixed core only")
	}
	if c.LIF != nil {
		model, err := dynamics.NewLIF(*c.LIF)
		if err != nil {
			return nil, err
		}
		owned := model.Config()
		c.LIF, c.Dynamics = &owned, dynamics.Config{}
		return lifCore{model, owned.Nodes, len(owned.Sources)}, nil
	}
	// A state dimension above one declares vector nodes, which only the vector
	// continuous core runs; it is still a Dynamics declaration, exactly as the
	// scalar continuous core is. Root decision 6 of ticket 25 keeps C = 1 on
	// the scalar path, so the two models stay bit-identical there.
	if c.Dynamics.StateDimension > 1 {
		vc, err := newVectorCore(c.Dynamics)
		if err != nil {
			return nil, err
		}
		c.Dynamics = vc.config
		return vc, nil
	}
	model, err := dynamics.NewContinuous(c.Dynamics)
	if err != nil {
		return nil, err
	}
	owned := model.Config()
	c.Dynamics = owned
	return continuousCore{model, owned.Nodes, len(owned.Sources)}, nil
}

func sameIndex(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
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
	if c.Mixed != nil {
		return c.Mixed.Nodes
	}
	if c.LIF != nil {
		return c.LIF.Nodes
	}
	return c.Dynamics.Nodes
}

// configEdges reads the edge count of whichever core a configuration declares.
func configEdges(c Config) int {
	if c.Mixed != nil {
		return len(c.Mixed.Sources)
	}
	if c.LIF != nil {
		return len(c.LIF.Sources)
	}
	return len(c.Dynamics.Sources)
}

// configEdgeEnds reads the endpoint arrays of whichever core a configuration
// declares, in the canonical edge order every per-edge array follows.
func configEdgeEnds(c Config) (sources, targets []int) {
	if c.Mixed != nil {
		return c.Mixed.Sources, c.Mixed.Targets
	}
	if c.LIF != nil {
		return c.LIF.Sources, c.LIF.Targets
	}
	return c.Dynamics.Sources, c.Dynamics.Targets
}

// configSpikingNodes reports, per node, whether that node emits a 0/1 event.
// It is nil where the whole core answers the same way, which is both
// single-rule cores: the continuous core produces no event row at all and the
// spiking core produces one for every node. Only the mixed core needs the
// per-node answer, and only local plasticity asks for it.
func configSpikingNodes(c Config) []bool {
	if c.Mixed == nil {
		return nil
	}
	mask := make([]bool, c.Mixed.Nodes)
	for i, rule := range c.Mixed.NodeRule {
		mask[i] = rule == dynamics.RuleLIF
	}
	return mask
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

func copyMixed(c *dynamics.MixedConfig) *dynamics.MixedConfig {
	if c == nil {
		return nil
	}
	owned := *c
	owned.Sources = append([]int(nil), c.Sources...)
	owned.Targets = append([]int(nil), c.Targets...)
	owned.Delays = append([]int(nil), c.Delays...)
	owned.NodeRule = append([]uint8(nil), c.NodeRule...)
	if c.LIF.Homeostasis != nil {
		block := *c.LIF.Homeostasis
		owned.LIF.Homeostasis = &block
	}
	return &owned
}
