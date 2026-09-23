package asr

import (
	"fmt"
	"math"
	"math/rand/v2"
)

const (
	// The exact search is bounded by distinct group sizes and reachable count,
	// not by the number of utterances times the number of groups.
	splitMaxDPWork         int64 = 10_000_000
	splitMaxSubsetCapacity       = 2_000_000
)

// SplitBySpeakerSession partitions utterances into train and test sets without
// putting a speaker or recording session in both sets. Utterances connected by
// either identity are kept together, including connections through multiple
// intermediate utterances. The test groups are selected in a deterministic
// pseudo-random order and the returned slices retain input order.
func SplitBySpeakerSession(utterances []Utterance, testFraction float64, seed uint64) (train, test []Utterance, err error) {
	if !(testFraction > 0 && testFraction < 1) {
		return nil, nil, fmt.Errorf("asr: test fraction %v must be strictly between 0 and 1", testFraction)
	}
	if len(utterances) < 2 {
		return nil, nil, fmt.Errorf("asr: split needs at least 2 utterances, got %d", len(utterances))
	}
	for i, utterance := range utterances {
		if utterance.Speaker == "" {
			return nil, nil, fmt.Errorf("asr: utterance %d has an empty speaker", i)
		}
		if utterance.Session == "" {
			return nil, nil, fmt.Errorf("asr: utterance %d has an empty session", i)
		}
	}

	groups := connectedSpeakerSessionGroups(utterances)
	if len(groups) < 2 {
		return nil, nil, fmt.Errorf("asr: cannot split utterances: all records form one connected speaker/session group")
	}

	order := make([]int, len(groups))
	for i := range order {
		order[i] = i
	}
	rng := rand.New(rand.NewPCG(seed, 0))
	rng.Shuffle(len(order), func(i, j int) {
		order[i], order[j] = order[j], order[i]
	})

	target := testFraction * float64(len(utterances))
	selectedGroups, err := selectTestGroups(groups, order, target, len(utterances))
	if err != nil {
		return nil, nil, err
	}
	testCount := 0
	for _, groupIndex := range selectedGroups {
		testCount += len(groups[groupIndex])
	}

	inTest := make([]bool, len(utterances))
	for _, groupIndex := range selectedGroups {
		for _, utteranceIndex := range groups[groupIndex] {
			inTest[utteranceIndex] = true
		}
	}

	train = make([]Utterance, 0, len(utterances)-testCount)
	test = make([]Utterance, 0, testCount)
	for i, utterance := range utterances {
		if inTest[i] {
			test = append(test, utterance)
		} else {
			train = append(train, utterance)
		}
	}
	return train, test, nil
}

// selectTestGroups uses an exact bounded subset-sum search. Groups with the
// same size are compressed into a bounded count, so the search costs
// O(U*C) time and O(C) memory for U distinct group sizes and count capacity C.
// The nearest reachable proper subset is returned. If the configured work or
// capacity bound would be exceeded, the function returns an error instead of
// silently returning an unbounded heuristic result. Ties choose the smaller
// test count, then the seeded order of groups within each size.
func selectTestGroups(groups [][]int, order []int, target float64, total int) ([]int, error) {
	if allGroupSizesEqual(groups) {
		count := closestUniformGroupCount(len(groups), len(groups[0]), target)
		return append([]int(nil), order[:count]...), nil
	}

	buckets := make([]splitSizeBucket, 0, len(groups))
	bucketBySize := make(map[int]int, len(groups))
	for _, groupIndex := range order {
		size := len(groups[groupIndex])
		bucketIndex, ok := bucketBySize[size]
		if !ok {
			bucketIndex = len(buckets)
			bucketBySize[size] = bucketIndex
			buckets = append(buckets, splitSizeBucket{size: size})
		}
		buckets[bucketIndex].groups = append(buckets[bucketIndex].groups, groupIndex)
	}

	searchTarget := target
	complement := false
	if searchTarget > float64(total)/2 {
		searchTarget = float64(total) - searchTarget
		complement = true
	}
	// Start with a valid greedy incumbent. No sum farther above the target than
	// this incumbent can be optimal, so the bounded DP still has an exact result
	// guarantee without sizing its table from the largest group.
	greedyCount := 0
	smallestGroup := total
	for _, groupIndex := range order {
		size := len(groups[groupIndex])
		if size < smallestGroup {
			smallestGroup = size
		}
		if float64(greedyCount+size) <= searchTarget {
			greedyCount += size
		}
	}
	incumbentDistance := math.Abs(float64(greedyCount) - searchTarget)
	if greedyCount == 0 {
		incumbentDistance = math.Abs(float64(smallestGroup) - searchTarget)
	}
	capacityFloat := math.Ceil(searchTarget + incumbentDistance)
	if capacityFloat > float64(total-1) {
		capacityFloat = float64(total - 1)
	}
	if capacityFloat < 1 || capacityFloat > float64(splitMaxSubsetCapacity) {
		return nil, fmt.Errorf("asr: exact split search capacity %.0f exceeds bounded limit %d", capacityFloat, splitMaxSubsetCapacity)
	}
	capacity := int(capacityFloat)
	work := int64(len(buckets)) * int64(capacity)
	if work > splitMaxDPWork {
		return nil, fmt.Errorf("asr: exact split search requires %d size-count cells, exceeding bounded limit %d", work, splitMaxDPWork)
	}

	reachable := make([]bool, capacity+1)
	parentSum := make([]int, capacity+1)
	parentBucket := make([]int, capacity+1)
	parentTake := make([]int, capacity+1)
	reachable[0] = true
	for bucketIndex, bucket := range buckets {
		base := append([]bool(nil), reachable...)
		queue := make([]int, 0, capacity/bucket.size+1)
		for residue := 0; residue < bucket.size && residue <= capacity; residue++ {
			queue = queue[:0]
			head := 0
			for sum := residue; sum <= capacity; sum += bucket.size {
				if base[sum] {
					queue = append(queue, sum)
				}
				minimumPrevious := sum - len(bucket.groups)*bucket.size
				for head < len(queue) && queue[head] < minimumPrevious {
					head++
				}
				if reachable[sum] || head == len(queue) {
					continue
				}
				previous := queue[head]
				reachable[sum] = true
				parentSum[sum] = previous
				parentBucket[sum] = bucketIndex
				parentTake[sum] = (sum - previous) / bucket.size
			}
		}
	}

	bestSum := -1
	bestTestCount := -1
	bestDistance := math.Inf(1)
	for sum := 1; sum < total && sum <= capacity; sum++ {
		if !reachable[sum] {
			continue
		}
		distance := math.Abs(float64(sum) - searchTarget)
		testCount := sum
		if complement {
			testCount = total - sum
		}
		if distance < bestDistance || (distance == bestDistance && (bestTestCount < 0 || testCount < bestTestCount)) {
			bestSum = sum
			bestTestCount = testCount
			bestDistance = distance
		}
	}
	if bestSum < 0 {
		return nil, fmt.Errorf("asr: exact split search found no nonempty proper group subset")
	}

	counts := make([]int, len(buckets))
	for sum := bestSum; sum > 0; {
		bucketIndex := parentBucket[sum]
		take := parentTake[sum]
		if take <= 0 || bucketIndex < 0 || bucketIndex >= len(buckets) || parentSum[sum] >= sum {
			return nil, fmt.Errorf("asr: exact split search produced invalid reconstruction")
		}
		counts[bucketIndex] += take
		sum = parentSum[sum]
	}

	selected := make([]bool, len(groups))
	for bucketIndex, count := range counts {
		for _, groupIndex := range buckets[bucketIndex].groups[:count] {
			selected[groupIndex] = true
		}
	}
	selectedGroups := make([]int, 0)
	for _, groupIndex := range order {
		if selected[groupIndex] == complement {
			continue
		}
		selectedGroups = append(selectedGroups, groupIndex)
	}
	return selectedGroups, nil
}

type splitSizeBucket struct {
	size   int
	groups []int
}

func allGroupSizesEqual(groups [][]int) bool {
	if len(groups) < 2 {
		return true
	}
	size := len(groups[0])
	for _, group := range groups[1:] {
		if len(group) != size {
			return false
		}
	}
	return true
}

func closestUniformGroupCount(groupCount, groupSize int, target float64) int {
	if groupCount < 2 || groupSize < 1 {
		return 0
	}
	ideal := target / float64(groupSize)
	lower := int(math.Floor(ideal))
	if lower < 1 {
		lower = 1
	}
	if lower >= groupCount {
		lower = groupCount - 1
	}
	upper := lower + 1
	if upper >= groupCount {
		upper = groupCount - 1
	}
	if math.Abs(float64(upper*groupSize)-target) < math.Abs(float64(lower*groupSize)-target) {
		return upper
	}
	return lower
}

type speakerSessionUnionFind struct {
	parent []int
	rank   []uint8
}

func newSpeakerSessionUnionFind(size int) *speakerSessionUnionFind {
	parent := make([]int, size)
	for i := range parent {
		parent[i] = i
	}
	return &speakerSessionUnionFind{parent: parent, rank: make([]uint8, size)}
}

func (u *speakerSessionUnionFind) find(index int) int {
	if u.parent[index] != index {
		u.parent[index] = u.find(u.parent[index])
	}
	return u.parent[index]
}

func (u *speakerSessionUnionFind) union(first, second int) {
	firstRoot, secondRoot := u.find(first), u.find(second)
	if firstRoot == secondRoot {
		return
	}
	if u.rank[firstRoot] < u.rank[secondRoot] {
		firstRoot, secondRoot = secondRoot, firstRoot
	}
	u.parent[secondRoot] = firstRoot
	if u.rank[firstRoot] == u.rank[secondRoot] {
		u.rank[firstRoot]++
	}
}

func connectedSpeakerSessionGroups(utterances []Utterance) [][]int {
	unionFind := newSpeakerSessionUnionFind(len(utterances))
	speakerOwner := make(map[string]int, len(utterances))
	sessionOwner := make(map[string]int, len(utterances))
	for i, utterance := range utterances {
		if owner, ok := speakerOwner[utterance.Speaker]; ok {
			unionFind.union(i, owner)
		} else {
			speakerOwner[utterance.Speaker] = i
		}
		if owner, ok := sessionOwner[utterance.Session]; ok {
			unionFind.union(i, owner)
		} else {
			sessionOwner[utterance.Session] = i
		}
	}

	groupByRoot := make(map[int]int, len(utterances))
	groups := make([][]int, 0, len(utterances))
	for i := range utterances {
		root := unionFind.find(i)
		groupIndex, ok := groupByRoot[root]
		if !ok {
			groupIndex = len(groups)
			groupByRoot[root] = groupIndex
			groups = append(groups, nil)
		}
		groups[groupIndex] = append(groups[groupIndex], i)
	}
	return groups
}
