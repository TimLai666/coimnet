package signal

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
)

func samplingConfig(t *testing.T, mode ResamplingMode) ResampleConfig {
	t.Helper()
	c, err := NewClock(testVersion(), TimeUnitModelStep, 1)
	if err != nil {
		t.Fatal(err)
	}
	return ResampleConfig{SchemaVersion: testVersion(), Mode: mode, Clock: c,
		SourceUnit: TimeUnitMilliseconds, SourceTicks: 1, SimulationSteps: 2,
		StartStep: 0, EndStep: 7, OutputStreamID: "resampled", Tail: HoldLast,
		Limits: ResampleLimits{MaxBufferedSignals: 32, MaxOutputSignals: 100, MaxValues: 1000}}
}
func sample(t *testing.T, seq uint64, at int64, value float64) Signal {
	t.Helper()
	s := testSignalSpec(KindContinuous, seq, at)
	s.Duration = 0
	s.Shape = []int{1}
	s.Values = []float64{value}
	out, err := NewSignal(s)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func sampleValues(s []Signal) []float64 {
	v := make([]float64, len(s))
	for i := range s {
		v[i] = s[i].Values()[0]
	}
	return v
}
func TestResampleHandCalculatedModes(t *testing.T) {
	input := []Signal{sample(t, 1, 0, 2), sample(t, 2, 2, 6)}
	for _, tc := range []struct {
		mode ResamplingMode
		want []float64
	}{
		{CausalHold, []float64{2, 2, 2, 2, 6, 6, 6}},
		{OfflineLinear, []float64{2, 3, 4, 5, 6, 6, 6}},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			cfg := samplingConfig(t, tc.mode)
			got, err := ResampleOffline(context.Background(), cfg, input)
			if err != nil {
				t.Fatal(err)
			}
			if got.Mode != tc.mode || !reflect.DeepEqual(sampleValues(got.Signals), tc.want) {
				t.Fatalf("got %+v", got)
			}
			for i, s := range got.Signals {
				if s.StartTime() != (Timestamp{Value: int64(i), Unit: TimeUnitModelStep}) || s.SourceSequence() != uint64(i) || s.StreamID() != cfg.OutputStreamID || s.Duration() != 0 || s.Channel() != "vision" {
					t.Fatalf("lost output contract: %+v", s.Spec())
				}
			}
		})
	}
}
func TestResampleChunkBoundaryAndOverwrite(t *testing.T) {
	cfg := samplingConfig(t, CausalHold)
	a, b, c, d := sample(t, 1, 0, 2), sample(t, 3, 1, 7), sample(t, 2, 1, 5), sample(t, 4, 2, 9)
	want, err := ResampleOffline(context.Background(), cfg, []Signal{a, b, c, d})
	if err != nil {
		t.Fatal(err)
	}
	// Same-time sequence 3 arrives before sequence 2 in different chunks.
	r, err := NewStreamingResampler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := []Signal{}
	for _, ch := range []struct {
		s    []Signal
		mark int64
	}{
		{[]Signal{a, b}, 1}, {nil, 1}, {[]Signal{c}, 1}, {[]Signal{d}, 2},
	} {
		batch, err := r.Push(context.Background(), ch.s, testTimestamp(ch.mark))
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, batch.Signals...)
	}
	tail, err := r.Finish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, tail.Signals...)
	if !reflect.DeepEqual(got, want.Signals) || !reflect.DeepEqual(sampleValues(got), []float64{2, 2, 7, 7, 9, 9, 9}) {
		t.Fatalf("chunk result %v", sampleValues(got))
	}
	if _, err := r.Finish(context.Background()); err == nil {
		t.Fatal("second Finish accepted")
	}
	if _, err := r.Push(context.Background(), nil, testTimestamp(3)); err == nil {
		t.Fatal("push after Finish accepted")
	}
}
func TestResampleCausalPrefixIgnoresFutureValues(t *testing.T) {
	cfg := samplingConfig(t, CausalHold)
	for _, future := range []float64{-1e100, 1e100} {
		r, err := NewStreamingResampler(cfg)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.Push(context.Background(), []Signal{sample(t, 1, 0, 2), sample(t, 2, 2, future)}, testTimestamp(2))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(sampleValues(got.Signals), []float64{2, 2, 2, 2}) {
			t.Fatalf("future leaked: %v", sampleValues(got.Signals))
		}
	}
	cfg.Mode = OfflineLinear
	if _, err := NewStreamingResampler(cfg); err == nil {
		t.Fatal("noncausal mode accepted by realtime constructor")
	}
}
func TestResampleEmptyLeadingGapAndRationalBoundary(t *testing.T) {
	cfg := samplingConfig(t, CausalHold)
	empty, err := ResampleOffline(context.Background(), cfg, nil)
	if err != nil || empty.Signals == nil || len(empty.Signals) != 0 {
		t.Fatalf("empty = %+v %v", empty, err)
	}
	cfg.SourceTicks = 441
	cfg.SimulationSteps = 10
	cfg.EndStep = 12
	got, err := ResampleOffline(context.Background(), cfg, []Signal{sample(t, 1, 441, 3)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Signals) != 2 || got.Signals[0].SourceSequence() != 10 || got.Signals[1].SourceSequence() != 11 {
		t.Fatalf("rational boundary = %+v", got)
	}
	// Preserve event selection above the exact-integer range of float64.
	cfg.SourceTicks = 1
	cfg.SimulationSteps = 1
	cfg.StartStep = 1 << 53
	cfg.EndStep = cfg.StartStep + 3
	got, err = ResampleOffline(context.Background(), cfg, []Signal{sample(t, 1, 1<<53, 2), sample(t, 2, (1<<53)+1, 4)})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sampleValues(got.Signals), []float64{2, 4, 4}) {
		t.Fatalf("rounded large timestamp: %v", sampleValues(got.Signals))
	}
}
func TestResampleFailureIsAtomic(t *testing.T) {
	cfg := samplingConfig(t, CausalHold)
	r, err := NewStreamingResampler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a, b := sample(t, 1, 0, 2), sample(t, 2, 1, 4)
	if _, err = r.Push(context.Background(), []Signal{a}, testTimestamp(0)); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx   context.Context
		input []Signal
		mark  Timestamp
	}{
		{context.Background(), []Signal{a}, testTimestamp(1)},
		{context.Background(), []Signal{b, a}, testTimestamp(2)},
		{context.Background(), []Signal{b}, testTimestamp(0)},
		{canceled, []Signal{b}, testTimestamp(1)},
		{nil, []Signal{b}, testTimestamp(1)},
	} {
		if batch, err := r.Push(tc.ctx, tc.input, tc.mark); err == nil || len(batch.Signals) != 0 {
			t.Fatalf("failed push returned output/error: %+v %v", batch, err)
		}
	}
	got, err := r.Push(context.Background(), []Signal{b}, testTimestamp(1))
	if err != nil {
		t.Fatal(err)
	}
	tail, err := r.Finish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want, err := ResampleOffline(context.Background(), cfg, []Signal{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(append(got.Signals, tail.Signals...), want.Signals) {
		t.Fatal("failed calls changed state")
	}
	if _, err := ResampleOffline(canceled, cfg, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
func TestResampleRejectsUnsupportedSemanticsAndLimits(t *testing.T) {
	cfg := samplingConfig(t, CausalHold)
	for _, change := range []func(*ResampleConfig){
		func(c *ResampleConfig) { c.SourceTicks = 0 }, func(c *ResampleConfig) { c.SimulationSteps = 0 },
		func(c *ResampleConfig) { c.SourceTicks = math.MaxInt64 }, func(c *ResampleConfig) { c.EndStep = -1 },
		func(c *ResampleConfig) { c.Mode = "future" }, func(c *ResampleConfig) { c.Tail = "zero" },
		func(c *ResampleConfig) { c.Limits.MaxOutputSignals = 1 }, func(c *ResampleConfig) { c.Limits.MaxValues = 0 },
		func(c *ResampleConfig) { c.OutputStreamID = " " }, func(c *ResampleConfig) { c.SourceUnit = "sample" },
	} {
		bad := cfg
		change(&bad)
		if _, err := NewStreamingResampler(bad); err == nil {
			t.Fatalf("invalid config accepted: %+v", bad)
		}
	}
	for _, change := range []func(*SignalSpec){
		func(s *SignalSpec) { s.Duration = 1 }, func(s *SignalSpec) { s.Kind = KindPulse },
		func(s *SignalSpec) { s.StreamID = "different" }, func(s *SignalSpec) { s.Shape = []int{1, 1} },
		func(s *SignalSpec) { s.EncoderVersion.Major = 2 }, func(s *SignalSpec) { s.Unit = UnitVolt },
	} {
		bad := sample(t, 2, 1, 4).Spec()
		change(&bad)
		s, err := NewSignal(bad)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ResampleOffline(context.Background(), cfg, []Signal{sample(t, 1, 0, 2), s}); err == nil {
			t.Fatalf("unsupported/mixed sample accepted: %+v", bad)
		}
	}
	cfg.Limits.MaxBufferedSignals = 1
	if _, err := ResampleOffline(context.Background(), cfg, []Signal{sample(t, 1, 0, 2), sample(t, 2, 1, 4)}); err == nil {
		t.Fatal("buffer cap ignored")
	}
	cfg = samplingConfig(t, CausalHold)
	cfg.Limits.MaxValues = 2
	if _, err := ResampleOffline(context.Background(), cfg, []Signal{sample(t, 1, 0, 2)}); err == nil {
		t.Fatal("value cap ignored")
	}
}
