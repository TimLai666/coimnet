package replay

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
)

// Store is one capacity-limited replay store. It serializes its own operations,
// so one store can be handed to several goroutines; the values it returns never
// share a buffer with what it holds.
//
// The generator is math/rand/v2 PCG seeded with (Buffer.Seed, 0), the
// convention the simulate package's null models already use. Every value the
// store takes from it is counted, and bounded values are produced by rejection
// on whole draws, so the count alone is enough to replay the stream: that is
// what Restore does, and it is why a resumed run samples exactly what an
// uninterrupted run would have sampled.
type Store struct {
	mu      sync.Mutex
	buffer  Buffer
	items   []Experience
	seen    uint64
	draws   uint64
	current *rand.Rand
}

// New creates an empty store from a complete declaration.
func New(b Buffer) (*Store, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	return newStore(b), nil
}

func newStore(b Buffer) *Store {
	return &Store{buffer: b, items: []Experience{}, current: rand.New(rand.NewPCG(b.Seed, 0))}
}

// Restore rebuilds a store from its declaration and the state it had reached.
// Every stored item is validated again, because a state read back from a file
// is untrusted input: an item over the capacity, an item the store could not
// have accepted, or a fingerprint that does not match its own arrays is an
// error rather than a silently repaired store.
//
// The generator is rewound by replaying State.Draws values from the seeded
// stream, which leaves it in exactly the position the original store was in.
func Restore(b Buffer, s State) (*Store, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	if s.SchemaVersion != StateVersion {
		return nil, fmt.Errorf("replay: unsupported replay state %q", s.SchemaVersion)
	}
	if len(s.Items) > b.Capacity {
		return nil, fmt.Errorf("replay: state holds %d items, the declared capacity is %d", len(s.Items), b.Capacity)
	}
	if s.Seen < uint64(len(s.Items)) {
		return nil, fmt.Errorf("replay: state has seen %d items but holds %d", s.Seen, len(s.Items))
	}
	// A store that never removes a sampled item holds exactly as many items as
	// it has seen, up to the capacity. Under drop_after_sample it can hold
	// fewer, so only the keep policy can be checked this tightly.
	if held := min(s.Seen, uint64(b.Capacity)); b.Privacy.Retention == RetentionKeepUntilEvicted && uint64(len(s.Items)) != held {
		return nil, fmt.Errorf("replay: a store that has seen %d items under capacity %d must hold %d, not %d", s.Seen, b.Capacity, held, len(s.Items))
	}
	store := newStore(b)
	for i, item := range s.Items {
		owned, err := validateStored(item, b.Privacy.StoreRaw)
		if err != nil {
			return nil, fmt.Errorf("replay: item %d: %w", i, err)
		}
		store.items = append(store.items, owned)
	}
	store.seen = s.Seen
	store.draws = s.Draws
	for i := uint64(0); i < s.Draws; i++ {
		store.current.Uint64()
	}
	return store, nil
}

// RestoreSnapshot rebuilds a store from the pair a checkpoint carries.
func RestoreSnapshot(s Snapshot) (*Store, error) { return Restore(s.Buffer, s.State) }

// validateStored checks one item as it comes back from a persisted state and
// returns the store's own copy of it.
func validateStored(e Experience, storeRaw bool) (Experience, error) {
	if e.TaskID == "" {
		return Experience{}, fmt.Errorf("task_id must not be empty")
	}
	if err := validateSplit(e.Split); err != nil {
		return Experience{}, err
	}
	if !finite(e.Weight) || e.Weight < 0 {
		return Experience{}, fmt.Errorf("weight %v must be finite and non-negative", e.Weight)
	}
	if e.Raw != storeRaw {
		return Experience{}, fmt.Errorf("item declares raw %v under a store declared with store_raw %v", e.Raw, storeRaw)
	}
	if !e.Raw {
		if e.Input != nil || e.Target != nil {
			return Experience{}, fmt.Errorf("a store that keeps only fingerprints cannot hold arrays")
		}
		if len(e.InputHash) != 2*32 || len(e.TargetHash) != 2*32 {
			return Experience{}, fmt.Errorf("fingerprints %q/%q are not SHA-256 values", e.InputHash, e.TargetHash)
		}
		return e, nil
	}
	if err := validateRows("input", e.Input); err != nil {
		return Experience{}, err
	}
	if err := validateRows("target", e.Target); err != nil {
		return Experience{}, err
	}
	owned := copyExperience(e)
	if owned.InputHash != RowMajorHash(owned.Input) || owned.TargetHash != RowMajorHash(owned.Target) {
		return Experience{}, fmt.Errorf("fingerprint does not describe the stored arrays")
	}
	return owned, nil
}

// Add stores one experience under the declared capacity and eviction policy.
// A refused experience changes nothing at all, including the seen count and the
// generator, so a rejection cannot move the sample sequence.
func (s *Store) Add(e Experience) error {
	if err := e.validateIncoming(); err != nil {
		return err
	}
	stored := Experience{
		TaskID: e.TaskID, Step: e.Step, Weight: e.Weight, Split: e.Split,
		Raw:        s.buffer.Privacy.StoreRaw,
		InputHash:  RowMajorHash(e.Input),
		TargetHash: RowMajorHash(e.Target),
	}
	if s.buffer.Privacy.StoreRaw {
		stored.Input = copyRows(e.Input)
		stored.Target = copyRows(e.Target)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen == math.MaxUint64 {
		return fmt.Errorf("replay: seen counter overflow")
	}
	s.seen++
	if len(s.items) < s.buffer.Capacity {
		s.items = append(s.items, stored)
		return nil
	}
	switch s.buffer.Eviction {
	case EvictionFIFO:
		// The oldest item leaves first. Nothing is drawn, because nothing is
		// decided at random.
		s.items = append(s.items[:0], s.items[1:]...)
		s.items = append(s.items, stored)
	case EvictionReservoir:
		// Algorithm R: one bounded draw over the number seen so far keeps the
		// new item with probability Capacity/Seen and names the slot it
		// replaces, uniformly among the slots.
		if slot := s.draw(s.seen); slot < uint64(s.buffer.Capacity) {
			s.items[slot] = stored
		}
	}
	return nil
}

// State returns an independent copy of what the store holds.
func (s *Store) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state()
}

func (s *Store) state() State {
	items := make([]Experience, len(s.items))
	for i, item := range s.items {
		items[i] = copyExperience(item)
	}
	return State{SchemaVersion: StateVersion, Items: items, Seen: s.seen, Draws: s.draws}
}

// Snapshot returns the declaration and the state together, which is the pair
// Restore needs and the value a checkpoint carries.
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{Buffer: s.buffer, State: s.state()}
}

// Declaration returns the buffer this store was created with.
func (s *Store) Declaration() Buffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer
}

// Report names every declared policy and the counters a reader needs to
// reproduce the run.
func (s *Store) Report() Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Report{
		Replayed: true, Capacity: s.buffer.Capacity, Sampling: s.buffer.Sampling,
		Eviction: s.buffer.Eviction, Retention: s.buffer.Privacy.Retention,
		StoreRaw: s.buffer.Privacy.StoreRaw, Seed: s.buffer.Seed, PRNG: PRNG,
		Items: len(s.items), Seen: s.seen, Draws: s.draws,
	}
}

// draw returns a uniform value in [0, m) and counts every value it takes from
// the stream. Rejection happens on whole draws: a value at or above the largest
// multiple of m that fits in a uint64 is discarded and a new one taken, so the
// result is unbiased and the draw count alone reproduces the stream position.
func (s *Store) draw(m uint64) uint64 {
	if m == 0 {
		panic("replay: bounded draw needs a positive bound")
	}
	bound := uint64(math.MaxUint64) - uint64(math.MaxUint64)%m
	for {
		v := s.current.Uint64()
		s.draws++
		if v < bound {
			return v % m
		}
	}
}

func copyExperience(e Experience) Experience {
	owned := e
	owned.Input = copyRows(e.Input)
	owned.Target = copyRows(e.Target)
	return owned
}

func copyRows(rows [][]float64) [][]float64 {
	if rows == nil {
		return nil
	}
	owned := make([][]float64, len(rows))
	for i, row := range rows {
		owned[i] = append([]float64(nil), row...)
	}
	return owned
}
