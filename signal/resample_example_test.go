package signal_test

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/signal"
)

func ExampleNewStreamingResampler() {
	version := signal.CurrentSchemaVersion()
	clock, err := signal.NewClock(version, signal.TimeUnitModelStep, 1)
	if err != nil {
		panic(err)
	}
	cfg := signal.ResampleConfig{
		SchemaVersion: version, Mode: signal.CausalHold, Clock: clock,
		SourceUnit: signal.TimeUnitMilliseconds, SourceTicks: 1, SimulationSteps: 2,
		StartStep: 0, EndStep: 5, OutputStreamID: "aligned-vision", Tail: signal.HoldLast,
		Limits: signal.ResampleLimits{MaxBufferedSignals: 16, MaxOutputSignals: 16, MaxValues: 128},
	}
	resampler, err := signal.NewStreamingResampler(cfg)
	if err != nil {
		panic(err)
	}
	input := make([]signal.Signal, 2)
	for i, value := range []float64{2, 6} {
		input[i], err = signal.NewSignal(signal.SignalSpec{
			SchemaVersion: version, ExperienceID: "example", StreamID: "camera", Channel: "vision",
			Kind: signal.KindContinuous, Start: signal.Timestamp{Value: int64(i) * 2, Unit: signal.TimeUnitMilliseconds},
			Duration: 0, Shape: []int{1}, Values: []float64{value}, Unit: signal.UnitDimensionless,
			EncoderVersion: version, SourceSequence: uint64(i),
		})
		if err != nil {
			panic(err)
		}
	}
	batch, err := resampler.Push(context.Background(), input, signal.Timestamp{Value: 2, Unit: signal.TimeUnitMilliseconds})
	if err != nil {
		panic(err)
	}
	tail, err := resampler.Finish(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println(batch.Mode)
	for _, s := range append(batch.Signals, tail.Signals...) {
		fmt.Println(s.SourceSequence(), s.StartTime().Unit, s.Values()[0])
	}
	cfg.Mode = signal.OfflineLinear
	offline, err := signal.ResampleOffline(context.Background(), cfg, input)
	if err != nil {
		panic(err)
	}
	fmt.Println(offline.Mode)
	for _, s := range offline.Signals {
		fmt.Println(s.SourceSequence(), s.Values()[0])
	}
	// Output:
	// causal_hold
	// 0 model_step 2
	// 1 model_step 2
	// 2 model_step 2
	// 3 model_step 2
	// 4 model_step 6
	// offline_linear
	// 0 2
	// 1 3
	// 2 4
	// 3 5
	// 4 6
}
