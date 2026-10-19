package learning

import (
	"context"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

// TestPredictBatchMatchesPredict checks PredictBatch against Predict on a CPU
// trainer and, through the float64 stand-in core, on the batched path, for
// last-step and every-step readouts. Both paths are float64 here, so the
// readouts must agree bit for bit.
func TestPredictBatchMatchesPredict(t *testing.T) {
	ctx := context.Background()
	for name, f := range map[string]batchFixture{
		"last step input nodes": {loss: LossSoftmaxCrossEntropy, gate: dynamics.GateCurrentSoftsign, input: true},
		"every step all nodes":  {loss: LossMSE, everyStep: true, gate: dynamics.GateCurrent},
	} {
		t.Run(name, func(t *testing.T) {
			cpu, batched := newBatchCoreTrainers(t, f, 1)
			inputs, _ := batchEpisodes(5, 6, 3)
			for _, tr := range []*Trainer{cpu, batched} {
				got, err := tr.PredictBatch(ctx, inputs, 2)
				if err != nil {
					t.Fatal(err)
				}
				for i, input := range inputs {
					want, err := cpu.Predict(ctx, input)
					if err != nil {
						t.Fatal(err)
					}
					if len(got[i]) != len(want) {
						t.Fatalf("input %d: %d outputs, want %d", i, len(got[i]), len(want))
					}
					for j := range want {
						if math.Float64bits(got[i][j]) != math.Float64bits(want[j]) {
							t.Fatalf("batched=%v input %d output %d = %v, want %v", tr.batch != nil, i, j, got[i][j], want[j])
						}
					}
				}
			}
			uneven := [][][]float64{inputs[0], inputs[1][:3]}
			if _, err := batched.PredictBatch(ctx, uneven, 1); err == nil {
				t.Fatal("batched path accepted unequal lengths")
			}
		})
	}
}
