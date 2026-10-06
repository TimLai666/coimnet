package distill_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/distill"
)

func numericalDistiller(temp, scale, mix float64) distill.DistributionDistiller {
	return distill.DistributionDistiller{
		Temperature: temp,
		Scale:       scale,
		Mix:         mix,
		Alignment: distill.Alignment{
			Rule:             distill.AlignmentIdentical,
			TeacherVocabHash: "vocab",
			StudentVocabHash: "vocab",
		},
	}
}

// numericalDistributionExpected uses direct exponentiation of small differences, not package LSE.
func numericalDistributionExpected(student, p []float64, label int, temp, scale, mix float64) (float64, []float64) {
	maxStudent := student[0]
	for _, v := range student[1:] {
		if v > maxStudent {
			maxStudent = v
		}
	}
	qT := make([]float64, len(student))
	q1 := make([]float64, len(student))
	sumT, sum1 := 0.0, 0.0
	for i, v := range student {
		qT[i] = math.Exp((v - maxStudent) / temp)
		q1[i] = math.Exp(v - maxStudent)
		sumT += qT[i]
		sum1 += q1[i]
	}
	for i := range student {
		qT[i] /= sumT
		q1[i] /= sum1
	}
	pMass, kl := 0.0, 0.0
	for i, pi := range p {
		pMass += pi
		if pi > 0 {
			kl += pi * (math.Log(pi) - math.Log(qT[i]))
		}
	}
	loss := mix * temp * temp * scale * kl
	grad := make([]float64, len(student))
	factor := mix * temp * scale
	for i := range grad {
		grad[i] = factor * (pMass*qT[i] - p[i])
	}
	if label >= 0 {
		loss += (1 - mix) * (-math.Log(q1[label]))
		for i := range grad {
			g := q1[i]
			if i == label {
				g--
			}
			grad[i] += (1 - mix) * g
		}
	}
	return loss, grad
}

func assertNumericalClose(t *testing.T, got, want, tol float64, label string) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("%s = %v, want finite value near %.17g", label, got, want)
	}
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.17g, want %.17g (diff %.3g > %.3g)", label, got, want, math.Abs(got-want), tol)
	}
}

func assertFloatBits(t *testing.T, got, want []float64, label string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d", label, len(got), len(want))
	}
	for i := range want {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			t.Fatalf("%s[%d] bits = %#x, want %#x", label, i, math.Float64bits(got[i]), math.Float64bits(want[i]))
		}
	}
}

func assertDistributionResult(t *testing.T, loss float64, grad, wantGrad []float64, wantLoss float64) {
	t.Helper()
	assertNumericalClose(t, loss, wantLoss, 2e-12, "loss")
	if len(grad) != len(wantGrad) {
		t.Fatalf("grad length = %d, want %d", len(grad), len(wantGrad))
	}
	for i := range wantGrad {
		assertNumericalClose(t, grad[i], wantGrad[i], 2e-12, "grad["+string(rune('0'+i))+"]")
	}
}

func TestDistributionNumericalUniformExtremeScores(t *testing.T) {
	d := numericalDistiller(1, 1, 1)
	teacher := distill.TeacherDistribution{Probabilities: []float64{0.5, 0.5}}
	for _, shift := range []float64{0, 1e16, -1e16, 1e308, -1e308} {
		student := []float64{shift, shift}
		studentBefore := append([]float64(nil), student...)
		teacherBefore := append([]float64(nil), teacher.Probabilities...)
		loss, grad, report, err := d.Loss(teacher, student, -1)
		if err != nil {
			t.Fatalf("shift %g: Loss = %v, want nil", shift, err)
		}
		assertDistributionResult(t, loss, grad, []float64{0, 0}, 0)
		if report.Temperature != d.Temperature || report.Scale != d.Scale || report.Mix != d.Mix || report.Partial {
			t.Fatalf("shift %g: report = %+v, want distiller parameters and full distribution", shift, report)
		}
		assertFloatBits(t, student, studentBefore, "student")
		assertFloatBits(t, teacher.Probabilities, teacherBefore, "teacher probabilities")
	}
}

func TestDistributionNumericalEqualExtremeMixedCE(t *testing.T) {
	teacher := distill.TeacherDistribution{Probabilities: []float64{0.5, 0.5}}
	wantLoss := (1 - 0.35) * math.Ln2
	wantGrad := []float64{(1 - 0.35) * 0.5, (1 - 0.35) * -0.5}
	for _, tc := range []struct {
		name string
		temp float64
	}{
		{name: "half", temp: 0.5},
		{name: "double", temp: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := numericalDistiller(tc.temp, 1.75, 0.35)
			student := []float64{1e308, 1e308}
			before := append([]float64(nil), student...)
			loss, grad, _, err := d.Loss(teacher, student, 1)
			if err != nil {
				t.Fatalf("Loss = %v, want nil", err)
			}
			assertDistributionResult(t, loss, grad, wantGrad, wantLoss)
			assertFloatBits(t, student, before, "student")
		})
	}
}

func TestDistributionNumericalMixedTemperatureAndScale(t *testing.T) {
	student := []float64{0, 2, 4}
	teacher := distill.TeacherDistribution{Probabilities: []float64{0.2, 0.3, 0.5}}
	for _, tc := range []struct {
		name  string
		temp  float64
		scale float64
	}{
		{name: "half", temp: 0.5, scale: 2.25},
		{name: "double", temp: 2, scale: 1.75},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := numericalDistiller(tc.temp, tc.scale, 0.35)
			wantLoss, wantGrad := numericalDistributionExpected(student, teacher.Probabilities, 2, tc.temp, tc.scale, d.Mix)
			loss, grad, _, err := d.Loss(teacher, student, 2)
			if err != nil {
				t.Fatalf("Loss = %v, want nil", err)
			}
			assertDistributionResult(t, loss, grad, wantGrad, wantLoss)
		})
	}

	// The KL term is disabled; tiny T makes accidental KL computation non-finite, but closed KL must not turn 0*Inf into NaN.
	t.Run("closed_kl", func(t *testing.T) {
		closed := numericalDistiller(math.SmallestNonzeroFloat64, 1, 0)
		closedLoss, closedGrad, _, err := closed.Loss(
			distill.TeacherDistribution{Probabilities: []float64{0, 1}},
			[]float64{1000, -1000},
			0,
		)
		if err != nil {
			t.Fatalf("closed-KL Loss = %v, want nil", err)
		}
		assertDistributionResult(t, closedLoss, closedGrad, []float64{0, 0}, 0)
	})

	shifted := []float64{1e15, 1e15 + 2, 1e15 + 4}
	shiftedDistiller := numericalDistiller(2, 1.75, 0.35)
	shiftedWantLoss, shiftedWantGrad := numericalDistributionExpected(
		student, teacher.Probabilities, 2, shiftedDistiller.Temperature, shiftedDistiller.Scale, shiftedDistiller.Mix,
	)
	for name, input := range map[string][]float64{"base": student, "shifted": shifted} {
		t.Run("mixed_"+name, func(t *testing.T) {
			loss, grad, _, err := shiftedDistiller.Loss(teacher, input, 2)
			if err != nil {
				t.Fatalf("Loss = %v, want nil", err)
			}
			assertDistributionResult(t, loss, grad, shiftedWantGrad, shiftedWantLoss)
		})
	}
}

func TestDistributionNumericalShiftPartialAndDeclaredMap(t *testing.T) {
	base := []float64{0, 2, 4}
	shifted := []float64{1e15, 1e15 + 2, 1e15 + 4}
	teacher := distill.TeacherDistribution{Probabilities: []float64{0.2, 0.3, 0.5}}
	d := numericalDistiller(2, 1.75, 1)
	wantLoss, wantGrad := numericalDistributionExpected(base, teacher.Probabilities, -1, d.Temperature, d.Scale, d.Mix)
	loss, grad, _, err := d.Loss(teacher, shifted, -1)
	if err != nil {
		t.Fatalf("common shift Loss = %v, want nil", err)
	}
	assertDistributionResult(t, loss, grad, wantGrad, wantLoss)
	assertFloatBits(t, shifted, []float64{1e15, 1e15 + 2, 1e15 + 4}, "shifted student")

	partial := numericalDistiller(1, 1, 1)
	partial.TopK = 2
	partialTeacher := distill.TeacherDistribution{Classes: []int{0, 2}, Probabilities: []float64{0.5, 0.3}}
	partialP := []float64{0.5, 0, 0.3, 0}
	partialStudent := []float64{0, 0, 0, 0}
	partialWantLoss, partialWantGrad := numericalDistributionExpected(partialStudent, partialP, -1, 1, 1, 1)
	partialLoss, partialGrad, partialReport, err := partial.Loss(partialTeacher, partialStudent, -1)
	if err != nil {
		t.Fatalf("partial Loss = %v, want nil", err)
	}
	assertDistributionResult(t, partialLoss, partialGrad, partialWantGrad, partialWantLoss)
	if !partialReport.Partial || partialReport.TopK != 2 {
		t.Fatalf("partial report = %+v, want Partial=true and TopK=2", partialReport)
	}

	mapped := distill.DistributionDistiller{
		Temperature: 1,
		Scale:       1,
		Mix:         1,
		Alignment:   distill.Alignment{Rule: distill.AlignmentDeclaredMap, Map: []int{2, 0}},
	}
	mappedTeacher := distill.TeacherDistribution{Probabilities: []float64{0.25, 0.75}}
	mappedP := []float64{0.75, 0, 0.25}
	mappedWantLoss, mappedWantGrad := numericalDistributionExpected(base, mappedP, -1, 1, 1, 1)
	mappedLoss, mappedGrad, mappedReport, err := mapped.Loss(mappedTeacher, base, -1)
	if err != nil {
		t.Fatalf("declared_map Loss = %v, want nil", err)
	}
	assertDistributionResult(t, mappedLoss, mappedGrad, mappedWantGrad, mappedWantLoss)
	if mappedReport.Alignment.Rule != distill.AlignmentDeclaredMap {
		t.Fatalf("declared_map report = %+v, want declared_map alignment", mappedReport)
	}
}

func TestDistributionNumericalRejectsNonFiniteAndOverflow(t *testing.T) {
	d := numericalDistiller(1, 1, 1)
	teacher := distill.TeacherDistribution{Probabilities: []float64{0.5, 0.5}}
	for _, tc := range []struct {
		name  string
		value float64
	}{
		{name: "nan", value: math.NaN()},
		{name: "plus inf", value: math.Inf(1)},
		{name: "minus inf", value: math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			student := []float64{tc.value, 0}
			before := append([]float64(nil), student...)
			loss, grad, report, err := d.Loss(teacher, student, -1)
			if err == nil {
				t.Fatalf("Loss = nil, want error for %s", tc.name)
			}
			if loss != 0 || grad != nil || !reflect.DeepEqual(report, distill.DistributionReport{}) {
				t.Fatalf("error result = loss %v, grad %v, report %+v, want cleared outputs", loss, grad, report)
			}
			assertFloatBits(t, student, before, "student")
		})
	}

	t.Run("normalization_overflow", func(t *testing.T) {
		student := []float64{math.MaxFloat64, -math.MaxFloat64}
		before := append([]float64(nil), student...)
		loss, grad, report, err := d.Loss(teacher, student, -1)
		if err == nil {
			t.Fatal("Loss with an overflowing normalization difference = nil, want error")
		}
		if loss != 0 || grad != nil || !reflect.DeepEqual(report, distill.DistributionReport{}) {
			t.Fatalf("overflow result = loss %v, grad %v, report %+v, want cleared outputs", loss, grad, report)
		}
		assertFloatBits(t, student, before, "overflow student")
	})

	t.Run("scaled_result_overflow", func(t *testing.T) {
		resultOverflow := numericalDistiller(1, math.MaxFloat64, 1)
		overflowTeacher := distill.TeacherDistribution{Probabilities: []float64{0, 1}}
		overflowStudent := []float64{0, -10}
		overflowBefore := append([]float64(nil), overflowStudent...)
		loss, grad, report, err := resultOverflow.Loss(overflowTeacher, overflowStudent, -1)
		if err == nil {
			t.Fatal("Loss with an overflowing scaled result = nil, want error")
		}
		if loss != 0 || grad != nil || !reflect.DeepEqual(report, distill.DistributionReport{}) {
			t.Fatalf("scaled overflow result = loss %v, grad %v, report %+v, want cleared outputs", loss, grad, report)
		}
		assertFloatBits(t, overflowStudent, overflowBefore, "scaled overflow student")
	})
}
