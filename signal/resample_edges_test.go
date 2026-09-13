package signal

import (
	"context"
	"math"
	"math/big"
	"reflect"
	"testing"
)

func TestResampleConstantRangeAndExtremeInterpolation(t *testing.T) {
	cfg := samplingConfig(t, OfflineLinear)
	cfg.SourceTicks = 1
	cfg.SimulationSteps = 100
	cfg.EndStep = 100
	input := make([]Signal, 2)
	for i := range input {
		s := sample(t, uint64(i+1), int64(i), 0.1).Spec()
		s.ValidRange = &ValueRange{Min: 0.1, Max: 0.1}
		var err error
		input[i], err = NewSignal(s)
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := ResampleOffline(context.Background(), cfg, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range got.Signals {
		if s.Values()[0] != 0.1 {
			t.Fatalf("constant signal changed: %v", s.Values())
		}
	}
	cfg = samplingConfig(t, OfflineLinear)
	cfg.EndStep = 3
	got, err = ResampleOffline(context.Background(), cfg, []Signal{sample(t, 1, 0, -math.MaxFloat64), sample(t, 2, 1, math.MaxFloat64)})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sampleValues(got.Signals), []float64{-math.MaxFloat64, 0, math.MaxFloat64}) {
		t.Fatalf("extreme interpolation: %v", sampleValues(got.Signals))
	}
}

func TestResampleExactSelectionAgainstBigRational(t *testing.T) {
	// Independent arbitrary-precision oracle; ratios include a 128-bit product.
	for _, ratio := range [][2]int64{{2, 3}, {441, 10}, {math.MaxInt64 - 2, math.MaxInt64 - 1}} {
		cfg := samplingConfig(t, CausalHold)
		cfg.SourceTicks = ratio[0]
		cfg.SimulationSteps = ratio[1]
		cfg.StartStep = 37
		cfg.EndStep = 45
		inputs := make([]Signal, 2200)
		for i := range inputs {
			inputs[i] = sample(t, uint64(i), int64(i), float64(i))
		}
		cfg.Limits = ResampleLimits{MaxBufferedSignals: 3000, MaxOutputSignals: 100, MaxValues: 10000}
		got, err := ResampleOffline(context.Background(), cfg, inputs)
		if err != nil {
			t.Fatal(err)
		}
		for j, s := range got.Signals {
			product := new(big.Int).Mul(big.NewInt(cfg.StartStep+int64(j)), big.NewInt(ratio[0]))
			want := new(big.Int).Quo(product, big.NewInt(ratio[1])).Int64()
			if s.Values()[0] != float64(want) {
				t.Fatalf("ratio %v step %d got %v want %d", ratio, cfg.StartStep+int64(j), s.Values(), want)
			}
		}
	}
}

func TestResampleEveryChunkSplitMatchesHandValues(t *testing.T) {
	cfg := samplingConfig(t, CausalHold)
	all := []Signal{sample(t, 1, 0, 2), sample(t, 3, 1, 7), sample(t, 2, 1, 5), sample(t, 4, 2, 9)}
	for mask := 0; mask < 1<<(len(all)-1); mask++ {
		r, err := NewStreamingResampler(cfg)
		if err != nil {
			t.Fatal(err)
		}
		got := []Signal{}
		start := 0
		for i := range all {
			if i == len(all)-1 || mask&(1<<i) != 0 {
				b, err := r.Push(context.Background(), all[start:i+1], all[i].StartTime())
				if err != nil {
					t.Fatalf("split %d: %v", mask, err)
				}
				got = append(got, b.Signals...)
				start = i + 1
			}
		}
		b, err := r.Finish(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, b.Signals...)
		if !reflect.DeepEqual(sampleValues(got), []float64{2, 2, 7, 7, 9, 9, 9}) {
			t.Fatalf("split %d = %v", mask, sampleValues(got))
		}
	}
}

func TestResampleFinalizedSequenceAndWatermarkRejection(t *testing.T) {
	cfg := samplingConfig(t, CausalHold)
	r, err := NewStreamingResampler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Push(context.Background(), []Signal{sample(t, 10, 0, 2)}, testTimestamp(1)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input []Signal
		mark  Timestamp
	}{
		{nil, testTimestamp(0)},
		{nil, Timestamp{Value: 2, Unit: TimeUnitSeconds}},
		{[]Signal{sample(t, 11, 0, 5)}, testTimestamp(2)},
		{[]Signal{sample(t, 10, 1, 5)}, testTimestamp(2)},
		{[]Signal{sample(t, 9, 1, 5)}, testTimestamp(2)},
	} {
		if _, err := r.Push(context.Background(), tc.input, tc.mark); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
	if _, err := r.Push(context.Background(), []Signal{sample(t, 11, 1, 5)}, testTimestamp(2)); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Finish(canceled); err == nil {
		t.Fatal("Finish ignored cancellation")
	}
	if _, err := r.Finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	var zero StreamingResampler
	if _, err := zero.Finish(context.Background()); err == nil {
		t.Fatal("zero accepted")
	}
	var missing *StreamingResampler
	if _, err := missing.Push(context.Background(), nil, testTimestamp(0)); err == nil {
		t.Fatal("nil accepted")
	}
}

func TestResampleMultipleRatesShareNeuralClock(t *testing.T) {
	a := samplingConfig(t, CausalHold)
	a.SourceTicks = 1
	a.SimulationSteps = 2
	b := a
	b.SourceUnit = TimeUnitMicroseconds
	b.SourceTicks = 1000
	first, err := ResampleOffline(context.Background(), a, []Signal{sample(t, 1, 0, 2), sample(t, 2, 1, 4)})
	if err != nil {
		t.Fatal(err)
	}
	other := []Signal{}
	for i, v := range []float64{3, 7} {
		s := sample(t, uint64(i+1), int64(i)*1000, v).Spec()
		s.Start.Unit = TimeUnitMicroseconds
		s.StreamID = "audio"
		s.Channel = "audio"
		sig, err := NewSignal(s)
		if err != nil {
			t.Fatal(err)
		}
		other = append(other, sig)
	}
	second, err := ResampleOffline(context.Background(), b, other)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sampleValues(first.Signals), []float64{2, 2, 4, 4, 4, 4, 4}) || !reflect.DeepEqual(sampleValues(second.Signals), []float64{3, 3, 7, 7, 7, 7, 7}) {
		t.Fatal("aligned values differ")
	}
	for i := range first.Signals {
		if first.Signals[i].StartTime() != second.Signals[i].StartTime() {
			t.Fatal("output clocks differ")
		}
	}
	values := first.Signals[0].Values()
	values[0] = 999
	if first.Signals[1].Values()[0] != 2 {
		t.Fatal("output aliases held values")
	}
}
