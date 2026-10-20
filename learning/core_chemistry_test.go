package learning_test

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// chemClassifierFixture is classifierFixture with a two-channel chemistry
// block: channel 0 released by the input nodes, channel 1 by the readout
// nodes, receptors on four of the six nodes.
func chemClassifierFixture() (learning.Config, learning.Parameters) {
	c, p := classifierFixture()
	c.Dynamics.Chemistry = &dynamics.Chemistry{
		Channels: []dynamics.ChemistryChannel{{Tau: 3, Kd: 1.2, Hill: 2, Sources: []int{0, 1}}, {Tau: 1.5, Kd: .7, Hill: 1, Sources: []int{3, 4, 5}}},
		Receptors: []dynamics.ChemistryReceptor{
			{Node: 2, Channel: 0}, {Node: 3, Channel: 0}, {Node: 3, Channel: 1}, {Node: 4, Channel: 1}, {Node: 5, Channel: 0},
		},
		GammaMin: .1, GammaMax: 4, BetaAbsMax: 2,
	}
	p.Core.ChemRelease = []float64{1.3, -.8}
	p.Core.ChemGain = []float64{.6, -.4, .9, .3, -.7}
	p.Core.ChemOffset = []float64{.2, -.3, .1, .4, -.2}
	return c, p
}

// TestChemistryNetworkGradientMatchesFiniteDifferences checks that the
// chemistry gradients reach the learning Gradient unbroken through Insyra's
// encoder, the core and Insyra's readout. The encoder, readout and loss are
// float32, so the step and tolerance are those of a float32 boundary.
func TestChemistryNetworkGradientMatchesFiniteDifferences(t *testing.T) {
	ctx := context.Background()
	c, p := chemClassifierFixture()
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	inputs, targets := classifierEpisodes(2)
	for e := range inputs {
		_, g, err := n.LossGradientCrossEntropy(ctx, p, inputs[e], targets[e], 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		loss := func(q learning.Parameters) float64 {
			l, _, err := n.LossGradientCrossEntropy(ctx, q, inputs[e], targets[e], 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			return l
		}
		const h = 1e-3
		worst := 0.0
		for name, pair := range map[string][2][]float64{
			"chem_release": {p.Core.ChemRelease, g.Core.ChemRelease},
			"chem_gain":    {p.Core.ChemGain, g.Core.ChemGain},
			"chem_offset":  {p.Core.ChemOffset, g.Core.ChemOffset},
		} {
			for i := range pair[0] {
				q := clone(p)
				values := map[string][]float64{"chem_release": q.Core.ChemRelease, "chem_gain": q.Core.ChemGain, "chem_offset": q.Core.ChemOffset}[name]
				values[i] += h
				plus := loss(q)
				values[i] -= 2 * h
				minus := loss(q)
				want := (plus - minus) / (2 * h)
				rel := math.Abs(pair[1][i]-want) / (1e-2 + math.Abs(want))
				worst = math.Max(worst, rel)
				if rel > 2e-2 {
					t.Fatalf("episode %d %s[%d] gradient %.6g, finite difference %.6g", e, name, i, pair[1][i], want)
				}
			}
		}
		t.Logf("episode %d: max |grad-fd|/(0.01+|fd|) over chemistry groups %.3g", e, worst)
	}
}

func clone(p learning.Parameters) learning.Parameters {
	b, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	var q learning.Parameters
	if err := json.Unmarshal(b, &q); err != nil {
		panic(err)
	}
	return q
}

// TestChemistryTrainerTrainsOnlyWhenTrainable runs StepBatch on the
// chemistry fixture with the block frozen and trainable, checks that two
// workers end bit-identical to sequential Steps, and round-trips the
// snapshot through JSON into RestoreTrainer.
func TestChemistryTrainerTrainsOnlyWhenTrainable(t *testing.T) {
	ctx := context.Background()
	c, p := chemClassifierFixture()
	inputs, targets := classifierEpisodes(6)
	for _, train := range []bool{false, true} {
		o := learning.DefaultOptions()
		o.Loss = learning.LossSoftmaxCrossEntropy
		o.AccumulateSteps = 3
		o.Trainable.Chemistry = train
		tr, err := learning.NewTrainer(c, p, o)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tr.StepBatch(ctx, inputs, targets, 2); err != nil {
			t.Fatal(err)
		}
		seq, err := learning.NewTrainer(c, p, o)
		if err != nil {
			t.Fatal(err)
		}
		for i := range inputs {
			if _, err := seq.Step(ctx, inputs[i], targets[i]); err != nil {
				t.Fatal(err)
			}
		}
		s := tr.Snapshot()
		if !reflect.DeepEqual(s, seq.Snapshot()) {
			t.Fatal("StepBatch with two workers differs from sequential Steps")
		}
		chem := s.Parameters.Core
		moved := !reflect.DeepEqual(chem.ChemRelease, p.Core.ChemRelease) && !reflect.DeepEqual(chem.ChemGain, p.Core.ChemGain) && !reflect.DeepEqual(chem.ChemOffset, p.Core.ChemOffset)
		untouched := reflect.DeepEqual(chem.ChemRelease, p.Core.ChemRelease) && reflect.DeepEqual(chem.ChemGain, p.Core.ChemGain) && reflect.DeepEqual(chem.ChemOffset, p.Core.ChemOffset)
		if train && !moved || !train && !untouched {
			t.Fatalf("trainable chemistry %v: moved %v untouched %v", train, moved, untouched)
		}
		if reflect.DeepEqual(chem.Weights, p.Core.Weights) {
			t.Fatal("weights did not train")
		}
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var back learning.TrainingSnapshot
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		restored, err := learning.RestoreTrainer(back)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(restored.Snapshot(), s) {
			t.Fatal("restored snapshot differs")
		}
		// One more step on both must stay identical, optimizer state included.
		r1, err := tr.Step(ctx, inputs[0], targets[0])
		if err != nil {
			t.Fatal(err)
		}
		r2, err := restored.Step(ctx, inputs[0], targets[0])
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(r1, r2) || !reflect.DeepEqual(tr.Snapshot(), restored.Snapshot()) {
			t.Fatal("restored trainer diverged")
		}
	}
}

func TestChemistryTrainerValidation(t *testing.T) {
	c, p := chemClassifierFixture()
	o := learning.DefaultOptions()
	missing := clone(p)
	missing.Core.ChemGain = nil
	if _, err := learning.NewTrainer(c, missing, o); err == nil || !strings.Contains(err.Error(), "chemistry") {
		t.Fatalf("missing receptor gains: %v", err)
	}
	plain, plainParams := classifierFixture()
	trainable := o
	trainable.Trainable.Chemistry = true
	if _, err := learning.NewTrainer(plain, plainParams, trainable); err == nil {
		t.Fatal("trainable chemistry accepted without a chemistry core")
	}
	stray := clone(plainParams)
	stray.Core.ChemRelease = []float64{1}
	if _, err := learning.NewTrainer(plain, stray, o); err == nil {
		t.Fatal("chemistry parameters accepted without a chemistry core")
	}
	recompute := o
	recompute.Recompute = &learning.Recompute{SegmentSteps: 2}
	if _, err := learning.NewTrainer(c, p, recompute); err == nil || !strings.Contains(err.Error(), "chemistry") {
		t.Fatalf("recompute: %v", err)
	}
	if _, err := learning.NewIndividual(c, p, o, make([]float64, 6)); err == nil || !strings.Contains(err.Error(), "chemistry") {
		t.Fatalf("persistent individual: %v", err)
	}
}

// TestUndeclaredChemistryKeepsTrainerJSON pins that options and snapshots of
// a model without the block carry no chemistry key.
func TestUndeclaredChemistryKeepsTrainerJSON(t *testing.T) {
	c, p := classifierFixture()
	tr, err := learning.NewTrainer(c, p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(tr.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"chem_`) || strings.Contains(string(b), `"chemistry"`) {
		t.Fatalf("snapshot JSON mentions chemistry: %s", b)
	}
}
