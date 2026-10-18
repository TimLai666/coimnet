// Command driver2 dumps randomized persistent-individual cases (LIF and mixed
// cores with local plasticity, chemistry and consolidation) for the
// independent reference ../reference2.py, plus targeted repros of suspected
// doc/behaviour mismatches. Audit tool only.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"reflect"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

type obj = map[string]any

var ctx = context.Background()

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func normal(r *rand.Rand, mu, sd float64) float64 { return mu + sd*r.NormFloat64() }
func unif(r *rand.Rand, a, b float64) float64     { return a + (b-a)*r.Float64() }
func vec(r *rand.Rand, n int, mu, sd float64) []float64 {
	v := make([]float64, n)
	for i := range v {
		v[i] = normal(r, mu, sd)
	}
	return v
}
func mat(r *rand.Rand, t, n int, mu, sd float64) [][]float64 {
	m := make([][]float64, t)
	for i := range m {
		m[i] = vec(r, n, mu, sd)
	}
	return m
}

func lifConfig(r *rand.Rand, n int) dynamics.LIFConfig {
	var src, tgt, del []int
	e := 3 + r.IntN(3*n)
	for k := 0; k < e; k++ {
		src, tgt, del = append(src, r.IntN(n)), append(tgt, r.IntN(n)), append(del, r.IntN(3))
	}
	tmin := unif(r, 0.2, 0.6)
	c := dynamics.LIFConfig{
		Nodes: n, Sources: src, Targets: tgt, Delays: del, DT: unif(r, 0.5, 1), TauSyn: unif(r, 1, 5),
		ThetaMin: tmin, ThetaMax: tmin + unif(r, 0.3, 1), VReset: unif(r, -0.5, tmin-0.1), RefractorySteps: r.IntN(3),
		Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
	if r.IntN(2) == 0 {
		c.Adaptation = dynamics.LIFAdaptation{Enabled: true, TauAdapt: unif(r, 2, 10), Beta: unif(r, 0, 0.5)}
	}
	if r.IntN(2) == 0 {
		c.Homeostasis = &dynamics.LIFHomeostasis{Enabled: true, TauRate: unif(r, 2, 10), TargetRate: unif(r, 0, 0.5), Eta: unif(r, 0, 0.8), HMax: unif(r, 0, 1)}
	}
	return c
}

// individualCase: LIF or mixed core, optional plasticity (hebbian/stdp),
// optional chemistry (external timeline + hypothesized receptors + effects),
// split into two AdvanceGated calls with a snapshot JSON round trip between
// them; the uninterrupted run is compared bit for bit in Go and dumped for
// the reference.
func individualCase(r *rand.Rand, seed uint64, kind string, plastic, chemistry bool) obj {
	n := 4 + r.IntN(3)
	lc := lifConfig(r, n)
	cfg := learning.Config{InputSize: 2, OutputSize: 2, InputNodes: []int{0, 1}, ReadoutNodes: []int{n - 1, n - 2}}
	var p learning.Parameters
	lt := make([]float64, n)
	for i := range lt {
		lt[i] = math.Log(unif(r, 1, 5))
	}
	var rule []uint8
	if kind == "lif" {
		cfg.LIF = &lc
		p.ThetaRaw = vec(r, n, 0, 1)
	} else {
		rule = make([]uint8, n)
		nl := 0
		for i := range rule {
			rule[i] = uint8(r.IntN(2))
			nl += int(rule[i])
		}
		if nl == 0 {
			rule[0], nl = 1, 1
		}
		if nl == n {
			rule[n-1], nl = 0, nl-1
		}
		cfg.Mixed = &dynamics.MixedConfig{Nodes: n, Sources: lc.Sources, Targets: lc.Targets, Delays: lc.Delays, DT: lc.DT, NodeRule: rule,
			Continuous: dynamics.ContinuousRule{Activation: "tanh"},
			LIF: dynamics.LIFRule{TauSyn: lc.TauSyn, ThetaMin: lc.ThetaMin, ThetaMax: lc.ThetaMax, VReset: lc.VReset,
				RefractorySteps: lc.RefractorySteps, Adaptation: lc.Adaptation, Homeostasis: lc.Homeostasis, Surrogate: lc.Surrogate}}
		p.ThetaRaw = vec(r, nl, 0, 1)
	}
	E := len(lc.Sources)
	if r.IntN(2) == 0 {
		signs := make([]int8, E)
		for i := range signs {
			signs[i] = int8(r.IntN(3) - 1)
		}
		cfg.EdgeSigns = signs
	}
	p.Core = dynamics.Parameters{Weights: vec(r, E, 0, 1), Bias: vec(r, n, 0.2, 0.3), LogTau: lt}
	p.Encoder = vec(r, 4, 0.8, 0.5)
	p.Readout = vec(r, 4, 0, 1)
	initial := vec(r, n, 0, 0.3)
	ind, err := learning.NewIndividual(cfg, p, learning.DefaultOptions(), initial)
	must(err)
	var pc *plasticity.Config
	if plastic {
		ruleKind := plasticity.RuleHebbianRate
		if kind == "lif" && r.IntN(2) == 0 {
			ruleKind = plasticity.RuleSTDPPair
		}
		pr := plasticity.Rule{Kind: ruleKind, DecayE: unif(r, 0.3, 0.95), DecayP: unif(r, 0.5, 0.99), PlasticMax: unif(r, 0.2, 1), WMin: unif(r, 0.01, 0.1)}
		if ruleKind == plasticity.RuleSTDPPair {
			pr.DecayPre, pr.DecayPost, pr.APlus, pr.AMinus = unif(r, 0.3, 0.95), unif(r, 0.3, 0.95), unif(r, 0, 1), unif(r, 0, 1)
		}
		var edges []int
		for e := 0; e < E; e++ {
			if r.IntN(3) != 0 {
				edges = append(edges, e)
			}
		}
		if len(edges) == 0 {
			edges = []int{0}
		}
		c := plasticity.Config{Rule: pr, Edges: edges}
		must(ind.EnablePlasticity(c))
		pc = &c
	}
	var cc *modulation.ChemistryConfig
	if chemistry {
		regions := 1 + r.IntN(2)
		var transport *modulation.Transport
		if regions == 2 && r.IntN(2) == 0 {
			transport = &modulation.Transport{Fraction: [][]float64{{0, unif(r, 0, 0.5)}, {unif(r, 0, 0.5), 0}}}
		}
		var entries []modulation.TimelineEntry
		for s := uint64(0); s < 20; s++ {
			if r.IntN(3) == 0 {
				entries = append(entries, modulation.TimelineEntry{Step: s, Channel: 0, Rate: unif(r, 0, 2)})
			}
		}
		nodeRegion := make([]int, n)
		for i := range nodeRegion {
			nodeRegion[i] = r.IntN(regions)
		}
		var lifCells []int
		for i := 0; i < n; i++ {
			if kind == "lif" || rule[i] == 1 {
				lifCells = append(lifCells, i)
			}
		}
		allCells := make([]int, n)
		for i := range allCells {
			allCells[i] = i
		}
		recs := []modulation.Receptor{
			{Cells: allCells, Signal: "s", Channel: 0, Status: modulation.StatusHypothesized, Kd: unif(r, 0.2, 2), N: 1 + r.Float64()*2, Evidence: "audit", MeasurementKind: "engineering", MappingVersion: "v0"},
			{Cells: lifCells, Signal: "s", Channel: 0, Status: modulation.StatusHypothesized, Kd: unif(r, 0.2, 2), N: 1 + r.Float64()*2, Evidence: "audit", MeasurementKind: "engineering", MappingVersion: "v0"},
		}
		c := modulation.ChemistryConfig{
			Chemistry: modulation.Chemistry{Regions: regions, Channels: 1, DT: unif(r, 0.5, 1), Tau: []float64{unif(r, 1, 5)}, Transport: transport},
			Sources:   []modulation.SourceSpec{{Kind: modulation.SourceExternalTimeline, Channel: 0, Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: entries}}},
			Receptors: modulation.Receptors{Records: recs, Mix: []string{"", modulation.MixMax}[r.IntN(2)]},
			Effects: []modulation.Effect{
				{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: normal(r, 0, 1), BetaScale: normal(r, 0, 0.5)},
				{Kind: modulation.EffectThreshold, Receptor: 1, ThetaScale: normal(r, 0, 0.5), ThetaAbsMax: unif(r, 0.1, 0.5)},
			},
			Regions: modulation.RegionAssignment{NodeRegion: nodeRegion},
		}
		must(ind.EnableChemistry(c))
		cc = &c
	}
	T := 8 + r.IntN(6)
	input := mat(r, T, 2, 0.8, 1)
	gate := vec(r, T, 0.5, 1)
	if !plastic {
		gate = make([]float64, T)
	}
	k := 1 + r.IntN(T-1)
	snap0 := ind.Snapshot()
	twin, err := learning.RestoreIndividual(roundTrip(snap0))
	must(err)
	outAll, repAll, err := twin.AdvanceGated(ctx, input, gate)
	must(err)
	snapAll := twin.Snapshot()
	out1, _, err := ind.AdvanceGated(ctx, input[:k], gate[:k])
	must(err)
	mid, err := learning.RestoreIndividual(roundTrip(ind.Snapshot()))
	must(err)
	out2, _, err := mid.AdvanceGated(ctx, input[k:], gate[k:])
	must(err)
	snapSplit := mid.Snapshot()
	checks := obj{
		"resume_outputs_equal": reflect.DeepEqual(append(out1, out2...), outAll),
		"resume_state_equal":   reflect.DeepEqual(roundTrip(snapSplit), roundTrip(snapAll)),
	}
	// Disabled-plasticity bit identity: a zero gate run with plasticity
	// frozen-at-zero still changes the fast state, so compare instead
	// "never enabled" against a fresh twin without the mechanism.
	if !plastic && !chemistry {
		ind2, err := learning.NewIndividual(cfg, p, learning.DefaultOptions(), initial)
		must(err)
		o, err := ind2.Advance(ctx, input)
		must(err)
		checks["plain_equals_gated"] = reflect.DeepEqual(o, outAll)
	}
	var consolidation obj
	if plastic && chemistry {
		must(twin.EnableConsolidation(3))
		before := twin.Snapshot()
		rep, err := twin.Consolidate(ctx, learning.ConsolidationTrigger{Receptor: 0, Threshold: 0, Rate: unif(r, 0.1, 1), Retain: unif(r, 0, 1)})
		after := twin.Snapshot()
		consolidation = obj{"before": before.Plastic, "after": after.Plastic, "report": rep, "err": fmt.Sprint(err)}
		// After consolidation, the next advance must integrate (base+slow)+plastic.
		nextIn := mat(r, 3, 2, 0.8, 1)
		o, _, err := twin.AdvanceGated(ctx, nextIn, []float64{0, 0, 0})
		must(err)
		consolidation["next_input"], consolidation["next_out"] = nextIn, o
		consolidation["state_before_next"] = after
	}
	return obj{
		"seed": seed, "kind": kind, "config": snap0.Config, "params": p, "initial": initial, "plasticity": pc, "chemistry": cc,
		"input": input, "gate": gate, "outputs": outAll, "report": repAll, "final": snapAll, "checks": checks, "consolidation": consolidation,
	}
}

func roundTrip(s learning.IndividualSnapshot) learning.IndividualSnapshot {
	data, err := json.Marshal(s)
	must(err)
	var out learning.IndividualSnapshot
	must(json.Unmarshal(data, &out))
	return out
}

// repros of targeted suspicions.
func repros() obj {
	out := obj{}
	// R1: a sensitivity effect whose declared gamma bounds exclude 1 changes
	// the gain at zero occupancy, contradicting "with every occupancy at zero
	// the result is exactly the neutral arrays".
	occ := []modulation.OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: 0}}
	g, o, th, rep, err := modulation.ApplyEffects([]modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaMin: 1.5, GammaMax: 2}}, occ, "", 2)
	out["R1_gamma_bounds_exclude_one"] = obj{"gain": g, "offset": o, "threshold": th, "clamped": rep, "err": fmt.Sprint(err)}
	g, _, _, rep, err = modulation.ApplyEffects([]modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaMin: 0.25, GammaMax: 0.5}}, occ, "", 2)
	out["R1b_gamma_max_below_one"] = obj{"gain": g, "clamped": rep, "err": fmt.Sprint(err)}
	// R1c: and ChemistryConfig.Validate accepts the declaration.
	cfg := modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources:   []modulation.SourceSpec{{Kind: modulation.SourceExternalTimeline, Channel: 0, Timeline: &modulation.ExternalTimeline{ChannelCount: 1}}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{Cells: []int{0}, Signal: "s", Channel: 0, Status: modulation.StatusUnresponsive, Evidence: "e", MeasurementKind: "m", MappingVersion: "v"}}},
		Effects:   []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaMin: 1.5, GammaMax: 2}},
		Regions:   modulation.RegionAssignment{NodeRegion: []int{0, 0}},
	}
	out["R1c_validate"] = fmt.Sprint(cfg.Validate(2))

	// R2: an individual whose chemistry has a neural_activity source cannot
	// take its first step from a fresh state.
	lc := dynamics.LIFConfig{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: 1, TauSyn: 1, ThetaMin: 0.1, ThetaMax: 1, VReset: 0, Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2}}
	ncfg := learning.Config{LIF: &lc, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1}, InputNodes: []int{0}}
	p := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{1}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}}, ThetaRaw: []float64{0, 0}, Encoder: []float64{1}, Readout: []float64{1}}
	ind, err := learning.NewIndividual(ncfg, p, learning.DefaultOptions(), []float64{0, 0})
	must(err)
	chem := modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources:   []modulation.SourceSpec{{Kind: modulation.SourceNeuralActivity, Channel: 0, Neural: &modulation.NeuralActivity{Nodes: []int{0}, Gain: 1, Channel: 0}}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{Cells: []int{1}, Signal: "s", Channel: 0, Status: modulation.StatusHypothesized, Kd: 1, N: 1, Evidence: "e", MeasurementKind: "m", MappingVersion: "v"}}},
		Effects:   []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 1}},
		Regions:   modulation.RegionAssignment{NodeRegion: []int{0, 0}},
	}
	errEnable := ind.EnableChemistry(chem)
	_, errAdvance := ind.Advance(ctx, [][]float64{{1}, {1}})
	out["R2_neural_source_fresh_individual"] = obj{"enable_err": fmt.Sprint(errEnable), "advance_err": fmt.Sprint(errAdvance)}
	// Same individual after one plain step (chemistry disabled) works?
	ind2, _ := learning.NewIndividual(ncfg, p, learning.DefaultOptions(), []float64{0, 0})
	_, e1 := ind2.Advance(ctx, [][]float64{{1}})
	e2 := ind2.EnableChemistry(chem)
	_, e3 := ind2.Advance(ctx, [][]float64{{1}})
	out["R2b_after_one_plain_step"] = obj{"advance0": fmt.Sprint(e1), "enable": fmt.Sprint(e2), "advance1": fmt.Sprint(e3)}

	// R3: modulated threshold floor: a negative shift that would take
	// theta_eff below theta_min is floored; a zero shift leaves theta_base+...
	m, err := dynamics.NewLIF(lc)
	must(err)
	s, _ := m.NewState([]float64{0, 0})
	lp := dynamics.LIFParameters{Weights: []float64{0}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{-50, -50}}
	in := [][]float64{{0.1 / (1 - math.Exp(-1)) * 0.999, 0}}
	_, _, sp, err := m.AdvanceModulated(ctx, lp, s, in, &dynamics.Modulation{Threshold: [][]float64{{-0.5, 0}}})
	out["R3_threshold_floor"] = obj{"spikes": sp, "err": fmt.Sprint(err)}
	_ = errors.New
	return out
}

func main() {
	outPath := flag.String("out", "cases2.json", "output path")
	count := flag.Int("n", 30, "cases per family")
	base := flag.Uint64("seed", 11, "base seed")
	flag.Parse()
	var cases []obj
	for k := 0; k < *count; k++ {
		seed := *base*1000 + uint64(k)
		r := rand.New(rand.NewPCG(seed, 9))
		for _, kind := range []string{"lif", "mixed"} {
			for _, pl := range []bool{false, true} {
				for _, ch := range []bool{false, true} {
					cases = append(cases, individualCase(r, seed, kind, pl, ch))
				}
			}
		}
	}
	data, err := json.Marshal(obj{"individual": cases, "repros": repros()})
	must(err)
	must(os.WriteFile(*outPath, data, 0o644))
	fmt.Println("wrote", *outPath)
}
