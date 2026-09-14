package experiment

import (
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// delayedLIFCore fixes the spiking counterpart of NewDelayedTrainer's topology:
// the same three-neuron chain with one self connection, here read through the
// delayed synaptic trace. Edge 0 carries the encoded pulse from neuron 0 to
// neuron 1, edge 1 is neuron 1's self connection at delay 1 (the memory this
// task needs), and edge 2 drives the single readout neuron 2.
//
// Every setting below is fixed for this fixture and chosen so that the
// untrained model spikes on part of the pulse distribution and stays silent on
// the rest, which is the only regime in which a trainable threshold can be
// observed:
//
//   - DT = 1 with log_tau = 0 (tau = 1): alpha = -expm1(-1) = 0.632 turns one
//     step of drive into membrane voltage, and lambda = exp(-1) = 0.368 leaks
//     the rest away, so a five-step episode is neither instantaneous nor frozen.
//   - TauSyn = 1: the synaptic trace the readout observes decays by
//     kappa = exp(-1) = 0.368 per step, so one event is still visible two steps
//     later without the trace saturating.
//   - ThetaMin = 0.05 and ThetaMax = 1: the whole trainable threshold range
//     stays inside this task's drive scale, and a strictly positive minimum
//     means a negative pulse can never make a neuron fire. That keeps the
//     holdout spike rate below one by construction.
//   - VReset = -0.5 with RefractorySteps = 1: a neuron that fires holds
//     v_reset for the next step and ignores that step's drive, so events stay
//     separated inside a five-step episode.
//   - Surrogate fast_sigmoid with Scale = 2: the declared reverse-pass rule
//     psi(u) = 1/(1+2|u|)^2, the only kind the core supports.
//   - Adaptation disabled: this fixture isolates the trainable base threshold
//     from the fixed short-term adaptation mechanism, which has its own
//     acceptance evidence in the dynamics package.
func delayedLIFCore() dynamics.LIFConfig {
	return dynamics.LIFConfig{
		Nodes: 3, Sources: []int{0, 1, 1}, Targets: []int{1, 1, 2}, Delays: []int{0, 1, 0},
		DT: 1, TauSyn: 1, ThetaMin: .05, ThetaMax: 1, VReset: -.5, RefractorySteps: 1,
		Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
}

// NewDelayedLIFTrainer builds the spiking counterpart of NewDelayedTrainer on
// the unchanged five-step delayed-pulse generator. The caller selects the
// trainable groups, so the same constructor serves the trained, threshold-only
// and fully frozen controls of the threshold example.
//
// Initialization reuses NewDelayedTrainer's deterministic SplitMix64 mixer, so
// the seed dependence stays explicit: only the first edge weight varies, as
// 1.2 + mix(seed)%100/1000. Every other value is fixed, and the whole set is
// chosen to make the base threshold the limiting parameter of this fixture:
//
//   - Encoder 2 into neuron 0 only: the encoded pulse is 2a, so a positive
//     pulse (a in [0.4,1]) always drives neuron 0 above any reachable
//     threshold while a negative pulse can never reach the positive
//     theta_min. Neuron 0 therefore keeps firing on part of the data whatever
//     the threshold group learns, and the holdout spike rate cannot collapse
//     to zero.
//   - Edge weights 1.2 (neuron 0 to 1), 0.25 (neuron 1's delayed self edge)
//     and 0.7 (neuron 1 to the readout neuron 2): one event on neuron 0 puts
//     neuron 1 well above threshold, so the pulse propagates as an event chain
//     and reaches neuron 2 two steps later.
//   - Bias 0.3 on neuron 2 only: the readout neuron carries a tonic drive that
//     is independent of the observation. Its membrane approaches 0.3 on every
//     episode, including the negative pulses whose target is negative and
//     therefore unreachable for a non-negative synaptic trace.
//   - theta_raw = -2 for every neuron, that is
//     theta_base = 0.05+0.95*sigmoid(-2) = 0.163: below the tonic drive. The
//     untrained readout neuron fires from its own bias on every episode,
//     including negative pulses, which is exactly the regime a trainable
//     threshold has to correct. Raising theta_base above the tonic membrane
//     (about 0.3) and below the driven membrane (about 0.73) makes neuron 2
//     fire only when the delayed pulse actually arrives.
//
// This initialization is deliberately mistuned in the threshold direction. It
// is a numerical fixture for the threshold mechanism, not a claim about a
// biological firing regime, and the untrained model still spikes on part of
// the neuron-steps rather than on all of them.
func NewDelayedLIFTrainer(seed uint64, rate float64, trainable learning.Trainable) (*learning.Trainer, error) {
	core := delayedLIFCore()
	c := learning.Config{LIF: &core, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2}}
	p := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{1.2 + float64(mix(seed)%100)/1000, .25, .7},
			Bias:    []float64{0, 0, .3},
			LogTau:  []float64{0, 0, 0},
		},
		ThetaRaw: []float64{-2, -2, -2},
		Encoder:  []float64{2, 0, 0},
		Readout:  []float64{1},
	}
	o := learning.DefaultOptions()
	o.LearningRate = rate
	o.Trainable = trainable
	return learning.NewTrainer(c, p, o)
}
