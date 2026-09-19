package distill_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/distill"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/learning"
)

// fixtureVocabHash gives the fixture student and its distillers one shared
// class vocabulary, satisfying identical alignment without a real tokenizer.
const fixtureVocabHash = "coimnet-distill-fixture-v1"

// newFixtureTrainer is the three-neuron delayed chain with OutputSize 2 and a
// fixed readout {1, −1}: the same fixture family as experiment.NewDelayedTrainer
// under experiment seed 7 (first weight 0.15 + 87/1000).
func newFixtureTrainer(t *testing.T) *learning.Trainer {
	t.Helper()
	c := learning.Config{
		Dynamics:  dynamics.Config{Nodes: 3, Sources: []int{0, 1, 1}, Targets: []int{1, 1, 2}, DT: 1, Activation: "tanh"},
		InputSize: 1, OutputSize: 2, ReadoutNodes: []int{2},
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{0.237, 0.2, 0.2}, Bias: []float64{0, 0, 0}, LogTau: []float64{math.Log(2), math.Log(2), math.Log(2)}},
		Encoder: []float64{1, 0, 0}, Readout: []float64{1, -1},
	}
	tr, err := learning.NewTrainer(c, p, learning.DefaultOptions())
	if err != nil {
		t.Fatalf("NewTrainer = %v, want nil", err)
	}
	return tr
}

func fixtureAlignment() distill.Alignment {
	return distill.Alignment{Rule: distill.AlignmentIdentical, TeacherVocabHash: fixtureVocabHash, StudentVocabHash: fixtureVocabHash}
}

// pulseLabel maps the delayed pulse sign to its class: positive pulse is class
// 1, negative pulse class 0.
func pulseLabel(ep experiment.Episode) int {
	if ep.Input[0][0] > 0 {
		return 1
	}
	return 0
}

// teacherFor emits the two-point teacher distribution whose argmax is the
// pulse class: {0.9, 0.1} for class 0 and {0.1, 0.9} for class 1.
func teacherFor(label int) distill.TeacherDistribution {
	if label == 1 {
		return distill.TeacherDistribution{Probabilities: []float64{0.1, 0.9}}
	}
	return distill.TeacherDistribution{Probabilities: []float64{0.9, 0.1}}
}

// trackingStudent wraps a trainer and records every StepResult StepDistribution
// or StepLabel caused.
type trackingStudent struct {
	tr      *learning.Trainer
	results []learning.StepResult
}

func (s *trackingStudent) Predict(ctx context.Context, input [][]float64) ([]float64, error) {
	return s.tr.Predict(ctx, input)
}

func (s *trackingStudent) StepFrom(ctx context.Context, input, upstream [][]float64) (learning.StepResult, error) {
	r, err := s.tr.StepFrom(ctx, input, upstream)
	s.results = append(s.results, r)
	return r, err
}

// recordingStudent hands out canned logits, photographs the upstream it
// receives on StepFrom and counts Predict/StepFrom calls.
type recordingStudent struct {
	logits     []float64
	upstream   [][]float64
	stepResult learning.StepResult
	stepped    bool
	predicts   int
	steps      int
}

func (s *recordingStudent) Predict(context.Context, [][]float64) ([]float64, error) {
	s.predicts++
	return s.logits, nil
}

func (s *recordingStudent) StepFrom(_ context.Context, _ [][]float64, upstream [][]float64) (learning.StepResult, error) {
	s.stepped = true
	s.steps++
	s.upstream = upstream
	return s.stepResult, nil
}

func TestStepDistributionLowersLossAndRaisesAgreement(t *testing.T) {
	tr := newFixtureTrainer(t)
	beforeTR := mustRestore(t, tr.Snapshot())
	ctx := context.Background()
	d := distill.DistributionDistiller{Temperature: 2, Scale: 1, Mix: 0.7, Alignment: fixtureAlignment()}

	inputs := make([][][]float64, 32)
	argmaxes := make([]int, 32)
	for i := range inputs {
		ep := experiment.DelayedEpisode(1003, uint64(i))
		inputs[i] = ep.Input
		argmaxes[i] = pulseLabel(ep)
	}
	before, err := distill.Agreement(ctx, beforeTR, inputs, argmaxes)
	if err != nil {
		t.Fatalf("Agreement(before) = %v, want nil", err)
	}

	var losses []float64
	for e := 0; e < 40; e++ {
		ep := experiment.DelayedEpisode(1001, uint64(e))
		label := pulseLabel(ep)
		rep, err := distill.StepDistribution(ctx, tr, d, ep.Input, teacherFor(label), label)
		if err != nil {
			t.Fatalf("StepDistribution(#%d) = %v, want nil", e, err)
		}
		if rep.Partial || rep.Report.Partial {
			t.Fatalf("StepDistribution(#%d) reported a full teacher as partial: %+v", e, rep)
		}
		losses = append(losses, rep.Loss)
	}

	head := mean(losses[:5])
	tail := mean(losses[len(losses)-5:])
	t.Logf("loss: head(first 5) mean %.6g, tail(last 5) mean %.6g, head values %v, tail values %v", head, tail, losses[:5], losses[len(losses)-5:])
	if !(head > tail) {
		t.Fatalf("loss did not fall: head mean %.6g <= tail mean %.6g", head, tail)
	}

	after, err := distill.Agreement(ctx, tr, inputs, argmaxes)
	if err != nil {
		t.Fatalf("Agreement(after) = %v, want nil", err)
	}
	t.Logf("held-out agreement: before %.3f, after %.3f (%d of 32)", before, after, int(math.Round(after*32)))
	if after < before {
		t.Fatalf("held-out agreement fell from %.3f to %.3f", before, after)
	}
}

func TestStepLabelMatchesMixZeroDistiller(t *testing.T) {
	snap := newFixtureTrainer(t).Snapshot()
	labelTr := &trackingStudent{tr: mustRestore(t, snap)}
	distTr := &trackingStudent{tr: mustRestore(t, snap)}
	ctx := context.Background()
	input := [][]float64{{0.3}, {0}, {0}, {0}, {0}}

	labelRep, err := distill.StepLabel(ctx, labelTr, input, 1, 2)
	if err != nil {
		t.Fatalf("StepLabel = %v, want nil", err)
	}
	distRep, err := distill.StepDistribution(ctx, distTr, distill.DistributionDistiller{
		Temperature: 1, Scale: 1, Mix: 0, Alignment: fixtureAlignment(),
	}, input, distill.TeacherDistribution{Probabilities: []float64{0.5, 0.5}}, 1)
	if err != nil {
		t.Fatalf("StepDistribution(Mix 0) = %v, want nil", err)
	}
	if labelRep.Partial || distRep.Partial {
		t.Fatalf("StepLabel or Mix-0 step reported partial: %+v vs %+v", labelRep, distRep)
	}
	if !reflect.DeepEqual(labelTr.results[0], distTr.results[0]) {
		t.Fatalf("StepResult differs:\nlabel-only %+v\nmix-zero   %+v", labelTr.results[0], distTr.results[0])
	}
	if !reflect.DeepEqual(labelTr.tr.Snapshot(), distTr.tr.Snapshot()) {
		t.Fatalf("trainer snapshots differ after StepLabel and Mix-0 StepDistribution")
	}
}

func TestUpstreamOnlyOnTheLastRow(t *testing.T) {
	s := &recordingStudent{logits: []float64{0.4, -0.2}}
	d := distill.DistributionDistiller{Temperature: 2, Scale: 1, Mix: 0.7, Alignment: fixtureAlignment()}
	teacher := distill.TeacherDistribution{Probabilities: []float64{0.1, 0.9}}
	input := [][]float64{{1}, {0}, {0}, {0}}

	if _, err := distill.StepDistribution(context.Background(), s, d, input, teacher, 1); err != nil {
		t.Fatalf("StepDistribution = %v, want nil", err)
	}
	if len(s.upstream) != len(input) {
		t.Fatalf("upstream has %d rows, want %d", len(s.upstream), len(input))
	}
	for i, row := range s.upstream[:len(s.upstream)-1] {
		for j, v := range row {
			if v != 0 {
				t.Fatalf("upstream[%d][%d] = %g, want 0 before the last row", i, j, v)
			}
		}
	}
	_, want, _, err := d.Loss(teacher, s.logits, 1)
	if err != nil {
		t.Fatalf("Loss = %v, want nil", err)
	}
	if !reflect.DeepEqual(s.upstream[len(s.upstream)-1], want) {
		t.Fatalf("last upstream row = %v, want dL/dstudent %v", s.upstream[len(s.upstream)-1], want)
	}
}

func TestPartialTeacherIsReported(t *testing.T) {
	s := &recordingStudent{logits: []float64{0, 0}}
	d := distill.DistributionDistiller{Temperature: 1, Scale: 1, Mix: 1, Alignment: fixtureAlignment()}
	rep, err := distill.StepDistribution(context.Background(), s, d,
		[][]float64{{1}, {0}}, distill.TeacherDistribution{Classes: []int{1}, Probabilities: []float64{0.8}}, -1)
	if err != nil {
		t.Fatalf("StepDistribution = %v, want nil", err)
	}
	if !rep.Partial || !rep.Report.Partial {
		t.Fatalf("StepReport = %+v, want Partial true for a top-k teacher", rep)
	}
}

func TestStepRejectsBadShapes(t *testing.T) {
	s := &recordingStudent{logits: []float64{0, 0}}
	d := distill.DistributionDistiller{Temperature: 1, Scale: 1, Mix: 1, Alignment: fixtureAlignment()}
	ctx := context.Background()

	if _, err := distill.StepDistribution(ctx, s, d, [][]float64{{1}},
		distill.TeacherDistribution{Probabilities: []float64{0.5, 0.3, 0.2}}, -1); err == nil {
		t.Fatalf("StepDistribution with 3 teacher classes on 2 student logits = nil, want an error")
	}
	if _, err := distill.StepDistribution(ctx, s, d, nil,
		distill.TeacherDistribution{Probabilities: []float64{0.5, 0.5}}, -1); err == nil {
		t.Fatalf("StepDistribution with empty input = nil, want an error")
	}
	if s.stepped {
		t.Fatalf("StepFrom ran on a rejected shape")
	}
}

func TestStepLabelRatioIsPreStepAgreement(t *testing.T) {
	for _, label := range []int{1, 0} {
		want := 0.0
		if label == 1 {
			want = 1
		}
		s := &recordingStudent{logits: []float64{0.2, 0.9}}
		rep, err := distill.StepLabel(context.Background(), s, [][]float64{{1}, {0}, {0}}, label, 2)
		if err != nil {
			t.Fatalf("StepLabel(label=%d) = %v, want nil", label, err)
		}
		if rep.Ratio != want {
			t.Fatalf("StepLabel(label=%d) Ratio = %g, want %g", label, rep.Ratio, want)
		}
		if s.predicts != 1 {
			t.Fatalf("StepLabel(label=%d) called Predict %d times, want 1", label, s.predicts)
		}
		if s.steps != 1 {
			t.Fatalf("StepLabel(label=%d) called StepFrom %d times, want 1", label, s.steps)
		}
	}
}

func TestStepDistributionCallsPredictOnce(t *testing.T) {
	s := &recordingStudent{logits: []float64{0.2, 0.9}}
	d := distill.DistributionDistiller{Temperature: 1, Scale: 1, Mix: 0.7, Alignment: fixtureAlignment()}
	teacher := distill.TeacherDistribution{Probabilities: []float64{0.9, 0.1}}
	if _, err := distill.StepDistribution(context.Background(), s, d, [][]float64{{1}, {0}}, teacher, 0); err != nil {
		t.Fatalf("StepDistribution = %v, want nil", err)
	}
	if s.predicts != 1 {
		t.Fatalf("StepDistribution called Predict %d times, want 1", s.predicts)
	}
	if s.steps != 1 {
		t.Fatalf("StepDistribution called StepFrom %d times, want 1", s.steps)
	}
}

func mean(v []float64) float64 {
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func mustRestore(t *testing.T, snap learning.TrainingSnapshot) *learning.Trainer {
	t.Helper()
	tr, err := learning.RestoreTrainer(snap)
	if err != nil {
		t.Fatalf("RestoreTrainer = %v, want nil", err)
	}
	return tr
}
