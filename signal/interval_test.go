package signal

import (
	"context"
	"errors"
	"math"
	"math/big"
	"reflect"
	"testing"
)

func intervalConfig(t *testing.T) IntervalResampleConfig {
	t.Helper()
	clock, err := NewClock(testVersion(), TimeUnitModelStep, 1)
	if err != nil {
		t.Fatal(err)
	}
	return IntervalResampleConfig{SchemaVersion: testVersion(), Clock: clock, SourceUnit: TimeUnitMilliseconds, SourceTicks: 1, SimulationSteps: 2, StartStep: 0, EndStep: 24, OutputStreamID: "aligned-intervals", Limits: ResampleLimits{MaxBufferedSignals: 32, MaxOutputSignals: 32, MaxValues: 1000}}
}
func interval(t *testing.T, seq uint64, start, duration int64, value float64) Signal {
	t.Helper()
	spec := sample(t, seq, start, value).Spec()
	spec.Duration = duration
	s, err := NewSignal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestIntervalOverlapGapsAndAllPartitions(t *testing.T) {
	input := []Signal{interval(t, 10, 1, 7, 2), interval(t, 30, 2, 1, 6), interval(t, 20, 2, 3, 4), interval(t, 40, 10, 1, 9)}
	var reference []Signal
	for mask := 0; mask < 8; mask++ {
		c := intervalConfig(t)
		r, err := NewIntervalResampler(c)
		if err != nil {
			t.Fatal(err)
		}
		got := []Signal{}
		start := 0
		for i := range input {
			if i == len(input)-1 || mask&(1<<i) != 0 {
				out, err := r.Push(context.Background(), input[start:i+1], input[i].StartTime())
				if err != nil {
					t.Fatalf("partition %d: %v", mask, err)
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
		wantSteps := []int64{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 20, 21}
		wantValues := []float64{2, 2, 6, 6, 4, 4, 4, 4, 2, 2, 2, 2, 2, 2, 9, 9}
		if !reflect.DeepEqual(sampleValues(got), wantValues) {
			t.Fatalf("partition %d: %v", mask, sampleValues(got))
		}
		for i, s := range got {
			if s.StartTime() != (Timestamp{Value: wantSteps[i], Unit: TimeUnitModelStep}) || s.SourceSequence() != uint64(wantSteps[i]) || s.Duration() != 0 || s.StreamID() != c.OutputStreamID {
				t.Fatalf("output %d: %+v", i, s.Spec())
			}
		}
		if _, err := NewObservation(testVersion(), "exp-1", c.OutputStreamID, got); err != nil {
			t.Fatal(err)
		}
		if mask == 0 {
			reference = got
		} else if !reflect.DeepEqual(got, reference) {
			t.Fatalf("partition %d differs", mask)
		}
	}
}
func TestIntervalWatermarkAndExpiry(t *testing.T) {
	c := intervalConfig(t)
	c.EndStep = 12
	r, err := NewIntervalResampler(c)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Push(context.Background(), []Signal{interval(t, 10, 1, 3, 2), interval(t, 30, 2, 1, 6)}, testTimestamp(2))
	if err != nil || !reflect.DeepEqual(sampleValues(out), []float64{2, 2}) {
		t.Fatalf("head %v %v", out, err)
	}
	out, err = r.Push(context.Background(), []Signal{interval(t, 20, 2, 3, 4)}, testTimestamp(2))
	if err != nil || len(out) != 0 {
		t.Fatalf("open group %v %v", out, err)
	}
	out, err = r.Push(context.Background(), nil, testTimestamp(5))
	if err != nil || !reflect.DeepEqual(sampleValues(out), []float64{6, 6, 4, 4, 4, 4}) {
		t.Fatalf("closed %v %v", out, err)
	}
	out, err = r.Finish(context.Background())
	if err != nil || len(out) != 0 {
		t.Fatalf("expired tail %v %v", out, err)
	}
	if _, err := r.Finish(context.Background()); err == nil {
		t.Fatal("finish twice")
	}
	if _, err := r.Push(context.Background(), nil, testTimestamp(6)); err == nil {
		t.Fatal("push after finish")
	}
}
func TestIntervalExactRationalReference(t *testing.T) {
	for _, pair := range [][2]int64{{441, 10}, {2, 3}, {math.MaxInt64, math.MaxInt64}} {
		c := intervalConfig(t)
		c.SourceTicks = pair[0]
		c.SimulationSteps = pair[1]
		c.EndStep = 8
		if pair[0] == math.MaxInt64 {
			c.StartStep = math.MaxInt64 - 8
			c.EndStep = math.MaxInt64
		}
		starts := []int64{1, 3, 5}
		durations := []int64{6, 1, 2}
		if c.StartStep > 0 {
			starts = []int64{math.MaxInt64 - 7, math.MaxInt64 - 5, math.MaxInt64 - 3}
		}
		input := []Signal{}
		for i, at := range starts {
			input = append(input, interval(t, uint64(i+1), at, durations[i], float64(i+1)))
		}
		r, err := NewIntervalResampler(c)
		if err != nil {
			t.Fatal(err)
		}
		head, err := r.Push(context.Background(), input, input[len(input)-1].StartTime())
		if err != nil {
			t.Fatal(err)
		}
		tail, err := r.Finish(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		got := append(head, tail...)
		j := 0
		for step := c.StartStep; step < c.EndStep; step++ {
			position := new(big.Int).Mul(big.NewInt(step), big.NewInt(pair[0]))
			winner := -1
			for i, at := range starts {
				begin := new(big.Int).Mul(big.NewInt(at), big.NewInt(pair[1]))
				end := new(big.Int).Mul(big.NewInt(at+durations[i]), big.NewInt(pair[1]))
				if position.Cmp(begin) >= 0 && position.Cmp(end) < 0 {
					winner = i
				}
			}
			if winner < 0 {
				continue
			}
			if j >= len(got) || got[j].StartTime().Value != step || got[j].Values()[0] != float64(winner+1) {
				t.Fatalf("ratio %v step %d output %v", pair, step, got)
			}
			j++
		}
		if j != len(got) {
			t.Fatalf("extra output: %v", got)
		}
	}
}
func TestIntervalFailuresAreAtomic(t *testing.T) {
	c := intervalConfig(t)
	c.EndStep = 12
	r, err := NewIntervalResampler(c)
	if err != nil {
		t.Fatal(err)
	}
	a := interval(t, 10, 0, 4, 2)
	b := interval(t, 20, 2, 1, 4)
	if _, err := r.Push(context.Background(), []Signal{a}, testTimestamp(1)); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	wrongSpec := b.Spec()
	wrongSpec.Channel = "other"
	wrong, err := NewSignal(wrongSpec)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ctx   context.Context
		input []Signal
		mark  Timestamp
	}{
		{nil, []Signal{b}, testTimestamp(2)}, {canceled, []Signal{b}, testTimestamp(2)},
		{context.Background(), []Signal{b, b}, testTimestamp(2)},
		{context.Background(), []Signal{interval(t, 9, 2, 1, 9)}, testTimestamp(2)},
		{context.Background(), []Signal{interval(t, 20, 0, 1, 9)}, testTimestamp(2)},
		{context.Background(), []Signal{b, interval(t, 30, 1, 1, 9)}, testTimestamp(2)},
		{context.Background(), []Signal{wrong}, testTimestamp(2)},
		{context.Background(), []Signal{b}, testTimestamp(1)},
		{context.Background(), nil, testTimestamp(0)},
		{context.Background(), nil, Timestamp{Value: 2, Unit: TimeUnitSeconds}},
		{context.Background(), []Signal{sample(t, 20, 2, 9)}, testTimestamp(2)},
		{context.Background(), []Signal{pulse(t, 20, 2, 9)}, testTimestamp(2)},
	} {
		if out, err := r.Push(tc.ctx, tc.input, tc.mark); err == nil || out != nil {
			t.Fatalf("invalid accepted %v %v", out, err)
		}
	}
	out, err := r.Push(context.Background(), []Signal{b}, testTimestamp(3))
	if err != nil || !reflect.DeepEqual(sampleValues(out), []float64{2, 2, 4, 4}) {
		t.Fatalf("retry %v %v", out, err)
	}
	if out, err := r.Finish(canceled); !errors.Is(err, context.Canceled) || out != nil {
		t.Fatalf("canceled finish %v %v", out, err)
	}
	out, err = r.Finish(context.Background())
	if err != nil || !reflect.DeepEqual(sampleValues(out), []float64{2, 2}) {
		t.Fatalf("finish retry %v %v", out, err)
	}
}
func TestIntervalCapacityExpiryAndFinalizedSequence(t *testing.T) {
	for _, kind := range []string{"buffer", "values"} {
		c := intervalConfig(t)
		c.EndStep = 8
		c.Limits.MaxOutputSignals = 8
		if kind == "buffer" {
			c.Limits.MaxBufferedSignals = 1
		} else {
			c.Limits.MaxValues = 10
		}
		r, err := NewIntervalResampler(c)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Push(context.Background(), []Signal{interval(t, 10, 0, 1, 2)}, testTimestamp(0)); err != nil {
			t.Fatal(err)
		}
		if out, err := r.Push(context.Background(), []Signal{interval(t, 20, 1, 1, 4)}, testTimestamp(1)); err == nil || out != nil {
			t.Fatalf("ignored %s", kind)
		}
		if _, err := r.Push(context.Background(), nil, testTimestamp(2)); err != nil {
			t.Fatal(err)
		}
		if out, err := r.Push(context.Background(), []Signal{interval(t, 9, 2, 1, 6)}, testTimestamp(2)); err == nil || out != nil {
			t.Fatal("expired sequence forgotten")
		}
		if _, err := r.Push(context.Background(), []Signal{interval(t, 20, 2, 1, 4)}, testTimestamp(3)); err != nil {
			t.Fatal(err)
		}
	}
}
func TestIntervalConfigurationEmptyAndOverflow(t *testing.T) {
	cfg := intervalConfig(t)
	for _, change := range []func(*IntervalResampleConfig){
		func(c *IntervalResampleConfig) { c.SchemaVersion.Major = 2 }, func(c *IntervalResampleConfig) { c.Clock = Clock{} },
		func(c *IntervalResampleConfig) { c.SourceTicks = 0 }, func(c *IntervalResampleConfig) { c.SimulationSteps = -1 },
		func(c *IntervalResampleConfig) { c.SourceUnit = "unknown" }, func(c *IntervalResampleConfig) { c.OutputStreamID = " " },
		func(c *IntervalResampleConfig) { c.StartStep = -1 }, func(c *IntervalResampleConfig) { c.EndStep = -1 },
		func(c *IntervalResampleConfig) { c.Limits.MaxBufferedSignals = 0 }, func(c *IntervalResampleConfig) { c.Limits.MaxOutputSignals = 1 }, func(c *IntervalResampleConfig) { c.Limits.MaxValues = 0 },
		func(c *IntervalResampleConfig) { c.SourceTicks = math.MaxInt64; c.SimulationSteps = 1 },
	} {
		c := cfg
		change(&c)
		if _, err := NewIntervalResampler(c); err == nil {
			t.Fatalf("bad config %+v", c)
		}
	}
	c := cfg
	c.StartStep = math.MaxInt64
	c.EndStep = math.MaxInt64
	r, err := NewIntervalResampler(c)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Finish(context.Background())
	if err != nil || out == nil || len(out) != 0 {
		t.Fatalf("empty %v %v", out, err)
	}
	c = cfg
	c.SourceTicks = 1
	c.SimulationSteps = 1
	c.StartStep = 1
	c.EndStep = 2
	c.Clock, err = NewClock(testVersion(), TimeUnitModelStep, math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	r, err = NewIntervalResampler(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Push(context.Background(), []Signal{interval(t, 1, 1, 1, 3)}, testTimestamp(1)); err != nil {
		t.Fatal(err)
	}
	out, err = r.Finish(context.Background())
	if err != nil || len(out) != 1 || out[0].StartTime().Value != math.MaxInt64 {
		t.Fatalf("exclusive end checked %v %v", out, err)
	}
	var zero IntervalResampler
	var missing *IntervalResampler
	if _, err := zero.Finish(context.Background()); err == nil {
		t.Fatal("zero accepted")
	}
	if _, err := missing.Push(context.Background(), nil, testTimestamp(0)); err == nil {
		t.Fatal("nil accepted")
	}
}
