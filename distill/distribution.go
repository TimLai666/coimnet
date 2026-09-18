package distill

import (
	"errors"
	"fmt"
	"math"
)

const (
	// AlignmentIdentical maps teacher class i to student class i; it requires
	// equal vocab hashes and equal class counts.
	AlignmentIdentical = "identical"
	// AlignmentDeclaredMap maps teacher classes through an explicit per-class
	// student class list.
	AlignmentDeclaredMap = "declared_map"
)

// ErrIncompatibleAlignment means the teacher and student class spaces cannot
// be aligned under the declared rule.
var ErrIncompatibleAlignment = errors.New("distill: teacher and student class spaces are not aligned")

// errInvalidTeacherDistribution is returned when teacher probabilities do not
// form a valid full (sums to 1, within 1e-9) or partial (sums to at most 1)
// distribution.
var errInvalidTeacherDistribution = errors.New("distill: teacher distribution invalid: full must sum to 1 (±1e-9), partial to at most 1")

// Alignment says how teacher classes map onto student classes. identical needs
// equal vocab hashes and equal class counts (teacher i → student i);
// declared_map needs Map with one student class per teacher class (len(Map) ==
// teacher classes, every value in [0, student classes)). Anything else is
// ErrIncompatibleAlignment.
type Alignment struct {
	TeacherVocabHash string `json:"teacher_vocab_hash"`
	StudentVocabHash string `json:"student_vocab_hash"`
	Rule             string `json:"rule"`
	Map              []int  `json:"map,omitempty"`
}

// Validate checks the alignment rule against the class counts. Unknown rules
// and rules whose requirements are not met return ErrIncompatibleAlignment.
func (a Alignment) Validate(teacherClasses, studentClasses int) error {
	if teacherClasses < 0 || studentClasses < 0 {
		return ErrIncompatibleAlignment
	}
	switch a.Rule {
	case AlignmentIdentical:
		if a.TeacherVocabHash != a.StudentVocabHash || teacherClasses != studentClasses {
			return ErrIncompatibleAlignment
		}
		return nil
	case AlignmentDeclaredMap:
		if len(a.Map) != teacherClasses {
			return ErrIncompatibleAlignment
		}
		for _, v := range a.Map {
			if v < 0 || v >= studentClasses {
				return ErrIncompatibleAlignment
			}
		}
		return nil
	default:
		return ErrIncompatibleAlignment
	}
}

// student returns the student class that a teacher class maps to under the
// alignment. Callers must have validated the alignment and the teacher class
// first.
func (a Alignment) student(teacherClass int) int {
	if a.Rule == AlignmentIdentical {
		return teacherClass
	}
	return a.Map[teacherClass]
}

// TeacherDistribution is the teacher's probabilities over its classes. Full:
// Probabilities has teacherClasses entries summing to 1 (1e-9). Partial
// (top-k): Classes lists the k teacher classes the probabilities belong to and
// they sum to at most 1; they are used exactly as given, never renormalized,
// and the loss reports Partial.
type TeacherDistribution struct {
	Probabilities []float64 `json:"probabilities"`
	Classes       []int     `json:"classes,omitempty"`
}

// DistributionDistiller distills a teacher class distribution into the
// student's class logits.
type DistributionDistiller struct {
	Temperature float64   `json:"temperature"`
	Scale       float64   `json:"scale"`
	Mix         float64   `json:"mix"`
	TopK        int       `json:"top_k"`
	Alignment   Alignment `json:"alignment"`
}

// Validate checks that T > 0, Scale > 0, 0 ≤ Mix ≤ 1 and TopK ≥ 0, and that
// the float parameters are finite.
func (d DistributionDistiller) Validate() error {
	if !finite(d.Temperature) || !finite(d.Scale) || !finite(d.Mix) {
		return errors.New("distill: distribution distiller parameters must be finite")
	}
	if d.Temperature <= 0 {
		return errors.New("distill: temperature must be positive")
	}
	if d.Scale <= 0 {
		return errors.New("distill: scale must be positive")
	}
	if d.Mix < 0 || d.Mix > 1 {
		return errors.New("distill: mix must be within [0, 1]")
	}
	if d.TopK < 0 {
		return errors.New("distill: top-k must be non-negative")
	}
	return nil
}

// DistributionReport records the parameters a Loss call was evaluated under,
// including whether the teacher only gave the top-k classes.
type DistributionReport struct {
	Temperature float64   `json:"temperature"`
	Scale       float64   `json:"scale"`
	Mix         float64   `json:"mix"`
	TopK        int       `json:"top_k"`
	Partial     bool      `json:"partial_distribution"`
	Alignment   Alignment `json:"alignment"`
}

// Loss is Mix*T²*Scale*KL + (1−Mix)*CE with
//
//	q = softmax(student/T) on the student classes, p the teacher probabilities
//	mapped through the alignment, KL = Σ_i p_i (log p_i − log q_{a(i)}) over the
//	given teacher classes (full or partial, p as given), CE = −log softmax(student)[label].
//
// Terms with p_i == 0 contribute 0. Everything goes through log-sum-exp so
// logits of 1e3 stay finite. grad is dLoss/dstudent. label < 0 skips CE (Mix
// must then be 1).
func (d DistributionDistiller) Loss(teacher TeacherDistribution, student []float64, label int) (loss float64, grad []float64, report DistributionReport, err error) {
	if err := d.Validate(); err != nil {
		return 0, nil, DistributionReport{}, err
	}
	studentClasses := len(student)
	if studentClasses == 0 {
		return 0, nil, DistributionReport{}, errors.New("distill: student logits must be non-empty")
	}
	teacherClasses := len(teacher.Probabilities)
	if teacherClasses == 0 {
		return 0, nil, DistributionReport{}, errors.New("distill: teacher distribution must be non-empty")
	}
	partial := len(teacher.Classes) > 0
	if partial && len(teacher.Classes) != teacherClasses {
		return 0, nil, DistributionReport{}, errors.New("distill: teacher classes and probabilities must have equal length")
	}

	classToStudent := make([]int, teacherClasses)
	if !partial {
		if err := d.Alignment.Validate(teacherClasses, studentClasses); err != nil {
			return 0, nil, DistributionReport{}, err
		}
		for i := range classToStudent {
			classToStudent[i] = d.Alignment.student(i)
		}
	} else {
		switch d.Alignment.Rule {
		case AlignmentIdentical:
			if d.Alignment.TeacherVocabHash != d.Alignment.StudentVocabHash {
				return 0, nil, DistributionReport{}, ErrIncompatibleAlignment
			}
		case AlignmentDeclaredMap:
			for _, v := range d.Alignment.Map {
				if v < 0 || v >= studentClasses {
					return 0, nil, DistributionReport{}, ErrIncompatibleAlignment
				}
			}
		default:
			return 0, nil, DistributionReport{}, ErrIncompatibleAlignment
		}
		for i, tc := range teacher.Classes {
			if tc < 0 {
				return 0, nil, DistributionReport{}, ErrIncompatibleAlignment
			}
			switch d.Alignment.Rule {
			case AlignmentIdentical:
				if tc >= studentClasses {
					return 0, nil, DistributionReport{}, ErrIncompatibleAlignment
				}
			case AlignmentDeclaredMap:
				if tc >= len(d.Alignment.Map) {
					return 0, nil, DistributionReport{}, ErrIncompatibleAlignment
				}
			}
			classToStudent[i] = d.Alignment.student(tc)
		}
	}

	sum := 0.0
	for _, p := range teacher.Probabilities {
		if !finite(p) || p < 0 {
			return 0, nil, DistributionReport{}, errInvalidTeacherDistribution
		}
		sum += p
	}
	if partial {
		if sum > 1+1e-9 {
			return 0, nil, DistributionReport{}, errInvalidTeacherDistribution
		}
	} else if math.Abs(sum-1) > 1e-9 {
		return 0, nil, DistributionReport{}, errInvalidTeacherDistribution
	}

	invT := 1 / d.Temperature
	x := make([]float64, studentClasses)
	for j, s := range student {
		x[j] = s * invT
	}
	q := softmax(x)
	logZ := lse(x)

	kl := 0.0
	pTilde := make([]float64, studentClasses)
	m := 1.0
	if partial {
		m = sum
	}
	for i, p := range teacher.Probabilities {
		j := classToStudent[i]
		pTilde[j] += p
		if p > 0 {
			// log q_j as x_j − logsumexp(x), so an underflowed q_j never
			// turns into −Inf.
			kl += p * (math.Log(p) - (x[j] - logZ))
		}
	}

	loss = d.Mix * d.Temperature * d.Temperature * d.Scale * kl
	grad = make([]float64, studentClasses)
	klGrad := d.Mix * d.Temperature * d.Temperature * d.Scale * invT
	for j := range grad {
		grad[j] = klGrad * (m*q[j] - pTilde[j])
	}

	if label >= 0 {
		if label >= studentClasses {
			return 0, nil, DistributionReport{}, fmt.Errorf("distill: label %d out of range for %d student classes", label, studentClasses)
		}
		if d.Mix < 1 {
			loss += (1 - d.Mix) * (lse(student) - student[label])
			q1 := softmax(student)
			for j := range grad {
				dq := q1[j]
				if j == label {
					dq--
				}
				grad[j] += (1 - d.Mix) * dq
			}
		}
	}

	return loss, grad, DistributionReport{
		Temperature: d.Temperature,
		Scale:       d.Scale,
		Mix:         d.Mix,
		TopK:        d.TopK,
		Partial:     partial,
		Alignment:   d.Alignment,
	}, nil
}

// lse returns the log-sum-exp of x.
func lse(x []float64) float64 {
	m := x[0]
	for _, v := range x[1:] {
		if v > m {
			m = v
		}
	}
	s := 0.0
	for _, v := range x {
		s += math.Exp(v - m)
	}
	return m + math.Log(s)
}

// softmax returns exp(x) normalized by the log-sum-exp of x.
func softmax(x []float64) []float64 {
	m := x[0]
	for _, v := range x[1:] {
		if v > m {
			m = v
		}
	}
	s := 0.0
	for _, v := range x {
		s += math.Exp(v - m)
	}
	q := make([]float64, len(x))
	for i, v := range x {
		q[i] = math.Exp(v-m) / s
	}
	return q
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
