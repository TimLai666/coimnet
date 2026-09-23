package experiment

import (
	"github.com/TimLai666/coimnet/internal/delayedfixture"
	"github.com/TimLai666/coimnet/learning"
)

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
	return delayedfixture.NewDelayedLIFTrainer(seed, rate, trainable)
}
