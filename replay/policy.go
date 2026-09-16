// Package replay is the capacity-limited experience store of the online
// learning loop. It holds past experiences under a declared capacity, sampling
// policy, eviction policy, seed and privacy policy, and it reports every one of
// them, so a run can be read back and rebuilt rather than trusted.
//
// The package deliberately depends on nothing else in the project: it stores
// arrays and fingerprints, it never looks at a model, and it is therefore usable
// from the trainer side, the individual side or a command line without either
// of them importing the other.
//
// Held-out data never enters the store. An experience declared as the test
// split is refused by every implementation here, including the NoReplay
// control, so "the test set leaked into replay" is not a mistake a caller can
// make quietly.
package replay

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
)

// StateVersion identifies the serialized replay state. It is separate from
// every checkpoint envelope version, because the store can be persisted by more
// than one of them.
const StateVersion = "coimnet-replay/v1"

// PRNG names the generator every policy here draws from, so a report says how
// to reproduce a run: math/rand/v2 PCG seeded with (Seed, 0), the same
// convention the null models of the simulate package use.
const PRNG = "pcg"

// Sampling policies.
const (
	// SamplingUniform draws from the whole store with equal probability.
	SamplingUniform = "uniform"
	// SamplingTaskBalanced takes one item per task per round, so a task with
	// few stored experiences is not drowned by a task with many.
	SamplingTaskBalanced = "task_balanced"
	// SamplingTimeBalanced buckets the store by Step and takes one item per
	// bucket per round, so recent experience does not crowd out older
	// experience simply by being more numerous.
	SamplingTimeBalanced = "time_balanced"
)

// Eviction policies.
const (
	// EvictionFIFO drops the oldest stored item when the capacity is full.
	EvictionFIFO = "fifo"
	// EvictionReservoir keeps a uniform sample of everything ever added: the
	// new item replaces a uniformly chosen slot with probability
	// Capacity/Seen and is dropped otherwise.
	EvictionReservoir = "reservoir"
)

// Retention policies. They are the removal rule the privacy policy declares,
// which is separate from the capacity rule the eviction policy declares.
const (
	// RetentionKeepUntilEvicted keeps an item until the capacity pushes it out.
	RetentionKeepUntilEvicted = "keep_until_evicted"
	// RetentionDropAfterSample removes an item from the store as soon as it has
	// been sampled once, so no experience is replayed twice across updates.
	// The privacy rule wins over the sampling guarantee: combined with the
	// reservoir eviction policy the store no longer holds a uniform sample of
	// everything it has seen, because items leave for a reason the reservoir
	// does not model. Report names both policies, so the combination is
	// visible rather than implied.
	RetentionDropAfterSample = "drop_after_sample"
)

// Declared data splits. The store knows these four names and refuses anything
// else, so a typo cannot become a fifth split that bypasses the test-split
// barrier. SplitTest is refused by every Add. The adaptive evaluation of the
// second stage of this work adds its own refusal for SplitScoring, counted
// where the contamination check can report it.
const (
	SplitTrain      = "train"
	SplitAdaptation = "adaptation"
	SplitScoring    = "scoring"
	SplitTest       = "test"
)

// PrivacyPolicy declares what the store is allowed to keep. StoreRaw false
// makes the store keep only the SHA-256 fingerprints of an experience, never
// the arrays, so a sample can prove which experience it refers to without the
// store holding the data itself. Retention is one of the declared retention
// policies above.
type PrivacyPolicy struct {
	StoreRaw  bool   `json:"store_raw"`
	Retention string `json:"retention"`
}

// Buffer is the complete declaration of one replay store. Every field is
// recorded in Report and in the persisted state, because LRN-07 asks for the
// capacity, the sampling, the removal rules and the seed to be readable after
// the fact rather than inferred.
type Buffer struct {
	Capacity int           `json:"capacity"`
	Sampling string        `json:"sampling"`
	Eviction string        `json:"eviction"`
	Seed     uint64        `json:"seed"`
	Privacy  PrivacyPolicy `json:"privacy"`
}

func (b Buffer) validate() error {
	if b.Capacity <= 0 {
		return fmt.Errorf("replay: capacity %d must be positive", b.Capacity)
	}
	switch b.Sampling {
	case SamplingUniform, SamplingTaskBalanced, SamplingTimeBalanced:
	default:
		return fmt.Errorf("replay: unsupported sampling policy %q", b.Sampling)
	}
	switch b.Eviction {
	case EvictionFIFO, EvictionReservoir:
	default:
		return fmt.Errorf("replay: unsupported eviction policy %q", b.Eviction)
	}
	switch b.Privacy.Retention {
	case RetentionKeepUntilEvicted, RetentionDropAfterSample:
	default:
		return fmt.Errorf("replay: unsupported retention policy %q", b.Privacy.Retention)
	}
	return nil
}

// Experience is one stored experience. A caller fills TaskID, Step, Input,
// Target, Weight and Split; Raw, InputHash and TargetHash are the store's own
// answer and must be left zero, so nobody can hand the store a fingerprint it
// did not compute.
//
// Raw says whether this value carries the arrays. A store declared with
// StoreRaw false returns items with Raw false, Input and Target absent, and the
// two fingerprints present. The fingerprints are recorded in both modes, so a
// raw store and a hashed store describe the same experience the same way.
type Experience struct {
	TaskID string      `json:"task_id"`
	Step   uint64      `json:"step"`
	Input  [][]float64 `json:"input,omitempty"`
	Target [][]float64 `json:"target,omitempty"`
	Weight float64     `json:"weight"`
	Split  string      `json:"split"`

	Raw        bool   `json:"raw"`
	InputHash  string `json:"input_hash,omitempty"`
	TargetHash string `json:"target_hash,omitempty"`
}

// validateIncoming checks one experience as a caller hands it in.
func (e Experience) validateIncoming() error {
	if e.Raw || e.InputHash != "" || e.TargetHash != "" {
		return fmt.Errorf("replay: raw, input_hash and target_hash are written by the store, not by the caller")
	}
	if e.TaskID == "" {
		return fmt.Errorf("replay: task_id must not be empty")
	}
	if err := validateSplit(e.Split); err != nil {
		return err
	}
	if !finite(e.Weight) || e.Weight < 0 {
		return fmt.Errorf("replay: weight %v must be finite and non-negative", e.Weight)
	}
	if err := validateRows("input", e.Input); err != nil {
		return err
	}
	return validateRows("target", e.Target)
}

// validateSplit refuses held-out data and any split this package does not know.
func validateSplit(split string) error {
	switch split {
	case SplitTrain, SplitAdaptation, SplitScoring:
		return nil
	case SplitTest:
		return fmt.Errorf("replay: the %q split never enters a replay store", SplitTest)
	default:
		return fmt.Errorf("replay: unsupported split %q", split)
	}
}

func validateRows(name string, rows [][]float64) error {
	if len(rows) == 0 {
		return fmt.Errorf("replay: %s must have at least one row", name)
	}
	width := len(rows[0])
	if width == 0 {
		return fmt.Errorf("replay: %s row 0 is empty", name)
	}
	for t, row := range rows {
		if len(row) != width {
			return fmt.Errorf("replay: %s row %d has width %d, row 0 has %d", name, t, len(row), width)
		}
		for i, v := range row {
			if !finite(v) {
				return fmt.Errorf("replay: %s[%d][%d] is not finite", name, t, i)
			}
		}
	}
	return nil
}

// RowMajorHash is the declared fingerprint of a value matrix: SHA-256 over the
// little-endian IEEE-754 bits of every value in row-major order, hex encoded.
// The shape is not part of the preimage, because every matrix a store holds for
// one task already has a fixed width.
func RowMajorHash(values [][]float64) string {
	digest := sha256.New()
	var buf [8]byte
	for _, row := range values {
		for _, v := range row {
			binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
			digest.Write(buf[:])
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// SampleReport describes one Sample call. Duplicates is measured rather than
// assumed: every policy samples without replacement, so it is always zero, and
// a nonzero value would mean the same experience was counted twice inside one
// update. Draws is how many values this call took from the generator; the
// store's cumulative count is State().Draws.
//
// PerTask counts the returned items of each task. PerBucket is present only for
// the time-balanced policy, which is the only one that buckets by Step.
type SampleReport struct {
	Requested  int            `json:"requested"`
	Returned   int            `json:"returned"`
	PerTask    map[string]int `json:"per_task"`
	PerBucket  []int          `json:"per_bucket,omitempty"`
	Duplicates int            `json:"duplicates"`
	Draws      uint64         `json:"draws"`
}

// Report is the auditable summary of one store: whether it replays at all, and
// every policy it was declared with. It is comparable on purpose, so a test or
// a reader can pin the whole declaration in one place.
type Report struct {
	Replayed  bool   `json:"replayed"`
	Capacity  int    `json:"capacity"`
	Sampling  string `json:"sampling"`
	Eviction  string `json:"eviction"`
	Retention string `json:"retention"`
	StoreRaw  bool   `json:"store_raw"`
	Seed      uint64 `json:"seed"`
	PRNG      string `json:"prng"`
	Items     int    `json:"items"`
	Seen      uint64 `json:"seen"`
	Draws     uint64 `json:"draws"`
}

// State is the serializable state one store has reached. Draws is the number of
// values already taken from the generator: a restored store replays exactly
// that many before it draws again, which is what makes a resumed sample
// sequence identical to an uninterrupted one.
type State struct {
	SchemaVersion string       `json:"schema_version"`
	Items         []Experience `json:"items"`
	Seen          uint64       `json:"seen"`
	Draws         uint64       `json:"draws"`
}

// Snapshot pairs the declaration with the state, because neither alone can
// rebuild a store: the state says what is held, the declaration says under
// which policies it was held. It is the value a checkpoint carries.
type Snapshot struct {
	Buffer Buffer `json:"buffer"`
	State  State  `json:"state"`
}

// Replay is the method set both the real store and the no-replay control
// implement, so an experiment can swap one for the other without changing the
// code around it.
type Replay interface {
	Add(e Experience) error
	Sample(n int) ([]Experience, SampleReport, error)
	State() State
	Report() Report
}

var (
	_ Replay = (*Store)(nil)
	_ Replay = NoReplay{}
)

// NoReplay is the control arm: it keeps nothing and replays nothing, so an
// experiment can measure what the replay store actually contributed. It still
// refuses held-out data and malformed experiences, because a control that
// accepted what the real store refuses would not be the same experiment.
type NoReplay struct{}

// Add validates the experience and discards it.
func (NoReplay) Add(e Experience) error { return e.validateIncoming() }

// Sample always returns nothing and draws nothing.
func (NoReplay) Sample(n int) ([]Experience, SampleReport, error) {
	if n <= 0 {
		return nil, SampleReport{}, fmt.Errorf("replay: sample size %d must be positive", n)
	}
	return []Experience{}, SampleReport{Requested: n, PerTask: map[string]int{}}, nil
}

// State is always empty: the control has nothing to persist and nothing to
// resume.
func (NoReplay) State() State {
	return State{SchemaVersion: StateVersion, Items: []Experience{}}
}

// Report names the control as the control.
func (NoReplay) Report() Report { return Report{Replayed: false, PRNG: PRNG} }

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
