// Command driver4 dumps modulation.Controller releases and objective
// gradients, plus Kinetics.Step / Occupancy samples, for ../reference4.py.
package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"

	"github.com/TimLai666/coimnet/modulation"
)

type obj = map[string]any

func main() {
	var out []obj
	for seed := uint64(0); seed < 50; seed++ {
		r := rand.New(rand.NewPCG(seed, 3))
		nodes, hidden, window := 1+r.IntN(4), 1+r.IntN(5), 1+r.IntN(4)
		c := &modulation.Controller{Inputs: modulation.ControllerInputs{SummaryWindow: window, UseFeedback: false, Resources: []string{"food"}}, Nodes: nodes, Hidden: hidden, Channel: 0}
		c.Parameters = make([]float64, c.ParameterCount())
		for i := range c.Parameters {
			c.Parameters[i] = r.NormFloat64()
		}
		params := append([]float64(nil), c.Parameters...)
		var acts [][]float64
		var rels []float64
		res := map[string]float64{"food": r.NormFloat64()}
		steps := 1 + r.IntN(6)
		for s := 0; s < steps; s++ {
			a := make([]float64, nodes)
			for i := range a {
				a[i] = r.NormFloat64()
			}
			if r.IntN(5) == 0 {
				a = nil
			}
			acts = append(acts, a)
			q, err := c.Release(uint64(s), modulation.SourceContext{Activity: a, Resources: res})
			if err != nil {
				panic(err)
			}
			rels = append(rels, q[0])
		}
		target := r.NormFloat64()
		loss, grad, err := c.Gradient(modulation.ControllerObjective{Kind: modulation.ObjectiveActivityTarget, Target: target})
		if err != nil {
			panic(err)
		}
		out = append(out, obj{"nodes": nodes, "hidden": hidden, "window": window, "params": params, "activity": acts, "resource": res["food"], "release": rels, "target": target, "loss": loss, "grad": grad})
	}
	// Kinetics samples with clearance boost and transport.
	var kin []obj
	for seed := uint64(0); seed < 50; seed++ {
		r := rand.New(rand.NewPCG(seed, 5))
		R, K := 1+r.IntN(3), 1+r.IntN(2)
		tau := make([]float64, K)
		for k := range tau {
			tau[k] = 0.5 + 5*r.Float64()
		}
		var tr *modulation.Transport
		if R > 1 {
			f := make([][]float64, R)
			for i := range f {
				f[i] = make([]float64, R)
				for j := range f[i] {
					if i != j {
						f[i][j] = r.Float64() / float64(R)
					}
				}
			}
			tr = &modulation.Transport{Fraction: f}
		}
		kn, err := modulation.NewChemistry(modulation.Chemistry{Regions: R, Channels: K, DT: 0.1 + r.Float64(), Tau: tau, Transport: tr})
		if err != nil {
			panic(err)
		}
		s := kn.NewState()
		grid := func(scale float64) [][]float64 {
			g := make([][]float64, R)
			for i := range g {
				g[i] = make([]float64, K)
				for k := range g[i] {
					if r.IntN(2) == 0 {
						g[i][k] = scale * r.Float64()
					}
				}
			}
			return g
		}
		var steps []obj
		for t := 0; t < 5; t++ {
			rel, boost := grid(2), grid(1)
			n, err := kn.Step(s, rel, boost)
			if err != nil {
				panic(err)
			}
			steps = append(steps, obj{"release": rel, "boost": boost, "state": n.Concentration})
			s = n
		}
		kin = append(kin, obj{"config": kn.Config(), "steps": steps})
	}
	var occ []obj
	for _, c := range []float64{0, 1e-300, 1e-5, 0.3, 1, 2.5, 1e5, 1e300} {
		for _, kd := range []float64{0.1, 1, 3} {
			for _, n := range []float64{1, 2.5, 8} {
				occ = append(occ, obj{"c": c, "kd": kd, "n": n, "occ": modulation.Occupancy(c, kd, n)})
			}
		}
	}
	data, _ := json.Marshal(obj{"controller": out, "kinetics": kin, "occupancy": occ})
	if err := os.WriteFile(os.Args[1], data, 0o644); err != nil {
		panic(err)
	}
	fmt.Println("wrote", os.Args[1])
}
