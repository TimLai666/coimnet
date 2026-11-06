package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/learning"
)

// decodeSnapshot reads either a --save TrainingSnapshot or a --checkpoint
// file, whose trainer field holds one.
func decodeSnapshot(b []byte) (learning.TrainingSnapshot, error) {
	var probe struct {
		Trainer json.RawMessage `json:"trainer"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return learning.TrainingSnapshot{}, err
	}
	if len(probe.Trainer) > 0 {
		b = probe.Trainer
	}
	var snapshot learning.TrainingSnapshot
	if err := json.Unmarshal(b, &snapshot); err != nil {
		return learning.TrainingSnapshot{}, err
	}
	if snapshot.Config.InputSize == 0 {
		return learning.TrainingSnapshot{}, errors.New("not a TrainingSnapshot or checkpoint")
	}
	return snapshot, nil
}

type fastComparison struct {
	accuracy float64
	agree    int
	maxDiff  float64
}

// compareFast scores xs with fp and counts the images whose argmax equals
// Trainer.Predict's, the float64-core reference.
func compareFast(ctx context.Context, tr *learning.Trainer, fp *learning.FastPredictor, xs [][][]float64, ys []int) (fastComparison, error) {
	var c fastComparison
	correct := 0
	for i, x := range xs {
		want, err := tr.Predict(ctx, x)
		if err != nil {
			return c, err
		}
		got, err := fp.Predict(ctx, x)
		if err != nil {
			return c, err
		}
		for j := range want {
			c.maxDiff = math.Max(c.maxDiff, math.Abs(want[j]-got[j]))
		}
		if argmax(got) == argmax(want) {
			c.agree++
		}
		if argmax(got) == ys[i] {
			correct++
		}
	}
	if len(xs) > 0 {
		c.accuracy = float64(correct) / float64(len(xs))
	}
	return c, nil
}

// block8Wiring gives every target degree sources made of distinct aligned
// windows of 8 consecutive nodes, the last window cut to degree mod 8. nodes
// must be a multiple of 8 and degree at most nodes.
func block8Wiring(rng *rand.Rand, nodes, degree int) (sources, targets []int) {
	windows := (degree + 7) / 8
	for t := range nodes {
		left := degree
		for _, w := range rng.Perm(nodes / 8)[:windows] {
			for j := range min(8, left) {
				sources, targets = append(sources, w*8+j), append(targets, t)
			}
			left -= 8
		}
	}
	return sources, targets
}
