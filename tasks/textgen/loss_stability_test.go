package textgen

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestTextgenUniformLoss146(t *testing.T) {
	m, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 8, Settle: 3, LearningRate: .01, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	ex := Example{IDs: []int{0, 1, 2}, Scored: []bool{false, true, true}}
	for name, offset := range map[string]float64{"zero": 0, "positive": math.Ldexp(1, 54), "negative": -math.Ldexp(1, 54), "maximum": math.MaxFloat64, "minimum": -math.MaxFloat64} {
		t.Run(name, func(t *testing.T) {
			rows := make([][]float64, len(ex.IDs))
			for i := range rows {
				rows[i] = make([]float64, m.vocabSize)
				for j := range rows[i] {
					rows[i][j] = offset
				}
			}
			loss, count, gradient, err := m.lossAndUpstream(ex, rows)
			want := 2 * math.Log(float64(m.vocabSize))
			if err != nil || count != 2 || math.IsNaN(loss) || math.IsInf(loss, 0) || math.Abs(loss-want) > 1e-13 {
				t.Fatalf("uniform NLL = %.17g, count=%d, err=%v; want %.17g", loss, count, err, want)
			}
			for position := 1; position < len(ex.IDs); position++ {
				for id, value := range gradient[position*m.config.Settle-1] {
					want := 1 / (2 * float64(m.vocabSize))
					if id == ex.IDs[position] {
						want -= .5
					}
					if math.Abs(value-want) > 1e-15 {
						t.Fatalf("gradient[%d][%d]=%g, want %g", position, id, value, want)
					}
				}
			}
		})
	}
}

func TestTextgenLargeReadoutLoss146(t *testing.T) {
	for _, mode := range []string{"NLL", "Step"} {
		t.Run(mode, func(t *testing.T) {
			m, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 8, Settle: 3, LearningRate: .01, Seed: 17})
			if err != nil {
				t.Fatal(err)
			}
			snapshot := m.Snapshot()
			for i := range snapshot.Parameters.Readout {
				snapshot.Parameters.Readout[i] = 1e20
			}
			snapshot.Options.Trainable = learning.Trainable{Readout: true}
			m.trainer, err = learning.NewTrainer(snapshot.Config, snapshot.Parameters, snapshot.Options)
			if err != nil {
				t.Fatal(err)
			}
			ex := Example{IDs: []int{0, 1}, Scored: []bool{false, true}}
			rows, err := m.Logits(context.Background(), ex.IDs)
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range rows[0] {
				if v != rows[0][0] || math.Abs(v) < math.Ldexp(1, 54) {
					t.Fatal("control did not create identical large logits")
				}
			}
			before := m.Snapshot()
			var loss float64
			if mode == "NLL" {
				var count int
				loss, count, err = m.NLL(context.Background(), ex)
				if count != 1 || !reflect.DeepEqual(before, m.Snapshot()) {
					t.Fatal("NLL changed state or scored count")
				}
			} else {
				_, loss, err = m.Step(context.Background(), ex)
			}
			want := math.Log(float64(m.vocabSize))
			if err != nil || math.IsNaN(loss) || math.IsInf(loss, 0) || math.Abs(loss-want) > 1e-13 {
				t.Fatalf("%s loss = %.17g, %v; want %.17g", mode, loss, err, want)
			}
		})
	}
}

func TestTextgenLossRejectsNonFiniteLogits146(t *testing.T) {
	m, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 4, Settle: 1, LearningRate: .01, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	ex := Example{IDs: []int{0, 1}, Scored: []bool{false, true}}
	for name, bad := range map[string]float64{"nan": math.NaN(), "positive_infinity": math.Inf(1), "negative_infinity": math.Inf(-1)} {
		t.Run(name, func(t *testing.T) {
			rows := [][]float64{make([]float64, m.vocabSize), make([]float64, m.vocabSize)}
			rows[0][0] = bad
			loss, count, gradient, err := m.lossAndUpstream(ex, rows)
			if err == nil || loss != 0 || count != 0 || gradient != nil {
				t.Fatalf("non-finite input gave loss=%g count=%d gradient=%v err=%v", loss, count, gradient, err)
			}
		})
	}
}
