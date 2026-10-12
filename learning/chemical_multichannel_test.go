package learning_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

// Channel count and region count are independent dimensions. Sources address
// channels while kinetics reverse returns a [region][channel] grid.
func TestChemicalEpisodeIndependentChannelAndRegionDimensions(t *testing.T) {
	for _, dimensions := range []struct {
		name              string
		regions, channels int
	}{
		{"one_region_two_channels", 1, 2},
		{"three_regions_two_channels", 3, 2},
		{"two_regions_three_channels", 2, 3},
	} {
		t.Run(dimensions.name, func(t *testing.T) {
			episode := chemicalEpisodePulse(0, nil, nil, nil)
			declaration := &episode.Part.Config
			declaration.Chemistry.Regions = dimensions.regions
			declaration.Chemistry.Channels = dimensions.channels
			declaration.Chemistry.Tau = make([]float64, dimensions.channels)
			declaration.Sources = make([]modulation.SourceSpec, dimensions.channels)
			episode.Part.State.Concentration = make([][]float64, dimensions.regions)
			for region := range episode.Part.State.Concentration {
				episode.Part.State.Concentration[region] = make([]float64, dimensions.channels)
				episode.Part.State.Concentration[region][dimensions.channels-1] = 0.2
			}
			for channel := range declaration.Sources {
				declaration.Chemistry.Tau[channel] = 3
				declaration.Sources[channel] = modulation.SourceSpec{
					Kind: modulation.SourceExternalTimeline, Channel: channel,
					Timeline: &modulation.ExternalTimeline{ChannelCount: channel + 1, Entries: []modulation.TimelineEntry{}},
				}
			}
			channel := dimensions.channels - 1
			declaration.Sources[channel] = modulation.SourceSpec{
				Kind: modulation.SourceNeuralActivity, Channel: channel,
				Neural: &modulation.NeuralActivity{Channel: channel, Nodes: []int{0}, Gain: 1.3},
			}
			declaration.Receptors.Records[0].Channel = channel
			episode.InitialActivity = []float64{0.3}
			config := chemicalEpisodeScalarConfig(episode, false)
			parameters := chemicalEpisodeScalarParameters()
			parameters.Core.Bias[0] = 0.1
			network, err := learning.NewNetwork(config)
			if err != nil {
				t.Fatal(err)
			}
			input, target := [][]float64{{0.2}, {0.4}, {0.1}}, []float64{0.25}
			loss, gradient, err := network.LossGradient(context.Background(), parameters, input, target, 0)
			if err != nil {
				t.Fatalf("valid %d-region %d-channel chemical reverse failed: %v", dimensions.regions, dimensions.channels, err)
			}
			if math.IsNaN(loss) || math.IsInf(loss, 0) {
				t.Fatalf("non-finite loss %g", loss)
			}
			plus, minus := chemicalEpisodeScalarParameters(), chemicalEpisodeScalarParameters()
			plus.Core.Bias[0], minus.Core.Bias[0] = parameters.Core.Bias[0]+chemicalEpisodeEpsilon, parameters.Core.Bias[0]-chemicalEpisodeEpsilon
			plusLoss, _, err := network.LossGradient(context.Background(), plus, input, target, 0)
			if err != nil {
				t.Fatal(err)
			}
			minusLoss, _, err := network.LossGradient(context.Background(), minus, input, target, 0)
			if err != nil {
				t.Fatal(err)
			}
			finiteDifference := (plusLoss - minusLoss) / (2 * chemicalEpisodeEpsilon)
			if math.Abs(gradient.Core.Bias[0]-finiteDifference) > 1e-4+0.003*math.Abs(finiteDifference) {
				t.Fatalf("multi-channel bias gradient %g differs from full-path finite difference %g", gradient.Core.Bias[0], finiteDifference)
			}
			trainer, err := learning.NewTrainer(config, parameters, learning.DefaultOptions())
			if err != nil {
				t.Fatal(err)
			}
			defer trainer.Close()
			before := trainer.Snapshot()
			result, err := trainer.Step(context.Background(), input, target)
			if err != nil {
				t.Fatalf("valid multi-channel Step failed: %v", err)
			}
			after := trainer.Snapshot()
			if !result.Applied || after.Updates != before.Updates+1 || reflect.DeepEqual(before.Parameters, after.Parameters) {
				t.Fatal("multi-channel Step did not apply an update")
			}
			if !reflect.DeepEqual(before.Config, after.Config) {
				t.Fatal("multi-channel Step changed the chemical boundary")
			}
		})
	}
}
