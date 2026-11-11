package dynamics

import "math"

// stepSigmoidTanhSSE runs stepSigmoidTanh's update (without the voltage gate
// term) four neurons at a time with SSE2, which every amd64 processor has.
// n is a multiple of 4 and every pointer addresses at least n values. It
// performs the same float32 operations in the same order as the Go loop, so
// the results are bit-identical.
//
//go:noescape
func stepSigmoidTanhSSE(drive, v, y, bias, gain, gb, alpha *float32, k *[sseConstants][4]uint32, n int)

const haveSSEStep = true

// sseConstants is the number of four-lane constants stepSigmoidTanhSSE reads;
// their order is the one infer32_amd64.s addresses by offset.
const sseConstants = 29

var sseTable = func() (t [sseConstants][4]uint32) {
	f := func(x float32) uint32 { return math.Float32bits(x) }
	vals := [sseConstants]uint32{
		0x7fffffff, 0x80000000, f(-87), f(1.44269504088896341), f(.5), f(0.693359375), f(-2.12194440e-4),
		f(1.9875691500e-4), f(1.3981999507e-3), f(8.3334519073e-3), f(4.1665795894e-2), f(1.6666665459e-1), f(5.0000001201e-1),
		f(1), f(2), 127, f(tanhClamp), f(-tanhClamp),
		f(4.89352455891786e-03), f(6.37261928875436e-04), f(1.48572235717979e-05), f(5.12229709037114e-08),
		f(-8.60467152213735e-11), f(2.00018790482477e-13), f(-2.76076847742355e-16),
		f(4.89352518554385e-03), f(2.26843463243900e-03), f(1.18534705686654e-04), f(1.19825839466702e-06),
	}
	for i, x := range vals {
		t[i] = [4]uint32{x, x, x, x}
	}
	return t
}()
