package thresholdtail

import "math"

// Recover handles raw >= 7 without losing precision in 1-s, and raw <= -700
// by halving the exponent before multiplying a span once exp(-abs(raw)) is subnormal.
func Recover(raw, lo, hi float64) (base, slope float64, recovered bool) {
	if !(raw >= 7 || raw <= -700) {
		return 0, 0, false
	}
	a := math.Abs(raw)
	e := math.Exp(-a)
	den := 1 + e
	span := hi - lo
	var scaled float64
	if e >= 0x1p-1022 {
		if math.IsInf(span, 1) {
			scaled = hi*e - lo*e
		} else {
			scaled = span * e
		}
	} else {
		h := math.Exp(-a / 2)
		if math.IsInf(span, 1) {
			scaled = (hi*h - lo*h) * h
		} else {
			scaled = (span * h) * h
		}
	}
	offset := scaled / den
	if raw >= 0 {
		base = hi - offset
	} else {
		base = lo + offset
	}
	return base, offset / den, true
}
