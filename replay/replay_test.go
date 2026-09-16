package replay_test

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/replay"
)

// pcgSeed7 is the first values math/rand/v2 produces from the convention the
// whole project uses, rand.New(rand.NewPCG(seed, 0)), at seed 7. Every expected
// sequence below is derived from these numbers by hand, so a change in the
// store's own draw rule shows up as a failing expectation rather than as a
// quietly different sample.
var pcgSeed7 = []uint64{
	4971928291090985182,
	5528821527542695328,
	2134937714366092113,
	3091447866230484581,
	15963695826586302654,
	14648062316321977721,
}

// TestPinnedPCGStream checks the pinned numbers against the generator itself,
// so the hand computations below rest on the declared convention and not on
// anything this package does.
func TestPinnedPCGStream(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 0))
	for i, want := range pcgSeed7 {
		if got := r.Uint64(); got != want {
			t.Fatalf("draw %d = %d, want %d", i, got, want)
		}
	}
}

func buffer(sampling, eviction string) replay.Buffer {
	return replay.Buffer{
		Capacity: 4, Sampling: sampling, Eviction: eviction, Seed: 7,
		Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
	}
}

func experience(task string, step uint64, value float64) replay.Experience {
	return replay.Experience{
		TaskID: task, Step: step,
		Input:  [][]float64{{value}, {0}},
		Target: [][]float64{{value * 2}},
		Weight: 1, Split: replay.SplitTrain,
	}
}

func newStore(t *testing.T, b replay.Buffer) *replay.Store {
	t.Helper()
	store, err := replay.New(b)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func add(t *testing.T, store *replay.Store, items ...replay.Experience) {
	t.Helper()
	for _, item := range items {
		if err := store.Add(item); err != nil {
			t.Fatal(err)
		}
	}
}

func steps(items []replay.Experience) []uint64 {
	out := make([]uint64, len(items))
	for i, item := range items {
		out[i] = item.Step
	}
	return out
}

func sample(t *testing.T, store *replay.Store, n int) ([]replay.Experience, replay.SampleReport) {
	t.Helper()
	items, report, err := store.Sample(n)
	if err != nil {
		t.Fatal(err)
	}
	if report.Duplicates != 0 {
		t.Fatalf("sampling without replacement reported %d duplicates", report.Duplicates)
	}
	if report.Requested != n || report.Returned != len(items) {
		t.Fatalf("report %+v does not describe %d returned items for a request of %d", report, len(items), n)
	}
	return items, report
}

// TestFIFOEvictionOrder is the hand-computed capacity rule: the oldest item
// leaves first and nothing is drawn from the generator to decide it.
func TestFIFOEvictionOrder(t *testing.T) {
	store := newStore(t, buffer(replay.SamplingUniform, replay.EvictionFIFO))
	for step := uint64(1); step <= 6; step++ {
		add(t, store, experience("a", step, float64(step)))
	}
	state := store.State()
	if got, want := steps(state.Items), []uint64{3, 4, 5, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fifo kept %v, want %v", got, want)
	}
	if state.Seen != 6 {
		t.Fatalf("seen = %d, want 6", state.Seen)
	}
	if state.Draws != 0 {
		t.Fatalf("fifo eviction drew %d values; it decides nothing at random", state.Draws)
	}
}

// TestReservoirAcceptRejectSequence is the hand-computed reservoir run. With
// capacity 4 and seed 7 the store draws one value per item past the capacity
// and keeps the item when that value, taken modulo the number seen, lands
// inside the capacity:
//
//	e5  seen 5  4971928291090985182 % 5 = 2  -> slot 2
//	e6  seen 6  5528821527542695328 % 6 = 2  -> slot 2
//	e7  seen 7  2134937714366092113 % 7 = 2  -> slot 2
//	e8  seen 8  3091447866230484581 % 8 = 5  -> dropped
//	e9  seen 9  15963695826586302654 % 9 = 0 -> slot 0
//	e10 seen 10 14648062316321977721 % 10 = 1 -> slot 1
func TestReservoirAcceptRejectSequence(t *testing.T) {
	store := newStore(t, buffer(replay.SamplingUniform, replay.EvictionReservoir))
	for step := uint64(1); step <= 10; step++ {
		add(t, store, experience("a", step, float64(step)))
	}
	state := store.State()
	if got, want := steps(state.Items), []uint64{9, 10, 7, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reservoir kept %v, want %v", got, want)
	}
	if state.Seen != 10 {
		t.Fatalf("seen = %d, want 10", state.Seen)
	}
	if state.Draws != 6 {
		t.Fatalf("draws = %d, want one per item past the capacity (6)", state.Draws)
	}
}

// TestUniformSampleOrder is the hand-computed shuffle. Sample settles the whole
// order first, so the draw count depends only on what the store holds:
// position i swaps with i + (draw % (4-i)).
//
//	i=0  4971928291090985182 % 4 = 2 -> [2 1 0 3]
//	i=1  5528821527542695328 % 3 = 2 -> [2 3 0 1]
//	i=2  2134937714366092113 % 2 = 1 -> [2 3 1 0]
func TestUniformSampleOrder(t *testing.T) {
	store := newStore(t, buffer(replay.SamplingUniform, replay.EvictionFIFO))
	for step := uint64(1); step <= 4; step++ {
		add(t, store, experience("a", step, float64(step)))
	}
	items, report := sample(t, store, 2)
	if got, want := steps(items), []uint64{3, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("uniform sample = %v, want %v", got, want)
	}
	if report.Draws != 3 {
		t.Fatalf("sample drew %d values, want 3", report.Draws)
	}
	if !reflect.DeepEqual(report.PerTask, map[string]int{"a": 2}) {
		t.Fatalf("per task = %v", report.PerTask)
	}
	if report.PerBucket != nil {
		t.Fatalf("a uniform sample reported time buckets: %v", report.PerBucket)
	}
}

// TestTaskBalancedSampleCounts is the hand-computed round robin. The tasks keep
// their first-appearance order, each task's own order is shuffled, and the
// round robin then takes one item per task per round:
//
//	task a holds steps 1, 2, 4; 4971928291090985182 % 3 = 1 and
//	5528821527542695328 % 2 = 0 leave it in the order 2, 1, 4.
//	task b holds step 3 alone and needs no draw.
func TestTaskBalancedSampleCounts(t *testing.T) {
	store := newStore(t, buffer(replay.SamplingTaskBalanced, replay.EvictionFIFO))
	add(t, store,
		experience("a", 1, 1), experience("a", 2, 2),
		experience("b", 3, 3), experience("a", 4, 4))
	items, report := sample(t, store, 4)
	if got, want := steps(items), []uint64{2, 3, 1, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task balanced sample = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(report.PerTask, map[string]int{"a": 3, "b": 1}) {
		t.Fatalf("per task = %v", report.PerTask)
	}
	if report.Draws != 2 {
		t.Fatalf("sample drew %d values, want 2", report.Draws)
	}
	// A smaller request keeps the proportion the round robin can give: one from
	// each task before either task gets a second item.
	fresh := newStore(t, buffer(replay.SamplingTaskBalanced, replay.EvictionFIFO))
	add(t, fresh,
		experience("a", 1, 1), experience("a", 2, 2),
		experience("b", 3, 3), experience("a", 4, 4))
	short, shortReport := sample(t, fresh, 2)
	if got, want := steps(short), []uint64{2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("short task balanced sample = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(shortReport.PerTask, map[string]int{"a": 1, "b": 1}) {
		t.Fatalf("short per task = %v", shortReport.PerTask)
	}
}

// TestTimeBalancedSampleBuckets is the hand-computed time bucketing. Four items
// give ceil(sqrt(4)) = 2 buckets of two, split over the items ordered by step,
// and each bucket is shuffled before the round robin:
//
//	bucket 0 holds steps 10, 20; 4971928291090985182 % 2 = 0 leaves it alone.
//	bucket 1 holds steps 30, 40; 5528821527542695328 % 2 = 0 leaves it alone.
func TestTimeBalancedSampleBuckets(t *testing.T) {
	store := newStore(t, buffer(replay.SamplingTimeBalanced, replay.EvictionFIFO))
	add(t, store,
		experience("a", 10, 1), experience("a", 20, 2),
		experience("b", 30, 3), experience("b", 40, 4))
	items, report := sample(t, store, 4)
	if got, want := steps(items), []uint64{10, 30, 20, 40}; !reflect.DeepEqual(got, want) {
		t.Fatalf("time balanced sample = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(report.PerBucket, []int{2, 2}) {
		t.Fatalf("per bucket = %v, want [2 2]", report.PerBucket)
	}
	if report.Draws != 2 {
		t.Fatalf("sample drew %d values, want 2", report.Draws)
	}
	fresh := newStore(t, buffer(replay.SamplingTimeBalanced, replay.EvictionFIFO))
	add(t, fresh,
		experience("a", 10, 1), experience("a", 20, 2),
		experience("b", 30, 3), experience("b", 40, 4))
	short, shortReport := sample(t, fresh, 2)
	if got, want := steps(short), []uint64{10, 30}; !reflect.DeepEqual(got, want) {
		t.Fatalf("short time balanced sample = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(shortReport.PerBucket, []int{1, 1}) {
		t.Fatalf("short per bucket = %v, want [1 1]", shortReport.PerBucket)
	}
}

// TestSampleWithoutReplacement covers the property every sampling policy has to
// keep: no item is counted twice inside one update, and a request larger than
// the store returns everything exactly once.
func TestSampleWithoutReplacement(t *testing.T) {
	for _, sampling := range []string{replay.SamplingUniform, replay.SamplingTaskBalanced, replay.SamplingTimeBalanced} {
		t.Run(sampling, func(t *testing.T) {
			store := newStore(t, buffer(sampling, replay.EvictionFIFO))
			add(t, store,
				experience("a", 1, 1), experience("b", 2, 2),
				experience("a", 3, 3), experience("b", 4, 4))
			items, report := sample(t, store, 9)
			if report.Returned != 4 || len(items) != 4 {
				t.Fatalf("a request of 9 returned %d items", len(items))
			}
			seen := map[uint64]int{}
			for _, item := range items {
				seen[item.Step]++
			}
			if len(seen) != 4 {
				t.Fatalf("sample repeated an item: %v", steps(items))
			}
			if report.Duplicates != 0 {
				t.Fatalf("duplicates = %d", report.Duplicates)
			}
		})
	}
}

// TestSampleRejectsNonPositiveSizeAndEmptyStoreIsEmpty separates "asked for
// nothing" from "holds nothing".
func TestSampleRejectsNonPositiveSizeAndEmptyStoreIsEmpty(t *testing.T) {
	store := newStore(t, buffer(replay.SamplingUniform, replay.EvictionFIFO))
	if _, _, err := store.Sample(0); err == nil {
		t.Fatal("a sample of zero items was accepted")
	}
	items, report, err := store.Sample(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 || report.Returned != 0 || report.Draws != 0 {
		t.Fatalf("an empty store sampled %d items with %d draws", len(items), report.Draws)
	}
}

// TestAddRejectsTestSplitAndMalformedExperiences is the contamination barrier:
// held-out data never reaches the replay store, and a split the store does not
// know is refused rather than guessed at.
func TestAddRejectsTestSplitAndMalformedExperiences(t *testing.T) {
	for _, eviction := range []string{replay.EvictionFIFO, replay.EvictionReservoir} {
		for name, damage := range map[string]func(*replay.Experience){
			"test split":      func(e *replay.Experience) { e.Split = replay.SplitTest },
			"unknown split":   func(e *replay.Experience) { e.Split = "tes" },
			"empty split":     func(e *replay.Experience) { e.Split = "" },
			"empty task":      func(e *replay.Experience) { e.TaskID = "" },
			"negative weight": func(e *replay.Experience) { e.Weight = -1 },
			"nan weight":      func(e *replay.Experience) { e.Weight = math.NaN() },
			"no input":        func(e *replay.Experience) { e.Input = nil },
			"no target":       func(e *replay.Experience) { e.Target = nil },
			"empty input row": func(e *replay.Experience) { e.Input = [][]float64{{}} },
			"ragged input":    func(e *replay.Experience) { e.Input = [][]float64{{1}, {1, 2}} },
			"non-finite":      func(e *replay.Experience) { e.Target = [][]float64{{math.Inf(1)}} },
			"caller hash":     func(e *replay.Experience) { e.InputHash = "00" },
			"caller raw flag": func(e *replay.Experience) { e.Raw = true },
		} {
			t.Run(eviction+"/"+name, func(t *testing.T) {
				store := newStore(t, buffer(replay.SamplingUniform, eviction))
				item := experience("a", 1, 1)
				damage(&item)
				if err := store.Add(item); err == nil {
					t.Fatal("the store accepted an experience it cannot describe")
				}
				if state := store.State(); len(state.Items) != 0 || state.Seen != 0 || state.Draws != 0 {
					t.Fatalf("a refused experience changed the store: %+v", state)
				}
			})
		}
	}
}

// TestPrivacyStoreRawFalseKeepsOnlyHashes is the declared privacy policy: the
// arrays never enter the store and a sample can only carry the fingerprints.
func TestPrivacyStoreRawFalseKeepsOnlyHashes(t *testing.T) {
	b := buffer(replay.SamplingUniform, replay.EvictionFIFO)
	b.Privacy.StoreRaw = false
	store := newStore(t, b)
	add(t, store, experience("a", 1, 1), experience("a", 2, 2))
	items, _ := sample(t, store, 2)
	for _, item := range items {
		if item.Raw || item.Input != nil || item.Target != nil {
			t.Fatalf("a hashed store returned arrays: %+v", item)
		}
		if len(item.InputHash) != sha256.Size*2 || len(item.TargetHash) != sha256.Size*2 {
			t.Fatalf("a hashed store returned %q/%q", item.InputHash, item.TargetHash)
		}
	}
	// The fingerprint is the declared encoding: little-endian IEEE-754 bits of
	// every value in row-major order.
	want := rowMajorHash(t, [][]float64{{1}, {0}})
	var found bool
	for _, item := range items {
		if item.InputHash == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("no returned item carries the declared fingerprint %s: %+v", want, items)
	}
	// A store that keeps the arrays says so on every item it returns.
	rawStore := newStore(t, buffer(replay.SamplingUniform, replay.EvictionFIFO))
	add(t, rawStore, experience("a", 1, 1))
	raw, _ := sample(t, rawStore, 1)
	if !raw[0].Raw || raw[0].Input == nil || raw[0].InputHash != want {
		t.Fatalf("a raw store returned %+v", raw[0])
	}
}

func rowMajorHash(t *testing.T, values [][]float64) string {
	t.Helper()
	digest := sha256.New()
	var buf [8]byte
	for _, row := range values {
		for _, v := range row {
			binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
			if _, err := digest.Write(buf[:]); err != nil {
				t.Fatal(err)
			}
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// TestNewRejectsUndeclaredPolicies keeps every knob a declared value rather
// than a default the report would not name.
func TestNewRejectsUndeclaredPolicies(t *testing.T) {
	for name, damage := range map[string]func(*replay.Buffer){
		"zero capacity":     func(b *replay.Buffer) { b.Capacity = 0 },
		"negative capacity": func(b *replay.Buffer) { b.Capacity = -1 },
		"unknown sampling":  func(b *replay.Buffer) { b.Sampling = "priority" },
		"empty sampling":    func(b *replay.Buffer) { b.Sampling = "" },
		"unknown eviction":  func(b *replay.Buffer) { b.Eviction = "lru" },
		"empty eviction":    func(b *replay.Buffer) { b.Eviction = "" },
		"unknown retention": func(b *replay.Buffer) { b.Privacy.Retention = "forever" },
		"empty retention":   func(b *replay.Buffer) { b.Privacy.Retention = "" },
	} {
		t.Run(name, func(t *testing.T) {
			b := buffer(replay.SamplingUniform, replay.EvictionFIFO)
			damage(&b)
			if _, err := replay.New(b); err == nil {
				t.Fatal("an undeclared policy was accepted")
			}
		})
	}
}

// TestRetentionDropAfterSample is the removal rule the privacy policy names: a
// sampled experience leaves the store instead of waiting for the capacity to
// evict it.
func TestRetentionDropAfterSample(t *testing.T) {
	b := buffer(replay.SamplingUniform, replay.EvictionFIFO)
	b.Privacy.Retention = replay.RetentionDropAfterSample
	store := newStore(t, b)
	add(t, store,
		experience("a", 1, 1), experience("a", 2, 2),
		experience("a", 3, 3), experience("a", 4, 4))
	taken, _ := sample(t, store, 2)
	left := steps(store.State().Items)
	if len(left) != 2 {
		t.Fatalf("the store still holds %v after a sample under %s", left, replay.RetentionDropAfterSample)
	}
	for _, item := range taken {
		for _, step := range left {
			if step == item.Step {
				t.Fatalf("sampled step %d stayed in the store", step)
			}
		}
	}
	// The other policy is the one every test above relies on.
	keep := newStore(t, buffer(replay.SamplingUniform, replay.EvictionFIFO))
	add(t, keep, experience("a", 1, 1), experience("a", 2, 2))
	sample(t, keep, 2)
	if len(keep.State().Items) != 2 {
		t.Fatal("keep_until_evicted dropped a sampled experience")
	}
}

// TestRestoreContinuesTheSameSampleSequence is the mid-run snapshot contract:
// a store restored from its own state and carried on draws exactly what an
// uninterrupted store would have drawn.
func TestRestoreContinuesTheSameSampleSequence(t *testing.T) {
	build := func() *replay.Store {
		store := newStore(t, buffer(replay.SamplingUniform, replay.EvictionReservoir))
		for step := uint64(1); step <= 8; step++ {
			add(t, store, experience("a", step, float64(step)))
		}
		return store
	}
	straight, interrupted := build(), build()
	first, _ := sample(t, straight, 2)
	if got, _ := sample(t, interrupted, 2); !reflect.DeepEqual(steps(got), steps(first)) {
		t.Fatal("two identical runs diverged before the snapshot")
	}
	snapshot := interrupted.Snapshot()
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded replay.Snapshot
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	resumed, err := replay.RestoreSnapshot(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Snapshot(), snapshot) {
		t.Fatalf("the snapshot did not round trip:\n got %+v\nwant %+v", resumed.Snapshot(), snapshot)
	}
	for round := 0; round < 3; round++ {
		add(t, straight, experience("a", uint64(20+round), float64(round)))
		add(t, resumed, experience("a", uint64(20+round), float64(round)))
		want, wantReport := sample(t, straight, 3)
		got, gotReport := sample(t, resumed, 3)
		if !reflect.DeepEqual(steps(got), steps(want)) {
			t.Fatalf("round %d: resumed sample %v, uninterrupted %v", round, steps(got), steps(want))
		}
		if gotReport.Draws != wantReport.Draws {
			t.Fatalf("round %d: resumed drew %d, uninterrupted %d", round, gotReport.Draws, wantReport.Draws)
		}
	}
	if !reflect.DeepEqual(resumed.State(), straight.State()) {
		t.Fatal("the resumed store ended in a different state")
	}
}

// TestRestoreRejectsAnImpossibleState keeps a hand-edited state from becoming a
// store that cannot describe how it got there.
func TestRestoreRejectsAnImpossibleState(t *testing.T) {
	store := newStore(t, buffer(replay.SamplingUniform, replay.EvictionFIFO))
	add(t, store, experience("a", 1, 1), experience("a", 2, 2))
	good := store.State()
	for name, damage := range map[string]func(*replay.State){
		"unknown schema": func(s *replay.State) { s.SchemaVersion = "coimnet-replay/v0" },
		"more items than seen": func(s *replay.State) {
			s.Seen = 1
		},
		"over capacity": func(s *replay.State) {
			s.Items = append(s.Items, experience("a", 3, 3), experience("a", 4, 4), experience("a", 5, 5))
			s.Seen = 5
		},
		"test split inside": func(s *replay.State) { s.Items[0].Split = replay.SplitTest },
		"hash without flag": func(s *replay.State) { s.Items[0].Raw = false },
	} {
		t.Run(name, func(t *testing.T) {
			damaged := store.State()
			damaged.Items = append([]replay.Experience(nil), damaged.Items...)
			damage(&damaged)
			if _, err := replay.Restore(buffer(replay.SamplingUniform, replay.EvictionFIFO), damaged); err == nil {
				t.Fatal("an impossible replay state was restored")
			}
		})
	}
	if _, err := replay.Restore(buffer(replay.SamplingUniform, replay.EvictionFIFO), good); err != nil {
		t.Fatalf("the undamaged state stopped restoring: %v", err)
	}
}

// TestStateAndReportDoNotAliasTheStore keeps a reader from writing into the
// store through a report it was handed.
func TestStateAndReportDoNotAliasTheStore(t *testing.T) {
	store := newStore(t, buffer(replay.SamplingUniform, replay.EvictionFIFO))
	add(t, store, experience("a", 1, 1))
	state := store.State()
	state.Items[0].Step = 99
	state.Items[0].Input[0][0] = 99
	if again := store.State(); again.Items[0].Step != 1 || again.Items[0].Input[0][0] != 1 {
		t.Fatalf("State aliased the store: %+v", again.Items[0])
	}
	taken, _ := sample(t, store, 1)
	taken[0].Input[0][0] = 77
	if again := store.State(); again.Items[0].Input[0][0] != 1 {
		t.Fatal("Sample aliased the store")
	}
}

// TestNoReplayIsTheControl pins the comparison arm: same method set, nothing
// stored, nothing replayed, and the same contamination barrier.
func TestNoReplayIsTheControl(t *testing.T) {
	var control replay.Replay = replay.NoReplay{}
	if err := control.Add(experience("a", 1, 1)); err != nil {
		t.Fatalf("the control refused a legal experience: %v", err)
	}
	held := experience("a", 2, 2)
	held.Split = replay.SplitTest
	if err := control.Add(held); err == nil {
		t.Fatal("the control accepted held-out data")
	}
	items, report, err := control.Sample(4)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 || report.Returned != 0 || report.Requested != 4 || report.Draws != 0 {
		t.Fatalf("the control returned %d items: %+v", len(items), report)
	}
	if control.Report().Replayed {
		t.Fatal("the control reported that it replayed")
	}
	if state := control.State(); len(state.Items) != 0 || state.Seen != 0 || state.Draws != 0 {
		t.Fatalf("the control kept state: %+v", state)
	}
	var store replay.Replay = newStore(t, buffer(replay.SamplingUniform, replay.EvictionFIFO))
	if !store.Report().Replayed {
		t.Fatal("a real store reported that it does not replay")
	}
}

// TestReportNamesEveryDeclaredPolicy is the auditability requirement: capacity,
// sampling, eviction, retention, raw storage, seed and generator are all
// readable from the store itself.
func TestReportNamesEveryDeclaredPolicy(t *testing.T) {
	b := buffer(replay.SamplingTimeBalanced, replay.EvictionReservoir)
	b.Privacy.StoreRaw = false
	store := newStore(t, b)
	add(t, store, experience("a", 1, 1))
	report := store.Report()
	want := replay.Report{
		Replayed: true, Capacity: 4, Sampling: replay.SamplingTimeBalanced,
		Eviction: replay.EvictionReservoir, Retention: replay.RetentionKeepUntilEvicted,
		StoreRaw: false, Seed: 7, PRNG: replay.PRNG, Items: 1, Seen: 1, Draws: 0,
	}
	if report != want {
		t.Fatalf("report = %+v, want %+v", report, want)
	}
	if !reflect.DeepEqual(store.Declaration(), b) {
		t.Fatalf("declaration = %+v, want %+v", store.Declaration(), b)
	}
}
