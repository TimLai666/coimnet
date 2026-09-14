package signal_test

import (
	"context"
	"fmt"
	"github.com/TimLai666/coimnet/signal"
)

func ExampleNewPulseAligner() {
	version := signal.CurrentSchemaVersion()
	clock, err := signal.NewClock(version, signal.TimeUnitModelStep, 1)
	if err != nil {
		panic(err)
	}
	aligner, err := signal.NewPulseAligner(signal.PulseAlignConfig{
		SchemaVersion: version, Clock: clock,
		SourceUnit: signal.TimeUnitMilliseconds, SourceTicks: 2, SimulationSteps: 1,
		OutputStreamID: "aligned-pulses", MaxBufferedSignals: 8, MaxValues: 32,
	})
	if err != nil {
		panic(err)
	}
	input := make([]signal.Signal, 2)
	for i, value := range []float64{3, 5} {
		input[i], err = signal.NewSignal(signal.SignalSpec{
			SchemaVersion: version, ExperienceID: "example", StreamID: "sensor", Channel: "touch",
			Kind: signal.KindPulse, Start: signal.Timestamp{Value: int64(i + 1), Unit: signal.TimeUnitMilliseconds},
			Duration: 0, Shape: []int{1}, Values: []float64{value}, Unit: signal.UnitDimensionless,
			EncoderVersion: version, SourceSequence: uint64(10 + i*10),
		})
		if err != nil {
			panic(err)
		}
	}
	head, err := aligner.Push(context.Background(), input, signal.Timestamp{Value: 2, Unit: signal.TimeUnitMilliseconds})
	if err != nil {
		panic(err)
	}
	fmt.Println("before closure:", len(head))
	tail, err := aligner.Finish(context.Background())
	if err != nil {
		panic(err)
	}
	for _, s := range tail {
		fmt.Println(s.StartTime().Value, s.SourceSequence(), s.Values()[0])
	}
	// Output:
	// before closure: 0
	// 1 10 3
	// 1 20 5
}
