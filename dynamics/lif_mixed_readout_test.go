package dynamics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
)

const lifMixedReadoutRelativeTolerance = 1e-13

// The readout reverse receives the direct output adjoint u_t and returns
// (t+2)u_t. The ordinary, uncoupled Backward with those independently scaled
// rows is the reference for the coupled result. A relative tolerance is used
// because the non-nil coupled path can differ from the plain path by one ULP.
func lifMixedReadoutScaledRows(rows [][]float64) [][]float64 {
	result := make([][]float64, len(rows))
	for step, row := range rows {
		result[step] = make([]float64, len(row))
		for node, value := range row {
			result[step][node] = float64(step+2) * value
		}
	}
	return result
}

func lifMixedReadoutFloatClose(got, want float64) bool {
	if got == want {
		return true
	}
	if math.IsNaN(got) || math.IsNaN(want) || math.IsInf(got, 0) || math.IsInf(want, 0) {
		return false
	}
	return math.Abs(got-want) <= lifMixedReadoutRelativeTolerance*math.Max(1, math.Abs(want))
}

func lifMixedReadoutCloseVector(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has length %d, want %d", name, len(got), len(want))
	}
	for i, wantValue := range want {
		if !lifMixedReadoutFloatClose(got[i], wantValue) {
			t.Fatalf("%s[%d] = %.17g, want %.17g", name, i, got[i], wantValue)
		}
	}
}

func lifMixedReadoutCloseRows(t *testing.T, name string, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d rows, want %d", name, len(got), len(want))
	}
	for row := range want {
		lifMixedReadoutCloseVector(t, fmt.Sprintf("%s[%d]", name, row), got[row], want[row])
	}
}

func lifMixedReadoutCloseLIFGradient(t *testing.T, got, want LIFGradient) {
	t.Helper()
	lifMixedReadoutCloseVector(t, "weights", got.Weights, want.Weights)
	lifMixedReadoutCloseVector(t, "bias", got.Bias, want.Bias)
	lifMixedReadoutCloseVector(t, "log_tau", got.LogTau, want.LogTau)
	lifMixedReadoutCloseVector(t, "theta_raw", got.ThetaRaw, want.ThetaRaw)
	lifMixedReadoutCloseRows(t, "inputs", got.Inputs, want.Inputs)
	lifMixedReadoutCloseVector(t, "initial", got.Initial, want.Initial)
}

func lifMixedReadoutCloseMixedGradient(t *testing.T, got, want MixedGradient) {
	t.Helper()
	lifMixedReadoutCloseVector(t, "weights", got.Weights, want.Weights)
	lifMixedReadoutCloseVector(t, "bias", got.Bias, want.Bias)
	lifMixedReadoutCloseVector(t, "log_tau", got.LogTau, want.LogTau)
	lifMixedReadoutCloseVector(t, "theta_raw", got.ThetaRaw, want.ThetaRaw)
	lifMixedReadoutCloseRows(t, "inputs", got.Inputs, want.Inputs)
	lifMixedReadoutCloseVector(t, "initial", got.Initial, want.Initial)
}

func TestLIFCoupledReadout(t *testing.T) {
	m, p, initial, inputs, upstream := lifCoupledSmoothFixture(t)
	scaled := lifMixedReadoutScaledRows(upstream)

	plain, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	want, err := m.Backward(context.Background(), plain, scaled, 0)
	if err != nil {
		t.Fatal(err)
	}

	originalUpstream := lifCoupledCloneRows(upstream)
	sequence := &coupledReadoutTestSequence{}
	trace, err := m.ForwardCoupled(context.Background(), p, initial, inputs, sequence)
	if err != nil {
		t.Fatal(err)
	}
	beforeOutputs := trace.Outputs()
	beforeSpikes := trace.Spikes()
	beforeVoltages := trace.Voltages()
	beforeFinalVoltage := trace.FinalVoltage()
	beforeParameters := lifCoupledCloneParameters(trace.parameters)

	for pass := 0; pass < 2; pass++ {
		got, err := m.Backward(context.Background(), trace, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		lifMixedReadoutCloseLIFGradient(t, got, want)
		if len(sequence.reverses) != pass+1 {
			t.Fatalf("reverse sessions = %d, want %d", len(sequence.reverses), pass+1)
		}
		reverse := sequence.reverses[pass]
		if !reflect.DeepEqual(reverse.observed, originalUpstream) {
			t.Fatalf("readout callback observed %v, want ascending direct upstream %v", reverse.observed, originalUpstream)
		}
		if !reflect.DeepEqual(upstream, originalUpstream) {
			t.Fatal("readout callback changed caller-owned upstream")
		}
		lifCoupledRowsBitsEqual(t, "trace outputs after readout reverse", trace.Outputs(), beforeOutputs)
		lifCoupledRowsBitsEqual(t, "trace spikes after readout reverse", trace.Spikes(), beforeSpikes)
		lifCoupledRowsBitsEqual(t, "trace voltages after readout reverse", trace.Voltages(), beforeVoltages)
		lifCoupledBitsEqual(t, "trace final voltage after readout reverse", trace.FinalVoltage(), beforeFinalVoltage)
		if !reflect.DeepEqual(trace.parameters, beforeParameters) {
			t.Fatal("readout reverse changed trace parameters")
		}
	}
	if sequence.reverses[0] == sequence.reverses[1] {
		t.Fatal("repeated Backward reused the readout reverse session")
	}

	for _, mode := range []string{"shape", "nonfinite", "error", "cancel"} {
		t.Run("failure_"+mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failureSequence := &coupledReadoutTestSequence{mode: mode, cancel: cancel}
			failureTrace, err := m.ForwardCoupled(ctx, p, initial, inputs, failureSequence)
			if err != nil {
				t.Fatal(err)
			}
			failureOutputs := failureTrace.Outputs()
			failureSpikes := failureTrace.Spikes()
			failureVoltages := failureTrace.Voltages()
			failureFinalVoltage := failureTrace.FinalVoltage()
			failureParameters := lifCoupledCloneParameters(failureTrace.parameters)
			got, err := m.Backward(ctx, failureTrace, upstream, 0)
			if err == nil || !lifCoupledGradientIsZero(got) {
				t.Fatalf("gradient = %#v, error = %v, want zero gradient and an error", got, err)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation error = %v, want context.Canceled", err)
			}
			lifCoupledRowsBitsEqual(t, "failed trace outputs", failureTrace.Outputs(), failureOutputs)
			lifCoupledRowsBitsEqual(t, "failed trace spikes", failureTrace.Spikes(), failureSpikes)
			lifCoupledRowsBitsEqual(t, "failed trace voltages", failureTrace.Voltages(), failureVoltages)
			lifCoupledBitsEqual(t, "failed trace final voltage", failureTrace.FinalVoltage(), failureFinalVoltage)
			if !reflect.DeepEqual(failureTrace.parameters, failureParameters) {
				t.Fatal("failed readout reverse changed trace parameters")
			}
		})
	}
}

func TestMixedCoupledReadout(t *testing.T) {
	m, p, initial, inputs, upstream := mixedCoupledSmoothFixture(t)
	scaled := lifMixedReadoutScaledRows(upstream)

	plain, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	want, err := m.Backward(context.Background(), plain, scaled, 0)
	if err != nil {
		t.Fatal(err)
	}

	originalUpstream := mixedCoupledCloneRows(upstream)
	sequence := &coupledReadoutTestSequence{}
	trace, err := m.ForwardCoupled(context.Background(), p, initial, inputs, sequence)
	if err != nil {
		t.Fatal(err)
	}
	beforeOutputs := trace.Outputs()
	beforeSpikes := trace.Spikes()
	beforeVoltages := trace.Voltages()
	beforeFinalVoltage := trace.FinalVoltage()
	beforeParameters := mixedCoupledCloneParameters(trace.parameters)

	for pass := 0; pass < 2; pass++ {
		got, err := m.Backward(context.Background(), trace, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		lifMixedReadoutCloseMixedGradient(t, got, want)
		if len(sequence.reverses) != pass+1 {
			t.Fatalf("reverse sessions = %d, want %d", len(sequence.reverses), pass+1)
		}
		reverse := sequence.reverses[pass]
		if !reflect.DeepEqual(reverse.observed, originalUpstream) {
			t.Fatalf("readout callback observed %v, want ascending direct upstream %v", reverse.observed, originalUpstream)
		}
		if !reflect.DeepEqual(upstream, originalUpstream) {
			t.Fatal("readout callback changed caller-owned upstream")
		}
		mixedCoupledRowsBitsEqual(t, "trace outputs after readout reverse", trace.Outputs(), beforeOutputs)
		mixedCoupledRowsBitsEqual(t, "trace spikes after readout reverse", trace.Spikes(), beforeSpikes)
		mixedCoupledRowsBitsEqual(t, "trace voltages after readout reverse", trace.Voltages(), beforeVoltages)
		mixedCoupledVectorBitsEqual(t, "trace final voltage after readout reverse", trace.FinalVoltage(), beforeFinalVoltage)
		if !reflect.DeepEqual(trace.parameters, beforeParameters) {
			t.Fatal("readout reverse changed trace parameters")
		}
	}
	if sequence.reverses[0] == sequence.reverses[1] {
		t.Fatal("repeated Backward reused the readout reverse session")
	}

	for _, mode := range []string{"shape", "nonfinite", "error", "cancel"} {
		t.Run("failure_"+mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failureSequence := &coupledReadoutTestSequence{mode: mode, cancel: cancel}
			failureTrace, err := m.ForwardCoupled(ctx, p, initial, inputs, failureSequence)
			if err != nil {
				t.Fatal(err)
			}
			failureOutputs := failureTrace.Outputs()
			failureSpikes := failureTrace.Spikes()
			failureVoltages := failureTrace.Voltages()
			failureFinalVoltage := failureTrace.FinalVoltage()
			failureParameters := mixedCoupledCloneParameters(failureTrace.parameters)
			got, err := m.Backward(ctx, failureTrace, upstream, 0)
			if err == nil || !mixedCoupledGradientIsZero(got) {
				t.Fatalf("gradient = %#v, error = %v, want zero gradient and an error", got, err)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation error = %v, want context.Canceled", err)
			}
			mixedCoupledRowsBitsEqual(t, "failed trace outputs", failureTrace.Outputs(), failureOutputs)
			mixedCoupledRowsBitsEqual(t, "failed trace spikes", failureTrace.Spikes(), failureSpikes)
			mixedCoupledRowsBitsEqual(t, "failed trace voltages", failureTrace.Voltages(), failureVoltages)
			mixedCoupledVectorBitsEqual(t, "failed trace final voltage", failureTrace.FinalVoltage(), failureFinalVoltage)
			if !reflect.DeepEqual(failureTrace.parameters, failureParameters) {
				t.Fatal("failed readout reverse changed trace parameters")
			}
		})
	}
}
