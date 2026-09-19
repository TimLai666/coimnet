package experiment

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
)

// TaskComparison is the per-task result of a preregistered paired bootstrap:
// Pairs is the count of seeds where both the last-stage matrix cell and the
// independent score succeeded, MeanDifference is the mean of
// Matrix[last][j] - Independent[j] over those pairs, and Lower/Upper are the
// bootstrap percentile bounds of that mean at the declared Interval. With
// Pairs < 2 no interval is informative: Insufficient is set and the bounds are
// 0, so no claim is attached to the number.
type TaskComparison struct {
	Task           string  `json:"task"`
	Pairs          int     `json:"pairs"`
	MeanDifference float64 `json:"mean_difference"`
	Lower          float64 `json:"lower"`
	Upper          float64 `json:"upper"`
	Insufficient   bool    `json:"insufficient"`
}

// ComparisonResult carries the preregistered method and its realized interval
// for every task, in protocol task order, so the report can be read against
// the protocol without re-deriving the plan.
type ComparisonResult struct {
	Method    string           `json:"method"`
	Baseline  string           `json:"baseline"`
	Interval  float64          `json:"interval"`
	Resamples int              `json:"resamples"`
	Seed      uint64           `json:"seed"`
	Tasks     []TaskComparison `json:"tasks"`
}

// pairedBootstrap draws Resamples resamples of the paired differences with
// replacement from rand.New(rand.NewPCG(seed, 0)) (math/rand/v2), takes the
// mean of each, sorts the means and reads the nearest-rank quantiles at
// (1-Interval)/2 and 1-(1-Interval)/2. seed is p.Seeds[0].
func pairedBootstrap(differences []float64, interval float64, resamples int, seed uint64) (mean, lower, upper float64) {
	n := len(differences)
	rng := rand.New(rand.NewPCG(seed, 0))
	means := make([]float64, resamples)
	for r := range means {
		var sum float64
		for i := 0; i < n; i++ {
			sum += differences[rng.IntN(n)]
		}
		means[r] = sum / float64(n)
	}
	sort.Float64s(means)
	for _, d := range differences {
		mean += d
	}
	mean /= float64(n)
	lo := (1 - interval) / 2
	hi := 1 - lo
	return mean, nearestRankQuantile(means, lo), nearestRankQuantile(means, hi)
}

// nearestRankQuantile reads the nearest-rank quantile of an already sorted
// slice: the ceil(q*n)-th order (1-based), clamped to the slice.
func nearestRankQuantile(sorted []float64, q float64) float64 {
	n := len(sorted)
	r := int(math.Ceil(q * float64(n)))
	if r < 1 {
		r = 1
	}
	if r > n {
		r = n
	}
	return sorted[r-1]
}

// compareToIndependent is LRN-08: with a declared Comparison the report carries
// one paired comparison per task, pairing the last-stage matrix cell of each
// seed with the same seed's independent re-initialized control. Within a task,
// only seeds whose matrix cell and independent score both succeeded form the
// pair set; fewer than 2 pairs is an Insufficient TaskComparison with zero
// bounds and no claim. The bootstrap seed is the protocol's first seed.
func compareToIndependent(report ContinualReport, c PreRegistered) (*ComparisonResult, error) {
	if len(report.Protocol.Tasks) == 0 {
		return nil, fmt.Errorf("comparison: protocol declares no tasks")
	}
	if len(report.Protocol.Stages) == 0 {
		return nil, fmt.Errorf("comparison: protocol declares no stages")
	}
	if len(report.Protocol.Seeds) == 0 {
		return nil, fmt.Errorf("comparison: protocol declares no seeds")
	}
	last := len(report.Protocol.Stages) - 1
	independent := make(map[string]IndependentResult, len(report.Independent))
	for _, ir := range report.Independent {
		independent[ir.Task] = ir
	}
	res := &ComparisonResult{
		Method:    c.Method,
		Baseline:  c.Baseline,
		Interval:  c.Interval,
		Resamples: c.Resamples,
		Seed:      report.Protocol.Seeds[0],
	}
	for j, ts := range report.Protocol.Tasks {
		ir, ok := independent[ts.Name]
		if !ok {
			return nil, fmt.Errorf("comparison: task %q has no independent control", ts.Name)
		}
		score := make(map[uint64]SeedScore, len(ir.Seeds))
		for _, sc := range ir.Seeds {
			score[sc.Seed] = sc
		}
		tc := TaskComparison{Task: ts.Name}
		var diffs []float64
		for _, sm := range report.Seeds {
			if last >= len(sm.Scores) || last >= len(sm.Failed) || j >= len(sm.Scores[last]) || j >= len(sm.Failed[last]) {
				return nil, fmt.Errorf("comparison: seed %d matrix has no cell (%d,%d)", sm.Seed, last, j)
			}
			sc, ok := score[sm.Seed]
			if !ok {
				return nil, fmt.Errorf("comparison: task %q has no independent score for seed %d", ts.Name, sm.Seed)
			}
			if sm.Failed[last][j] || sc.Failed {
				continue
			}
			diffs = append(diffs, sm.Scores[last][j]-sc.Score)
			tc.Pairs++
		}
		for _, d := range diffs {
			tc.MeanDifference += d
		}
		if tc.Pairs > 0 {
			tc.MeanDifference /= float64(tc.Pairs)
		}
		if tc.Pairs < 2 {
			tc.Insufficient = true
			tc.Lower, tc.Upper = 0, 0
		} else {
			_, lo, hi := pairedBootstrap(diffs, c.Interval, c.Resamples, res.Seed)
			tc.Lower, tc.Upper = lo, hi
		}
		res.Tasks = append(res.Tasks, tc)
	}
	return res, nil
}
