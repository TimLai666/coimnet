package bench

import "github.com/TimLai666/coimnet/learning"

// CountParameters is the number of stored values a learning model reads at
// inference: core weights, biases and time constants, LIF thresholds, the
// encoder and the readout.
func CountParameters(p learning.Parameters) int64 {
	return int64(len(p.Core.Weights) + len(p.Core.Bias) + len(p.Core.LogTau) + len(p.ThetaRaw) + len(p.Encoder) + len(p.Readout) + len(p.Core.GateGain) + len(p.Core.GateBias))
}

// CountTrainable is the number of those values o lets training change: each
// group its Trainable flag enables, narrowed by Masks where they are given
// (edge masks for weights, node masks for bias and time constant).
func CountTrainable(p learning.Parameters, o learning.Options) int64 {
	var edges, nodes []bool
	if o.Masks != nil {
		edges, nodes = o.Masks.Edges, o.Masks.Nodes
	}
	count := func(on bool, n int, mask []bool) int64 {
		if !on {
			return 0
		}
		if mask == nil {
			return int64(n)
		}
		var k int64
		for i := 0; i < n && i < len(mask); i++ {
			if mask[i] {
				k++
			}
		}
		return k
	}
	t := o.Trainable
	return count(t.Weights, len(p.Core.Weights), edges) + count(t.Bias, len(p.Core.Bias), nodes) +
		count(t.Tau, len(p.Core.LogTau), nodes) + count(t.Theta, len(p.ThetaRaw), nil) +
		count(t.Encoder, len(p.Encoder), nil) + count(t.Readout, len(p.Readout), nil) +
		count(t.Gate, len(p.Core.GateGain), nodes) + count(t.Gate, len(p.Core.GateBias), nodes)
}
