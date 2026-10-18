// Command driver3 reproduces the reward-baseline side effect of
// learning.NewOnlineLearner's construction-time probe gate.Applied(0) on a
// LIF individual with local plasticity. Audit tool only.
package main

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
	"github.com/TimLai666/coimnet/signal"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func run(probeFree bool) []modulation.RewardRecord {
	ctx := context.Background()
	lc := dynamics.LIFConfig{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: 1, TauSyn: 1, ThetaMin: 0.1, ThetaMax: 1, VReset: 0, Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2}}
	cfg := learning.Config{LIF: &lc, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1}, InputNodes: []int{0}}
	p := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{1}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}}, ThetaRaw: []float64{0, 0}, Encoder: []float64{1}, Readout: []float64{1}}
	ind, err := learning.NewIndividual(cfg, p, learning.DefaultOptions(), []float64{0, 0})
	must(err)
	must(ind.EnablePlasticity(plasticity.Config{Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625}, Edges: []int{0}}))
	mapper := &modulation.RewardMapper{Baseline: modulation.BaselineRunningMean, Window: 2, Kind: modulation.KindRelu}
	var records []modulation.RewardRecord
	constructing := true
	gate := learning.RewardGateFunc(func(score float64) ([]float64, error) {
		if constructing && probeFree {
			// What a side-effect-free probe would return.
			return []float64{0}, nil
		}
		rec, err := mapper.Map(score)
		if !constructing {
			records = append(records, rec)
		}
		return rec.Applied, err
	})
	l, err := learning.NewOnlineLearner(ind, learning.OnlinePolicy{AllowPlastic: true, ClockUnit: signal.TimeUnitModelStep}, gate)
	must(err)
	constructing = false
	obs, err := signal.NewObservation(signal.CurrentSchemaVersion(), "exp-1", "s", nil)
	must(err)
	version, _ := signal.NewVersion(1, 0)
	for k := 0; k < 2; k++ {
		rec, err := l.Act(ctx, obs, [][]float64{{1}, {0}})
		must(err)
		fb, err := signal.NewFeedback(signal.FeedbackSpec{SchemaVersion: signal.CurrentSchemaVersion(), ExperienceID: "exp-1", ActionID: rec.ActionID,
			ProducedAt: signal.Timestamp{Value: int64(rec.Step), Unit: signal.TimeUnitModelStep}, AvailableAt: signal.Timestamp{Value: int64(rec.Step), Unit: signal.TimeUnitModelStep},
			Source: "teacher", Score: 1, ModelVersion: version})
		must(err)
		must(l.Receive(ctx, fb))
		_, err = l.Update(ctx, signal.Timestamp{Value: int64(rec.Step), Unit: signal.TimeUnitModelStep})
		must(err)
	}
	return records
}

func main() {
	fmt.Println("as shipped (probe goes through the mapper):")
	for i, r := range run(false) {
		fmt.Printf("  feedback %d: raw=%g expected=%g applied=%v\n", i, r.Raw, r.Expected, r.Applied)
	}
	fmt.Println("with a side-effect-free probe (what the documented rule gives):")
	for i, r := range run(true) {
		fmt.Printf("  feedback %d: raw=%g expected=%g applied=%v\n", i, r.Raw, r.Expected, r.Applied)
	}
}
