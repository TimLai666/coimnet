package distill

import (
	"context"
	"errors"
	"fmt"

	"github.com/TimLai666/coimnet/learning"
)

// Student is the learning-side handle distillation trains through: Predict
// gives the student's last-row logits, StepFrom takes dL/dy on the last row.
type Student interface {
	Predict(ctx context.Context, input [][]float64) ([]float64, error)
	StepFrom(ctx context.Context, input [][]float64, upstream [][]float64) (learning.StepResult, error)
}

// StepReport records one distillation step: the distribution loss this step
// used, the parameters the distiller evaluated under, whether the teacher only
// covered part of the classes, and whether the student's and teacher's argmax
// agreed on this input.
type StepReport struct {
	Loss    float64
	Report  DistributionReport
	Partial bool
	Ratio   float64
}

// StepDistribution runs one distillation step: logits := student.Predict(input);
// loss, grad := d.Loss(teacher, logits, label); upstream is len(input) rows of
// zeros except the last row = grad; then student.StepFrom(ctx, input, upstream).
func StepDistribution(ctx context.Context, student Student, d DistributionDistiller, input [][]float64, teacher TeacherDistribution, label int) (StepReport, error) {
	var zero StepReport
	if ctx == nil {
		return zero, errors.New("distill: nil context")
	}
	if student == nil {
		return zero, errors.New("distill: nil student")
	}
	logits, err := student.Predict(ctx, input)
	if err != nil {
		return zero, err
	}
	loss, grad, report, err := d.Loss(teacher, logits, label)
	if err != nil {
		return zero, err
	}
	if len(input) == 0 {
		return zero, errors.New("distill: empty input sequence")
	}
	upstream := make([][]float64, len(input))
	for i := range upstream {
		upstream[i] = make([]float64, len(grad))
	}
	copy(upstream[len(upstream)-1], grad)
	if _, err := student.StepFrom(ctx, input, upstream); err != nil {
		return zero, err
	}
	return StepReport{Loss: loss, Report: report, Partial: report.Partial, Ratio: argmaxAgreement(logits, teacher, report)}, nil
}

// StepLabel is the label-only case: Mix 0 through the same path (a distiller
// with Mix 0, Temperature 1, Scale 1 and identical alignment over the
// student's classes); the teacher distribution is ignored.
func StepLabel(ctx context.Context, student Student, input [][]float64, label int, classes int) (StepReport, error) {
	var zero StepReport
	if ctx == nil {
		return zero, errors.New("distill: nil context")
	}
	if student == nil {
		return zero, errors.New("distill: nil student")
	}
	if classes <= 0 {
		return zero, fmt.Errorf("distill: classes must be positive, got %d", classes)
	}
	alignment := Alignment{
		Rule:             AlignmentIdentical,
		TeacherVocabHash: labelVocabHash,
		StudentVocabHash: labelVocabHash,
	}
	p := make([]float64, classes)
	for i := range p {
		p[i] = 1 / float64(classes)
	}
	rep, err := StepDistribution(ctx, student, DistributionDistiller{Temperature: 1, Scale: 1, Mix: 0, Alignment: alignment}, input, TeacherDistribution{Probabilities: p}, label)
	if err != nil {
		return zero, err
	}
	// The label-only case has no teacher distribution: evaluate agreement from
	// the pre-step logits against the label itself.
	if len(input) > 0 {
		if logits, perr := student.Predict(ctx, input); perr == nil {
			rep.Ratio = ratioFromStudent(logits, label)
		}
	}
	return rep, nil
}

// Agreement is the fraction of examples whose student argmax equals the
// teacher argmax, computed with Predict only (no step).
func Agreement(ctx context.Context, student Student, inputs [][][]float64, teacherArgmax []int) (float64, error) {
	if ctx == nil {
		return 0, errors.New("distill: nil context")
	}
	if student == nil {
		return 0, errors.New("distill: nil student")
	}
	if len(inputs) != len(teacherArgmax) {
		return 0, fmt.Errorf("distill: %d inputs for %d teacher argmaxes", len(inputs), len(teacherArgmax))
	}
	if len(inputs) == 0 {
		return 0, errors.New("distill: empty input set")
	}
	match := 0
	for i, in := range inputs {
		logits, err := student.Predict(ctx, in)
		if err != nil {
			return 0, err
		}
		if argmax(logits) == teacherArgmax[i] {
			match++
		}
	}
	return float64(match) / float64(len(inputs)), nil
}

// labelVocabHash is the student-class vocabulary the label-only path declares,
// shared by the step and the student side of its identical alignment.
const labelVocabHash = "coimnet-distill-label/v1"

// argmaxAgreement is 1 when the student's argmax equals the teacher
// distribution's argmax mapped to a student class.
func argmaxAgreement(logits []float64, teacher TeacherDistribution, report DistributionReport) float64 {
	t := argmax(teacher.Probabilities)
	if t < 0 {
		return 0
	}
	return ratioFromStudent(logits, alignedTeacherArgmax(t, teacher, report))
}

// alignedTeacherArgmax maps teacher class t through the report's alignment.
func alignedTeacherArgmax(t int, teacher TeacherDistribution, report DistributionReport) int {
	if report.Alignment.Rule == AlignmentIdentical {
		return t
	}
	return report.Alignment.Map[t]
}

// ratioFromStudent is 1 when the student argmax equals the teacher argmax.
func ratioFromStudent(logits []float64, target int) float64 {
	if argmax(logits) == target {
		return 1
	}
	return 0
}

// argmax returns the index of the first largest value, or −1 when empty.
func argmax(v []float64) int {
	if len(v) == 0 {
		return -1
	}
	best := 0
	for i := 1; i < len(v); i++ {
		if v[i] > v[best] {
			best = i
		}
	}
	return best
}
