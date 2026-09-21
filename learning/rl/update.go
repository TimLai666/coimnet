package rl

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/TimLai666/coimnet/learning"
)

// Rollout is what one collection wrote down before any update: the policy
// version (SHA-256 hex of the model configuration and flattened base
// parameters at collection time), the initial recurrent state and the
// fast/chemical parts the individual had when the first observation was seen,
// and the transitions.
type Rollout struct {
	PolicyVersion   string                 `json:"policy_version"`
	InitialNeural   learning.NeuralState   `json:"initial_neural"`
	InitialPlastic  *learning.PlasticPart  `json:"initial_plastic,omitempty"`
	InitialChemical *learning.ChemicalPart `json:"initial_chemical,omitempty"`
	Steps           []Transition           `json:"steps"`
}

// policyVersionDomain separates this digest from any other SHA-256 over the
// same bytes, so an empty parameter set hashes to something that belongs to
// this function rather than to the digest of nothing. Version 2 includes the
// complete network configuration as well as every base parameter group; a
// rollout collected under the old parameter-only identity must not be reused.
const policyVersionDomain = "coimnet-policy-version/v2"

// PolicyVersion returns the version string of an individual's current policy:
// SHA-256 over the domain tag, the canonical JSON configuration, and then each
// base parameter group in the order weights, bias, log_tau, theta_raw, encoder
// and readout. Each byte slice and value is length/bit encoded, so two versions
// are equal only when both configuration and parameters are equal bit for bit.
// Fast weights, chemical state and optimizer moments are deliberately outside
// it; the rollout carries those separately and Update rejects unsupported fast
// mechanisms for the StepFrom path.
func PolicyVersion(ind *learning.Individual) string {
	return policyVersionSnapshot(ind.Snapshot())
}

func policyVersionSnapshot(s learning.IndividualSnapshot) string {
	h := sha256.New()
	h.Write([]byte(policyVersionDomain))
	var buf [8]byte
	config, err := json.Marshal(s.Config)
	if err != nil {
		// Config contains only JSON-native values, so this is unreachable for a
		// valid learning.Config. Keep the public no-error API deterministic if a
		// future field introduces an unsupported value.
		config = []byte("config-marshal-error:" + err.Error())
	}
	binary.LittleEndian.PutUint64(buf[:], uint64(len(config)))
	h.Write(buf[:])
	h.Write(config)
	for _, group := range [][]float64{s.Parameters.Core.Weights, s.Parameters.Core.Bias, s.Parameters.Core.LogTau, s.Parameters.ThetaRaw, s.Parameters.Encoder, s.Parameters.Readout} {
		binary.LittleEndian.PutUint64(buf[:], uint64(len(group)))
		h.Write(buf[:])
		for _, v := range group {
			binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
			h.Write(buf[:])
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// PPOReport summarises one Update call. The four loss means, the ratio mean
// and the clipped fraction are averaged over the scored transitions of the
// last epoch; a burn-in row is never scored, so it enters Transitions but none
// of the means.
type PPOReport struct {
	Rollouts            int     `json:"rollouts"`
	Transitions         int     `json:"transitions"`
	Epochs              int     `json:"epochs"`
	MeanLoss            float64 `json:"mean_loss"`
	MeanPolicy          float64 `json:"mean_policy"`
	MeanValue           float64 `json:"mean_value"`
	MeanEntropy         float64 `json:"mean_entropy"`
	MeanRatio           float64 `json:"mean_ratio"`
	ClippedFraction     float64 `json:"clipped_fraction"`
	VersionChecks       int     `json:"version_checks"`
	PolicyVersionBefore string  `json:"policy_version_before"`
	PolicyVersionAfter  string  `json:"policy_version_after"`
}

// ErrStalePolicy is the refusal of a rollout whose policy version or initial
// fast/chemical parts no longer match the individual being updated. Such a
// rollout is never used, not even in part.
var ErrStalePolicy = errors.New("rl: rollout was collected under a different policy version or initial state")

// ErrUnsupportedMechanism is returned before any candidate gradient is built
// when an individual carries a mechanism that Trainer.StepFrom cannot replay.
var ErrUnsupportedMechanism = errors.New("rl: unsupported mechanism for PPO StepFrom")

// Update runs PPO on the individual whose readout is
// [logits(actions)..., value].
//
// Every rollout is checked before anything moves: its PolicyVersion must equal
// PolicyVersion(ind) and its InitialPlastic / InitialChemical must deep-equal
// the individual's current snapshot parts, or the whole call is refused with
// ErrStalePolicy and the individual is left untouched. Each accepted rollout
// then gets its advantages and value targets from
// Advantages(steps, GAEConfig{Gamma, Lambda}), computed once from the values
// the collector recorded.
//
// Each of c.Epochs then walks every rollout in order and takes one StepFrom per
// rollout: the individual's neural state is set to that rollout's
// InitialNeural, a forward pass over the rollout's observations gives logits_t
// and value_t, and upstream row t is zeros for t < c.BurnIn and otherwise
// [dloss/dlogits..., dloss/dvalue] from
// Loss(logits_t, action_t, LogProb_t, A_t, value_t, target_t, c). c.MiniBatch
// currently must be one and c.TimeLimit bounds each rollout. The unit here is
// one complete rollout because a recurrent policy cannot be cut into shuffled
// minibatches without breaking the state the gradient flows through.
//
// Two interface facts shape the signature and the mechanics:
//
//   - learning.Individual has no method that sets an arbitrary NeuralState, so
//     the only route is Snapshot, replace Neural, RestoreIndividual, which
//     builds a new individual. Update therefore returns the updated individual
//     and callers must use the returned pointer; the one they passed in is
//     never mutated.
//   - learning.Individual exposes no StepFrom, so the gradient goes through
//     one learning.Trainer restored from the individual's own snapshot. It
//     carries the parameters, options, optimizer moments, update count and any
//     open accumulation window across every epoch, and the result is written
//     back through a snapshot rather than ResetParameters/ResetOptimizer,
//     which would clear the moments this update just built.
//
// The individual's persistent neural state is preserved: the returned
// individual carries the state the caller's individual had, exactly as
// TrainEpisode retains it. Note that learning.Trainer.StepFrom runs its own
// forward pass from zero voltage, so the gradient matches the logits scored
// here exactly when InitialNeural is the zero-voltage start state.
//
// The model must declare Config.ReadoutEveryStep and a readout width of
// actions+1; anything else is refused.
func Update(ctx context.Context, ind *learning.Individual, rollouts []Rollout, actions int, c PPOConfig) (*learning.Individual, PPOReport, error) {
	var report PPOReport
	if ctx == nil {
		return ind, report, errors.New("rl: update needs a context")
	}
	if err := ctx.Err(); err != nil {
		return ind, report, err
	}
	if ind == nil {
		return nil, report, errors.New("rl: update needs an individual")
	}
	if err := c.Validate(); err != nil {
		return ind, report, err
	}
	if c.MiniBatch != 1 {
		return ind, report, fmt.Errorf("rl: mini_batch=%d is unsupported; PPO currently accepts exactly 1 rollout per StepFrom", c.MiniBatch)
	}
	if actions < 1 {
		return ind, report, fmt.Errorf("rl: actions must be >= 1, got %d", actions)
	}
	base := ind.Snapshot()
	if base.SchemaVersion == "" {
		return ind, report, errors.New("rl: update needs an initialized individual")
	}
	if base.Config.OutputSize != actions+1 {
		return ind, report, fmt.Errorf("rl: readout width %d, want %d for %d actions plus one value", base.Config.OutputSize, actions+1, actions)
	}
	if !base.Config.ReadoutEveryStep {
		return ind, report, errors.New("rl: readout_every_step is off, so only the last step would carry a gradient")
	}
	if len(rollouts) == 0 {
		return ind, report, errors.New("rl: update needs at least one rollout")
	}

	version := policyVersionSnapshot(base)
	report = PPOReport{Rollouts: len(rollouts), Epochs: c.Epochs, PolicyVersionBefore: version, PolicyVersionAfter: version}
	if base.Plastic != nil {
		if base.Plastic.Slow != nil {
			return ind, report, fmt.Errorf("%w: plasticity slow state", ErrUnsupportedMechanism)
		}
		return ind, report, fmt.Errorf("%w: plasticity", ErrUnsupportedMechanism)
	}
	if base.Chemical != nil {
		return ind, report, fmt.Errorf("%w: chemical state", ErrUnsupportedMechanism)
	}
	zeroNeural, err := freshZeroNeural(base)
	if err != nil {
		return ind, report, fmt.Errorf("rl: build fresh zero-voltage state: %w", err)
	}

	// Every rollout is validated before the first gradient, so a stale one
	// cannot contribute a single step to the update.
	gae := GAEConfig{Gamma: c.Gamma, Lambda: c.Lambda}
	advantages := make([][]float64, len(rollouts))
	targets := make([][]float64, len(rollouts))
	inputs := make([][][]float64, len(rollouts))
	for i, r := range rollouts {
		if r.PolicyVersion != version ||
			!reflect.DeepEqual(r.InitialPlastic, base.Plastic) ||
			!reflect.DeepEqual(r.InitialChemical, base.Chemical) {
			return ind, report, fmt.Errorf("rl: rollout %d: %w", i, ErrStalePolicy)
		}
		if !reflect.DeepEqual(r.InitialNeural, zeroNeural) {
			return ind, report, fmt.Errorf("rl: rollout %d initial neural state must be a fresh zero-voltage state", i)
		}
		if len(r.Steps) == 0 {
			return ind, report, fmt.Errorf("rl: rollout %d: rollout must contain at least one step", i)
		}
		if len(r.Steps) > c.TimeLimit {
			return ind, report, fmt.Errorf("rl: rollout %d has %d steps, exceeding time_limit %d", i, len(r.Steps), c.TimeLimit)
		}
		if c.BurnIn >= len(r.Steps) {
			return ind, report, fmt.Errorf("rl: rollout %d burn_in=%d must be smaller than rollout length %d", i, c.BurnIn, len(r.Steps))
		}
		for t, s := range r.Steps {
			if s.Action < 0 || s.Action >= actions {
				return ind, report, fmt.Errorf("rl: rollout %d step %d action %d out of range [0, %d)", i, t, s.Action, actions)
			}
			if !finite(s.LogProb) {
				return ind, report, fmt.Errorf("rl: rollout %d step %d log_prob is non-finite", i, t)
			}
			if s.LogProb > 0 {
				return ind, report, fmt.Errorf("rl: rollout %d step %d log_prob must be <= 0", i, t)
			}
			if t < len(r.Steps)-1 && (s.Done || s.Timeout) {
				return ind, report, fmt.Errorf("rl: rollout %d step %d has an end marker before the final step", i, t)
			}
		}
		last := r.Steps[len(r.Steps)-1]
		if last.Timeout && len(r.Steps) != c.TimeLimit {
			return ind, report, fmt.Errorf("rl: rollout %d timeout requires length %d, got %d", i, c.TimeLimit, len(r.Steps))
		}
		a, target, err := Advantages(r.Steps, gae)
		if err != nil {
			return ind, report, fmt.Errorf("rl: rollout %d: %w", i, err)
		}
		in := make([][]float64, len(r.Steps))
		for t, s := range r.Steps {
			if len(s.Obs) != base.Config.InputSize {
				return ind, report, fmt.Errorf("rl: rollout %d step %d observation width %d, want %d", i, t, len(s.Obs), base.Config.InputSize)
			}
			in[t] = append([]float64(nil), s.Obs...)
		}
		advantages[i], targets[i], inputs[i] = a, target, in
		report.VersionChecks++
		report.Transitions += len(r.Steps)
	}

	tr, err := restoreTrainer(base)
	if err != nil {
		return ind, report, err
	}
	var sumLoss, sumPolicy, sumValue, sumEntropy, sumRatio float64
	var clipped, scored int
	for epoch := 0; epoch < c.Epochs; epoch++ {
		last := epoch == c.Epochs-1
		for i, r := range rollouts {
			if err := ctx.Err(); err != nil {
				return ind, report, err
			}
			at, err := individualAt(tr, base, r.InitialNeural)
			if err != nil {
				return ind, report, fmt.Errorf("rl: rollout %d initial state: %w", i, err)
			}
			out, err := at.Advance(ctx, inputs[i])
			if err != nil {
				return ind, report, fmt.Errorf("rl: rollout %d forward pass: %w", i, err)
			}
			upstream := make([][]float64, len(r.Steps))
			for t, s := range r.Steps {
				row := make([]float64, actions+1)
				upstream[t] = row
				if t < c.BurnIn {
					continue
				}
				step, dLogits, dValue, err := Loss(out[t][:actions], s.Action, s.LogProb, advantages[i][t], out[t][actions], targets[i][t], c)
				if err != nil {
					return ind, report, fmt.Errorf("rl: rollout %d step %d: %w", i, t, err)
				}
				copy(row, dLogits)
				row[actions] = dValue
				if !last {
					continue
				}
				sumLoss += step.Loss
				sumPolicy += step.Policy
				sumValue += step.Value
				sumEntropy += step.Entropy
				sumRatio += step.Ratio
				if step.Clipped {
					clipped++
				}
				scored++
			}
			if _, err := tr.StepFrom(ctx, inputs[i], upstream); err != nil {
				return ind, report, fmt.Errorf("rl: rollout %d update: %w", i, err)
			}
		}
	}
	updated, err := individualAt(tr, base, base.Neural)
	if err != nil {
		return ind, report, err
	}
	if scored > 0 {
		n := float64(scored)
		report.MeanLoss, report.MeanPolicy = sumLoss/n, sumPolicy/n
		report.MeanValue, report.MeanEntropy = sumValue/n, sumEntropy/n
		report.MeanRatio, report.ClippedFraction = sumRatio/n, float64(clipped)/n
	}
	report.PolicyVersionAfter = policyVersionSnapshot(updated.Snapshot())
	return updated, report, nil
}

// freshZeroNeural reconstructs exactly the state StepFrom's forward path uses.
// Arbitrary recurrent states need a VJP that carries the derivative through
// the initial state; Trainer.StepFrom deliberately has no such input, so this
// limited PPO entry point accepts only the state produced by a fresh individual
// with the same model and training options.
func freshZeroNeural(s learning.IndividualSnapshot) (learning.NeuralState, error) {
	nodes := s.Config.Dynamics.Nodes
	switch {
	case s.Config.LIF != nil:
		nodes = s.Config.LIF.Nodes
	case s.Config.Mixed != nil:
		nodes = s.Config.Mixed.Nodes
	}
	if nodes <= 0 {
		return learning.NeuralState{}, fmt.Errorf("configuration has no positive node count")
	}
	fresh, err := learning.NewIndividual(s.Config, s.Parameters, s.Optimizer.Options, make([]float64, nodes))
	if err != nil {
		return learning.NeuralState{}, err
	}
	return fresh.Snapshot().Neural, nil
}

// restoreTrainer builds the gradient entry point of one update from an
// individual's snapshot. The schema string of a training snapshot is not
// exported by learning, so a fresh trainer supplies it and only the optimizer
// half is replaced; the parameters and options are already the ones this
// trainer was built with.
func restoreTrainer(s learning.IndividualSnapshot) (*learning.Trainer, error) {
	fresh, err := learning.NewTrainer(s.Config, s.Parameters, s.Optimizer.Options)
	if err != nil {
		return nil, err
	}
	shaped := fresh.Snapshot()
	shaped.Optimizer = s.Optimizer.State
	shaped.Updates = s.Optimizer.Updates
	shaped.Accumulator = s.Optimizer.Accumulator
	return learning.RestoreTrainer(shaped)
}

// individualAt is the "set the neural state" step this package cannot ask for
// directly: it rebuilds the individual from the snapshot it started with, the
// trainer's current parameters and optimizer, and the requested recurrent
// state. Plastic and chemical parts come from the snapshot, so every forward
// pass of an update starts from the same fast state the rollout was collected
// under.
func individualAt(tr *learning.Trainer, base learning.IndividualSnapshot, neural learning.NeuralState) (*learning.Individual, error) {
	current := tr.Snapshot()
	at := base
	at.Parameters = current.Parameters
	at.Neural = neural
	at.Optimizer = learning.OptimizerSnapshot{
		Options:     current.Options,
		State:       current.Optimizer,
		Updates:     current.Updates,
		Accumulator: current.Accumulator,
		Episodes:    base.Optimizer.Episodes,
	}
	return learning.RestoreIndividual(at)
}
