package dynamics_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

// recomputeSequenceRecorder is deliberately separate from the production
// sequence. It records global calls and replay calls so a passing result also
// proves that source effects were not run twice on the original sequence.
type recomputeSequenceRecorder struct {
	originalCalls []int
	replayCalls   []recomputeReplayCall
	restarts      []int
	reverseCount  int
}

type recomputeReplayCall struct {
	restart int
	local   int
	global  int
}

type recomputeSequence struct {
	recorder  *recomputeSequenceRecorder
	nodes     int
	offset    int
	restartID int
	prior     []float64
	clock     int
	spiking   bool
	rows      [][]float64
}

func newRecomputeSequence(nodes int, spiking bool) *recomputeSequence {
	return &recomputeSequence{
		recorder: &recomputeSequenceRecorder{},
		nodes:    nodes,
		spiking:  spiking,
		prior:    make([]float64, nodes),
	}
}

func (s *recomputeSequence) Forward(ctx context.Context, step int, previous []float64) (*dynamics.Modulation, error) {
	if ctx == nil {
		return nil, errors.New("recompute sequence received nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.recorder == nil || len(previous) != s.nodes || step != len(s.rows) {
		return nil, fmt.Errorf("recompute sequence received an invalid step or row")
	}
	global := s.offset + step
	if global != s.clock {
		return nil, fmt.Errorf("recompute sequence global clock %d, want %d", s.clock, global)
	}
	s.clock++
	if s.restartID == 0 {
		s.recorder.originalCalls = append(s.recorder.originalCalls, step)
	} else {
		s.recorder.replayCalls = append(s.recorder.replayCalls, recomputeReplayCall{
			restart: s.restartID,
			local:   step,
			global:  global,
		})
	}
	current := append([]float64(nil), previous...)
	older := append([]float64(nil), s.prior...)
	gain := make([]float64, s.nodes)
	offset := make([]float64, s.nodes)
	threshold := make([]float64, s.nodes)
	for i := range current {
		j := (i + 1) % s.nodes
		gain[i] = 1.17 + 0.035*current[i] + 0.017*older[i]
		offset[i] = 0.04 + 0.025*current[j] - 0.009*older[j]
		if s.spiking {
			threshold[i] = 0.015 + 0.008*current[i] + 0.003*older[i]
		}
	}
	s.rows = append(s.rows, current)
	s.prior = current
	modulation := &dynamics.Modulation{Gain: [][]float64{gain}, Offset: [][]float64{offset}}
	if s.spiking {
		modulation.Threshold = [][]float64{threshold}
	}
	return modulation, nil
}

func (s *recomputeSequence) NewReverse(ctx context.Context, totalsteps, window int) (dynamics.ModulationReverse, error) {
	if ctx == nil {
		return nil, errors.New("recompute sequence reverse received nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || totalsteps != len(s.rows) || window < 0 {
		return nil, fmt.Errorf("recompute sequence reverse shape or window is invalid")
	}
	s.recorder.reverseCount++
	return &recomputeReverse{
		totalsteps: totalsteps,
		window:     window,
		rows:       coupledRowsClone(s.rows),
		spiking:    s.spiking,
	}, nil
}

func (s *recomputeSequence) Checkpoint(ctx context.Context, nextStep int) (dynamics.ModulationRestart, error) {
	if ctx == nil {
		return nil, errors.New("recompute sequence checkpoint received nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || nextStep != len(s.rows) || nextStep < 0 || s.clock != s.offset+nextStep {
		return nil, fmt.Errorf("recompute sequence checkpoint boundary is invalid")
	}
	prior := append([]float64(nil), s.prior...)
	clock := s.clock
	offset := s.offset + nextStep
	id := len(s.recorder.restarts) + 1
	s.recorder.restarts = append(s.recorder.restarts, nextStep)
	return func(restartCtx context.Context) (dynamics.ModulationSequence, error) {
		if restartCtx == nil {
			return nil, errors.New("recompute sequence restart received nil context")
		}
		if err := restartCtx.Err(); err != nil {
			return nil, err
		}
		return &recomputeSequence{
			recorder:  s.recorder,
			nodes:     s.nodes,
			offset:    offset,
			restartID: id,
			prior:     append([]float64(nil), prior...),
			clock:     clock,
			spiking:   s.spiking,
		}, nil
	}, nil
}

type recomputeReverse struct {
	totalsteps int
	window     int
	rows       [][]float64
	spiking    bool
}

func (r *recomputeReverse) BackwardStep(ctx context.Context, step int, gradient dynamics.ModulationGradient) ([]dynamics.ActivityGradient, error) {
	if ctx == nil {
		return nil, errors.New("recompute reverse received nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil || step < 0 || step >= r.totalsteps || len(gradient.Gain) == 0 {
		return nil, fmt.Errorf("recompute reverse step or gradient is invalid")
	}
	if len(gradient.Gain) != len(r.rows[0]) || len(gradient.Offset) != len(r.rows[0]) {
		return nil, fmt.Errorf("recompute reverse gradient shape is invalid")
	}
	start := 0
	if r.window > 0 {
		start = step / r.window * r.window
	}
	appendIfLive := func(rows []dynamics.ActivityGradient, target int, values []float64) []dynamics.ActivityGradient {
		if target < -1 || target >= step {
			return rows
		}
		if target == -1 {
			if start != 0 {
				return rows
			}
		} else if target < start {
			return rows
		}
		return append(rows, dynamics.ActivityGradient{Step: target, Values: values})
	}
	valuesFor := func(older bool) []float64 {
		values := make([]float64, len(r.rows[0]))
		for i := range values {
			j := (i + len(values) - 1) % len(values)
			if older {
				values[i] = recomputeValueAt(gradient.Gain, i)*0.017 - recomputeValueAt(gradient.Offset, j)*0.009
				if r.spiking {
					values[i] += recomputeValueAt(gradient.Threshold, i) * 0.003
				}
			} else {
				values[i] = recomputeValueAt(gradient.Gain, i)*0.035 + recomputeValueAt(gradient.Offset, j)*0.025
				if r.spiking {
					values[i] += recomputeValueAt(gradient.Threshold, i) * 0.008
				}
			}
		}
		return values
	}
	rows := make([]dynamics.ActivityGradient, 0, 2)
	rows = appendIfLive(rows, step-1, valuesFor(false))
	if step > 0 {
		rows = appendIfLive(rows, step-2, valuesFor(true))
	}
	return rows, nil
}

func recomputeValueAt(values []float64, index int) float64 {
	if index < 0 || index >= len(values) {
		return 0
	}
	return values[index]
}

func lifGradientBitsEqual(t *testing.T, label string, got, want dynamics.LIFGradient) {
	t.Helper()
	coupledVectorBitsEqual(t, label+" weights", got.Weights, want.Weights)
	coupledVectorBitsEqual(t, label+" bias", got.Bias, want.Bias)
	coupledVectorBitsEqual(t, label+" log tau", got.LogTau, want.LogTau)
	coupledVectorBitsEqual(t, label+" theta", got.ThetaRaw, want.ThetaRaw)
	coupledRowsBitsEqual(t, label+" inputs", got.Inputs, want.Inputs)
	coupledVectorBitsEqual(t, label+" initial", got.Initial, want.Initial)
}

func coupledContinuousFixture(t *testing.T) (*dynamics.Continuous, dynamics.Parameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	model, err := dynamics.NewContinuous(dynamics.Config{
		Nodes: 3, Sources: []int{2, 0, 1, 0}, Targets: []int{0, 1, 2, 2},
		Delays: []int{2, 0, 1, 2}, DT: 0.35, Activation: "tanh",
	})
	if err != nil {
		t.Fatal(err)
	}
	parameters := dynamics.Parameters{
		Weights: []float64{0.27, -0.22, 0.19, -0.11},
		Bias:    []float64{0.11, -0.16, 0.07},
		LogTau:  []float64{0.18, -0.12, 0.09},
	}
	initial := []float64{0.21, -0.29, 0.17}
	const steps = 23
	inputs := make([][]float64, steps)
	upstream := make([][]float64, steps)
	for step := range inputs {
		f := float64(step + 1)
		inputs[step] = []float64{0.19 + 0.013*f, -0.23 + 0.017*f, 0.11 - 0.009*f}
		upstream[step] = []float64{0.07 - 0.003*f, -0.04 + 0.005*f, 0.09 + 0.002*f}
	}
	return model, parameters, initial, inputs, upstream
}

func coupledLIFFixture(t *testing.T) (*dynamics.LIF, dynamics.LIFParameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	homeostasis := &dynamics.LIFHomeostasis{Enabled: true, TauRate: 1.7, TargetRate: 0.2, Eta: 0.35, HMax: 0.5}
	model, err := dynamics.NewLIF(dynamics.LIFConfig{
		Nodes: 3, Sources: []int{2, 0, 1, 0}, Targets: []int{0, 1, 2, 2},
		Delays: []int{2, 0, 1, 2}, DT: 0.25, TauSyn: 0.8,
		ThetaMin: 0.1, ThetaMax: 1.8, VReset: -0.5, RefractorySteps: 2,
		Adaptation:  dynamics.LIFAdaptation{Enabled: true, TauAdapt: 1.1, Beta: 0.25},
		Homeostasis: homeostasis,
		Surrogate:   dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 1.7},
	})
	if err != nil {
		t.Fatal(err)
	}
	parameters := dynamics.LIFParameters{
		Weights:  []float64{0.55, -0.35, 0.4, 0.3},
		Bias:     []float64{0.2, 0.15, 0.2},
		LogTau:   []float64{-0.2, -0.18, -0.15},
		ThetaRaw: []float64{-0.2, -0.15, -0.1},
	}
	initial := []float64{0, 0, 0}
	const steps = 23
	inputs := make([][]float64, steps)
	upstream := make([][]float64, steps)
	for step := range inputs {
		f := float64(step % 5)
		inputs[step] = []float64{1.7 + 0.12*f, 1.5 + 0.1*float64((step+1)%4), 1.8 + 0.08*float64((step+2)%3)}
		upstream[step] = []float64{0.03 + 0.002*float64(step), -0.02 + 0.001*float64(step), 0.04 - 0.001*float64(step)}
	}
	return model, parameters, initial, inputs, upstream
}

func expectedRestartSteps(steps, segment int) int {
	if segment <= 0 || segment >= steps {
		return 0
	}
	return (steps - 1) / segment
}

func assertRestartReplayBoundaries(t *testing.T, recorder *recomputeSequenceRecorder, steps, segment int) {
	t.Helper()
	if len(recorder.originalCalls) != steps {
		t.Fatalf("original calls=%v, want one global call per step", recorder.originalCalls)
	}
	for step, got := range recorder.originalCalls {
		if got != step {
			t.Fatalf("original call %d=%d, want global step %d", step, got, step)
		}
	}
	wantRestarts := expectedRestartSteps(steps, segment)
	if wantRestarts > 0 && len(recorder.restarts) == 0 {
		t.Fatalf("segment %d made no checkpoints, want at least one", segment)
	}
	for _, boundary := range recorder.restarts {
		if boundary == 0 {
			continue
		}
		seen := false
		for _, call := range recorder.replayCalls {
			if call.global == boundary {
				if call.local != 0 {
					t.Fatalf("restart at global %d first local step=%d, want 0", boundary, call.local)
				}
				seen = true
				break
			}
		}
		if !seen {
			t.Fatalf("checkpoint at global %d was never replayed", boundary)
		}
	}
	firstLocal := make(map[int]bool)
	for i, call := range recorder.replayCalls {
		if call.local < 0 || call.local >= segment && segment < steps {
			t.Fatalf("replay call %d has invalid local step %+v", i, call)
		}
		if !firstLocal[call.restart] {
			if call.local != 0 {
				t.Fatalf("restart %d first observed local step=%d, want 0", call.restart, call.local)
			}
			firstLocal[call.restart] = true
		}
	}
}

func TestContinuousCoupledRecomputeMatchesFullHistory(t *testing.T) {
	ctx := context.Background()
	for _, segment := range []int{1, 4, 7, 30} {
		for _, window := range []int{0, 3, 5} {
			t.Run(fmt.Sprintf("segment=%d/window=%d", segment, window), func(t *testing.T) {
				model, parameters, initial, inputs, upstream := coupledContinuousFixture(t)
				fullSequence := newRecomputeSequence(3, false)
				full, err := model.ForwardCoupled(ctx, parameters, initial, inputs, fullSequence)
				if err != nil {
					t.Fatal(err)
				}
				fullGradient, err := model.Backward(ctx, full, upstream, window)
				if err != nil {
					t.Fatal(err)
				}
				recomputeSequence := newRecomputeSequence(3, false)
				recomputed, err := model.ForwardCoupledRecompute(ctx, parameters, initial, inputs, recomputeSequence, segment)
				if err != nil {
					t.Fatal(err)
				}
				recomputedGradient, err := model.BackwardCoupledRecompute(ctx, recomputed, upstream, window)
				if err != nil {
					t.Fatal(err)
				}
				coupledRowsBitsEqual(t, "outputs", recomputed.Outputs(), full.Outputs())
				coupledVectorBitsEqual(t, "final voltage", recomputed.FinalVoltage(), full.FinalVoltage())
				coupledGradientsBitsEqual(t, "gradient", recomputedGradient, fullGradient)
				if recomputed.Stats().Steps != len(inputs) || recomputed.Stats().Segment != segment || recomputed.Stats().FullHistoryRows != len(inputs)+1 {
					t.Fatalf("stats=%+v, want steps=%d segment=%d full rows=%d", recomputed.Stats(), len(inputs), segment, len(inputs)+1)
				}
				if segment < len(inputs) && recomputed.Stats().MaxSegmentRows >= recomputed.Stats().FullHistoryRows {
					t.Fatalf("retained neural rows=%d, full voltage rows=%d", recomputed.Stats().MaxSegmentRows, recomputed.Stats().FullHistoryRows)
				}
				if segment < len(inputs) && (recomputed.Stats().CheckpointRows <= 0 || recomputed.Stats().CheckpointRows >= recomputed.Stats().FullHistoryRows) {
					t.Fatalf("stats=%+v, want retained checkpoint rows below the full neural history", recomputed.Stats())
				}
				beforeRepeat := recomputeSequence.recorder.reverseCount
				repeat, err := model.BackwardCoupledRecompute(ctx, recomputed, upstream, window)
				if err != nil {
					t.Fatal(err)
				}
				coupledGradientsBitsEqual(t, "repeat gradient", repeat, recomputedGradient)
				if recomputeSequence.recorder.reverseCount != beforeRepeat+1 {
					t.Fatalf("reverse sessions=%d, want a fresh session for repeated backward", recomputeSequence.recorder.reverseCount)
				}
				assertRestartReplayBoundaries(t, recomputeSequence.recorder, len(inputs), segment)
				if len(recomputeSequence.recorder.originalCalls) != len(inputs) {
					t.Fatalf("original source effects were repeated: %v", recomputeSequence.recorder.originalCalls)
				}
			})
		}
	}
}

func TestLIFCoupledRecomputeMatchesFullHistory(t *testing.T) {
	ctx := context.Background()
	for _, segment := range []int{1, 4, 7, 30} {
		for _, window := range []int{0, 3, 5} {
			t.Run(fmt.Sprintf("segment=%d/window=%d", segment, window), func(t *testing.T) {
				model, parameters, initial, inputs, upstream := coupledLIFFixture(t)
				fullSequence := newRecomputeSequence(3, true)
				full, err := model.ForwardCoupled(ctx, parameters, initial, inputs, fullSequence)
				if err != nil {
					t.Fatal(err)
				}
				spikeCount := 0
				for _, row := range full.Spikes() {
					for _, spike := range row {
						if spike != 0 {
							spikeCount++
						}
					}
				}
				if spikeCount == 0 {
					t.Fatal("LIF fixture produced no hard spikes; adaptation/refractory/homeostasis were not exercised")
				}
				fullGradient, err := model.Backward(ctx, full, upstream, window)
				if err != nil {
					t.Fatal(err)
				}
				recomputeSequence := newRecomputeSequence(3, true)
				recomputed, err := model.ForwardCoupledRecompute(ctx, parameters, initial, inputs, recomputeSequence, segment)
				if err != nil {
					t.Fatal(err)
				}
				recomputedGradient, err := model.BackwardCoupledRecompute(ctx, recomputed, upstream, window)
				if err != nil {
					t.Fatal(err)
				}
				coupledRowsBitsEqual(t, "synaptic outputs", recomputed.Outputs(), full.Outputs())
				coupledVectorBitsEqual(t, "final voltage", recomputed.FinalVoltage(), full.FinalVoltage())
				lifGradientBitsEqual(t, "gradient", recomputedGradient, fullGradient)
				stats := recomputed.Stats()
				if stats.Steps != len(inputs) || stats.Segment != segment || stats.FullHistoryRows != len(inputs)+1 {
					t.Fatalf("stats=%+v, want steps=%d segment=%d full rows=%d", stats, len(inputs), segment, len(inputs)+1)
				}
				if segment < len(inputs) && stats.MaxSegmentRows >= stats.FullHistoryRows {
					t.Fatalf("retained neural rows=%d, full voltage rows=%d", stats.MaxSegmentRows, stats.FullHistoryRows)
				}
				if segment < len(inputs) && (stats.CheckpointRows <= 0 || stats.CheckpointRows >= stats.FullHistoryRows) {
					t.Fatalf("stats=%+v, want retained checkpoint rows below the full neural history", stats)
				}
				beforeRepeat := recomputeSequence.recorder.reverseCount
				repeat, err := model.BackwardCoupledRecompute(ctx, recomputed, upstream, window)
				if err != nil {
					t.Fatal(err)
				}
				lifGradientBitsEqual(t, "repeat gradient", repeat, recomputedGradient)
				if recomputeSequence.recorder.reverseCount != beforeRepeat+1 {
					t.Fatalf("reverse sessions=%d, want a fresh session for repeated backward", recomputeSequence.recorder.reverseCount)
				}
				assertRestartReplayBoundaries(t, recomputeSequence.recorder, len(inputs), segment)
			})
		}
	}
}

func TestCoupledRecomputeRejectsInvalidInputsAndForeignTraces(t *testing.T) {
	model, parameters, initial, inputs, upstream := coupledContinuousFixture(t)
	valid := func() *recomputeSequence { return newRecomputeSequence(3, false) }
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		seq  dynamics.RecomputableModulationSequence
		seg  int
		init []float64
		in   [][]float64
	}{
		{"nil context", nil, valid(), 4, initial, inputs},
		{"canceled context", canceled, valid(), 4, initial, inputs},
		{"nil sequence", context.Background(), nil, 4, initial, inputs},
		{"zero segment", context.Background(), valid(), 0, initial, inputs},
		{"negative segment", context.Background(), valid(), -1, initial, inputs},
		{"bad initial shape", context.Background(), valid(), 4, []float64{0}, inputs},
		{"bad input shape", context.Background(), valid(), 4, initial, append(coupledRowsClone(inputs), []float64{math.NaN()})},
		{"nonfinite input", context.Background(), valid(), 4, initial, func() [][]float64 { rows := coupledRowsClone(inputs); rows[0][0] = math.NaN(); return rows }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trace, err := model.ForwardCoupledRecompute(tc.ctx, parameters, tc.init, tc.in, tc.seq, tc.seg)
			if err == nil || trace != nil {
				t.Fatalf("trace=%v err=%v, want a nil trace and explicit error", trace, err)
			}
		})
	}

	sequence := valid()
	trace, err := model.ForwardCoupledRecompute(context.Background(), parameters, initial, inputs, sequence, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		ctx  context.Context
		up   [][]float64
		win  int
	}{
		{"nil context", nil, upstream, 0},
		{"canceled context", canceled, upstream, 0},
		{"negative window", context.Background(), upstream, -1},
		{"bad upstream shape", context.Background(), append(coupledRowsClone(upstream), []float64{0}), 0},
		{"nonfinite upstream", context.Background(), func() [][]float64 { rows := coupledRowsClone(upstream); rows[0][0] = math.NaN(); return rows }(), 0},
	} {
		t.Run("backward/"+tc.name, func(t *testing.T) {
			gradient, err := model.BackwardCoupledRecompute(tc.ctx, trace, tc.up, tc.win)
			if err == nil || !coupledRecomputeGradientIsZero(gradient) {
				t.Fatalf("gradient=%+v err=%v, want zero gradient and explicit error", gradient, err)
			}
		})
	}
	other, err := dynamics.NewContinuous(model.Config())
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := other.BackwardCoupledRecompute(context.Background(), trace, upstream, 0)
	if err == nil || !coupledRecomputeGradientIsZero(foreign) {
		t.Fatalf("foreign trace gradient=%+v err=%v, want explicit ownership error", foreign, err)
	}
	if !reflect.DeepEqual(sequence.recorder.originalCalls, makeRange(len(inputs))) {
		t.Fatalf("invalid backward calls changed original sequence: %v", sequence.recorder.originalCalls)
	}
}

func coupledRecomputeGradientIsZero(g dynamics.Gradient) bool {
	return g.Weights == nil && g.Bias == nil && g.LogTau == nil && g.Inputs == nil && g.Initial == nil
}

func lifGradientIsZero(g dynamics.LIFGradient) bool {
	return g.Weights == nil && g.Bias == nil && g.LogTau == nil && g.ThetaRaw == nil && g.Inputs == nil && g.Initial == nil
}

func TestLIFCoupledRecomputeRejectsInvalidInputsAndForeignTraces(t *testing.T) {
	model, parameters, initial, inputs, upstream := coupledLIFFixture(t)
	valid := func() *recomputeSequence { return newRecomputeSequence(3, true) }
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		seq  dynamics.RecomputableModulationSequence
		seg  int
		init []float64
		in   [][]float64
	}{
		{"nil context", nil, valid(), 4, initial, inputs},
		{"canceled context", canceled, valid(), 4, initial, inputs},
		{"nil sequence", context.Background(), nil, 4, initial, inputs},
		{"zero segment", context.Background(), valid(), 0, initial, inputs},
		{"negative segment", context.Background(), valid(), -1, initial, inputs},
		{"bad initial shape", context.Background(), valid(), 4, []float64{0}, inputs},
		{"bad input shape", context.Background(), valid(), 4, initial, append(coupledRowsClone(inputs), []float64{math.NaN()})},
		{"nonfinite input", context.Background(), valid(), 4, initial, func() [][]float64 { rows := coupledRowsClone(inputs); rows[0][0] = math.NaN(); return rows }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trace, err := model.ForwardCoupledRecompute(tc.ctx, parameters, tc.init, tc.in, tc.seq, tc.seg)
			if err == nil || trace != nil {
				t.Fatalf("trace=%v err=%v, want a nil trace and explicit error", trace, err)
			}
		})
	}
	sequence := valid()
	trace, err := model.ForwardCoupledRecompute(context.Background(), parameters, initial, inputs, sequence, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		ctx  context.Context
		up   [][]float64
		win  int
	}{
		{"nil context", nil, upstream, 0},
		{"canceled context", canceled, upstream, 0},
		{"negative window", context.Background(), upstream, -1},
		{"bad upstream shape", context.Background(), append(coupledRowsClone(upstream), []float64{0}), 0},
		{"nonfinite upstream", context.Background(), func() [][]float64 { rows := coupledRowsClone(upstream); rows[0][0] = math.NaN(); return rows }(), 0},
	} {
		t.Run("backward/"+tc.name, func(t *testing.T) {
			gradient, err := model.BackwardCoupledRecompute(tc.ctx, trace, tc.up, tc.win)
			if err == nil || !lifGradientIsZero(gradient) {
				t.Fatalf("gradient=%+v err=%v, want zero gradient and explicit error", gradient, err)
			}
		})
	}
	other, err := dynamics.NewLIF(model.Config())
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := other.BackwardCoupledRecompute(context.Background(), trace, upstream, 0)
	if err == nil || !lifGradientIsZero(foreign) {
		t.Fatalf("foreign trace gradient=%+v err=%v, want explicit ownership error", foreign, err)
	}
}

func makeRange(n int) []int {
	values := make([]int, n)
	for i := range values {
		values[i] = i
	}
	return values
}
