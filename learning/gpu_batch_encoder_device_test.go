package learning

import (
	"context"
	"testing"

	"github.com/HazelnutParadise/insyra/accel"
	"github.com/TimLai666/coimnet/dynamics"
)

// TestBatchCoreDeviceEncoderMatchesStepBatch runs the batched path with the
// encoder product and its gradient on Insyra's resident device, where one is
// available, against the float64 per-pair StepBatch. The device product is
// rounded once to float32, so losses, parameters and optimizer state agree
// to the batch-sum rounding rather than bit for bit. Without a device the
// test is skipped: the host path is TestBatchCoreStepMatchesStepBatch.
func TestBatchCoreDeviceEncoderMatchesStepBatch(t *testing.T) {
	device, err := accel.ResidentDevice()
	if err != nil {
		t.Skipf("no resident device: %v", err)
	}
	ctx := context.Background()
	for name, f := range map[string]batchFixture{
		"cross entropy gated":   {loss: LossSoftmaxCrossEntropy, gate: dynamics.GateCurrent, input: true},
		"softsign gate all in":  {loss: LossSoftmaxCrossEntropy, gate: dynamics.GateCurrentSoftsign},
		"every step cross gate": {loss: LossSoftmaxCrossEntropy, everyStep: true, gate: dynamics.GateCurrent},
	} {
		t.Run(name, func(t *testing.T) {
			cpu, batched := newBatchCoreTrainers(t, f, 4)
			batched.network.encoderDevice = device
			for round := range 3 {
				inputs, targets := batchEpisodes(4, 5, uint64(round))
				targets = batchTargets(f, targets, 5)
				want, err := cpu.StepBatch(ctx, inputs, targets, 2)
				if err != nil {
					t.Fatal(err)
				}
				got, err := batched.StepBatch(ctx, inputs, targets, 3)
				if err != nil {
					t.Fatal(err)
				}
				for i := range want {
					w, g := want[i], got[i]
					if !batchSumNear(w.Loss, g.Loss) || w.Applied != g.Applied || w.Updates != g.Updates {
						t.Fatalf("round %d pair %d: CPU %+v, device encoder %+v", round, i, w, g)
					}
				}
			}
			ws, gs := cpu.Snapshot(), batched.Snapshot()
			nearBatchSums(t, "parameters", flatParameters(ws.Parameters), flatParameters(gs.Parameters))
			nearBatchSums(t, "first moment", ws.Optimizer.First, gs.Optimizer.First)
			nearBatchSums(t, "second moment", ws.Optimizer.Second, gs.Optimizer.Second)
			inputs, _ := batchEpisodes(4, 5, 9)
			onDevice, err := batched.PredictBatch(ctx, inputs, 2)
			if err != nil {
				t.Fatal(err)
			}
			batched.network.encoderDevice = accel.Device{}
			onHost, err := batched.PredictBatch(ctx, inputs, 2)
			if err != nil {
				t.Fatal(err)
			}
			for i := range onHost {
				nearBatchSums(t, "prediction", onHost[i], onDevice[i])
			}
		})
	}
}
