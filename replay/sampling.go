package replay

import (
	"cmp"
	"fmt"
	"math"
	"slices"
)

// Sample returns up to n stored experiences without replacement, in the order
// the declared sampling policy produces. A request larger than the store
// returns everything it holds exactly once; a request of zero or less is an
// error rather than an empty answer, because it usually means a miscalculated
// batch size.
//
// Every policy settles the whole order before taking the first n items, so the
// number of values a Sample draws from the generator depends only on what the
// store holds and how the policy groups it, never on n. That is what makes a
// snapshot taken between two samples enough to continue the same sequence.
func (s *Store) Sample(n int) ([]Experience, SampleReport, error) {
	if n <= 0 {
		return nil, SampleReport{}, fmt.Errorf("replay: sample size %d must be positive", n)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before := s.draws
	report := SampleReport{Requested: n, PerTask: map[string]int{}}
	if len(s.items) == 0 {
		return []Experience{}, report, nil
	}
	var (
		order    []int
		bucketOf []int
		buckets  int
	)
	switch s.buffer.Sampling {
	case SamplingUniform:
		order = s.shuffled(indices(len(s.items)))
	case SamplingTaskBalanced:
		order = s.taskBalancedOrder()
	case SamplingTimeBalanced:
		order, bucketOf, buckets = s.timeBalancedOrder()
	default:
		return nil, SampleReport{}, fmt.Errorf("replay: unsupported sampling policy %q", s.buffer.Sampling)
	}
	taken := order[:min(n, len(order))]
	items := make([]Experience, len(taken))
	seen := make(map[int]bool, len(taken))
	if buckets > 0 {
		report.PerBucket = make([]int, buckets)
	}
	for i, index := range taken {
		if seen[index] {
			report.Duplicates++
		}
		seen[index] = true
		items[i] = copyExperience(s.items[index])
		report.PerTask[s.items[index].TaskID]++
		if report.PerBucket != nil {
			report.PerBucket[bucketOf[index]]++
		}
	}
	report.Returned = len(items)
	report.Draws = s.draws - before
	if s.buffer.Privacy.Retention == RetentionDropAfterSample {
		s.items = remaining(s.items, seen)
	}
	return items, report, nil
}

// shuffled settles the whole order with a Fisher-Yates pass: position i swaps
// with a uniformly chosen position in [i, len). The last position needs no
// draw, because there is only one value it could take.
func (s *Store) shuffled(idx []int) []int {
	for i := 0; i+1 < len(idx); i++ {
		j := i + int(s.draw(uint64(len(idx)-i)))
		idx[i], idx[j] = idx[j], idx[i]
	}
	return idx
}

// taskBalancedOrder groups the store by task, shuffles each task's own order
// and then takes one item per task per round. Tasks keep the order in which
// they first appeared in the store, so the round robin is reproducible without
// depending on Go's map iteration.
func (s *Store) taskBalancedOrder() []int {
	var names []string
	groups := map[string][]int{}
	for i, item := range s.items {
		if _, ok := groups[item.TaskID]; !ok {
			names = append(names, item.TaskID)
		}
		groups[item.TaskID] = append(groups[item.TaskID], i)
	}
	ordered := make([][]int, len(names))
	for i, name := range names {
		ordered[i] = s.shuffled(groups[name])
	}
	return roundRobin(ordered)
}

// timeBalancedOrder buckets the store by Step into ceil(sqrt(len)) buckets of
// as equal a size as the item count allows, shuffles each bucket and then takes
// one item per bucket per round. The buckets hold equal counts rather than
// equal step ranges, so a burst of experience in one moment cannot fill a
// bucket that then dominates every sample. Items are ordered by Step, and items
// that share a Step keep the order they were added in.
func (s *Store) timeBalancedOrder() ([]int, []int, int) {
	sorted := indices(len(s.items))
	slices.SortStableFunc(sorted, func(a, b int) int { return cmp.Compare(s.items[a].Step, s.items[b].Step) })
	buckets := int(math.Ceil(math.Sqrt(float64(len(sorted)))))
	ordered := make([][]int, buckets)
	bucketOf := make([]int, len(s.items))
	for b := range buckets {
		low, high := b*len(sorted)/buckets, (b+1)*len(sorted)/buckets
		ordered[b] = append(ordered[b], sorted[low:high]...)
		for _, index := range ordered[b] {
			bucketOf[index] = b
		}
		s.shuffled(ordered[b])
	}
	return roundRobin(ordered), bucketOf, buckets
}

// roundRobin takes one item from each group per round, skipping groups that
// have run out, until every group is exhausted.
func roundRobin(groups [][]int) []int {
	var total int
	for _, group := range groups {
		total += len(group)
	}
	out := make([]int, 0, total)
	for round := 0; len(out) < total; round++ {
		for _, group := range groups {
			if round < len(group) {
				out = append(out, group[round])
			}
		}
	}
	return out
}

// remaining keeps the stored items whose index was not sampled, preserving the
// order the remaining items were added in.
func remaining(items []Experience, sampled map[int]bool) []Experience {
	kept := make([]Experience, 0, len(items))
	for i, item := range items {
		if !sampled[i] {
			kept = append(kept, item)
		}
	}
	return kept
}

func indices(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
