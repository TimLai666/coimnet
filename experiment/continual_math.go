package experiment

import (
	"fmt"
	"math"
)

// ContinualEpisode is the counter-based generator of delayed-correlation/v1
// for one task: delay+2 rows of width `width` (every task of a protocol
// shares the width, see ContinualProtocol.TaskWidth), row 0 carries the pulse
// a on the task's channel and every other value is 0; the target is gain*a
// (gain defaults to 0.4). flipped negates the gain, which is what a
// rule_change stage does to a task. a is drawn exactly like DelayedEpisode
// from seed and index. Errors: unknown generator, channel >= width, delay
// outside 1..8.
func ContinualEpisode(spec TaskSpec, width int, flipped bool, seed, index uint64) (Episode, error) {
	if spec.Generator != GeneratorDelayedCorrelation {
		return Episode{}, fmt.Errorf("unknown generator %q", spec.Generator)
	}
	dv, ok := spec.Params["delay"]
	if !ok || dv != math.Trunc(dv) || dv < 1 || dv > 8 {
		return Episode{}, fmt.Errorf("delay must be an integer in 1..8, got %v", dv)
	}
	cv, ok := spec.Params["channel"]
	if !ok || cv != math.Trunc(cv) {
		return Episode{}, fmt.Errorf("channel must be an integer, got %v", cv)
	}
	channel := int(cv)
	if channel < 0 || channel >= width {
		return Episode{}, fmt.Errorf("channel %d out of range for width %d", channel, width)
	}
	gain := 0.4
	if gv, ok := spec.Params["gain"]; ok {
		gain = gv
	}
	v := mix(seed + index*0x9e3779b97f4a7c15)
	a := .4 + .6*float64(v>>11)/float64(uint64(1)<<53)
	if v&1 == 0 {
		a = -a
	}
	input := make([][]float64, int(dv)+2)
	for r := range input {
		row := make([]float64, width)
		if r == 0 {
			row[channel] = a
		}
		input[r] = row
	}
	target := gain * a
	if flipped {
		target = -target
	}
	return Episode{Input: input, Target: []float64{target}}, nil
}

// Forgetting is F[i][j] = max_{k<=i, !failed[k][j]} scores[k][j] − scores[i][j]
// with the "larger is better" convention. A failed cell has F 0 (the caller
// keeps its own failed mask); a failed earlier cell is skipped in the max.
// Ragged inputs or mismatched masks are an error.
func Forgetting(scores [][]float64, failed [][]bool) ([][]float64, error) {
	if len(scores) != len(failed) {
		return nil, fmt.Errorf("scores and failed mask must have the same number of rows")
	}
	width := 0
	if len(scores) > 0 {
		width = len(scores[0])
	}
	for i := range scores {
		if len(scores[i]) != width || len(failed[i]) != width {
			return nil, fmt.Errorf("row %d: scores and failed mask must both have width %d", i, width)
		}
	}
	F := make([][]float64, len(scores))
	for i := range scores {
		row := make([]float64, width)
		for j := 0; j < width; j++ {
			if failed[i][j] {
				continue
			}
			best := scores[i][j]
			for k := 0; k <= i; k++ {
				if !failed[k][j] && scores[k][j] > best {
					best = scores[k][j]
				}
			}
			row[j] = best - scores[i][j]
		}
		F[i] = row
	}
	return F, nil
}

// CellStat summarises one cell across seeds. json snake_case.
type CellStat struct {
	Mean      float64 `json:"mean"`
	Std       float64 `json:"std"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	Succeeded int     `json:"succeeded"`
	Total     int     `json:"total"`
}

// AggregateCells summarises one cell across seeds using only the seeds whose
// cell did not fail: Std is the population standard deviation (divide by
// Succeeded); a cell with no successful seed reports zeros and Succeeded 0.
// Every seed must have the same shape.
func AggregateCells(perSeed [][][]float64, failed [][][]bool) ([][]CellStat, error) {
	if len(perSeed) != len(failed) {
		return nil, fmt.Errorf("perSeed and failed mask must have the same number of seeds")
	}
	rows, cols := 0, 0
	if len(perSeed) > 0 {
		rows = len(perSeed[0])
		if rows > 0 {
			cols = len(perSeed[0][0])
		}
	}
	for s := range perSeed {
		if len(perSeed[s]) != rows || len(failed[s]) != rows {
			return nil, fmt.Errorf("seed %d: perSeed/failed rows = %d/%d, want %d", s, len(perSeed[s]), len(failed[s]), rows)
		}
		for r := range perSeed[s] {
			if len(perSeed[s][r]) != cols || len(failed[s][r]) != cols {
				return nil, fmt.Errorf("seed %d: perSeed/failed row %d has width %d/%d, want %d", s, r, len(perSeed[s][r]), len(failed[s][r]), cols)
			}
		}
	}
	cells := make([][]CellStat, rows)
	for r := range cells {
		cells[r] = make([]CellStat, cols)
	}
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			cs := CellStat{Total: len(perSeed)}
			var vals []float64
			var sum float64
			for s := range perSeed {
				if failed[s][r][c] {
					continue
				}
				v := perSeed[s][r][c]
				vals = append(vals, v)
				sum += v
			}
			cs.Succeeded = len(vals)
			if len(vals) > 0 {
				mean := sum / float64(len(vals))
				var dev float64
				for _, v := range vals {
					d := v - mean
					dev += d * d
				}
				cs.Mean = mean
				cs.Std = math.Sqrt(dev / float64(len(vals)))
				cs.Min, cs.Max = vals[0], vals[0]
				for _, v := range vals[1:] {
					if v < cs.Min {
						cs.Min = v
					}
					if v > cs.Max {
						cs.Max = v
					}
				}
			}
			cells[r][c] = cs
		}
	}
	return cells, nil
}
