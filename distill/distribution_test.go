package distill_test

import (
	"errors"
	"math"
	"math/rand"
	"testing"

	"github.com/TimLai666/coimnet/distill"
)

func TestAlignmentRules(t *testing.T) {
	ok := distill.Alignment{
		Rule:             distill.AlignmentIdentical,
		TeacherVocabHash: "vocab",
		StudentVocabHash: "vocab",
	}
	if err := ok.Validate(3, 3); err != nil {
		t.Fatalf("identical with equal hashes and equal class counts = %v, want nil", err)
	}

	differing := distill.Alignment{
		Rule:             distill.AlignmentIdentical,
		TeacherVocabHash: "vocab-teacher",
		StudentVocabHash: "vocab-student",
	}
	if err := differing.Validate(3, 3); !errors.Is(err, distill.ErrIncompatibleAlignment) {
		t.Fatalf("identical with differing hashes = %v, want ErrIncompatibleAlignment", err)
	}

	if err := ok.Validate(3, 4); !errors.Is(err, distill.ErrIncompatibleAlignment) {
		t.Fatalf("identical with mismatched class counts = %v, want ErrIncompatibleAlignment", err)
	}

	mapped := distill.Alignment{Rule: distill.AlignmentDeclaredMap, Map: []int{2, 1, 0}}
	if err := mapped.Validate(3, 3); err != nil {
		t.Fatalf("declared_map with full coverage = %v, want nil", err)
	}

	short := distill.Alignment{Rule: distill.AlignmentDeclaredMap, Map: []int{0, 1}}
	if err := short.Validate(3, 3); !errors.Is(err, distill.ErrIncompatibleAlignment) {
		t.Fatalf("declared_map missing one teacher class = %v, want ErrIncompatibleAlignment", err)
	}

	outOfRange := distill.Alignment{Rule: distill.AlignmentDeclaredMap, Map: []int{0, 1, 3}}
	if err := outOfRange.Validate(3, 3); !errors.Is(err, distill.ErrIncompatibleAlignment) {
		t.Fatalf("declared_map with a class out of range = %v, want ErrIncompatibleAlignment", err)
	}

	unknown := distill.Alignment{Rule: "fancy"}
	if err := unknown.Validate(3, 3); !errors.Is(err, distill.ErrIncompatibleAlignment) {
		t.Fatalf("unknown rule = %v, want ErrIncompatibleAlignment", err)
	}
}

func TestLossHandComputation(t *testing.T) {
	d := distill.DistributionDistiller{
		Temperature: 1,
		Scale:       1,
		Mix:         1,
		Alignment: distill.Alignment{
			Rule:             distill.AlignmentIdentical,
			TeacherVocabHash: "vocab",
			StudentVocabHash: "vocab",
		},
	}
	loss, grad, report, err := d.Loss(
		distill.TeacherDistribution{Probabilities: []float64{0.75, 0.25}},
		[]float64{0, 0},
		-1,
	)
	if err != nil {
		t.Fatalf("Loss = %v, want nil", err)
	}
	// KL = 0.75*(ln 0.75 - ln 0.5) + 0.25*(ln 0.25 - ln 0.5)
	//     = 0.75*ln 1.5 + 0.25*ln 0.5
	const want = 0.75*0.40546510810816438197801311546434913657 + 0.25*(-0.69314718055994530941723212145817656808)
	if math.Abs(loss-want) > 1e-12 {
		t.Fatalf("loss = %.17g, want %.17g (±1e-12)", loss, want)
	}
	if len(grad) != 2 {
		t.Fatalf("len(grad) = %d, want 2", len(grad))
	}
	for j, want := range []float64{0.5 - 0.75, 0.5 - 0.25} {
		if math.Abs(grad[j]-want) > 1e-12 {
			t.Fatalf("grad[%d] = %.17g, want %.17g (±1e-12)", j, grad[j], want)
		}
	}
	if report.Partial {
		t.Fatalf("report.Partial = true, want false for a full teacher distribution")
	}
}

func TestLossFiniteDifferences(t *testing.T) {
	const (
		classes = 5
		eps     = 1e-6
	)
	rng := rand.New(rand.NewSource(3))
	p := make([]float64, classes)
	sum := 0.0
	for j := range p {
		p[j] = rng.Float64() + 1e-3
		sum += p[j]
	}
	for j := range p {
		p[j] /= sum
	}
	student := make([]float64, classes)
	for j := range student {
		student[j] = (rng.Float64() - 0.5) * 10
	}
	d := distill.DistributionDistiller{
		Temperature: 2,
		Scale:       1.5,
		Mix:         0.7,
		Alignment: distill.Alignment{
			Rule:             distill.AlignmentIdentical,
			TeacherVocabHash: "vocab",
			StudentVocabHash: "vocab",
		},
	}
	teacher := distill.TeacherDistribution{Probabilities: p}
	_, grad, _, err := d.Loss(teacher, student, 3)
	if err != nil {
		t.Fatalf("Loss = %v, want nil", err)
	}
	if len(grad) != classes {
		t.Fatalf("len(grad) = %d, want %d", len(grad), classes)
	}
	worst := 0.0
	for j := 0; j < classes; j++ {
		fwd := append([]float64(nil), student...)
		rev := append([]float64(nil), student...)
		fwd[j] += eps
		rev[j] -= eps
		plus, _, _, err := d.Loss(teacher, fwd, 3)
		if err != nil {
			t.Fatalf("Loss(+eps) = %v, want nil", err)
		}
		minus, _, _, err := d.Loss(teacher, rev, 3)
		if err != nil {
			t.Fatalf("Loss(-eps) = %v, want nil", err)
		}
		fd := (plus - minus) / (2 * eps)
		rel := math.Abs(fd-grad[j]) / math.Max(math.Abs(grad[j]), 1e-9)
		if rel > worst {
			worst = rel
		}
		if rel > 1e-6 {
			t.Fatalf("grad[%d] = %.9g, central difference %.9g, relative error %.3g > 1e-6",
				j, grad[j], fd, rel)
		}
	}
	t.Logf("worst relative finite-difference error: %.3g", worst)
}

func TestLargeLogitsStayFinite(t *testing.T) {
	d := distill.DistributionDistiller{
		Temperature: 2,
		Scale:       1.5,
		Mix:         0.5,
		Alignment: distill.Alignment{
			Rule:             distill.AlignmentIdentical,
			TeacherVocabHash: "vocab",
			StudentVocabHash: "vocab",
		},
	}
	loss, grad, _, err := d.Loss(
		distill.TeacherDistribution{Probabilities: []float64{1, 0, 0}},
		[]float64{1000, -1000, 0},
		0,
	)
	if err != nil {
		t.Fatalf("Loss = %v, want nil", err)
	}
	if math.IsNaN(loss) || math.IsInf(loss, 0) {
		t.Fatalf("loss = %v, want finite", loss)
	}
	for j, g := range grad {
		if math.IsNaN(g) || math.IsInf(g, 0) {
			t.Fatalf("grad[%d] = %v, want finite", j, g)
		}
	}
}

func TestPartialTopKIsNotRenormalized(t *testing.T) {
	d := distill.DistributionDistiller{
		Temperature: 1,
		Scale:       1,
		Mix:         1,
		TopK:        2,
		Alignment: distill.Alignment{
			Rule:             distill.AlignmentIdentical,
			TeacherVocabHash: "vocab",
			StudentVocabHash: "vocab",
		},
	}
	student := []float64{0, 0, 0, 0}
	loss, _, report, err := d.Loss(
		distill.TeacherDistribution{Classes: []int{0, 2}, Probabilities: []float64{0.5, 0.3}},
		student,
		-1,
	)
	if err != nil {
		t.Fatalf("Loss = %v, want nil", err)
	}
	if !report.Partial {
		t.Fatalf("report.Partial = false, want true for a top-k teacher distribution")
	}
	// KL = 0.5*(ln 0.5 - ln 0.25) + 0.3*(ln 0.3 - ln 0.25), p used exactly as
	// given, never renormalized.
	const want = 0.5*0.69314718055994530941723212145817656808 + 0.3*0.1823215567939546262117180251545146331972
	if math.Abs(loss-want) > 1e-12 {
		t.Fatalf("loss = %.17g, want %.17g (p used exactly as given, ±1e-12)", loss, want)
	}
	renormalized := 0.625*math.Log(2.5) + 0.375*math.Log(1.5)
	if math.Abs(loss-renormalized) <= 1e-6 {
		t.Fatalf("loss = %.17g, must not equal the renormalized top-k value %.17g", loss, renormalized)
	}

	if _, _, _, err := d.Loss(
		distill.TeacherDistribution{Classes: []int{0, 1}, Probabilities: []float64{0.6, 0.5}},
		student,
		-1,
	); err == nil {
		t.Fatalf("Loss with probabilities summing to 1.1 = nil, want an error")
	}
}

func TestDeclaredMapRoutesClasses(t *testing.T) {
	student := []float64{0.2, -0.7}
	lossMapped, gradMapped, _, err := (distill.DistributionDistiller{
		Temperature: 1,
		Scale:       1,
		Mix:         1,
		Alignment:   distill.Alignment{Rule: distill.AlignmentDeclaredMap, Map: []int{1, 0}},
	}).Loss(distill.TeacherDistribution{Probabilities: []float64{1, 0}}, student, -1)
	if err != nil {
		t.Fatalf("Loss(declared_map) = %v, want nil", err)
	}
	lossIdentical, gradIdentical, _, err := (distill.DistributionDistiller{
		Temperature: 1,
		Scale:       1,
		Mix:         1,
		Alignment: distill.Alignment{
			Rule:             distill.AlignmentIdentical,
			TeacherVocabHash: "vocab",
			StudentVocabHash: "vocab",
		},
	}).Loss(distill.TeacherDistribution{Probabilities: []float64{0, 1}}, student, -1)
	if err != nil {
		t.Fatalf("Loss(identical) = %v, want nil", err)
	}
	if math.Abs(lossMapped-lossIdentical) > 1e-12 {
		t.Fatalf("KL under declared_map {1,0} with p {1,0} = %.17g, want identical p {0,1} KL %.17g (±1e-12)",
			lossMapped, lossIdentical)
	}
	for j := range gradMapped {
		if math.Abs(gradMapped[j]-gradIdentical[j]) > 1e-12 {
			t.Fatalf("grad[%d] under declared_map = %.17g, want %.17g (±1e-12)",
				j, gradMapped[j], gradIdentical[j])
		}
	}
}
