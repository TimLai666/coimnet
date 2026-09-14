package experiment

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// The preregistered threshold protocol. Every value below is fixed before the
// run; only the update budget is chosen by the caller.
const (
	lifThresholdSchemaVersion = "coimnet-lif-threshold-example/v1"

	// LIFThresholdDefaultUpdates, LIFThresholdMinUpdates and
	// LIFThresholdMaxUpdates are the budget the command line entry points
	// document. RunLIFThreshold also accepts zero, which produces an honest
	// failing report instead of an error.
	LIFThresholdDefaultUpdates = 300
	LIFThresholdMinUpdates     = 1
	LIFThresholdMaxUpdates     = 100000

	// Training and holdout samples come from the same counter-based generator,
	// so the two splits are checked for overlapping counters, not only for
	// different seeds. The increment is the generator's own stride.
	lifThresholdTrainSeed        = 1001
	lifThresholdHoldoutSeed      = 1003
	lifThresholdHoldoutCount     = 128
	lifThresholdCounterIncrement = uint64(0x9e3779b97f4a7c15)

	lifThresholdLearningRate = .02

	// lifThresholdMinThetaBaseChange is a preregistered gate. It is fixed
	// before the run and is never relaxed to make a report pass.
	lifThresholdMinThetaBaseChange = 1e-3

	// LIFThresholdGateDescription states the complete acceptance rule, so the
	// report and every help text quote the same sentence.
	LIFThresholdGateDescription = "for every seed: theta-only holdout MSE < frozen holdout MSE; at least one neuron's theta_base moves by more than 1e-3 under theta-only training; the theta-only holdout spike rate is strictly inside (0,1)"
)

// lifThresholdSeeds are the delayed-association benchmark's preregistered seeds.
var lifThresholdSeeds = []uint64{7, 42, 123}

// lifThresholdConditions are the three matched controls. They differ only in
// which parameter groups the optimizer may move.
var lifThresholdConditions = []struct {
	name      string
	trainable learning.Trainable
}{
	{"all", learning.Trainable{Encoder: true, Weights: true, Bias: true, Tau: true, Theta: true, Readout: true}},
	{"theta_only", learning.Trainable{Theta: true}},
	{"frozen", learning.Trainable{}},
}

// LIFThresholdReport keeps the synthetic threshold-learning evidence separate
// from any biological claim: it records every seed, control and gate.
type LIFThresholdReport struct {
	SchemaVersion string                `json:"schema_version"`
	Profile       string                `json:"profile"`
	Generator     string                `json:"generator"`
	Protocol      LIFThresholdProtocol  `json:"protocol"`
	Core          dynamics.LIFConfig    `json:"core"`
	Gates         LIFThresholdGates     `json:"gates"`
	Runs          []LIFThresholdSeedRun `json:"runs"`
	Passed        bool                  `json:"passed"`
}

// LIFThresholdProtocol is the fixed protocol actually executed.
// CountersDisjoint is a verified property, not an option.
type LIFThresholdProtocol struct {
	Seeds            []uint64 `json:"seeds"`
	Updates          int      `json:"updates"`
	TrainSeed        uint64   `json:"train_seed"`
	HoldoutSeed      uint64   `json:"holdout_seed"`
	HoldoutCount     int      `json:"holdout_count"`
	LearningRate     float64  `json:"learning_rate"`
	CountersDisjoint bool     `json:"counters_disjoint"`
}

// LIFThresholdGates are the acceptance gates fixed before the run.
type LIFThresholdGates struct {
	MinThetaBaseChange float64 `json:"min_theta_base_change"`
	Description        string  `json:"description"`
}

// LIFThresholdSeedRun scores one seed over all three controls.
type LIFThresholdSeedRun struct {
	Seed             uint64                  `json:"seed"`
	Conditions       []LIFThresholdCondition `json:"conditions"`
	ThetaBeatsFrozen bool                    `json:"theta_beats_frozen"`
	ThetaBaseMoved   bool                    `json:"theta_base_moved"`
	SpikeRateInside  bool                    `json:"spike_rate_inside_open_unit_interval"`
	Passed           bool                    `json:"passed"`
}

// LIFThresholdCondition records one control's before and after measurements.
type LIFThresholdCondition struct {
	Name               string             `json:"name"`
	Trainable          learning.Trainable `json:"trainable"`
	HoldoutMSEBefore   float64            `json:"holdout_mse_before"`
	HoldoutMSEAfter    float64            `json:"holdout_mse_after"`
	ThetaBaseBefore    []float64          `json:"theta_base_before"`
	ThetaBaseAfter     []float64          `json:"theta_base_after"`
	MaxThetaBaseChange float64            `json:"max_theta_base_change"`
	SpikeRateBefore    float64            `json:"spike_rate_before"`
	SpikeRateAfter     float64            `json:"spike_rate_after"`
}

// RunLIFThreshold trains the base firing threshold of the three-neuron spiking
// fixture on the unchanged five-step delayed-pulse generator and scores the
// preregistered gates. Holdout observations and labels never enter Step.
//
// An update budget of zero is accepted and produces an honest failing report;
// the command line entry points keep their documented 1..100000 range. Canceled
// runs return an error and no partial report.
func RunLIFThreshold(ctx context.Context, updates int) (LIFThresholdReport, error) {
	var report LIFThresholdReport
	if ctx == nil {
		return report, errors.New("nil context")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if updates < 0 || updates > LIFThresholdMaxUpdates {
		return report, fmt.Errorf("update budget %d is outside 0..%d", updates, LIFThresholdMaxUpdates)
	}
	if err := checkDisjointLIFThresholdCounters(updates, lifThresholdHoldoutCount); err != nil {
		return report, err
	}
	report = LIFThresholdReport{
		SchemaVersion: lifThresholdSchemaVersion,
		Profile:       "fixture",
		Generator:     "delayed-pulse-splitmix64-v1",
		Protocol: LIFThresholdProtocol{
			Seeds: append([]uint64(nil), lifThresholdSeeds...), Updates: updates,
			TrainSeed: lifThresholdTrainSeed, HoldoutSeed: lifThresholdHoldoutSeed,
			HoldoutCount: lifThresholdHoldoutCount, LearningRate: lifThresholdLearningRate,
			CountersDisjoint: true,
		},
		Gates:  LIFThresholdGates{MinThetaBaseChange: lifThresholdMinThetaBaseChange, Description: LIFThresholdGateDescription},
		Passed: true,
	}
	for _, seed := range lifThresholdSeeds {
		seedRun := LIFThresholdSeedRun{Seed: seed}
		for _, condition := range lifThresholdConditions {
			trainer, err := NewDelayedLIFTrainer(seed, lifThresholdLearningRate, condition.trainable)
			if err != nil {
				return LIFThresholdReport{}, err
			}
			initial := trainer.Snapshot()
			if report.Core.Nodes == 0 {
				report.Core = *initial.Config.LIF
			}
			beforeMSE, err := EvaluateDelayed(ctx, trainer, lifThresholdHoldoutSeed, lifThresholdHoldoutCount)
			if err != nil {
				return LIFThresholdReport{}, err
			}
			beforeRate, err := lifThresholdHoldoutSpikeRate(ctx, trainer)
			if err != nil {
				return LIFThresholdReport{}, err
			}
			for i := 0; i < updates; i++ {
				episode := DelayedEpisode(lifThresholdTrainSeed, uint64(i))
				if _, err := trainer.Step(ctx, episode.Input, episode.Target); err != nil {
					return LIFThresholdReport{}, fmt.Errorf("seed %d condition %s update %d: %w", seed, condition.name, i+1, err)
				}
			}
			afterMSE, err := EvaluateDelayed(ctx, trainer, lifThresholdHoldoutSeed, lifThresholdHoldoutCount)
			if err != nil {
				return LIFThresholdReport{}, err
			}
			afterRate, err := lifThresholdHoldoutSpikeRate(ctx, trainer)
			if err != nil {
				return LIFThresholdReport{}, err
			}
			final := trainer.Snapshot()
			core := *initial.Config.LIF
			before := thetaBases(initial.Parameters.ThetaRaw, core)
			after := thetaBases(final.Parameters.ThetaRaw, core)
			result := LIFThresholdCondition{
				Name: condition.name, Trainable: condition.trainable,
				HoldoutMSEBefore: beforeMSE, HoldoutMSEAfter: afterMSE,
				ThetaBaseBefore: before, ThetaBaseAfter: after,
				MaxThetaBaseChange: maxAbsoluteDifference(before, after),
				SpikeRateBefore:    beforeRate, SpikeRateAfter: afterRate,
			}
			for _, value := range []float64{result.HoldoutMSEBefore, result.HoldoutMSEAfter, result.MaxThetaBaseChange, result.SpikeRateBefore, result.SpikeRateAfter} {
				if !finite(value) {
					return LIFThresholdReport{}, fmt.Errorf("seed %d condition %s produced a non-finite result", seed, condition.name)
				}
			}
			seedRun.Conditions = append(seedRun.Conditions, result)
		}
		thetaOnly, frozen := seedRun.Conditions[1], seedRun.Conditions[2]
		seedRun.ThetaBeatsFrozen = thetaOnly.HoldoutMSEAfter < frozen.HoldoutMSEAfter
		seedRun.ThetaBaseMoved = thetaOnly.MaxThetaBaseChange > lifThresholdMinThetaBaseChange
		seedRun.SpikeRateInside = thetaOnly.SpikeRateAfter > 0 && thetaOnly.SpikeRateAfter < 1
		seedRun.Passed = seedRun.ThetaBeatsFrozen && seedRun.ThetaBaseMoved && seedRun.SpikeRateInside
		report.Runs = append(report.Runs, seedRun)
		report.Passed = report.Passed && seedRun.Passed
	}
	if err := ctx.Err(); err != nil {
		return LIFThresholdReport{}, err
	}
	return report, nil
}

// checkDisjointLIFThresholdCounters mirrors the delayed benchmark: distinct
// split seeds do not by themselves guarantee distinct sample counters, so the
// generated counters are compared directly.
func checkDisjointLIFThresholdCounters(updates, holdout int) error {
	if updates < 0 || holdout <= 0 {
		return fmt.Errorf("invalid split sizes %d and %d", updates, holdout)
	}
	training := make(map[uint64]bool, updates)
	for i := 0; i < updates; i++ {
		training[lifThresholdTrainSeed+uint64(i)*lifThresholdCounterIncrement] = true
	}
	for i := 0; i < holdout; i++ {
		if training[lifThresholdHoldoutSeed+uint64(i)*lifThresholdCounterIncrement] {
			return errors.New("training and holdout counter streams overlap")
		}
	}
	return nil
}

// lifThresholdHoldoutSpikeRate is the fraction of neuron-steps carrying an
// event over the complete holdout split, using the frozen episode path of the
// trainer.
func lifThresholdHoldoutSpikeRate(ctx context.Context, trainer *learning.Trainer) (float64, error) {
	var events, cells float64
	for i := 0; i < lifThresholdHoldoutCount; i++ {
		episode := DelayedEpisode(lifThresholdHoldoutSeed, uint64(i))
		spikes, err := trainer.Spikes(ctx, episode.Input)
		if err != nil {
			return 0, err
		}
		for _, row := range spikes {
			for _, value := range row {
				if value != 0 && value != 1 {
					return 0, fmt.Errorf("non-binary spike %g", value)
				}
				events += value
				cells++
			}
		}
	}
	if cells == 0 {
		return 0, errors.New("holdout produced no neuron steps")
	}
	return events / cells, nil
}

// thetaBases applies the core's declared bounded transform
// theta_base = theta_min + (theta_max-theta_min)*sigmoid(theta_raw).
func thetaBases(raw []float64, core dynamics.LIFConfig) []float64 {
	out := make([]float64, len(raw))
	for i, value := range raw {
		out[i] = core.ThetaMin + (core.ThetaMax-core.ThetaMin)*logistic(value)
	}
	return out
}

func logistic(v float64) float64 {
	if v >= 0 {
		return 1 / (1 + math.Exp(-v))
	}
	e := math.Exp(v)
	return e / (1 + e)
}

func maxAbsoluteDifference(a, b []float64) float64 {
	var largest float64
	for i, value := range a {
		if i >= len(b) {
			break
		}
		if d := math.Abs(b[i] - value); d > largest {
			largest = d
		}
	}
	return largest
}
