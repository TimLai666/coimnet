package signal

import (
	"context"
	"errors"
	"math"
	"math/big"
	"reflect"
	"testing"
)

func pulseConfig(t *testing.T) PulseAlignConfig {
	t.Helper()
	clock, err := NewClock(testVersion(), TimeUnitModelStep, 1)
	if err != nil {
		t.Fatal(err)
	}
	return PulseAlignConfig{SchemaVersion: testVersion(), Clock: clock, SourceUnit: TimeUnitMilliseconds, SourceTicks: 2, SimulationSteps: 1, OutputStreamID: "aligned-pulses", MaxBufferedSignals: 32, MaxValues: 1000}
}
func pulse(t *testing.T, seq uint64, at int64, value float64) Signal {
	t.Helper()
	spec := sample(t, seq, at, value).Spec()
	spec.Kind = KindPulse
	s, err := NewSignal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestPulseAlignCeilingPreservesEveryEvent(t *testing.T) {
	cfg := pulseConfig(t)
	input := []Signal{pulse(t, 1, 0, 2), pulse(t, 2, 1, 3), pulse(t, 4, 2, 7), pulse(t, 3, 2, 5), pulse(t, 5, 3, 11)}
	r, err := NewPulseAligner(cfg)
	if err != nil {
		t.Fatal(err)
	}
	head, err := r.Push(context.Background(), input, testTimestamp(3))
	if err != nil {
		t.Fatal(err)
	}
	if len(head) != 4 {
		t.Fatalf("head has %d events", len(head))
	}
	tail, err := r.Finish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := append(head, tail...)
	wantTimes := []int64{0, 1, 1, 1, 2}
	wantValues := []float64{2, 3, 5, 7, 11}
	if len(got) != 5 || !reflect.DeepEqual(sampleValues(got), wantValues) {
		t.Fatalf("pulse loss/duplication: %v", sampleValues(got))
	}
	for i, s := range got {
		if s.StartTime() != (Timestamp{Value: wantTimes[i], Unit: TimeUnitModelStep}) || s.SourceSequence() != uint64(i+1) || s.Kind() != KindPulse || s.Duration() != 0 || s.StreamID() != cfg.OutputStreamID {
			t.Fatalf("event %d: %+v", i, s.Spec())
		}
	}
	if _, err := NewObservation(testVersion(), "exp-1", cfg.OutputStreamID, got); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Finish(context.Background()); err == nil {
		t.Fatal("Finish twice accepted")
	}
	if _, err := r.Push(context.Background(), nil, testTimestamp(4)); err == nil {
		t.Fatal("push after finish accepted")
	}
}
func TestPulseAlignAllChunkPartitions(t *testing.T) {
	cfg := pulseConfig(t)
	input := []Signal{pulse(t, 1, 0, 2), pulse(t, 2, 1, 3), pulse(t, 4, 2, 7), pulse(t, 3, 2, 5), pulse(t, 5, 3, 11)}
	for mask := 0; mask < 16; mask++ {
		r, err := NewPulseAligner(cfg)
		if err != nil {
			t.Fatal(err)
		}
		got := []Signal{}
		start := 0
		for i := range input {
			if i == len(input)-1 || mask&(1<<i) != 0 {
				out, err := r.Push(context.Background(), input[start:i+1], input[i].StartTime())
				if err != nil {
					t.Fatalf("split %d: %v", mask, err)
				}
				got = append(got, out...)
				start = i + 1
			}
		}
		tail, err := r.Finish(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, tail...)
		if !reflect.DeepEqual(sampleValues(got), []float64{2, 3, 5, 7, 11}) {
			t.Fatalf("split %d: %v", mask, sampleValues(got))
		}
	}
}
func TestPulseAlignHoldsWholeDestinationStep(t *testing.T) {
	r, err := NewPulseAligner(pulseConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Push(context.Background(), []Signal{pulse(t, 1, 1, 3)}, testTimestamp(1))
	if err != nil || len(out) != 0 {
		t.Fatalf("premature: %v %v", out, err)
	}
	out, err = r.Push(context.Background(), nil, testTimestamp(2))
	if err != nil || len(out) != 0 {
		t.Fatalf("closed exact watermark: %v %v", out, err)
	}
	out, err = r.Push(context.Background(), []Signal{pulse(t, 2, 2, 5)}, testTimestamp(3))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sampleValues(out), []float64{3, 5}) || out[0].StartTime() != out[1].StartTime() {
		t.Fatalf("split destination bucket: %v", out)
	}
	out, err = r.Push(context.Background(), nil, testTimestamp(100))
	if err != nil || len(out) != 0 {
		t.Fatalf("held pulse repeated: %v %v", out, err)
	}
}
func TestPulseAlignExactRationalAndLimits(t *testing.T) {
	for _, pair := range [][2]int64{{441, 10}, {2, 3}, {math.MaxInt64, math.MaxInt64}, {1, 1}} {
		cfg := pulseConfig(t)
		cfg.SourceTicks = pair[0]
		cfg.SimulationSteps = pair[1]
		for _, at := range []int64{0, 1, 44, 441, 1 << 53, math.MaxInt64} {
			product := new(big.Int).Mul(big.NewInt(at), big.NewInt(pair[1]))
			product.Add(product, big.NewInt(pair[0]-1))
			want := new(big.Int).Quo(product, big.NewInt(pair[0]))
			r, err := NewPulseAligner(cfg)
			if err != nil {
				t.Fatal(err)
			}
			out, err := r.Push(context.Background(), []Signal{pulse(t, 1, at, 3)}, testTimestamp(at))
			if !want.IsInt64() {
				if err == nil {
					t.Fatal("step overflow accepted")
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			tail, err := r.Finish(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, tail...)
			if len(out) != 1 || out[0].StartTime().Value != want.Int64() {
				t.Fatalf("ratio %v source %d = %v want %v", pair, at, out, want)
			}
		}
	}
	cfg := pulseConfig(t)
	clock, err := NewClock(testVersion(), TimeUnitModelStep, math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Clock = clock
	r, err := NewPulseAligner(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Push(context.Background(), []Signal{pulse(t, 1, 3, 3)}, testTimestamp(3)); err == nil {
		t.Fatal("output timestamp overflow accepted")
	}
}
func TestPulseAlignErrorsAreAtomic(t *testing.T) {
	cfg := pulseConfig(t)
	r, err := NewPulseAligner(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a, b := pulse(t, 10, 0, 2), pulse(t, 11, 1, 3)
	if _, err := r.Push(context.Background(), []Signal{a}, testTimestamp(1)); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	changed := b.Spec()
	changed.Channel = "different"
	wrong, err := NewSignal(changed)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ctx   context.Context
		input []Signal
		mark  Timestamp
	}{
		{nil, []Signal{b}, testTimestamp(2)}, {canceled, []Signal{b}, testTimestamp(2)},
		{context.Background(), []Signal{pulse(t, 10, 1, 7)}, testTimestamp(2)},
		{context.Background(), []Signal{b, b}, testTimestamp(2)},
		{context.Background(), []Signal{pulse(t, 12, 0, 7)}, testTimestamp(2)},
		{context.Background(), []Signal{pulse(t, 12, 2, 7), b}, testTimestamp(3)},
		{context.Background(), []Signal{wrong}, testTimestamp(2)},
		{context.Background(), []Signal{b}, testTimestamp(0)},
		{context.Background(), nil, Timestamp{Value: 2, Unit: TimeUnitSeconds}},
	} {
		if out, err := r.Push(tc.ctx, tc.input, tc.mark); err == nil || len(out) != 0 {
			t.Fatalf("accepted/partially applied bad call: %v %v", out, err)
		}
	}
	if out, err := r.Push(context.Background(), []Signal{b}, testTimestamp(2)); err != nil || len(out) != 0 {
		t.Fatalf("retry: %v %v", out, err)
	}
	if out, err := r.Finish(canceled); !errors.Is(err, context.Canceled) || len(out) != 0 {
		t.Fatalf("canceled finish: %v %v", out, err)
	}
	out, err := r.Finish(context.Background())
	if err != nil || !reflect.DeepEqual(sampleValues(out), []float64{3}) {
		t.Fatalf("failed calls changed state: %v %v", out, err)
	}
}
func TestPulseAlignRejectsConfigurationSemanticsAndCapacity(t *testing.T) {
	cfg := pulseConfig(t)
	for _, change := range []func(*PulseAlignConfig){func(c *PulseAlignConfig) { c.SourceTicks = 0 }, func(c *PulseAlignConfig) { c.SimulationSteps = -1 }, func(c *PulseAlignConfig) { c.OutputStreamID = " " }, func(c *PulseAlignConfig) { c.SourceUnit = "sample" }, func(c *PulseAlignConfig) { c.MaxBufferedSignals = 0 }, func(c *PulseAlignConfig) { c.MaxValues = 0 }, func(c *PulseAlignConfig) { c.SchemaVersion.Major = 2 }} {
		bad := cfg
		change(&bad)
		if _, err := NewPulseAligner(bad); err == nil {
			t.Fatalf("bad config accepted: %+v", bad)
		}
	}
	for _, change := range []func(*SignalSpec){func(s *SignalSpec) { s.Kind = KindContinuous }, func(s *SignalSpec) { s.Duration = 1 }} {
		spec := pulse(t, 1, 1, 3).Spec()
		change(&spec)
		s, err := NewSignal(spec)
		if err != nil {
			t.Fatal(err)
		}
		r, err := NewPulseAligner(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Push(context.Background(), []Signal{s}, testTimestamp(1)); err == nil {
			t.Fatal("unsupported semantics accepted")
		}
	}
	for _, limitValues := range []bool{false, true} {
		c := cfg
		if limitValues {
			c.MaxValues = 2
		} else {
			c.MaxBufferedSignals = 1
		}
		r, err := NewPulseAligner(c)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Push(context.Background(), []Signal{pulse(t, 1, 1, 3), pulse(t, 2, 1, 5)}, testTimestamp(1)); err == nil {
			t.Fatal("capacity ignored")
		}
	}
	var zero PulseAligner
	if _, err := zero.Finish(context.Background()); err == nil {
		t.Fatal("zero accepted")
	}
	var missing *PulseAligner
	if _, err := missing.Push(context.Background(), nil, testTimestamp(0)); err == nil {
		t.Fatal("nil accepted")
	}
	r, err := NewPulseAligner(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Finish(context.Background())
	if err != nil || out == nil || len(out) != 0 {
		t.Fatalf("empty result %v %v", out, err)
	}
}
