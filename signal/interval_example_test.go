package signal_test

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/signal"
)

func ExampleNewIntervalResampler() {
	v := signal.CurrentSchemaVersion()
	clock, err := signal.NewClock(v, signal.TimeUnitModelStep, 1)
	if err != nil {
		panic(err)
	}
	r, err := signal.NewIntervalResampler(signal.IntervalResampleConfig{
		SchemaVersion: v, Clock: clock, SourceUnit: signal.TimeUnitMilliseconds,
		SourceTicks: 1, SimulationSteps: 1, StartStep: 0, EndStep: 8, OutputStreamID: "aligned-current",
		Limits: signal.ResampleLimits{MaxBufferedSignals: 8, MaxOutputSignals: 8, MaxValues: 32},
	})
	if err != nil {
		panic(err)
	}
	input := []signal.Signal{}
	for i, event := range [][3]int64{{0, 6, 2}, {2, 1, 8}, {4, 1, 4}} {
		s, err := signal.NewSignal(signal.SignalSpec{
			SchemaVersion: v, ExperienceID: "example", StreamID: "sensor", Channel: "current",
			Kind: signal.KindContinuous, Start: signal.Timestamp{Value: event[0], Unit: signal.TimeUnitMilliseconds},
			Duration: event[1], Shape: []int{1}, Values: []float64{float64(event[2])}, Unit: signal.UnitDimensionless,
			EncoderVersion: v, SourceSequence: uint64(i + 10),
		})
		if err != nil {
			panic(err)
		}
		input = append(input, s)
	}
	head, err := r.Push(context.Background(), input, signal.Timestamp{Value: 4, Unit: signal.TimeUnitMilliseconds})
	if err != nil {
		panic(err)
	}
	tail, err := r.Finish(context.Background())
	if err != nil {
		panic(err)
	}
	for _, s := range append(head, tail...) {
		fmt.Println(s.StartTime().Value, s.Values()[0])
	}
	// Output:
	// 0 2
	// 1 2
	// 2 8
	// 3 2
	// 4 4
	// 5 2
}
