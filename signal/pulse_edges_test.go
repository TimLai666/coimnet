package signal

import (
	"context"
	"math"
	"reflect"
	"testing"
)

func TestPulseAlignRetainsNonconsecutiveSequences(t *testing.T) {
	r, err := NewPulseAligner(pulseConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	input := []Signal{pulse(t, 10, 0, 2), pulse(t, 20, 1, 3), pulse(t, 42, 2, 7), pulse(t, 41, 2, 5), pulse(t, 99, 3, 11)}
	head, err := r.Push(context.Background(), input, testTimestamp(3))
	if err != nil {
		t.Fatal(err)
	}
	tail, err := r.Finish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := append(head, tail...)
	want := []uint64{10, 20, 41, 42, 99}
	if len(got) != len(want) {
		t.Fatalf("got %d events", len(got))
	}
	for i, s := range got {
		if s.SourceSequence() != want[i] {
			t.Fatalf("sequence %d = %d", i, s.SourceSequence())
		}
	}
}

func TestPulseAlignOverflowAndCapacityRetry(t *testing.T) {
	for _, capacity := range []string{"events", "values", "overflow"} {
		t.Run(capacity, func(t *testing.T) {
			c := pulseConfig(t)
			switch capacity {
			case "events":
				c.MaxBufferedSignals = 1
			case "values":
				c.MaxValues = 3
			case "overflow":
				c.SourceTicks = math.MaxInt64 - 2
				c.SimulationSteps = math.MaxInt64 - 1
			}
			r, err := NewPulseAligner(c)
			if err != nil {
				t.Fatal(err)
			}
			a := pulse(t, 10, 0, 2)
			if out, err := r.Push(context.Background(), []Signal{a}, testTimestamp(0)); err != nil || len(out) != 0 {
				t.Fatalf("initial: %v %v", out, err)
			}
			at := int64(1)
			if capacity == "overflow" {
				at = math.MaxInt64 - 1
			}
			if out, err := r.Push(context.Background(), []Signal{pulse(t, 20, at, 3)}, testTimestamp(at)); err == nil || out != nil {
				t.Fatalf("bad input: %v %v", out, err)
			}
			out, err := r.Push(context.Background(), nil, testTimestamp(1))
			if err != nil || !reflect.DeepEqual(sampleValues(out), []float64{2}) {
				t.Fatalf("retry: %v %v", out, err)
			}
			if out, err := r.Push(context.Background(), []Signal{pulse(t, 9, 1, 3)}, testTimestamp(1)); err == nil || out != nil {
				t.Fatalf("old sequence accepted: %v %v", out, err)
			}
			if _, err := r.Push(context.Background(), []Signal{pulse(t, 20, 1, 3)}, testTimestamp(1)); err != nil {
				t.Fatal(err)
			}
			out, err = r.Finish(context.Background())
			if err != nil || !reflect.DeepEqual(sampleValues(out), []float64{3}) {
				t.Fatalf("finish after retry: %v %v", out, err)
			}
		})
	}
}
