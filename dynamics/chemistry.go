package dynamics

import (
	"fmt"
	"math"
)

// Chemistry declares an optional activity-driven neuromodulation block that
// lives inside the scalar continuous forward pass, so Backward differentiates
// exactly the modulated trajectory Forward and Infer run. It is an engineering
// mechanism with the shape of the persistent modulation layer (release,
// first-order concentration kinetics, Hill receptor occupancy and a gain /
// offset effect on the input current), not a measured biological model: one
// well-mixed region, no transport, and every constant below is a declared
// engineering value.
//
// Each step t of a fresh episode, in this order (concentration c_k(-1) = 0):
//
//	m_k(t)   = mean over j in Channels[k].Sources of y_j(t), the outputs at the start of step t
//	q_k(t)   = softplus(r_k * m_k(t))                    r_k = Parameters.ChemRelease[k]
//	c_k(t)   = lambda_k*c_k(t-1) + tau_k*(1-lambda_k)*q_k(t),  lambda_k = exp(-dt/tau_k)
//	occ_k(t) = c^n/(Kd^n + c^n) with the channel's Kd and n (Hill)
//	gamma_i  = clamp(1 + sum_e gs_e*occ_{k(e)}, GammaMin, GammaMax)
//	beta_i   = clamp(sum_e bs_e*occ_{k(e)}, -BetaAbsMax, BetaAbsMax)
//	drive_i  = gamma_i*(external_i + synaptic_i) + beta_i + bias_i
//
// where e runs over the receptors declared on node i, gs_e =
// Parameters.ChemGain[e] and bs_e = Parameters.ChemOffset[e]. The drive then
// enters the gate or the leaky integrator unchanged. The effect at step t uses
// the concentration computed from activity before step t, so the loop is
// causal. Softplus keeps the release positive and differentiable; note that a
// zero release gain still releases log(2) per step, a constant baseline. An
// active clamp passes no gradient (equality is treated as inside the range).
//
// Only Forward, Infer, InferLast, Backward and BackwardAligned on fresh
// episodes support it; persistent state, recompute, vector nodes and both GPU
// paths refuse a model that declares it.
type Chemistry struct {
	Channels   []ChemistryChannel  `json:"channels"`
	Receptors  []ChemistryReceptor `json:"receptors"`
	GammaMin   float64             `json:"gamma_min"`
	GammaMax   float64             `json:"gamma_max"`
	BetaAbsMax float64             `json:"beta_abs_max"`
}

// ChemistryChannel is one released substance: its fixed time constant (in the
// units of Config.DT), the dissociation constant and Hill exponent its
// receptors share, and the nodes whose mean output drives its release.
type ChemistryChannel struct {
	Tau     float64 `json:"tau"`
	Kd      float64 `json:"kd"`
	Hill    float64 `json:"hill"`
	Sources []int   `json:"sources"`
}

// ChemistryReceptor declares that Node responds to Channel. Each declared
// receptor owns one learnable gain (ChemGain) and offset (ChemOffset) entry,
// in declared order.
type ChemistryReceptor struct {
	Node    int `json:"node"`
	Channel int `json:"channel"`
}

// MaxChemistryChannels bounds the declared channel count.
const MaxChemistryChannels = 1024

// chemPlan is a validated Chemistry resolved for the step loops.
type chemPlan struct {
	lambda, weight []float64 // per channel: exp(-dt/tau), tau*(1-lambda)
	kd, hill       []float64
	sources        [][]int
	// nodeReceptors lists, per node, its receptor indices in declared order.
	nodeReceptors               [][]int
	receptorChannel             []int
	gammaMin, gammaMax, betaAbs float64
	receptors                   int
}

func copyChemistry(c *Chemistry) *Chemistry {
	if c == nil {
		return nil
	}
	out := &Chemistry{GammaMin: c.GammaMin, GammaMax: c.GammaMax, BetaAbsMax: c.BetaAbsMax}
	out.Channels = make([]ChemistryChannel, len(c.Channels))
	for k, ch := range c.Channels {
		ch.Sources = append([]int(nil), ch.Sources...)
		out.Channels[k] = ch
	}
	out.Receptors = append([]ChemistryReceptor(nil), c.Receptors...)
	return out
}

func newChemPlan(c *Chemistry, nodes int, dt float64) (*chemPlan, error) {
	if c == nil {
		return nil, nil
	}
	if len(c.Channels) == 0 || len(c.Channels) > MaxChemistryChannels {
		return nil, fmt.Errorf("chemistry needs 1 to %d channels, got %d", MaxChemistryChannels, len(c.Channels))
	}
	if !finite(c.GammaMin) || !finite(c.GammaMax) || c.GammaMin > 1 || c.GammaMax < 1 {
		return nil, fmt.Errorf("chemistry gamma range [%g, %g] must be finite and contain 1", c.GammaMin, c.GammaMax)
	}
	if !finite(c.BetaAbsMax) || c.BetaAbsMax < 0 {
		return nil, fmt.Errorf("chemistry beta_abs_max %g must be finite and nonnegative", c.BetaAbsMax)
	}
	k := len(c.Channels)
	p := &chemPlan{
		lambda: make([]float64, k), weight: make([]float64, k), kd: make([]float64, k), hill: make([]float64, k),
		sources: make([][]int, k), nodeReceptors: make([][]int, nodes), receptorChannel: make([]int, len(c.Receptors)),
		gammaMin: c.GammaMin, gammaMax: c.GammaMax, betaAbs: c.BetaAbsMax, receptors: len(c.Receptors),
	}
	for ch, channel := range c.Channels {
		if !finite(channel.Tau) || channel.Tau <= 0 || !finite(channel.Kd) || channel.Kd <= 0 || !finite(channel.Hill) || channel.Hill <= 0 || channel.Hill > 16 {
			return nil, fmt.Errorf("chemistry channel %d needs finite positive tau and kd and a hill exponent in (0, 16]", ch)
		}
		if len(channel.Sources) == 0 {
			return nil, fmt.Errorf("chemistry channel %d declares no source node", ch)
		}
		seen := make(map[int]bool, len(channel.Sources))
		for _, s := range channel.Sources {
			if s < 0 || s >= nodes || seen[s] {
				return nil, fmt.Errorf("chemistry channel %d has an invalid or duplicate source %d", ch, s)
			}
			seen[s] = true
		}
		p.lambda[ch] = math.Exp(-dt / channel.Tau)
		// tau*(1-lambda) without cancellation for small dt/tau.
		p.weight[ch] = channel.Tau * -math.Expm1(-dt/channel.Tau)
		if !finite(p.weight[ch]) {
			return nil, fmt.Errorf("chemistry channel %d release weight is not representable", ch)
		}
		p.kd[ch], p.hill[ch] = channel.Kd, channel.Hill
		p.sources[ch] = append([]int(nil), channel.Sources...)
	}
	seen := make(map[[2]int]bool, len(c.Receptors))
	for e, r := range c.Receptors {
		if r.Node < 0 || r.Node >= nodes || r.Channel < 0 || r.Channel >= k {
			return nil, fmt.Errorf("chemistry receptor %d has node %d or channel %d out of range", e, r.Node, r.Channel)
		}
		key := [2]int{r.Node, r.Channel}
		if seen[key] {
			return nil, fmt.Errorf("chemistry receptor %d repeats node %d channel %d", e, r.Node, r.Channel)
		}
		seen[key] = true
		p.nodeReceptors[r.Node] = append(p.nodeReceptors[r.Node], e)
		p.receptorChannel[e] = r.Channel
	}
	return p, nil
}

// checkChemistryParameters requires one finite release gain per channel and
// one gain and offset per receptor on a model that declares chemistry, and
// none of them otherwise.
func (m *Continuous) checkChemistryParameters(p Parameters) error {
	if m.chem == nil {
		if len(p.ChemRelease) != 0 || len(p.ChemGain) != 0 || len(p.ChemOffset) != 0 {
			return fmt.Errorf("chemistry parameters require Config.Chemistry")
		}
		return nil
	}
	if err := vector(p.ChemRelease, len(m.chem.lambda), "chem_release"); err != nil {
		return err
	}
	if err := vector(p.ChemGain, m.chem.receptors, "chem_gain"); err != nil {
		return err
	}
	return vector(p.ChemOffset, m.chem.receptors, "chem_offset")
}

// refuseChemistry is the error of an execution path chemistry does not cover.
func (m *Continuous) refuseChemistry(path string) error {
	if m != nil && m.config.Chemistry != nil {
		return fmt.Errorf("%s does not support a model with chemistry", path)
	}
	return nil
}

func softplus(x float64) float64 { return math.Max(x, 0) + math.Log1p(math.Exp(-math.Abs(x))) }

func sigmoid(x float64) float64 {
	if x >= 0 {
		return 1 / (1 + math.Exp(-x))
	}
	e := math.Exp(x)
	return e / (1 + e)
}

// occupancy is c^n/(Kd^n + c^n) and its derivative in c, computed as
// sigmoid(n*(log c - log Kd)) so large exponents cannot overflow. A
// concentration at or below zero (only reachable when the release underflows)
// is unoccupied; its derivative is the one-sided limit, 1/Kd for n = 1 and 0
// for n > 1, and 0 is used for n < 1 as well, where the limit is infinite.
func occupancy(c, kd, n float64) (occ, slope float64) {
	if c <= 0 {
		if n == 1 {
			return 0, 1 / kd
		}
		return 0, 0
	}
	u := n * (math.Log(c) - math.Log(kd))
	occ = sigmoid(u)
	return occ, n * occ * sigmoid(-u) / c
}

// clampRange returns x limited to [lo, hi] and whether x was inside it.
func clampRange(x, lo, hi float64) (float64, bool) {
	if x < lo {
		return lo, false
	}
	if x > hi {
		return hi, false
	}
	return x, true
}

// chemStep advances the concentrations from the outputs y at the start of a
// step and writes the unclamped gain sum G and offset sum B of every node.
// conc holds c(t-1) on entry and c(t) on exit; mean receives m(t) and occ the
// per-channel occupancy.
func (pl *chemPlan) step(p Parameters, y, conc, mean, occ, G, B []float64) {
	for k, sources := range pl.sources {
		var sum float64
		for _, j := range sources {
			sum += y[j]
		}
		m := sum / float64(len(sources))
		mean[k] = m
		conc[k] = pl.lambda[k]*conc[k] + pl.weight[k]*softplus(p.ChemRelease[k]*m)
		occ[k], _ = occupancy(conc[k], pl.kd[k], pl.hill[k])
	}
	for i, recs := range pl.nodeReceptors {
		g, b := 1.0, 0.0
		for _, e := range recs {
			o := occ[pl.receptorChannel[e]]
			g += p.ChemGain[e] * o
			b += p.ChemOffset[e] * o
		}
		G[i], B[i] = g, b
	}
}

// modulate turns the input current I of node i into gamma*I + beta. A neutral
// pair returns I untouched, so a zero block keeps the sign of a zero current
// and stays bit-identical to an undeclared one.
func (pl *chemPlan) modulate(current, G, B float64) float64 {
	gamma, _ := clampRange(G, pl.gammaMin, pl.gammaMax)
	beta, _ := clampRange(B, -pl.betaAbs, pl.betaAbs)
	if gamma == 1 && beta == 0 {
		return current
	}
	return gamma*current + beta
}

// chemBackward returns the gradient of node i's input current at step t from
// dDrive, the gradient of its modulated drive, and adds the receptor gain and
// offset gradients and the per-channel occupancy gradient docc. Without a plan
// the current is the drive and dDrive returns unchanged.
func chemBackward(pl *chemPlan, tr *Trace, g *Gradient, t, i int, dDrive float64, occ, docc []float64) float64 {
	if pl == nil {
		return dDrive
	}
	gamma, gammaFree := clampRange(tr.chemG[t][i], pl.gammaMin, pl.gammaMax)
	_, betaFree := clampRange(tr.chemB[t][i], -pl.betaAbs, pl.betaAbs)
	if recs := pl.nodeReceptors[i]; len(recs) != 0 {
		var dgamma, dbeta float64
		if gammaFree {
			dgamma = dDrive * tr.chemCurrent[t][i]
		}
		if betaFree {
			dbeta = dDrive
		}
		for _, e := range recs {
			k := pl.receptorChannel[e]
			g.ChemGain[e] += dgamma * occ[k]
			g.ChemOffset[e] += dbeta * occ[k]
			docc[k] += dgamma*tr.parameters.ChemGain[e] + dbeta*tr.parameters.ChemOffset[e]
		}
	}
	return gamma * dDrive
}
