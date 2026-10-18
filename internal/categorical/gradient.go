// Package categorical provides the internal one-hot classification derivative.
package categorical

import (
	"fmt"
	"math"
)

// Gradient overwrites fresh dst with factor/divisor times the derivative of
// cross-entropy. Logits must be finite and nonempty, target must be in range,
// factor must be finite and nonnegative, and divisor must be finite and >= 1.
// dst must match logits in length and must not alias logits. Invalid arguments
// return an error before changing dst.
func Gradient(dst, logits []float64, target int, factor, divisor float64) error {
	if len(logits) == 0 {
		return fmt.Errorf("categorical: logits must be non-empty")
	}
	if len(dst) != len(logits) {
		return fmt.Errorf("categorical: dst length %d does not match logits length %d", len(dst), len(logits))
	}
	if target < 0 || target >= len(logits) {
		return fmt.Errorf("categorical: target %d out of range", target)
	}
	if math.IsNaN(factor) || math.IsInf(factor, 0) || factor < 0 {
		return fmt.Errorf("categorical: factor must be finite and non-negative")
	}
	if math.IsNaN(divisor) || math.IsInf(divisor, 0) || divisor < 1 {
		return fmt.Errorf("categorical: divisor must be finite and >= 1")
	}
	for i, logit := range logits {
		if math.IsNaN(logit) || math.IsInf(logit, 0) {
			return fmt.Errorf("categorical: logits[%d] must be finite", i)
		}
	}
	if factor == 0 || len(logits) == 1 {
		for i := range dst {
			dst[i] = 0
		}
		return nil
	}

	maximum := logits[0]
	winner := 0
	for i := 1; i < len(logits); i++ {
		if logits[i] > maximum {
			maximum = logits[i]
			winner = i
		}
	}

	// Smaller relative terms cannot affect an aggregate of at most MaxInt64 classes.
	const relativeGapCutoff = -700.0

	// Use the largest non-winner gap as a common aggregate scale. Relative
	// terms are ordinary-sized, while the absolute scale retains the tail
	// exponent for the final factor/divisor combination.
	tailMaximum := math.Inf(-1)
	for i, logit := range logits {
		if i != winner && logit > tailMaximum {
			tailMaximum = logit
		}
	}
	tailScaleMantissa, tailScaleExponent := expDifference(tailMaximum, maximum)
	tailSum, tailCompensation := 0.0, 0.0
	nonTargetTailSum, nonTargetTailCompensation := 0.0, 0.0
	for i, logit := range logits {
		if i == winner {
			continue
		}
		relativeGap := logit - tailMaximum
		term := 0.0
		if relativeGap >= relativeGapCutoff {
			termMantissa, termExponent := expDifference(logit, tailMaximum)
			term = math.Ldexp(termMantissa, termExponent)
		}
		adjusted := term - tailCompensation
		next := tailSum + adjusted
		tailCompensation = (next - tailSum) - adjusted
		tailSum = next
		if i != target {
			adjusted = term - nonTargetTailCompensation
			next = nonTargetTailSum + adjusted
			nonTargetTailCompensation = (next - nonTargetTailSum) - adjusted
			nonTargetTailSum = next
		}
	}
	if math.IsNaN(tailSum) || math.IsInf(tailSum, 0) || math.IsNaN(nonTargetTailSum) || math.IsInf(nonTargetTailSum, 0) {
		return fmt.Errorf("categorical: non-finite tail aggregate")
	}

	// Convert an aggregate to an ordinary float only for the normalizer. The
	// gradient path below keeps the same representation through factor/divisor.
	toFloat := func(relative, scaleMantissa float64, scaleExponent int) (float64, error) {
		if relative == 0 || scaleMantissa == 0 {
			return 0, nil
		}
		relativeMantissa, relativeExponent := math.Frexp(relative)
		value := math.Ldexp(relativeMantissa*scaleMantissa, relativeExponent+scaleExponent)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, fmt.Errorf("categorical: non-finite tail mass")
		}
		return value, nil
	}
	tailMass, err := toFloat(tailSum, tailScaleMantissa, tailScaleExponent)
	if err != nil {
		return err
	}
	normalizer := 1 + tailMass
	if math.IsNaN(normalizer) || math.IsInf(normalizer, 0) || normalizer <= 0 {
		return fmt.Errorf("categorical: non-finite normalizer")
	}
	inverseNormalizer := 1 / normalizer
	if math.IsNaN(inverseNormalizer) || math.IsInf(inverseNormalizer, 0) || inverseNormalizer <= 0 {
		return fmt.Errorf("categorical: non-finite inverse normalizer")
	}

	needsPreciseUnderflow := false
	inverseMantissa, inverseExponent := math.Frexp(inverseNormalizer)
	factorMantissa, factorExponent := math.Frexp(factor)
	divisorMantissa, divisorExponent := math.Frexp(divisor)
	applyScaled := func(relative, scaleMantissa float64, scaleExponent int) (float64, error) {
		if relative == 0 || scaleMantissa == 0 {
			return 0, nil
		}
		relativeMantissa, relativeExponent := math.Frexp(relative)
		mantissa := relativeMantissa * scaleMantissa
		mantissa *= inverseMantissa
		mantissa *= factorMantissa
		mantissa /= divisorMantissa
		exponent := relativeExponent + scaleExponent + inverseExponent + factorExponent - divisorExponent
		result := math.Ldexp(mantissa, exponent)
		if result <= math.SmallestNonzeroFloat64 && nearHalfMinimum(mantissa, exponent) {
			needsPreciseUnderflow = true
		}
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return 0, fmt.Errorf("categorical: final gradient is non-finite")
		}
		return result, nil
	}
	applyValue := func(value float64) (float64, error) {
		return applyScaled(value, 1, 0)
	}

	for i, logit := range logits {
		if i == target && target != winner {
			continue
		}
		var value float64
		if i == winner {
			if target == winner {
				value, err = applyScaled(tailSum, tailScaleMantissa, tailScaleExponent)
			} else {
				value, err = applyValue(1)
			}
		} else {
			classMantissa, classExponent := expDifference(logit, maximum)
			if classMantissa != 0 {
				value, err = applyScaled(1, classMantissa, classExponent)
			}
		}
		if err != nil {
			return err
		}
		if i == target {
			dst[i] = -value
		} else {
			dst[i] = value
		}
	}

	if target != winner {
		nonTargetTailMass, err := toFloat(nonTargetTailSum, tailScaleMantissa, tailScaleExponent)
		if err != nil {
			return err
		}
		value, err := applyValue(1 + nonTargetTailMass)
		if err != nil {
			return err
		}
		dst[target] = -value
	}

	if needsPreciseUnderflow {
		preciseGradient(dst, logits, target, factor, divisor)
	}
	return nil
}
