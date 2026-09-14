package signal

import (
	"context"
	"math/big"
	"math/rand"
	"reflect"
	"testing"
)

func TestIntervalRandomizedAgainstCoverageReference(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	for trial := 0; trial < 40; trial++ {
		c := intervalConfig(t)
		c.SourceTicks = int64(rng.Intn(8) + 1)
		c.SimulationSteps = int64(rng.Intn(5) + 1)
		input := []Signal{}
		start := int64(0)
		for i := 0; i < 12; i++ {
			start += int64(rng.Intn(3))
			input = append(input, interval(t, uint64(i*3+10), start, int64(rng.Intn(15)+1), float64(i)))
		}
		r, err := NewIntervalResampler(c)
		if err != nil {
			t.Fatal(err)
		}
		got := []Signal{}
		for _, s := range input {
			out, err := r.Push(context.Background(), []Signal{s}, s.StartTime())
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, out...)
		}
		tail, err := r.Finish(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, tail...)
		expected := []float64{}
		steps := []int64{}
		for step := c.StartStep; step < c.EndStep; step++ {
			at := new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(step), big.NewInt(c.SourceTicks)), big.NewInt(c.SimulationSteps))
			winner := -1
			for i, s := range input {
				if at.Cmp(new(big.Rat).SetInt64(s.StartTime().Value)) >= 0 && at.Cmp(new(big.Rat).SetInt64(s.StartTime().Value+s.Duration())) < 0 {
					winner = i
				}
			}
			if winner >= 0 {
				steps = append(steps, step)
				expected = append(expected, input[winner].Values()[0])
			}
		}
		if !reflect.DeepEqual(sampleValues(got), expected) {
			t.Fatalf("trial %d values %v want %v", trial, sampleValues(got), expected)
		}
		for i, s := range got {
			if s.StartTime().Value != steps[i] {
				t.Fatalf("trial %d step %d", trial, i)
			}
		}
	}
}

func TestIntervalMetadataOwnershipAndKinds(t *testing.T) {
	for _, kind := range []SignalKind{KindContinuous, KindActivity, KindModulation} {
		c := intervalConfig(t)
		c.EndStep = 4
		spec := interval(t, 9, 0, 2, 2).Spec()
		spec.Kind = kind
		spec.Shape = []int{2}
		spec.Values = []float64{2, 3}
		spec.ValidRange = &ValueRange{Min: 1, Max: 4}
		s, err := NewSignal(spec)
		if err != nil {
			t.Fatal(err)
		}
		spec.Values[0] = 99
		r, err := NewIntervalResampler(c)
		if err != nil {
			t.Fatal(err)
		}
		head, err := r.Push(context.Background(), []Signal{s}, testTimestamp(1))
		if err != nil {
			t.Fatal(err)
		}
		edited := head[0].Spec()
		edited.Values[0] = 88
		edited.Shape[0] = 9
		edited.ValidRange.Min = -1
		tail, err := r.Finish(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, out := range append(head, tail...) {
			got := out.Spec()
			want := s.Spec()
			want.Start = got.Start
			want.Duration = 0
			want.StreamID = c.OutputStreamID
			want.SourceSequence = got.SourceSequence
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("metadata changed: %+v", got)
			}
		}
	}
}
