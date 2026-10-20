package learning

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/HazelnutParadise/insyra/nn"
	"github.com/TimLai666/coimnet/dynamics"
)

// oneHotEpisodes draws count sequences of one-hot rows of width 2 (the batch
// fixture's input size); zero leaves one row of the first sequence all zero,
// which is not one-hot.
func oneHotEpisodes(count, steps int, seed uint64, zero bool) [][][]float64 {
	rng := rand.New(rand.NewPCG(seed, 11))
	inputs := make([][][]float64, count)
	for i := range inputs {
		inputs[i] = make([][]float64, steps)
		for t := range inputs[i] {
			inputs[i][t] = make([]float64, 2)
			inputs[i][t][rng.IntN(2)] = 1
		}
	}
	if zero {
		inputs[0][1] = []float64{0, 0}
	}
	return inputs
}

// TestEncodeBatchMatchesDenseProduct checks both encoder routes of a chunk
// against the dense product of the stacked rows on one Insyra tape: a one-hot
// chunk (looked up with Embedding) and a chunk with an all-zero row (which is
// multiplied). Encoded rows must equal the per-pair encode and the encoder
// gradient must equal the dense product's, bit for bit.
func TestEncodeBatchMatchesDenseProduct(t *testing.T) {
	c, p, _ := newBatchFixture(batchFixture{loss: LossMSE, input: true}, 4)
	n, err := NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	for name, zero := range map[string]bool{"one-hot": false, "with a zero row": true} {
		t.Run(name, func(t *testing.T) {
			inputs := oneHotEpisodes(3, 4, 1, zero)
			host, encoded, err := n.encodeBatch(p, inputs, true)
			if err != nil {
				t.Fatal(err)
			}
			for i, input := range inputs {
				_, want, err := n.encode(p, input, false)
				if err != nil {
					t.Fatal(err)
				}
				for s := range want {
					for j := range want[s] {
						if math.Float64bits(encoded[i][s][j]) != math.Float64bits(want[s][j]) {
							t.Fatalf("pair %d step %d value %d = %v, per-pair %v", i, s, j, encoded[i][s][j], want[s][j])
						}
					}
				}
			}
			rng := rand.New(rand.NewPCG(3, 3))
			seed := make([][][]float64, len(inputs))
			var flatSeed, flatX []float64
			for i := range seed {
				seed[i] = make([][]float64, 4)
				for s := range seed[i] {
					seed[i][s] = []float64{rng.NormFloat64(), rng.NormFloat64()}
					flatSeed = append(flatSeed, seed[i][s]...)
					flatX = append(flatX, inputs[i][s]...)
				}
			}
			got, err := n.encoderBatchReverse(host, seed)
			if err != nil {
				t.Fatal(err)
			}
			tape := nn.NewTape()
			x, _ := tensor([]int{12, 2}, flatX)
			e, _ := tensor([]int{2, 2}, p.Encoder)
			if _, err := tape.Param(e); err != nil {
				t.Fatal(err)
			}
			z, err := tape.MatMul(x, e)
			if err != nil {
				t.Fatal(err)
			}
			up, _ := tensor([]int{12, 2}, flatSeed)
			if err := tape.BackwardFrom(z, up); err != nil {
				t.Fatal(err)
			}
			de, err := tape.Grad(e)
			if err != nil {
				t.Fatal(err)
			}
			want := doubles(de.Data())
			for j := range want {
				if math.Float64bits(got[j]) != math.Float64bits(want[j]) {
					t.Fatalf("encoder gradient[%d] = %v, dense product %v", j, got[j], want[j])
				}
			}
		})
	}
}

// TestBatchCoreOneHotMatchesStepBatch trains on one-hot sequences, the
// Embedding route, and checks it against StepBatch end to end.
func TestBatchCoreOneHotMatchesStepBatch(t *testing.T) {
	ctx := context.Background()
	f := batchFixture{loss: LossSoftmaxCrossEntropy, gate: dynamics.GateCurrentSoftsign, input: true}
	cpu, batched := newBatchCoreTrainers(t, f, 4)
	for round := range 3 {
		inputs := oneHotEpisodes(4, 5, uint64(30+round), false)
		_, targets := batchEpisodes(4, 5, uint64(round))
		want, err := cpu.StepBatch(ctx, inputs, targets, 2)
		if err != nil {
			t.Fatal(err)
		}
		got, err := batched.StepBatch(ctx, inputs, targets, 2)
		if err != nil {
			t.Fatal(err)
		}
		for i := range want {
			if !lossesAgree(want[i], got[i]) || want[i].Applied != got[i].Applied {
				t.Fatalf("round %d pair %d: CPU %+v, batched %+v", round, i, want[i], got[i])
			}
		}
	}
	nearBatchSums(t, "parameters", flatParameters(cpu.parameters), flatParameters(batched.parameters))
}
