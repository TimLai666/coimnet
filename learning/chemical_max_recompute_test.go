package learning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

const chemicalMaxReferenceEpsilon = 0.002

func TestChemicalMaxNegativeScaleUsesRawOccupancy(t *testing.T) {
	occupancies := []modulation.OccupancyRecord{
		{Receptor: 0, Cell: 0, Status: modulation.StatusHypothesized, Occupancy: 0.8},
		{Receptor: 1, Cell: 0, Status: modulation.StatusHypothesized, Occupancy: 0.2},
	}
	effects := []modulation.Effect{
		{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: -0.5},
		{Kind: modulation.EffectSensitivity, Receptor: 1, GammaScale: -0.5},
	}
	gain, _, _, _, err := modulation.ApplyEffects(effects, occupancies, modulation.MixMax, 1)
	if err != nil {
		t.Fatalf("ApplyEffects: %v", err)
	}
	if len(gain) != 1 || math.Abs(gain[0]-0.6) > 1e-15 {
		t.Fatalf("MixMax gain = %v, want [0.6] from 1-.5*.8", gain)
	}
	gradient, err := modulation.BackwardEffects(effects, occupancies, modulation.MixMax, 1, []float64{2}, []float64{0}, []float64{0})
	if err != nil {
		t.Fatalf("BackwardEffects: %v", err)
	}
	if len(gradient) != 2 || gradient[0] != -1 || gradient[1] != 0 {
		t.Fatalf("MixMax occupancy VJP = %v, want [-1 0]", gradient)
	}
}

type chemicalMaxProfileSpec struct {
	gammaScale, betaScale, gammaMin, gammaMax, betaMax float64
	thetaScale, thetaMax                               float64
}

var chemicalMaxCommonProfiles = []string{
	"open-positive", "open-negative", "gain-upper", "gain-lower", "offset-upper", "offset-lower",
}

func chemicalMaxProfiles(lif bool) []string {
	profiles := append([]string(nil), chemicalMaxCommonProfiles...)
	if lif {
		profiles = append(profiles, "threshold-upper", "threshold-lower")
	}
	return profiles
}

func chemicalMaxProfile(name string) (chemicalMaxProfileSpec, error) {
	s := chemicalMaxProfileSpec{gammaScale: .05, betaScale: .02, gammaMin: .5, gammaMax: 2, betaMax: 1, thetaScale: .1, thetaMax: .8}
	switch name {
	case "open-positive":
	case "open-negative":
		s.gammaScale, s.betaScale, s.thetaScale = -.05, -.02, -.1
	case "gain-upper":
		s.gammaScale, s.gammaMin, s.gammaMax = 1, .75, 1.005
	case "gain-lower":
		s.gammaScale, s.gammaMin, s.gammaMax = -1, .995, 1.25
		s.betaScale, s.thetaScale = -.02, -.1
	case "offset-upper":
		s.betaScale, s.betaMax = .1, .0005
	case "offset-lower":
		s.gammaScale, s.betaScale, s.betaMax, s.thetaScale = -.05, -.1, .0005, -.1
	case "threshold-upper":
		s.thetaMax = .0005
	case "threshold-lower":
		s.gammaScale, s.betaScale, s.thetaScale, s.thetaMax = -.05, -.02, -.1, .0005
	default:
		return chemicalMaxProfileSpec{}, fmt.Errorf("unknown profile %q", name)
	}
	return s, nil
}

func chemicalMaxControllerState(t *testing.T, state *modulation.ControllerState, filled int) {
	t.Helper()
	if state == nil || filled < 0 || filled > len(state.Window) {
		t.Fatalf("invalid controller state/fill: state=%+v filled=%d", state, filled)
	}
	for row := 0; row < len(state.Window)-filled; row++ {
		for column := range state.Window[row] {
			state.Window[row][column] = 0
		}
	}
	state.Filled = filled
}

func chemicalMaxSetContinuousFixture(t *testing.T, profile string, filled int) (learning.Config, learning.Parameters, [][]float64, []float64, [][]float64) {
	t.Helper()
	config, parameters, inputs, target, upstream := chemicalRecomputeFixture(t)
	spec, err := chemicalMaxProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	config.Dynamics.Delays = []int{5, 5}
	episode := config.Chemistry
	if episode == nil || len(episode.Part.SourceStates) == 0 {
		t.Fatal("continuous fixture has no chemical controller state")
	}
	episode.Part.Config.Receptors.Mix = modulation.MixMax
	chemicalMaxControllerState(t, episode.Part.SourceStates[0].Controller, filled)
	for i := range episode.Part.Config.Effects {
		effect := &episode.Part.Config.Effects[i]
		if effect.Kind != modulation.EffectSensitivity {
			t.Fatalf("continuous effect %d has kind %q", i, effect.Kind)
		}
		effect.GammaScale, effect.BetaScale = spec.gammaScale, spec.betaScale
		effect.GammaMin, effect.GammaMax, effect.BetaAbsMax = spec.gammaMin, spec.gammaMax, spec.betaMax
	}
	return config, parameters, inputs, target, upstream
}

func chemicalMaxSetLIFFixture(t *testing.T, profile string, filled int) chemicalRecomputeLIFFixture {
	t.Helper()
	fixture := newChemicalRecomputeLIFFixture(t, 9, .8)
	spec, err := chemicalMaxProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	fixture.config.LIF.Delays = []int{5, 5}
	episode := fixture.config.Chemistry
	if episode == nil || len(episode.Part.SourceStates) == 0 {
		t.Fatal("LIF fixture has no chemical controller state")
	}
	episode.Part.Config.Receptors.Mix = modulation.MixMax
	chemicalMaxControllerState(t, episode.Part.SourceStates[0].Controller, filled)
	episode.Part.Config.Effects = append(episode.Part.Config.Effects, modulation.Effect{Kind: modulation.EffectThreshold, Receptor: 2})
	for i := range episode.Part.Config.Effects {
		effect := &episode.Part.Config.Effects[i]
		switch effect.Kind {
		case modulation.EffectSensitivity:
			effect.GammaScale, effect.BetaScale = spec.gammaScale, spec.betaScale
			effect.GammaMin, effect.GammaMax, effect.BetaAbsMax = spec.gammaMin, spec.gammaMax, spec.betaMax
		case modulation.EffectThreshold:
			effect.ThetaScale, effect.ThetaAbsMax = spec.thetaScale, spec.thetaMax
		default:
			t.Fatalf("unknown LIF effect kind %q", effect.Kind)
		}
	}
	return fixture
}

func chemicalMaxOptions(lif bool, segment int, recompute bool) learning.Options {
	options := chemicalRecomputeOptions(segment, 3, 2)
	options.Masks = nil
	options.Trainable = learning.Trainable{Encoder: true, Weights: true, Bias: true, Tau: true, Theta: lif, Readout: true}
	if !recompute {
		options.Recompute = nil
	}
	return options
}

func chemicalMaxConfigWithMix(config learning.Config, mix string) learning.Config {
	copyConfig := config
	if config.Chemistry == nil {
		return copyConfig
	}
	episode := *config.Chemistry
	part := episode.Part
	chemistry := part.Config
	chemistry.Receptors.Mix = mix
	part.Config = chemistry
	episode.Part = part
	copyConfig.Chemistry = &episode
	return copyConfig
}

func chemicalMaxAssertCompleteSnapshot(t *testing.T, label string, got, want learning.TrainingSnapshot) {
	t.Helper()
	assertChemicalRecomputeSnapshot(t, label, got, want)
	got.Options.Recompute = nil
	want.Options.Recompute = nil
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s complete snapshot differs: got=%+v want=%+v", label, got, want)
	}
}

func chemicalMaxFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func chemicalMaxAssertRowsFinite(t *testing.T, label string, rows [][]float64) {
	t.Helper()
	for row, values := range rows {
		for column, value := range values {
			if !chemicalMaxFinite(value) {
				t.Fatalf("%s[%d][%d]=%g is non-finite", label, row, column, value)
			}
		}
	}
}

func chemicalMaxAssertResultFinite(t *testing.T, label string, result learning.StepResult) {
	t.Helper()
	for name, value := range map[string]float64{"loss": result.Loss, "gradient_norm": result.GradientNorm, "update_norm": result.UpdateNorm, "learning_rate": result.LearningRate} {
		if !chemicalMaxFinite(value) {
			t.Fatalf("%s %s=%g is non-finite", label, name, value)
		}
	}
}

func TestChemicalMaxRecomputeSignedAndClampedTraining(t *testing.T) {
	ctx := context.Background()
	leafCases := 0
	for _, core := range []string{"continuous", "lif"} {
		lif := core == "lif"
		for _, profile := range chemicalMaxProfiles(lif) {
			for _, filled := range []int{1, 3} {
				for _, segment := range []int{1, 5} {
					leafCases++
					t.Run(fmt.Sprintf("%s/%s/filled%d/segment%d", core, profile, filled, segment), func(t *testing.T) {
						var config learning.Config
						var parameters learning.Parameters
						var inputs [][]float64
						var target []float64
						var upstream [][]float64
						if lif {
							fixture := chemicalMaxSetLIFFixture(t, profile, filled)
							config, parameters, inputs, target, upstream = fixture.config, fixture.parameters, fixture.inputs, fixture.target, fixture.upstream
						} else {
							config, parameters, inputs, target, upstream = chemicalMaxSetContinuousFixture(t, profile, filled)
						}
						maxNetwork, err := learning.NewNetwork(config)
						if err != nil {
							t.Fatalf("MixMax NewNetwork: %v", err)
						}
						sumConfig := chemicalMaxConfigWithMix(config, modulation.MixSum)
						sumNetwork, err := learning.NewNetwork(sumConfig)
						if err != nil {
							t.Fatalf("MixSum NewNetwork: %v", err)
						}
						maxPrediction, err := maxNetwork.PredictAll(ctx, parameters, chemicalRecomputeRowsCopy(inputs))
						if err != nil {
							t.Fatalf("MixMax prediction: %v", err)
						}
						sumPrediction, err := sumNetwork.PredictAll(ctx, parameters, chemicalRecomputeRowsCopy(inputs))
						if err != nil {
							t.Fatalf("MixSum prediction: %v", err)
						}
						chemicalMaxAssertRowsFinite(t, "MixMax prediction", maxPrediction)
						chemicalMaxAssertRowsFinite(t, "MixSum prediction", sumPrediction)
						different := !reflect.DeepEqual(maxPrediction, sumPrediction)
						if !different {
							maxGradient, maxErr := maxNetwork.LossGradientFrom(ctx, parameters, chemicalRecomputeRowsCopy(inputs), chemicalRecomputeRowsCopy(upstream), 0)
							sumGradient, sumErr := sumNetwork.LossGradientFrom(ctx, parameters, chemicalRecomputeRowsCopy(inputs), chemicalRecomputeRowsCopy(upstream), 0)
							if maxErr != nil || sumErr != nil {
								t.Fatalf("MixMax/MixSum control gradients: max=%v sum=%v", maxErr, sumErr)
							}
							different = !reflect.DeepEqual(maxGradient, sumGradient)
						}
						if !different {
							t.Fatal("switching only Receptors.Mix from MixMax to MixSum was unobservable")
						}

						fullOptions := chemicalMaxOptions(lif, segment, false)
						recomputeOptions := chemicalMaxOptions(lif, segment, true)
						full, err := learning.NewTrainer(config, parameters, fullOptions)
						if err != nil {
							t.Fatalf("full NewTrainer: %v", err)
						}
						defer full.Close()
						recompute, err := learning.NewTrainer(config, parameters, recomputeOptions)
						if err != nil {
							t.Fatalf("recompute NewTrainer: %v", err)
						}
						defer func() { _ = recompute.Close() }()
						initial := full.Snapshot()
						for operation := 0; operation < 5; operation++ {
							var want, got learning.StepResult
							if operation%2 == 0 {
								want, err = full.StepFrom(ctx, chemicalRecomputeRowsCopy(inputs), chemicalRecomputeRowsCopy(upstream))
								if err != nil {
									t.Fatalf("full StepFrom %d: %v", operation, err)
								}
								got, err = recompute.StepFrom(ctx, chemicalRecomputeRowsCopy(inputs), chemicalRecomputeRowsCopy(upstream))
								if err != nil {
									t.Fatalf("recompute StepFrom %d: %v", operation, err)
								}
							} else {
								want, err = full.Step(ctx, chemicalRecomputeRowsCopy(inputs), append([]float64(nil), target...))
								if err != nil {
									t.Fatalf("full Step %d: %v", operation, err)
								}
								got, err = recompute.Step(ctx, chemicalRecomputeRowsCopy(inputs), append([]float64(nil), target...))
								if err != nil {
									t.Fatalf("recompute Step %d: %v", operation, err)
								}
							}
							chemicalMaxAssertResultFinite(t, fmt.Sprintf("full result %d", operation), want)
							chemicalMaxAssertResultFinite(t, fmt.Sprintf("recompute result %d", operation), got)
							assertRecomputeResult(t, fmt.Sprintf("operation %d", operation), got, want)
							fullPrediction, err := full.PredictAll(ctx, chemicalRecomputeRowsCopy(inputs))
							if err != nil {
								t.Fatalf("full prediction %d: %v", operation, err)
							}
							recomputePrediction, err := recompute.PredictAll(ctx, chemicalRecomputeRowsCopy(inputs))
							if err != nil {
								t.Fatalf("recompute prediction %d: %v", operation, err)
							}
							chemicalMaxAssertRowsFinite(t, fmt.Sprintf("full prediction %d", operation), fullPrediction)
							chemicalMaxAssertRowsFinite(t, fmt.Sprintf("recompute prediction %d", operation), recomputePrediction)
							assertRecomputeRows(t, fmt.Sprintf("prediction %d", operation), recomputePrediction, fullPrediction)
							chemicalMaxAssertCompleteSnapshot(t, fmt.Sprintf("snapshot %d", operation), recompute.Snapshot(), full.Snapshot())

							if operation == 0 {
								snapshot := recompute.Snapshot()
								if snapshot.Accumulator == nil || snapshot.Accumulator.Count != 1 {
									t.Fatalf("first pending recompute snapshot accumulator=%+v, want count 1", snapshot.Accumulator)
								}
								fullPending := full.Snapshot()
								if fullPending.Accumulator == nil || fullPending.Accumulator.Count != 1 {
									t.Fatalf("first pending full snapshot accumulator=%+v, want count 1", fullPending.Accumulator)
								}
								data, err := json.Marshal(snapshot)
								if err != nil {
									t.Fatalf("marshal pending snapshot: %v", err)
								}
								var decoded learning.TrainingSnapshot
								if err := json.Unmarshal(data, &decoded); err != nil {
									t.Fatalf("unmarshal pending snapshot: %v", err)
								}
								restored, err := learning.RestoreTrainer(decoded)
								if err != nil {
									t.Fatalf("RestoreTrainer pending snapshot: %v", err)
								}
								previous := recompute
								recompute = restored // The deferred close owns restored even if a check fails.
								if err := previous.Close(); err != nil {
									t.Fatalf("close pre-restore trainer: %v", err)
								}
								if restored.Snapshot().Options.Recompute == nil || restored.Snapshot().Options.Recompute.SegmentSteps != segment {
									t.Fatalf("restored recompute options = %+v, want segment %d", restored.Snapshot().Options.Recompute, segment)
								}
								chemicalMaxAssertCompleteSnapshot(t, "restored pending snapshot", recompute.Snapshot(), full.Snapshot())
							}
						}
						final := full.Snapshot()
						chemicalMaxAssertCompleteSnapshot(t, "final snapshot", recompute.Snapshot(), final)
						if final.Updates != 2 || final.Accumulator == nil || final.Accumulator.Count != 1 {
							t.Fatalf("final full state updates=%d accumulator=%+v, want updates=2 and pending count=1", final.Updates, final.Accumulator)
						}
						if got := recompute.Snapshot(); got.Updates != 2 || got.Accumulator == nil || got.Accumulator.Count != 1 {
							t.Fatalf("final recompute state = updates %d accumulator %+v, want updates=2 and pending count=1", got.Updates, got.Accumulator)
						}
						if reflect.DeepEqual(initial.Parameters, final.Parameters) {
							t.Fatal("parameters did not change after two accumulated updates")
						}
					})
				}
			}
		}
	}
	if leafCases != 56 {
		t.Fatalf("training leaf cases=%d, want 56", leafCases)
	}
}

type chemicalMaxParameterCoordinate struct {
	name string
	set  func(*learning.Parameters, float64)
}

func chemicalMaxParameterCoordinates(parameters learning.Parameters) []chemicalMaxParameterCoordinate {
	coordinates := make([]chemicalMaxParameterCoordinate, 0, 10)
	for i := range parameters.Core.Weights {
		index := i
		coordinates = append(coordinates, chemicalMaxParameterCoordinate{fmt.Sprintf("weights[%d]", i), func(p *learning.Parameters, delta float64) { p.Core.Weights[index] += delta }})
	}
	for i := range parameters.Core.Bias {
		index := i
		coordinates = append(coordinates, chemicalMaxParameterCoordinate{fmt.Sprintf("bias[%d]", i), func(p *learning.Parameters, delta float64) { p.Core.Bias[index] += delta }})
	}
	for i := range parameters.Core.LogTau {
		index := i
		coordinates = append(coordinates, chemicalMaxParameterCoordinate{fmt.Sprintf("log_tau[%d]", i), func(p *learning.Parameters, delta float64) { p.Core.LogTau[index] += delta }})
	}
	for i := range parameters.Encoder {
		index := i
		coordinates = append(coordinates, chemicalMaxParameterCoordinate{fmt.Sprintf("encoder[%d]", i), func(p *learning.Parameters, delta float64) { p.Encoder[index] += delta }})
	}
	for i := range parameters.Readout {
		index := i
		coordinates = append(coordinates, chemicalMaxParameterCoordinate{fmt.Sprintf("readout[%d]", i), func(p *learning.Parameters, delta float64) { p.Readout[index] += delta }})
	}
	return coordinates
}

func chemicalMaxGradientValues(gradient learning.Gradient) []float64 {
	values := make([]float64, 0, 10)
	values = append(values, gradient.Core.Weights...)
	values = append(values, gradient.Core.Bias...)
	values = append(values, gradient.Core.LogTau...)
	values = append(values, gradient.Encoder...)
	values = append(values, gradient.Readout...)
	return values
}

func chemicalMaxObjective(ctx context.Context, network *learning.Network, parameters learning.Parameters, inputs, upstream [][]float64) (float64, error) {
	rows, err := network.PredictAll(ctx, chemicalEpisodeCopyParameters(parameters), chemicalRecomputeRowsCopy(inputs))
	if err != nil {
		return 0, err
	}
	if len(rows) != len(upstream) {
		return 0, fmt.Errorf("prediction rows=%d, upstream rows=%d", len(rows), len(upstream))
	}
	var objective float64
	for row := range rows {
		if len(rows[row]) != len(upstream[row]) {
			return 0, fmt.Errorf("objective row %d width=%d, upstream width=%d", row, len(rows[row]), len(upstream[row]))
		}
		for column := range rows[row] {
			objective += rows[row][column] * upstream[row][column]
		}
	}
	if !chemicalMaxFinite(objective) {
		return 0, fmt.Errorf("objective is non-finite: %g", objective)
	}
	return objective, nil
}

func TestChemicalMaxContinuousGradientReference(t *testing.T) {
	ctx := context.Background()
	checked := 0
	for _, profile := range chemicalMaxCommonProfiles {
		t.Run(profile, func(t *testing.T) {
			config, parameters, inputs, _, upstream := chemicalMaxSetContinuousFixture(t, profile, 3)
			for row := range upstream {
				for column := range upstream[row] {
					upstream[row][column] *= 10
				}
			}
			network, err := learning.NewNetwork(config)
			if err != nil {
				t.Fatalf("NewNetwork: %v", err)
			}
			gradient, err := network.LossGradientFrom(ctx, parameters, chemicalRecomputeRowsCopy(inputs), chemicalRecomputeRowsCopy(upstream), 0)
			if err != nil {
				t.Fatalf("LossGradientFrom: %v", err)
			}
			if len(gradient.Core.Initial) != config.Dynamics.Nodes || len(gradient.Inputs) != len(inputs) {
				t.Fatalf("gradient shape initial=%d inputs=%d, want initial=%d inputs=%d", len(gradient.Core.Initial), len(gradient.Inputs), config.Dynamics.Nodes, len(inputs))
			}
			coordinates := chemicalMaxParameterCoordinates(parameters)
			if len(coordinates) != 10 {
				t.Fatalf("parameter coordinates=%d, want 10", len(coordinates))
			}
			values := chemicalMaxGradientValues(gradient)
			if len(values) != len(coordinates) {
				t.Fatalf("parameter gradient values=%d, coordinates=%d", len(values), len(coordinates))
			}
			informative := 0
			localChecked := 0
			check := func(name string, got float64, reference float64) {
				t.Helper()
				if !chemicalMaxFinite(got) || !chemicalMaxFinite(reference) {
					t.Fatalf("%s got/reference non-finite: %g/%g", name, got, reference)
				}
				tolerance := chemicalMaxReferenceEpsilon + chemicalMaxReferenceEpsilon*math.Abs(reference)
				if math.Abs(got-reference) > tolerance {
					t.Fatalf("%s got %.17g reference %.17g tolerance %.17g", name, got, reference, tolerance)
				}
				if math.Abs(reference) > tolerance {
					informative++
					if got == 0 {
						t.Fatalf("%s reference %.17g rejects a zero gradient", name, reference)
					}
				}
				localChecked++
			}
			for i, coordinate := range coordinates {
				plus, minus := chemicalEpisodeCopyParameters(parameters), chemicalEpisodeCopyParameters(parameters)
				coordinate.set(&plus, chemicalMaxReferenceEpsilon)
				coordinate.set(&minus, -chemicalMaxReferenceEpsilon)
				positive, err := chemicalMaxObjective(ctx, network, plus, inputs, upstream)
				if err != nil {
					t.Fatalf("%s positive objective: %v", coordinate.name, err)
				}
				negative, err := chemicalMaxObjective(ctx, network, minus, inputs, upstream)
				if err != nil {
					t.Fatalf("%s negative objective: %v", coordinate.name, err)
				}
				check(coordinate.name, values[i], (positive-negative)/(2*chemicalMaxReferenceEpsilon))
			}
			if len(gradient.Inputs) != 8 {
				t.Fatalf("input gradient rows=%d, want 8", len(gradient.Inputs))
			}
			for row := range inputs {
				if len(inputs[row]) != 1 || len(gradient.Inputs[row]) != 1 {
					t.Fatalf("input/gradient row %d widths=%d/%d, want 1/1", row, len(inputs[row]), len(gradient.Inputs[row]))
				}
				plus, minus := chemicalRecomputeRowsCopy(inputs), chemicalRecomputeRowsCopy(inputs)
				plus[row][0] += chemicalMaxReferenceEpsilon
				minus[row][0] -= chemicalMaxReferenceEpsilon
				positive, err := chemicalMaxObjective(ctx, network, parameters, plus, upstream)
				if err != nil {
					t.Fatalf("inputs[%d] positive objective: %v", row, err)
				}
				negative, err := chemicalMaxObjective(ctx, network, parameters, minus, upstream)
				if err != nil {
					t.Fatalf("inputs[%d] negative objective: %v", row, err)
				}
				check(fmt.Sprintf("inputs[%d]", row), gradient.Inputs[row][0], (positive-negative)/(2*chemicalMaxReferenceEpsilon))
			}
			if localChecked != 18 || informative == 0 {
				t.Fatalf("checked_coordinates=%d informative=%d, want 18 and at least 1", localChecked, informative)
			}
			checked += localChecked
			t.Logf("profile=%s checked_coordinates=%d informative=%d", profile, localChecked, informative)
		})
	}
	if checked != 108 {
		t.Fatalf("checked coordinates=%d, want 108", checked)
	}
}
