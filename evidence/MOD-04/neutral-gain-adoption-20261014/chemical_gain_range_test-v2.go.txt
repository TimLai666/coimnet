package learning_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/learning"
)

func TestNeutralGainRejectsConstructionAndRestore(t *testing.T) {
	for _, tc := range []struct {
		name     string
		min, max float64
	}{
		{"lower_excludes", 1.25, 0}, {"upper_excludes", 0, .75}, {"point_excludes", 1.5, 1.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := chemicalEpisodePulse(0, nil, nil, nil)
			bad.Part.Config.Effects[0].GammaMin = tc.min
			bad.Part.Config.Effects[0].GammaMax = tc.max
			config := chemicalEpisodeScalarConfig(bad, true)
			parameters := chemicalEpisodeScalarParameters()
			options := learning.DefaultOptions()
			original, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			network, err := learning.NewNetwork(config)
			if err == nil || network != nil {
				t.Error("NewNetwork accepted range excluding neutral gain")
			}
			trainer, err := learning.NewTrainer(config, parameters, options)
			if err == nil || trainer != nil {
				t.Error("NewTrainer accepted range excluding neutral gain")
			}
			now, _ := json.Marshal(config)
			if string(now) != string(original) {
				t.Fatal("construction refusal changed caller config")
			}

			valid := chemicalEpisodePulse(0, nil, nil, nil)
			trainer, err = learning.NewTrainer(chemicalEpisodeScalarConfig(valid, true), parameters, options)
			if err != nil {
				t.Fatal(err)
			}
			saved := trainer.Snapshot()
			saved.Config.Chemistry.Part.Config.Effects[0].GammaMin = tc.min
			saved.Config.Chemistry.Part.Config.Effects[0].GammaMax = tc.max
			before, _ := json.Marshal(saved)
			restored, err := learning.RestoreTrainer(saved)
			if err == nil || restored != nil {
				t.Error("RestoreTrainer accepted range excluding neutral gain")
			}
			after, _ := json.Marshal(saved)
			if string(after) != string(before) {
				t.Fatal("RestoreTrainer changed caller snapshot")
			}

			individual, err := learning.NewIndividual(chemicalEpisodeScalarConfig(nil, true), parameters, options, []float64{0})
			if err != nil {
				t.Fatal(err)
			}
			if err = individual.EnableChemistry(valid.Part.Config); err != nil {
				t.Fatal(err)
			}
			if _, err = individual.Advance(context.Background(), [][]float64{{.4}}); err != nil {
				t.Fatal(err)
			}
			state := individual.Snapshot()
			err = individual.EnableChemistry(bad.Part.Config)
			if err == nil || !strings.Contains(err.Error(), "gamma") {
				t.Error("EnableChemistry accepted range excluding neutral gain")
			}
			if !reflect.DeepEqual(state, individual.Snapshot()) {
				t.Error("failed chemistry replacement changed live individual")
			}
			savedIndividual := state
			savedIndividual.Chemical.Config.Effects[0].GammaMin = tc.min
			savedIndividual.Chemical.Config.Effects[0].GammaMax = tc.max
			before, _ = json.Marshal(savedIndividual)
			loaded, err := learning.RestoreIndividual(savedIndividual)
			if err == nil || loaded != nil {
				t.Error("RestoreIndividual accepted range excluding neutral gain")
			}
			encoded, err := checkpoint.EncodeIndividual(savedIndividual)
			if err == nil || encoded != nil {
				t.Error("checkpoint encoder accepted range excluding neutral gain")
			}
			after, _ = json.Marshal(savedIndividual)
			if string(after) != string(before) {
				t.Fatal("restore or encode changed caller snapshot")
			}
		})
	}
}
