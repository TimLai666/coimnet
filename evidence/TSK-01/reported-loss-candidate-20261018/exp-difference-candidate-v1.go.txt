package categorical

import "math"

// expDifference represents exp(left-right) as mantissa*2^exponent.
// Callers provide finite operands with left <= right. Zero is returned only
// when no finite factor and MaxInt64-class aggregate can rescue the value.
func expDifference(left, right float64) (float64, int) {
	gap := left - right
	if gap < -1600 {
		return 0, 0
	}

	virtual := gap - left
	residual := (left - (gap - virtual)) - (right + virtual)
	if gap >= -700 && residual == 0 {
		return math.Frexp(math.Exp(gap))
	}

	n := int(math.Round(gap / math.Ln2))
	const ln2Hi = 0x1.62e42fefa39efp-1
	const ln2Lo = 0x1.abc9e3b39803fp-56
	remainder := math.FMA(-float64(n), ln2Hi, gap)
	remainder = math.FMA(-float64(n), ln2Lo, remainder)
	remainder += residual
	mantissa, exponent := math.Frexp(math.Exp(remainder))
	return mantissa, exponent + n
}
