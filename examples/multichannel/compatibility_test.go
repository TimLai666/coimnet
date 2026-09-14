package multichannel_test

import (
	"context"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/examples/multichannel"
	"github.com/TimLai666/coimnet/signal"
)

func TestAdaptRejectsValuesOutsideLearningRange(t *testing.T) {
	for _, name := range []string{"level", "gate", "pulses", "finite pulse sum"} {
		t.Run(name, func(t *testing.T) {
			input := fixture()
			switch name {
			case "level":
				input.Level = observation("level", signal.KindContinuous, [][3]float64{{0, 0, math.MaxFloat64}})
			case "gate":
				input.Gate = observation("gate", signal.KindActivity, [][3]float64{{0, 1, -math.MaxFloat64}})
			case "pulses":
				input.Pulses = observation("pulses", signal.KindPulse, [][3]float64{{0, 0, math.MaxFloat64}})
			case "finite pulse sum":
				input.Pulses = observation("pulses", signal.KindPulse, [][3]float64{{0, 0, math.MaxFloat32}, {0, 0, math.MaxFloat32}})
			}
			got, err := multichannel.Adapt(context.Background(), input, 1)
			if err == nil || got != nil {
				t.Fatalf("incompatible learning input returned: %v, %v", got, err)
			}
		})
	}
	// Apply the range check to the aggregate, not individual events: the final
	// feature is zero and is representable after this declared cancellation.
	input := fixture()
	input.Pulses = observation("pulses", signal.KindPulse, [][3]float64{{0, 0, math.MaxFloat64}, {0, 0, -math.MaxFloat64}})
	got, err := multichannel.Adapt(context.Background(), input, 1)
	if err != nil || got[0][4] != 0 || got[0][5] != 1 {
		t.Fatalf("representable aggregate rejected: %v %v", got, err)
	}
}
