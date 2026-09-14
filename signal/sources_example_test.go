package signal_test

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/TimLai666/coimnet/signal"
)

func ExampleNewSignal() {
	// Artificial, already decoded features. Media decoding belongs to the caller.
	sources := []struct {
		channel  string
		kind     signal.SignalKind
		values   []float64
		unit     signal.SignalUnit
		duration int64
	}{
		{"vision", signal.KindContinuous, []float64{0, 1}, signal.UnitNormalized, 0},
		{"audio-activity", signal.KindActivity, []float64{20}, signal.UnitHertz, 4},
		{"events", signal.KindPulse, []float64{1}, signal.UnitDimensionless, 0},
		{"drive", signal.KindModulation, []float64{0.5}, signal.UnitDimensionless, 4},
	}
	for _, source := range sources {
		input, err := signal.NewSignal(signal.SignalSpec{
			SchemaVersion: signal.CurrentSchemaVersion(),
			ExperienceID:  "artificial-example", StreamID: source.channel, Channel: source.channel,
			Kind: source.kind, Start: signal.Timestamp{Value: 0, Unit: signal.TimeUnitMilliseconds},
			Duration: source.duration, Shape: []int{len(source.values)}, Values: source.values, Unit: source.unit,
			EncoderVersion: signal.Version{Major: 2, Minor: 1}, SourceSequence: 0,
		})
		if err != nil {
			panic(err)
		}
		data, err := json.Marshal(input)
		if err != nil {
			panic(err)
		}
		restored, err := signal.DecodeSignal(bytes.NewReader(data))
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s: %s %v %s encoder=%d.%d\n", restored.Channel(), restored.Kind(), restored.Values(), restored.Unit(), restored.EncoderVersion().Major, restored.EncoderVersion().Minor)
	}
	// Output:
	// vision: continuous [0 1] normalized encoder=2.1
	// audio-activity: activity [20] Hz encoder=2.1
	// events: pulse [1] 1 encoder=2.1
	// drive: modulation [0.5] 1 encoder=2.1
}
