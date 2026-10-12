package dynamics

import (
	"context"
	"fmt"
	"math"
)

// coupledTrace is the extra tape owned by a coupled forward pass. The
// ordinary Trace tape remains unchanged, while these rows retain the raw
// current and the gain that was actually used at each step.
type coupledTrace struct {
	sequence ModulationSequence
	gain     [][]float64
	rawDrive [][]float64
}

// ModulationGradient contains the signed reverse signals for the gain,
// offset and threshold of one step. Each array has one value per node.
// It differentiates the declared numerical effects, not biological constants.
type ModulationGradient struct {
	Gain, Offset, Threshold []float64
}

// ActivityGradient adds a reverse signal to a prior raw core-output row.
// Step uses output indexing: 0 is the output after the first input, -1 is
// the initial output. A modulator may return several prior rows, as an
// activity-window source does. Values are added, never substituted.
type ActivityGradient struct {
	Step   int
	Values []float64
}

// ModulationSequence supplies coupled per-step effects. Forward receives the
// previous raw core output and returns exactly one modulation row. It must
// validate and own its forward observations. NewReverse creates an independent
// reverse session from completed observations for every Backward call; it
// cannot modify a trace or reuse adjoints from a previous reverse pass.
// Neither method receives a training target.
type ModulationSequence interface {
	Forward(context.Context, int, []float64) (*Modulation, error)
	NewReverse(context.Context, int, int) (ModulationReverse, error)
}

// ModulationReverse accounts for concentration recurrence and source/activity
// feedback before the core visits earlier rows. BackwardStep is called once
// per step in decreasing order. It returns prior activity increments, rejecting
// invalid input instead of partially updating caller-owned state. Both its
// internal recurrence and returned rows obey the fixed truncation boundaries
// declared to NewReverse. Model/source settings remain fixed here.
type ModulationReverse interface {
	BackwardStep(context.Context, int, ModulationGradient) ([]ActivityGradient, error)
}

// ForwardCoupled is Forward with activity-dependent chemical effects. A nil
// sequence is exactly Forward. Effects precede bias: gain*(input+synapses) +
// offset + bias. Its trace's ordinary Backward includes source/activity
// feedback, with a new reverse session per call. Nonzero threshold is invalid
// on this continuous core; malformed effects or sequence errors return nil.
func (m *Continuous) ForwardCoupled(ctx context.Context, p Parameters, initial []float64, inputs [][]float64, sequence ModulationSequence) (*Trace, error) {
	if sequence == nil {
		return m.Forward(ctx, p, initial, inputs)
	}
	return m.forwardSequence(ctx, p, initial, inputs, nil, sequence)
}

// forwardSequence is the coupled counterpart of forward. It intentionally
// keeps the ordinary forward implementation separate: ForwardCoupled(nil)
// must retain the old arithmetic and allocation path exactly.
func (m *Continuous) forwardSequence(ctx context.Context, p Parameters, initial []float64, inputs [][]float64, saved *State, sequence ModulationSequence) (*Trace, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("nil model")
	}
	if sequence == nil {
		return m.forward(ctx, p, initial, inputs, saved)
	}
	n := m.config.Nodes
	if n <= 0 {
		return nil, fmt.Errorf("uninitialized model")
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("empty sequence")
	}
	if err := vector(initial, n, "initial voltage"); err != nil {
		return nil, err
	}
	if err := vector(p.Weights, len(m.config.Sources), "weights"); err != nil {
		return nil, err
	}
	if err := vector(p.Bias, n, "bias"); err != nil {
		return nil, err
	}
	if err := vector(p.LogTau, n, "log_tau"); err != nil {
		return nil, err
	}
	if len(inputs) > int(^uint(0)>>1)/n/3-1 {
		return nil, fmt.Errorf("sequence shape overflows")
	}
	lambda := make([]float64, n)
	alpha := make([]float64, n)
	for i, raw := range p.LogTau {
		tau := math.Exp(raw)
		if !finite(tau) || tau <= 0 {
			return nil, fmt.Errorf("tau[%d] is not representable and positive", i)
		}
		lambda[i] = math.Exp(-m.config.DT / tau)
		alpha[i] = -math.Expm1(-m.config.DT / tau)
	}
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(in) != n {
			return nil, fmt.Errorf("input[%d] length %d, want %d", t, len(in), n)
		}
		for i, x := range in {
			if !finite(x) {
				return nil, fmt.Errorf("input[%d][%d] is non-finite", t, i)
			}
		}
	}
	var history [][]float64
	var startSteps uint64
	if saved != nil {
		history = cloneRows(saved.History)
		startSteps = saved.Steps
	}
	tr := &Trace{
		model:      m,
		parameters: cloneParameters(p),
		voltage:    make([][]float64, len(inputs)+1),
		output:     make([][]float64, len(inputs)+1),
		drive:      make([][]float64, len(inputs)),
		lambda:     lambda,
		alpha:      alpha,
		coupled:    &coupledTrace{sequence: sequence, gain: make([][]float64, len(inputs)), rawDrive: make([][]float64, len(inputs))},
		history:    history,
		startSteps: startSteps,
	}
	tr.voltage[0] = append([]float64(nil), initial...)
	if saved != nil {
		tr.output[0] = append([]float64(nil), history[len(history)-1]...)
	} else {
		tr.output[0] = make([]float64, n)
		for i, v := range initial {
			tr.output[0][i] = m.activate(v)
		}
	}
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rawDrive := append([]float64(nil), in...)
		if m.partition != nil {
			if err := m.synapticDriveParallel(rawDrive, p.Weights, &forwardReader{output: tr.output, sources: m.config.Sources, delays: m.config.Delays, t: t, history: tr.history, startSteps: tr.startSteps}); err != nil {
				return nil, err
			}
		} else {
			r := forwardReader{output: tr.output, sources: m.config.Sources, delays: m.config.Delays, t: t, history: tr.history, startSteps: tr.startSteps}
			r.accumulateSerial(rawDrive, p.Weights, m.config.Targets)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		previous := append([]float64(nil), tr.output[t]...)
		mod, err := sequence.Forward(ctx, t, previous)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if mod == nil {
			return nil, fmt.Errorf("modulation sequence returned nil modulation at step %d", t)
		}
		if err := mod.validate(1, n, false); err != nil {
			return nil, fmt.Errorf("modulation at step %d: %w", t, err)
		}
		// Own every row before applying it. The gain row is retained by the
		// trace for the reverse pass; offset and threshold are temporary but
		// are copied as well so no caller-owned row is read after validation.
		gain := make([]float64, n)
		for i := range gain {
			gain[i] = 1
		}
		offset := make([]float64, n)
		if mod.Gain != nil {
			copy(gain, mod.Gain[0])
		}
		if mod.Offset != nil {
			copy(offset, mod.Offset[0])
		}
		owned := &Modulation{Gain: [][]float64{gain}, Offset: [][]float64{offset}}
		if mod.Threshold != nil {
			owned.Threshold = [][]float64{append([]float64(nil), mod.Threshold[0]...)}
		}
		drive := append([]float64(nil), rawDrive...)
		vnext, ynext := make([]float64, n), make([]float64, n)
		for i := range drive {
			drive[i] = modulatedDrive(rawDrive[i], owned, 0, i)
			drive[i] += p.Bias[i]
			vnext[i] = lambda[i]*tr.voltage[t][i] + alpha[i]*drive[i]
			ynext[i] = m.activate(vnext[i])
			if !finite(drive[i]) || !finite(vnext[i]) || !finite(ynext[i]) {
				return nil, fmt.Errorf("non-finite state at step %d neuron %d", t, i)
			}
		}
		tr.coupled.gain[t] = gain
		tr.coupled.rawDrive[t] = rawDrive
		tr.drive[t] = drive
		tr.voltage[t+1] = vnext
		tr.output[t+1] = ynext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return tr, nil
}

func (m *Continuous) backwardCoupled(ctx context.Context, tr *Trace, upstream [][]float64, window int) (Gradient, error) {
	var empty Gradient
	steps, n := len(tr.drive), m.config.Nodes
	if tr.coupled == nil || tr.coupled.sequence == nil {
		return empty, fmt.Errorf("coupled trace has no modulation sequence")
	}
	if len(tr.coupled.gain) != steps || len(tr.coupled.rawDrive) != steps || len(tr.voltage) != steps+1 || len(tr.output) != steps+1 || len(tr.alpha) != n || len(tr.lambda) != n {
		return empty, fmt.Errorf("invalid coupled trace")
	}
	return m.backwardCoupledTape(ctx, tr, upstream, window, func(_ context.Context, step int) (*Trace, int, error) { return tr, step, nil })
}
