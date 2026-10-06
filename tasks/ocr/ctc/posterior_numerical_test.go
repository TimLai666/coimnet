package ctc_test

import (
	"github.com/TimLai666/coimnet/tasks/ocr/ctc"
	"testing"
)

// With two identical frames and one rare target, the two dominant paths
// place that target on either frame with equal conditional probability.
// Losing ln(2) in path merging used to return [[0,-1],[0,-1]], whose rows
// do not conserve posterior mass. A float64 precision failure must be an
// error, never a successful finite but incorrect gradient.
func TestCTCNumericalRejectsLostPosteriorMass(t *testing.T) {
	logits := [][]float64{{0, -1e16}, {0, -1e16}}
	loss, grad, report, err := ctc.LossWith(logits, []int{1}, ctc.Options{ZeroOnImpossible: true})
	if err == nil {
		t.Fatalf("lost posterior mass: loss=%g grad=%v report=%+v; want numerical error", loss, grad, report)
	}
	if loss != 0 || grad != nil || report != (ctc.Report{}) {
		t.Fatalf("numerical error must clear outputs: loss=%g grad=%v report=%+v", loss, grad, report)
	}
	assertCTCBits(t, logits, [][]float64{{0, -1e16}, {0, -1e16}}, "posterior logits")
}
