package multichannel_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/examples/multichannel"
	"github.com/TimLai666/coimnet/signal"
)

func observation(channel string, kind signal.SignalKind, points [][3]float64) signal.Observation {
	signals := make([]signal.Signal, len(points))
	for i, p := range points {
		s, err := signal.NewSignal(signal.SignalSpec{SchemaVersion: signal.CurrentSchemaVersion(), ExperienceID: "synthetic", StreamID: channel, Channel: channel, Kind: kind, Start: signal.Timestamp{Value: int64(p[0]), Unit: signal.TimeUnitNormalizedTime}, Duration: int64(p[1]), Shape: []int{1}, Values: []float64{p[2]}, Unit: signal.UnitDimensionless, EncoderVersion: signal.Version{Major: 1}, SourceSequence: uint64(i + 1)})
		if err != nil {
			panic(err)
		}
		signals[i] = s
	}
	o, err := signal.NewObservation(signal.CurrentSchemaVersion(), "synthetic", channel, signals)
	if err != nil {
		panic(err)
	}
	return o
}

func fixture() multichannel.Inputs {
	return multichannel.Inputs{
		Level:  observation("level", signal.KindContinuous, [][3]float64{{2, 0, 0}, {6, 0, .5}, {6, 0, .75}}),
		Gate:   observation("gate", signal.KindActivity, [][3]float64{{3, 9, .25}, {6, 3, .75}}),
		Pulses: observation("pulses", signal.KindPulse, [][3]float64{{1, 0, 1}, {2, 0, -1}, {9, 0, .5}}),
	}
}

// Hand-derived rows, independent of all signal resampling implementations.
func reference() [][]float64 {
	return [][]float64{{0, 0, 0, 0, 0, 0}, {0, 1, .25, 1, 0, 1}, {0, 1, .75, 1, 0, 0}, {.75, 1, .25, 1, .5, 1}, {.75, 1, 0, 0, 0, 0}, {.75, 1, 0, 0, 0, 0}, {.75, 1, 0, 0, 0, 0}, {.75, 1, 0, 0, 0, 0}}
}

func TestAdaptIndependentReferenceAndChunks(t *testing.T) {
	input := fixture()
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range []int{1, 2, 3, 32} {
		got, err := multichannel.Adapt(context.Background(), input, chunk)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, reference()) {
			t.Fatalf("chunk %d: got %v", chunk, got)
		}
		got[1][0] = 99 // returned rows do not share input or one another
		if got[2][0] != 0 {
			t.Fatal("shared rows")
		}
	}
	after, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("modified observations")
	}
}

func TestAdaptEmptyAndRejectedInputs(t *testing.T) {
	empty := multichannel.Inputs{Level: observation("level", signal.KindContinuous, nil), Gate: observation("gate", signal.KindActivity, nil), Pulses: observation("pulses", signal.KindPulse, nil)}
	got, err := multichannel.Adapt(context.Background(), empty, 1)
	if err != nil || len(got) != 8 {
		t.Fatalf("empty: %v %v", got, err)
	}
	for _, row := range got {
		for _, v := range row {
			if v != 0 {
				t.Fatal(got)
			}
		}
	}
	cases := map[string]multichannel.Inputs{}
	bad := fixture()
	bad.Level = signal.Observation{}
	cases["zero observation"] = bad
	bad = fixture()
	bad.Level = observation("unknown", signal.KindContinuous, nil)
	cases["unknown channel"] = bad
	bad = fixture()
	bad.Gate = observation("gate", signal.KindActivity, [][3]float64{{1, 0, 1}})
	cases["point as interval"] = bad
	bad = fixture()
	bad.Pulses = observation("pulses", signal.KindContinuous, [][3]float64{{1, 0, 1}})
	cases["wrong kind"] = bad
	bad = fixture()
	bad.Pulses = observation("pulses", signal.KindPulse, [][3]float64{{29, 0, 1}})
	cases["pulse outside horizon"] = bad
	bad = fixture()
	bad.Level = observation("level", signal.KindContinuous, [][3]float64{{16, 0, 1}})
	cases["point outside horizon"] = bad
	many := make([][3]float64, 33)
	bad = fixture()
	bad.Pulses = observation("pulses", signal.KindPulse, many)
	cases["capacity"] = bad
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := multichannel.Adapt(context.Background(), input, 1)
			if err == nil || got != nil {
				t.Fatalf("got %v err %v", got, err)
			}
		})
	}
	for _, chunk := range []int{0, -1, 33} {
		if got, err := multichannel.Adapt(context.Background(), fixture(), chunk); err == nil || got != nil {
			t.Fatalf("chunk %d accepted", chunk)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, canceled} {
		if got, err := multichannel.Adapt(ctx, fixture(), 1); err == nil || got != nil {
			t.Fatal("invalid context accepted")
		}
	}
	if _, err := multichannel.Adapt(context.Background(), fixture(), 1); err != nil {
		t.Fatal("failed call poisoned retry", err)
	}
}
