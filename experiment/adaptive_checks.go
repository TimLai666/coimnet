package experiment

import (
	"context"
	"fmt"
	"math/rand/v2"
	"reflect"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/replay"
)

// ReplayRejectionCount tries to add every scoring item to a fresh replay
// store as a test-split experience and counts how many the store refused.
// A correct store refuses all of them; the count is reported, never assumed.
func ReplayRejectionCount(e AdaptiveEvaluation) (int, error) {
	store, err := replay.New(replay.Buffer{
		Capacity: len(e.Scoring) + 1,
		Sampling: replay.SamplingUniform,
		Eviction: replay.EvictionFIFO,
		Seed:     e.Seed,
		Privacy:  replay.PrivacyPolicy{StoreRaw: false, Retention: replay.RetentionKeepUntilEvicted},
	})
	if err != nil {
		return 0, err
	}
	rejected := 0
	for i, item := range e.Scoring {
		exp := replay.Experience{
			TaskID: "scoring",
			Step:   uint64(i),
			Input:  item.Input,
			Target: [][]float64{item.Target},
			Weight: 1,
			Split:  replay.SplitTest,
		}
		if err := store.Add(exp); err != nil {
			rejected++
		}
	}
	return rejected, nil
}

// ShuffleInvariance runs the scoring partition twice on two individuals
// restored from the same snapshot, once in declared order and once in an
// order permuted by a PCG seeded with e.Seed, and reports whether every
// item's outputs are bit-identical between the two runs. It is only
// meaningful when e.Reset resets every state; with any reset off it is
// expected to be false on a stateful individual and the caller reports that
// as a discriminating check.
func ShuffleInvariance(ctx context.Context, snapshot learning.IndividualSnapshot, e AdaptiveEvaluation) (bool, error) {
	indA, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		return false, err
	}
	indB, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		return false, err
	}

	scoresA, err := scoreItems(ctx, indA, e.Scoring, e.Reset)
	if err != nil {
		return false, err
	}

	rng := rand.New(rand.NewPCG(e.Seed, 0))
	perm := rng.Perm(len(e.Scoring))
	permuted := make([]EvaluationItem, len(e.Scoring))
	for i, p := range perm {
		permuted[i] = e.Scoring[p]
	}
	scoresB, err := scoreItems(ctx, indB, permuted, e.Reset)
	if err != nil {
		return false, err
	}

	byID := make(map[string]ItemScore, len(scoresB))
	for _, s := range scoresB {
		byID[s.ID] = s
	}
	for _, a := range scoresA {
		b, ok := byID[a.ID]
		if !ok {
			return false, fmt.Errorf("shuffle invariance: permuted run is missing item %q", a.ID)
		}
		if !reflect.DeepEqual(a.Output, b.Output) {
			return false, nil
		}
	}
	return true, nil
}
