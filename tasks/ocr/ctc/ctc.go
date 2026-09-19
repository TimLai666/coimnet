// Package ctc implements Connectionist Temporal Classification (CTC): a
// log-space negative log-likelihood loss with its gradient, and greedy
// decoding. The blank is fixed as class 0; target labels are the remaining
// classes 1..classes−1.
package ctc

import (
	"errors"
	"fmt"
	"math"
)

// ErrNoValidPath: no alignment of the target fits the frame count (target
// longer than T, or repeated symbols that need a blank between them and there
// are not enough frames).
var ErrNoValidPath = errors.New("ctc: no valid path")

// Options selects the policy for impossible targets. ZeroOnImpossible must be
// set explicitly: with it a target that has no valid path yields loss 0, a zero
// gradient and Report.Impossible true instead of an error.
type Options struct {
	ZeroOnImpossible bool
}

// Report describes a LossWith computation: whether the target had no valid path
// (only reachable through Options.ZeroOnImpossible) and the frame and label
// counts used.
type Report struct {
	Impossible bool
	Frames     int
	Labels     int
}

// Loss is LossWith(logits, target, Options{}) without the report.
func Loss(logits [][]float64, target []int) (loss float64, grad [][]float64, err error) {
	loss, grad, _, err = LossWith(logits, target, Options{})
	return loss, grad, err
}

// LossWith computes the CTC negative log-likelihood −log p(target | logits)
// over unnormalised logits [T][classes] (log-softmax is taken inside, in log
// space with logsumexp; T ≥ 1, classes ≥ 2, every value finite, every row the
// same width) for target (class indices in 1..classes−1; blank 0 is not allowed
// in a target) and its gradient with respect to the logits (Graves 2006 eq. 16:
// grad[t][k] = y[t][k] − (1/(p·y[t][k])) Σ_{s: l'_s = k} α_t(s)β_t(s), with α, β
// computed in log space on the blank-extended sequence l' of length
// 2·len(target)+1). Bad inputs are errors; an impossible target is
// ErrNoValidPath unless Options.ZeroOnImpossible.
func LossWith(logits [][]float64, target []int, o Options) (loss float64, grad [][]float64, r Report, err error) {
	T := len(logits)
	if T == 0 {
		return 0, nil, Report{}, errors.New("ctc: logits need at least one frame")
	}
	classes := len(logits[0])
	if classes < 2 {
		return 0, nil, Report{}, fmt.Errorf("ctc: need at least two classes (blank plus one label), got %d", classes)
	}
	for i, c := range target {
		if c < 1 || c >= classes {
			return 0, nil, Report{}, fmt.Errorf("ctc: target label %d at position %d out of range 1..%d", c, i, classes-1)
		}
	}
	for t := range logits {
		row := logits[t]
		if len(row) != classes {
			return 0, nil, Report{}, fmt.Errorf("ctc: frame %d has %d classes, want %d", t, len(row), classes)
		}
		for k, v := range row {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return 0, nil, Report{}, fmt.Errorf("ctc: frame %d class %d value %v is not finite", t, k, v)
			}
		}
	}

	L := len(target)
	S := 2*L + 1
	lp := make([]int, S)
	for i, c := range target {
		lp[2*i+1] = c
	}

	// A valid path must emit each label in order; two equal adjacent labels
	// require a blank between them, so every such pair needs one extra frame.
	minFrames := L
	for i := 0; i+1 < L; i++ {
		if target[i] == target[i+1] {
			minFrames++
		}
	}
	if minFrames > T {
		r = Report{Impossible: true, Frames: T, Labels: L}
		if o.ZeroOnImpossible {
			grad = zeroTable(T, classes)
			return 0, grad, r, nil
		}
		return 0, nil, r, ErrNoValidPath
	}

	logY := make([][]float64, T)
	for t, row := range logits {
		m := row[0]
		for _, v := range row[1:] {
			if v > m {
				m = v
			}
		}
		s := 0.0
		for _, v := range row {
			s += math.Exp(v - m)
		}
		lse := m + math.Log(s)
		logY[t] = make([]float64, classes)
		for k, v := range row {
			logY[t][k] = v - lse
		}
	}

	negInf := math.Inf(-1)
	alpha := newTable(T, S, negInf)
	alpha[0][0] = logY[0][0]
	if S > 1 {
		alpha[0][1] = logY[0][lp[1]]
	}
	for t := 1; t < T; t++ {
		for s := range lp {
			a := alpha[t-1][s]
			if s > 0 {
				a = lse2(a, alpha[t-1][s-1])
				if s > 1 && lp[s] != 0 && lp[s] != lp[s-2] {
					a = lse2(a, alpha[t-1][s-2])
				}
			}
			alpha[t][s] = logY[t][lp[s]] + a
		}
	}

	beta := newTable(T, S, negInf)
	beta[T-1][S-1] = logY[T-1][0]
	if S > 1 {
		beta[T-1][S-2] = logY[T-1][lp[S-2]]
	}
	for t := T - 2; t >= 0; t-- {
		for s := range lp {
			a := beta[t+1][s]
			if s+1 < S {
				a = lse2(a, beta[t+1][s+1])
				if s+2 < S && lp[s] != 0 && lp[s] != lp[s+2] {
					a = lse2(a, beta[t+1][s+2])
				}
			}
			beta[t][s] = logY[t][lp[s]] + a
		}
	}

	logp := alpha[T-1][S-1]
	if S > 1 {
		logp = lse2(logp, alpha[T-1][S-2])
	}
	loss = -logp

	grad = make([][]float64, T)
	for t := range grad {
		grad[t] = make([]float64, classes)
		for k := range grad[t] {
			y := math.Exp(logY[t][k])
			g := y
			for s := range lp {
				if lp[s] != k {
					continue
				}
				g -= math.Exp(alpha[t][s] + beta[t][s] - logY[t][k] - logp)
			}
			grad[t][k] = g
		}
	}

	return loss, grad, Report{Frames: T, Labels: L}, nil
}

// lse2 returns log(exp(a)+exp(b)), stable against large magnitudes; −Inf
// operands are absorbed.
func lse2(a, b float64) float64 {
	if a == math.Inf(-1) {
		return b
	}
	if b == math.Inf(-1) {
		return a
	}
	m := a
	if b > m {
		m = b
	}
	return m + math.Log(math.Exp(a-m)+math.Exp(b-m))
}

// newTable allocates a T×W table filled with v.
func newTable(T, W int, v float64) [][]float64 {
	table := make([][]float64, T)
	for t := range table {
		table[t] = make([]float64, W)
		for w := range table[t] {
			table[t][w] = v
		}
	}
	return table
}

// zeroTable allocates a T×W table of zeros.
func zeroTable(T, W int) [][]float64 {
	table := make([][]float64, T)
	for t := range table {
		table[t] = make([]float64, W)
	}
	return table
}
