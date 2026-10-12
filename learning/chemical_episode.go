package learning

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
)

// ChemicalEpisode declares the isolated chemical boundary of an independent
// training/prediction episode. Part owns concentration, source state, resources,
// available feedback and expression settings. SourceStep is the model clock
// used by timelines/replay/feedback, not Part.State.Steps, which counts only
// kinetics updates. InitialActivity, when supplied, is a constant boundary for
// the first source release. Otherwise the fresh core's initial output is used.
// Within the episode later releases observe the actual previous core output.
// Episode execution does not advance persistent Individual state; successful
// training commits only the existing parameters/optimizer/episode transaction.
type ChemicalEpisode struct {
	Part            ChemicalPart `json:"part"`
	SourceStep      uint64       `json:"source_step"`
	InitialActivity []float64    `json:"initial_activity,omitempty"`
}

// prepareChemicalEpisode validates every state/declaration/boundary, validates
// expression against readout nodes, rejects unsupported cores and returns an
// owned canonical boundary. nil preserves the legacy configuration.
func prepareChemicalEpisode(e *ChemicalEpisode, core coreModel, readout []int) (*ChemicalEpisode, error) {
	if e == nil {
		return nil, nil
	}
	if core == nil {
		return nil, fmt.Errorf("chemical episode needs a configured CPU scalar core")
	}
	if core.stateDim() != 1 {
		return nil, fmt.Errorf("chemical episode supports CPU scalar cores only")
	}
	switch core.(type) {
	case continuousCore, lifCore, mixedCore:
	default:
		return nil, fmt.Errorf("chemical episode supports CPU scalar continuous, LIF or mixed cores only")
	}
	nodes := core.nodes()
	if nodes < 1 {
		return nil, fmt.Errorf("chemical episode needs at least one core node")
	}
	if e.SourceStep > math.MaxInt64 {
		return nil, fmt.Errorf("chemical episode source_step %d does not fit an int64 timestamp", e.SourceStep)
	}
	if err := e.Part.Config.Validate(nodes); err != nil {
		return nil, fmt.Errorf("chemical episode config: %w", err)
	}
	if err := e.Part.Config.ValidateState(e.Part.State); err != nil {
		return nil, fmt.Errorf("chemical episode state: %w", err)
	}
	if e.InitialActivity != nil {
		if len(e.InitialActivity) != nodes {
			return nil, fmt.Errorf("chemical episode initial_activity has %d values, want %d", len(e.InitialActivity), nodes)
		}
		for node, value := range e.InitialActivity {
			if !finite(value) {
				return nil, fmt.Errorf("chemical episode initial_activity[%d] is not finite", node)
			}
		}
	}
	for name, amount := range e.Part.Resources {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("chemical episode resource needs a name")
		}
		if !finite(amount) {
			return nil, fmt.Errorf("chemical episode resource %q is not finite", name)
		}
	}
	for index, spec := range e.Part.PendingFeedback {
		feedback, err := signal.NewFeedback(spec)
		if err != nil {
			return nil, fmt.Errorf("chemical episode pending feedback %d: %w", index, err)
		}
		if spec.AvailableAt.Unit != modulation.ReleaseTimeUnit {
			return nil, fmt.Errorf("chemical episode pending feedback %d uses time unit %q, want %q", index, spec.AvailableAt.Unit, modulation.ReleaseTimeUnit)
		}
		if err := feedback.Validate(); err != nil {
			return nil, fmt.Errorf("chemical episode pending feedback %d: %w", index, err)
		}
	}
	if e.Part.Expression != nil {
		if err := e.Part.Expression.Validate(readout, len(e.Part.Config.Receptors.Records)); err != nil {
			return nil, fmt.Errorf("chemical episode expression: %w", err)
		}
	}
	// newChemicalRuntime and restoreSourceStates validate controller/source
	// boundaries against this core and never mutate the caller's declaration.
	runtime, err := newChemicalRuntime(e.Part.Config, nodes)
	if err != nil {
		return nil, fmt.Errorf("chemical episode runtime: %w", err)
	}
	if err := runtime.restoreSourceStates(e.Part.SourceStates); err != nil {
		return nil, fmt.Errorf("chemical episode source state: %w", err)
	}
	return copyChemicalEpisode(e), nil
}

// copyChemicalEpisode copies every nested declaration, concentration, source
// window, resource, feedback and expression without changing nil semantics.
func copyChemicalEpisode(e *ChemicalEpisode) *ChemicalEpisode {
	if e == nil {
		return nil
	}
	owned := *e
	owned.Part.Config = e.Part.Config.Clone()
	owned.Part.State = copyChemicalState(e.Part.State)
	if e.Part.Resources != nil {
		owned.Part.Resources = make(map[string]float64, len(e.Part.Resources))
		for name, amount := range e.Part.Resources {
			owned.Part.Resources[name] = amount
		}
	}
	if e.Part.PendingFeedback != nil {
		owned.Part.PendingFeedback = append([]signal.FeedbackSpec(nil), e.Part.PendingFeedback...)
	}
	owned.Part.SourceStates = copyChemicalSourceStates(e.Part.SourceStates)
	owned.Part.Expression = copyExpression(e.Part.Expression)
	owned.InitialActivity = append([]float64(nil), e.InitialActivity...)
	return &owned
}

func copyChemicalSourceStates(states []ChemicalSourceState) []ChemicalSourceState {
	if states == nil {
		return nil
	}
	owned := make([]ChemicalSourceState, len(states))
	for index, state := range states {
		if state.Controller == nil {
			continue
		}
		controller := *state.Controller
		controller.Window = copyRows(state.Controller.Window)
		owned[index].Controller = &controller
	}
	return owned
}

// forwardChemical joins a private source/kinetics/receptor sequence with the
// CPU scalar continuous, LIF or mixed core and applies readout-only expression.
// The trace's ordinary reverse includes chemical recurrence, activity-window
// feedback and expression derivatives with a fresh private reverse session.
func (n *Network) forwardChemical(ctx context.Context, p Parameters, inputs [][]float64, segment int, initial *NeuralState) (coreTrace, [][]float64, error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("nil context")
	}
	if segment < 0 || (segment > 0 && initial != nil) {
		return nil, nil, fmt.Errorf("invalid chemical recompute segment or unsupported stateful recompute")
	}
	if n == nil || n.core == nil || n.config.Chemistry == nil {
		return nil, nil, fmt.Errorf("chemical forward needs a configured network and episode")
	}
	if initial != nil {
		if _, ok := n.core.(continuousCore); !ok || initial.Core != NeuralCoreContinuous || initial.Continuous == nil || initial.LIF != nil || initial.Mixed != nil {
			return nil, nil, fmt.Errorf("stateful chemical forward requires a CPU scalar continuous state")
		}
	}
	if len(inputs) == 0 {
		return nil, nil, fmt.Errorf("empty input sequence")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	sequence, err := newChemicalSequence(ctx, n.config.Chemistry, n.core.nodes(), n.chemicalFrozen, len(inputs))
	if err != nil {
		return nil, nil, err
	}
	if segment > 0 {
		var trace coreTrace
		var raw [][]float64
		switch core := n.core.(type) {
		case continuousCore:
			var concrete *dynamics.ContinuousCoupledRecomputeTrace
			concrete, err = core.model.ForwardCoupledRecompute(ctx, p.Core, make([]float64, n.core.nodes()), inputs, sequence, segment)
			if err == nil {
				trace, raw = concrete, concrete.Outputs()
			}
		case lifCore:
			var concrete *dynamics.LIFCoupledRecomputeTrace
			concrete, err = core.model.ForwardCoupledRecompute(ctx, dynamics.LIFParameters{Weights: p.Core.Weights, Bias: p.Core.Bias, LogTau: p.Core.LogTau, ThetaRaw: p.ThetaRaw}, make([]float64, n.core.nodes()), inputs, sequence, segment)
			if err == nil {
				trace, raw = concrete, concrete.Outputs()
			}
		default:
			err = fmt.Errorf("chemical recompute supports CPU scalar continuous and LIF cores only")
		}
		if err != nil {
			return nil, nil, err
		}
		if err := sequence.setRawOutputs(ctx, raw); err != nil {
			return nil, nil, err
		}
		out, err := sequence.applyExpression(ctx, raw)
		if err != nil {
			return nil, nil, err
		}
		return trace, out, nil
	}
	zero := make([]float64, n.core.nodes())
	var (
		trace coreTrace
		y     [][]float64
	)
	switch core := n.core.(type) {
	case continuousCore:
		var concrete *dynamics.Trace
		if initial == nil {
			concrete, err = core.model.ForwardCoupled(ctx, p.Core, zero, inputs, sequence)
		} else {
			concrete, err = core.model.ForwardCoupledFromState(ctx, p.Core, *initial.Continuous, inputs, sequence)
		}
		if err == nil {
			trace, y = concrete, concrete.Outputs()
		}
	case lifCore:
		var concrete *dynamics.LIFTrace
		concrete, err = core.model.ForwardCoupled(ctx, dynamics.LIFParameters{
			Weights: p.Core.Weights, Bias: p.Core.Bias, LogTau: p.Core.LogTau, ThetaRaw: p.ThetaRaw,
		}, zero, inputs, sequence)
		if err == nil {
			trace, y = concrete, concrete.Outputs()
		}
	case mixedCore:
		var concrete *dynamics.MixedTrace
		concrete, err = core.model.ForwardCoupled(ctx, dynamics.MixedParameters{
			Weights: p.Core.Weights, Bias: p.Core.Bias, LogTau: p.Core.LogTau, ThetaRaw: p.ThetaRaw,
		}, zero, inputs, sequence)
		if err == nil {
			trace, y = concrete, concrete.Outputs()
		}
	default:
		err = fmt.Errorf("chemical episode supports CPU scalar continuous, LIF or mixed cores only")
	}
	if err != nil {
		return nil, nil, err
	}
	if err := sequence.setRawOutputs(ctx, y); err != nil {
		return nil, nil, err
	}
	out, err := sequence.applyExpression(ctx, y)
	if err != nil {
		return nil, nil, err
	}
	return trace, out, nil
}
