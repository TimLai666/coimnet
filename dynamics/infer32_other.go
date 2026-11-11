//go:build !amd64

package dynamics

const haveSSEStep = false

const sseConstants = 1

var sseTable [sseConstants][4]uint32

func stepSigmoidTanhSSE(drive, v, y, bias, gain, gb, alpha *float32, k *[sseConstants][4]uint32, n int) {
	panic("stepSigmoidTanhSSE needs amd64")
}
