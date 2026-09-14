// Command lifthreshold trains the base firing threshold of a spiking CoImNet
// core on the synthetic delayed-pulse fixture and reports preregistered gates.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	osSignal "os/signal"
	"syscall"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/learning"
)

const (
	exampleSchemaVersion = "coimnet-lif-threshold-example/v1"
	exampleProfile       = "fixture"
	exampleGenerator     = "delayed-pulse-splitmix64-v1"

	defaultUpdates = 300
	minUpdates     = 1
	maxUpdates     = 100000

	// Training and holdout samples come from the same counter-based generator,
	// so the two splits must be checked for overlapping counters, not only for
	// different seeds. The increment is the generator's own stride.
	trainSeed        = 1001
	holdoutSeed      = 1003
	holdoutCount     = 128
	counterIncrement = uint64(0x9e3779b97f4a7c15)

	learningRate = .02

	// Preregistered gates. They are fixed before the run and are never relaxed
	// to make a report pass.
	minThetaBaseChange = 1e-3
	gateDescription    = "for every seed: theta-only holdout MSE < frozen holdout MSE; at least one neuron's theta_base moves by more than 1e-3 under theta-only training; the theta-only holdout spike rate is strictly inside (0,1)"
)

// exampleSeeds are the delayed-association benchmark's preregistered seeds.
var exampleSeeds = []uint64{7, 42, 123}

// conditions are the three matched controls. They differ only in which
// parameter groups the optimizer may move.
var conditions = []struct {
	name      string
	trainable learning.Trainable
}{
	{"all", learning.Trainable{Encoder: true, Weights: true, Bias: true, Tau: true, Theta: true, Readout: true}},
	{"theta_only", learning.Trainable{Theta: true}},
	{"frozen", learning.Trainable{}},
}

type exampleReport struct {
	SchemaVersion string             `json:"schema_version"`
	Profile       string             `json:"profile"`
	Generator     string             `json:"generator"`
	Protocol      exampleProtocol    `json:"protocol"`
	Core          dynamics.LIFConfig `json:"core"`
	Gates         exampleGates       `json:"gates"`
	Runs          []exampleSeedRun   `json:"runs"`
	Passed        bool               `json:"passed"`
}

type exampleProtocol struct {
	Seeds            []uint64 `json:"seeds"`
	Updates          int      `json:"updates"`
	TrainSeed        uint64   `json:"train_seed"`
	HoldoutSeed      uint64   `json:"holdout_seed"`
	HoldoutCount     int      `json:"holdout_count"`
	LearningRate     float64  `json:"learning_rate"`
	CountersDisjoint bool     `json:"counters_disjoint"`
}

type exampleGates struct {
	MinThetaBaseChange float64 `json:"min_theta_base_change"`
	Description        string  `json:"description"`
}

type exampleSeedRun struct {
	Seed             uint64             `json:"seed"`
	Conditions       []exampleCondition `json:"conditions"`
	ThetaBeatsFrozen bool               `json:"theta_beats_frozen"`
	ThetaBaseMoved   bool               `json:"theta_base_moved"`
	SpikeRateInside  bool               `json:"spike_rate_inside_open_unit_interval"`
	Passed           bool               `json:"passed"`
}

type exampleCondition struct {
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

func main() {
	ctx, stop := osSignal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses the command line and prints one JSON report to stdout. It writes
// no files and keeps no model parameters.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if ctx == nil || stdout == nil || stderr == nil {
		return errors.New("context and output writers are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fs := flag.NewFlagSet("lifthreshold", flag.ContinueOnError)
	diagnosticOutput := &errorTrackingWriter{Writer: stderr}
	fs.SetOutput(diagnosticOutput)
	usageOutput := &errorTrackingWriter{Writer: stdout}
	var updates int
	fs.IntVar(&updates, "updates", defaultUpdates, fmt.Sprintf("training updates per seed and condition (%d..%d)", minUpdates, maxUpdates))
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: go run ./examples/lifthreshold [--updates 300]")
		fmt.Fprintln(usageOutput, "Trains the base firing threshold of a three-neuron leaky integrate-and-fire core on the synthetic five-step delayed-pulse fixture and prints one JSON report. Three preregistered seeds run three matched controls: every group trainable, only the threshold group trainable, and every group frozen. This is a numerical learnability check on generated data, not a biological behavior claim.")
		fmt.Fprintln(usageOutput, "Gates: "+gateDescription+". A failing gate prints the full report and exits non-zero.")
		fmt.Fprintln(usageOutput, "Limitations: fixed LIF settings (dt=1, tau=1, tau_syn=1, theta in [0.05,1], v_reset=-0.5, one refractory step, fast_sigmoid surrogate with scale 2, adaptation disabled); fixed seeds, splits and learning rate; 1..100000 updates; holdout counters never overlap training counters; no files or model parameters are written.")
		fmt.Fprintln(usageOutput, "Errors: unknown flags, extra arguments, an update budget outside the documented range, overlapping sample counters, cancellation, numerical failure, failing gates or output failure.")
		fmt.Fprintln(usageOutput, "Options:")
		fs.SetOutput(usageOutput)
		fs.PrintDefaults()
		fs.SetOutput(diagnosticOutput)
	}
	if err := fs.Parse(args); err != nil {
		if diagnosticErr := diagnosticOutput.Err(); diagnosticErr != nil {
			return errors.Join(err, fmt.Errorf("write flag diagnostic: %w", diagnosticErr))
		}
		if usageErr := usageOutput.Err(); usageErr != nil {
			return fmt.Errorf("write usage: %w", usageErr)
		}
		if errors.Is(err, flag.ErrHelp) {
			if fs.NArg() != 0 {
				return errors.New("unexpected positional arguments")
			}
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if updates < minUpdates || updates > maxUpdates {
		return fmt.Errorf("--updates must be between %d and %d", minUpdates, maxUpdates)
	}
	return execute(ctx, updates, stdout)
}

// execute prints the report before deciding the exit status, so a failing gate
// is always visible with its evidence.
func execute(ctx context.Context, updates int, stdout io.Writer) error {
	report, err := buildReport(ctx, updates)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	if !report.Passed {
		return errors.New("preregistered threshold gates failed")
	}
	return nil
}

// buildReport runs every seed and condition and scores the fixed gates. An
// update budget of zero is accepted here and produces an honest failing report;
// the command line keeps its documented 1..100000 range.
func buildReport(ctx context.Context, updates int) (exampleReport, error) {
	var report exampleReport
	if ctx == nil {
		return report, errors.New("nil context")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if updates < 0 || updates > maxUpdates {
		return report, fmt.Errorf("update budget %d is outside 0..%d", updates, maxUpdates)
	}
	if err := checkDisjointCounters(updates, holdoutCount); err != nil {
		return report, err
	}
	report = exampleReport{
		SchemaVersion: exampleSchemaVersion,
		Profile:       exampleProfile,
		Generator:     exampleGenerator,
		Protocol: exampleProtocol{
			Seeds: append([]uint64(nil), exampleSeeds...), Updates: updates,
			TrainSeed: trainSeed, HoldoutSeed: holdoutSeed, HoldoutCount: holdoutCount,
			LearningRate: learningRate, CountersDisjoint: true,
		},
		Gates:  exampleGates{MinThetaBaseChange: minThetaBaseChange, Description: gateDescription},
		Passed: true,
	}
	for _, seed := range exampleSeeds {
		seedRun := exampleSeedRun{Seed: seed}
		for _, condition := range conditions {
			trainer, err := experiment.NewDelayedLIFTrainer(seed, learningRate, condition.trainable)
			if err != nil {
				return exampleReport{}, err
			}
			initial := trainer.Snapshot()
			if report.Core.Nodes == 0 {
				report.Core = *initial.Config.LIF
			}
			beforeMSE, err := experiment.EvaluateDelayed(ctx, trainer, holdoutSeed, holdoutCount)
			if err != nil {
				return exampleReport{}, err
			}
			beforeRate, err := holdoutSpikeRate(ctx, trainer)
			if err != nil {
				return exampleReport{}, err
			}
			for i := 0; i < updates; i++ {
				episode := experiment.DelayedEpisode(trainSeed, uint64(i))
				if _, err := trainer.Step(ctx, episode.Input, episode.Target); err != nil {
					return exampleReport{}, fmt.Errorf("seed %d condition %s update %d: %w", seed, condition.name, i+1, err)
				}
			}
			afterMSE, err := experiment.EvaluateDelayed(ctx, trainer, holdoutSeed, holdoutCount)
			if err != nil {
				return exampleReport{}, err
			}
			afterRate, err := holdoutSpikeRate(ctx, trainer)
			if err != nil {
				return exampleReport{}, err
			}
			final := trainer.Snapshot()
			core := *initial.Config.LIF
			before := thetaBases(initial.Parameters.ThetaRaw, core)
			after := thetaBases(final.Parameters.ThetaRaw, core)
			result := exampleCondition{
				Name: condition.name, Trainable: condition.trainable,
				HoldoutMSEBefore: beforeMSE, HoldoutMSEAfter: afterMSE,
				ThetaBaseBefore: before, ThetaBaseAfter: after,
				MaxThetaBaseChange: maxAbsoluteDifference(before, after),
				SpikeRateBefore:    beforeRate, SpikeRateAfter: afterRate,
			}
			for _, value := range []float64{result.HoldoutMSEBefore, result.HoldoutMSEAfter, result.MaxThetaBaseChange, result.SpikeRateBefore, result.SpikeRateAfter} {
				if !finite(value) {
					return exampleReport{}, fmt.Errorf("seed %d condition %s produced a non-finite result", seed, condition.name)
				}
			}
			seedRun.Conditions = append(seedRun.Conditions, result)
		}
		thetaOnly, frozen := seedRun.Conditions[1], seedRun.Conditions[2]
		seedRun.ThetaBeatsFrozen = thetaOnly.HoldoutMSEAfter < frozen.HoldoutMSEAfter
		seedRun.ThetaBaseMoved = thetaOnly.MaxThetaBaseChange > minThetaBaseChange
		seedRun.SpikeRateInside = thetaOnly.SpikeRateAfter > 0 && thetaOnly.SpikeRateAfter < 1
		seedRun.Passed = seedRun.ThetaBeatsFrozen && seedRun.ThetaBaseMoved && seedRun.SpikeRateInside
		report.Runs = append(report.Runs, seedRun)
		report.Passed = report.Passed && seedRun.Passed
	}
	if err := ctx.Err(); err != nil {
		return exampleReport{}, err
	}
	return report, nil
}

// checkDisjointCounters mirrors the delayed benchmark: distinct split seeds do
// not by themselves guarantee distinct sample counters, so the generated
// counters are compared directly.
func checkDisjointCounters(updates, holdout int) error {
	if updates < 0 || holdout <= 0 {
		return fmt.Errorf("invalid split sizes %d and %d", updates, holdout)
	}
	training := make(map[uint64]bool, updates)
	for i := 0; i < updates; i++ {
		training[trainSeed+uint64(i)*counterIncrement] = true
	}
	for i := 0; i < holdout; i++ {
		if training[holdoutSeed+uint64(i)*counterIncrement] {
			return errors.New("training and holdout counter streams overlap")
		}
	}
	return nil
}

// holdoutSpikeRate is the fraction of neuron-steps carrying an event over the
// complete holdout split, using the frozen episode path of the trainer.
func holdoutSpikeRate(ctx context.Context, trainer *learning.Trainer) (float64, error) {
	var events, cells float64
	for i := 0; i < holdoutCount; i++ {
		episode := experiment.DelayedEpisode(holdoutSeed, uint64(i))
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

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

type errorTrackingWriter struct {
	io.Writer
	err error
}

func (w *errorTrackingWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.Writer.Write(p)
	if err != nil {
		w.err = err
	}
	return n, err
}

func (w *errorTrackingWriter) Err() error { return w.err }
