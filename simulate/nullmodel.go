package simulate

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/params"
)

// Null model kinds. Each one keeps the graph store and the parameter set file
// untouched and produces an in-memory derivative instead: the report records
// the kind, the seed and the hashes, so a reader can rebuild the same
// derivative from the original store and verify it.
const (
	// NullDegreePreservingRewire keeps every in and out degree and swaps edge
	// targets, so the wiring changes while the degree sequence does not.
	NullDegreePreservingRewire = "degree_preserving_rewire"
	// NullSignShuffle permutes the derived edge signs between edges. It needs
	// the derived parameter source; the uniform source has only one sign.
	NullSignShuffle = "sign_shuffle"
	// NullWeightShuffle permutes the edge strengths and leaves signs alone.
	NullWeightShuffle = "weight_shuffle"

	// NullPRNG names the generator every null model draws from, so a report
	// says how to reproduce it: math/rand/v2 PCG seeded with (seed, 0).
	NullPRNG = "pcg"

	// VariantOriginal is the name of the untouched wiring and parameters.
	VariantOriginal = "original"

	// maxSwapFactor bounds the declared attempt multiplier. It exists so a
	// mistyped factor fails immediately instead of running for hours.
	maxSwapFactor = 1 << 20
)

// NullModelSpec declares one null model. SwapFactor applies to
// degree_preserving_rewire only, where the number of swap attempts is
// ceil(SwapFactor * edges); the two shuffles must leave it at zero rather than
// carry a knob that is silently ignored.
type NullModelSpec struct {
	Kind       string  `json:"kind"`
	Seed       uint64  `json:"seed,omitempty"`
	SwapFactor float64 `json:"swap_factor,omitempty"`
}

// NullModelRejections counts why a proposed swap was refused. The two reasons
// are disjoint and checked in this order, so they add up with Applied to the
// attempt count.
type NullModelRejections struct {
	SelfLoop  uint64 `json:"self_loop"`
	Duplicate uint64 `json:"duplicate"`
}

// NullModelReport is everything needed to reproduce and to recognise one
// derivative. Attempts and Applied are the swap attempts of a rewire; for the
// two shuffles they are both the number of Fisher-Yates draws, which is one
// less than the array length, and nothing can be rejected.
//
// TopologyHash is the SHA-256 of the derived edge arrays, ParameterHash the
// hash of the derived parameter set. A reader with the original store, the
// kind and the seed recomputes both and compares.
type NullModelReport struct {
	Kind          string              `json:"kind"`
	Seed          uint64              `json:"seed"`
	PRNG          string              `json:"prng"`
	SwapFactor    float64             `json:"swap_factor,omitempty"`
	Attempts      uint64              `json:"attempts"`
	Applied       uint64              `json:"applied"`
	Rejected      NullModelRejections `json:"rejected"`
	TopologyHash  string              `json:"topology_hash"`
	ParameterHash string              `json:"parameter_hash"`
}

// Variant is one complete wiring plus parameter assignment a runner can be
// built from. The original graph is a Variant with Null nil; every null model
// is a Variant that owns its own edge arrays and parameters, so building one
// never changes the graph, the parameter set or another variant.
type Variant struct {
	Name    string           `json:"name"`
	Sources []int            `json:"-"`
	Targets []int            `json:"-"`
	Params  ParameterSet     `json:"-"`
	Null    *NullModelReport `json:"null_model,omitempty"`
}

// OriginalVariant reads the untouched topology from the graph and builds the
// parameter set the protocol declares, by exactly the same construction the
// CLI uses: UniformPositive for the engineering source, FromDerived for the
// derived one. setSHA256 is the SHA-256 of the parameter set file and is
// required by the derived source, for the reason documented on FromDerived:
// the hash has to name the exact file and a params.Set does not carry it.
func OriginalVariant(ctx context.Context, g *connectome.Graph, set *params.Set, setSHA256 string, protocol Protocol, limits Limits) (Variant, error) {
	nodes, edges, err := variantPreflight(ctx, g, protocol, limits, "")
	if err != nil {
		return Variant{}, err
	}
	sources, targets, err := streamTopology(ctx, g, nodes, edges)
	if err != nil {
		return Variant{}, err
	}
	parameters, err := variantParameters(ctx, g, set, setSHA256, protocol)
	if err != nil {
		return Variant{}, err
	}
	return Variant{Name: VariantOriginal, Sources: sources, Targets: targets, Params: parameters}, nil
}

// DeriveNullModel produces one in-memory derivative of the graph and its
// parameters. Neither the graph, the parameter set nor any slice the caller
// owns is modified: every array the result carries is freshly allocated here.
//
// degree_preserving_rewire draws ceil(SwapFactor*edges) attempts from the PRNG.
// Each attempt picks two distinct edge indices i and j and proposes the double
// edge swap (a->b),(c->d) to (a->d),(c->b). The proposal is refused when it
// would create a self loop, and then when either new pair already exists, so
// in and out degrees are preserved exactly and pairs stay unique. Every edge
// keeps its own weight and sign, because only the target arrays move.
//
// sign_shuffle permutes the derived sign labels between edges with a
// Fisher-Yates shuffle and then applies the protocol's unknown sign policy
// through FromDerived, so the +1, -1 and unknown counts are preserved.
// weight_shuffle permutes the strengths: gain times the raw weight for the
// uniform source, the derived strength for the derived one; signs stay put.
func DeriveNullModel(ctx context.Context, g *connectome.Graph, set *params.Set, setSHA256 string, protocol Protocol, spec NullModelSpec, limits Limits) (Variant, NullModelReport, error) {
	var empty NullModelReport
	if err := spec.validate(protocol.ParameterSource); err != nil {
		return Variant{}, empty, err
	}
	nodes, edges, err := variantPreflight(ctx, g, protocol, limits, spec.Kind)
	if err != nil {
		return Variant{}, empty, err
	}
	sources, targets, err := streamTopology(ctx, g, nodes, edges)
	if err != nil {
		return Variant{}, empty, err
	}
	rng := rand.New(rand.NewPCG(spec.Seed, 0))
	report := NullModelReport{Kind: spec.Kind, Seed: spec.Seed, PRNG: NullPRNG, SwapFactor: spec.SwapFactor}
	var parameters ParameterSet
	switch spec.Kind {
	case NullDegreePreservingRewire:
		if err := rewire(ctx, rng, spec, sources, targets, &report); err != nil {
			return Variant{}, empty, err
		}
		if parameters, err = variantParameters(ctx, g, set, setSHA256, protocol); err != nil {
			return Variant{}, empty, err
		}
	case NullSignShuffle:
		if parameters, err = shuffledSigns(set, setSHA256, protocol, rng, &report); err != nil {
			return Variant{}, empty, err
		}
	default: // NullWeightShuffle
		if parameters, err = shuffledWeights(ctx, g, set, setSHA256, protocol, rng, &report); err != nil {
			return Variant{}, empty, err
		}
	}
	if report.TopologyHash, err = topologyHash(sources, targets); err != nil {
		return Variant{}, empty, err
	}
	report.ParameterHash = parameters.Hash
	variant := Variant{
		Name:    spec.Kind + "-seed-" + fmt.Sprint(spec.Seed),
		Sources: sources,
		Targets: targets,
		Params:  parameters,
		Null:    &NullModelReport{},
	}
	*variant.Null = report
	return variant, report, nil
}

// validate checks a spec on its own and against the parameter source that will
// supply the strengths and signs.
func (s NullModelSpec) validate(source string) error {
	switch s.Kind {
	case NullDegreePreservingRewire:
		if !finite(s.SwapFactor) || s.SwapFactor <= 0 {
			return fmt.Errorf("simulate: null model %q needs a finite swap_factor above zero, got %v", s.Kind, s.SwapFactor)
		}
		if s.SwapFactor > maxSwapFactor {
			return fmt.Errorf("simulate: null model swap_factor %v exceeds the %d attempt multiplier limit", s.SwapFactor, maxSwapFactor)
		}
	case NullSignShuffle, NullWeightShuffle:
		if s.SwapFactor != 0 {
			return fmt.Errorf("simulate: null model %q performs one shuffle and ignores swap_factor, so it must be absent or zero, got %v", s.Kind, s.SwapFactor)
		}
		if s.Kind == NullSignShuffle && source != ParameterSourceDerived {
			return fmt.Errorf("simulate: null model %q needs the %q parameter source; %q makes every edge excitatory, so there is no sign label to permute", s.Kind, ParameterSourceDerived, source)
		}
	default:
		return fmt.Errorf("simulate: unsupported null model kind %q; choose %q, %q or %q", s.Kind, NullDegreePreservingRewire, NullSignShuffle, NullWeightShuffle)
	}
	return nil
}

// variantPreflight repeats the graph and protocol checks Build performs and
// accounts the arrays a variant needs before any of them is allocated.
func variantPreflight(ctx context.Context, g *connectome.Graph, protocol Protocol, limits Limits, kind string) (int, int, error) {
	if ctx == nil {
		return 0, 0, errors.New("simulate: nil context")
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, fmt.Errorf("simulate: %w", err)
	}
	if g == nil || g.NodeCount() == 0 {
		return 0, 0, errors.New("simulate: nil or empty graph")
	}
	if err := protocol.Validate(); err != nil {
		return 0, 0, err
	}
	nodes, edges := int(g.NodeCount()), int(g.EdgeCount())
	if uint64(nodes) != g.NodeCount() || uint64(edges) != g.EdgeCount() {
		return 0, 0, fmt.Errorf("%w: graph size does not fit this platform's int", ErrCapacity)
	}
	if int64(nodes) > math.MaxUint32 {
		return 0, 0, fmt.Errorf("%w: %d nodes do not fit the 32 bit node index a topology hash and a pair key use", ErrCapacity, nodes)
	}
	if err := accountVariant(limits, edges, kind); err != nil {
		return 0, 0, err
	}
	return nodes, edges, nil
}

// accountVariant bounds the arrays one variant holds: its own source, target
// and weight arrays, the copy a shuffle makes, and for a rewire the pair hash
// table. It is the same accounting style as account: an arithmetic bound on
// the arrays this package allocates, not a process memory limit.
func accountVariant(limits Limits, edges int, kind string) error {
	if limits.MaxMemoryBytes <= 0 {
		return fmt.Errorf("%w: the memory limit must be positive, got %d", ErrCapacity, limits.MaxMemoryBytes)
	}
	total, err := checkedProduct(int64(edges), 8, 4)
	if err != nil {
		return err
	}
	if kind == NullDegreePreservingRewire {
		slots, err := pairSlots(edges)
		if err != nil {
			return err
		}
		table, err := checkedProduct(int64(slots), 8, 1)
		if err != nil {
			return err
		}
		if total > math.MaxInt64-table {
			return fmt.Errorf("%w: accounted bytes overflow", ErrCapacity)
		}
		total += table
	}
	if total > limits.MaxMemoryBytes {
		return fmt.Errorf("%w: a %s variant of %d edges accounts %d bytes, over the %d byte limit", ErrCapacity, variantLabel(kind), edges, total, limits.MaxMemoryBytes)
	}
	return nil
}

func variantLabel(kind string) string {
	if kind == "" {
		return VariantOriginal
	}
	return kind
}

// variantParameters builds the parameter set the protocol declares from the
// untouched sources, exactly as simulate run does.
func variantParameters(ctx context.Context, g *connectome.Graph, set *params.Set, setSHA256 string, protocol Protocol) (ParameterSet, error) {
	if protocol.ParameterSource != ParameterSourceDerived {
		if set != nil {
			return ParameterSet{}, fmt.Errorf("simulate: parameter source %q derives its weights from the graph and takes no parameter set", protocol.ParameterSource)
		}
		return UniformPositive(ctx, g, *protocol.Uniform)
	}
	if set == nil {
		return ParameterSet{}, fmt.Errorf("simulate: parameter source %q needs the parameter set its edge signs and strengths come from", ParameterSourceDerived)
	}
	parameters, _, err := FromDerived(set, setSHA256, protocol)
	return parameters, err
}

// rewire performs the double edge swaps in place on targets. sources never
// moves, so every node keeps its out degree, and each swap exchanges two
// targets, so every node keeps its in degree.
func rewire(ctx context.Context, rng *rand.Rand, spec NullModelSpec, sources, targets []int, report *NullModelReport) error {
	edges := len(sources)
	if edges < 2 {
		return fmt.Errorf("simulate: null model %q needs at least two edges, the graph has %d", NullDegreePreservingRewire, edges)
	}
	attempts := math.Ceil(spec.SwapFactor * float64(edges))
	if !finite(attempts) || attempts > math.MaxInt64 {
		return fmt.Errorf("%w: swap_factor %v over %d edges does not produce a usable attempt count", ErrCapacity, spec.SwapFactor, edges)
	}
	pairs, err := newPairSet(edges)
	if err != nil {
		return err
	}
	for i, source := range sources {
		if !pairs.add(pairKey(source, targets[i])) {
			return fmt.Errorf("simulate: null model %q needs unique pairs, but %d->%d appears more than once in the graph", NullDegreePreservingRewire, source, targets[i])
		}
	}
	report.Attempts = uint64(attempts)
	for attempt := uint64(0); attempt < report.Attempts; attempt++ {
		if attempt%(1<<20) == 0 {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("simulate: %w", err)
			}
		}
		// Two distinct indices from exactly two draws, so the sequence of
		// draws depends only on the seed and the attempt count.
		i := rng.IntN(edges)
		j := rng.IntN(edges - 1)
		if j >= i {
			j++
		}
		a, b, c, d := sources[i], targets[i], sources[j], targets[j]
		if a == d || c == b {
			report.Rejected.SelfLoop++
			continue
		}
		if pairs.has(pairKey(a, d)) || pairs.has(pairKey(c, b)) {
			report.Rejected.Duplicate++
			continue
		}
		pairs.remove(pairKey(a, b))
		pairs.remove(pairKey(c, d))
		pairs.add(pairKey(a, d))
		pairs.add(pairKey(c, b))
		targets[i], targets[j] = d, b
		report.Applied++
	}
	return nil
}

// shuffledSigns permutes the derived sign labels and rebuilds the parameters
// through the protocol's unknown sign policy. The caller's set is never
// written: the shuffle works on a copy and the set value is copied by value
// with only that array replaced.
func shuffledSigns(set *params.Set, setSHA256 string, protocol Protocol, rng *rand.Rand, report *NullModelReport) (ParameterSet, error) {
	if set == nil {
		return ParameterSet{}, fmt.Errorf("simulate: null model %q needs the derived parameter set", NullSignShuffle)
	}
	signs := append([]int8(nil), set.EdgeSign...)
	draws := shuffle(rng, len(signs), func(i, j int) { signs[i], signs[j] = signs[j], signs[i] })
	report.Attempts, report.Applied = draws, draws
	permuted := *set
	permuted.EdgeSign = signs
	parameters, _, err := FromDerived(&permuted, setSHA256, protocol)
	return parameters, err
}

// shuffledWeights permutes the edge strengths of either source and leaves the
// signs where they are.
func shuffledWeights(ctx context.Context, g *connectome.Graph, set *params.Set, setSHA256 string, protocol Protocol, rng *rand.Rand, report *NullModelReport) (ParameterSet, error) {
	if protocol.ParameterSource != ParameterSourceDerived {
		parameters, err := variantParameters(ctx, g, set, setSHA256, protocol)
		if err != nil {
			return ParameterSet{}, err
		}
		// UniformPositive returns its own array, so shuffling it in place
		// touches nothing the caller holds.
		weights := parameters.Weights
		draws := shuffle(rng, len(weights), func(i, j int) { weights[i], weights[j] = weights[j], weights[i] })
		report.Attempts, report.Applied = draws, draws
		parameters.Hash = parameters.fingerprint()
		return parameters, nil
	}
	if set == nil {
		return ParameterSet{}, fmt.Errorf("simulate: null model %q needs the derived parameter set", NullWeightShuffle)
	}
	weights := append([]float64(nil), set.EdgeWeight...)
	draws := shuffle(rng, len(weights), func(i, j int) { weights[i], weights[j] = weights[j], weights[i] })
	report.Attempts, report.Applied = draws, draws
	permuted := *set
	permuted.EdgeWeight = weights
	parameters, _, err := FromDerived(&permuted, setSHA256, protocol)
	return parameters, err
}

// shuffle is the Fisher-Yates permutation and returns the number of draws it
// took, which is one less than the length of a non-empty array.
func shuffle(rng *rand.Rand, n int, swap func(i, j int)) uint64 {
	draws := uint64(0)
	for i := n - 1; i > 0; i-- {
		swap(i, rng.IntN(i+1))
		draws++
	}
	return draws
}

// topologyHash is the SHA-256 of every source index followed by every target
// index, each encoded as a little-endian uint32. Two graphs with the same edge
// arrays hash equally on every platform.
func topologyHash(sources, targets []int) (string, error) {
	if len(sources) != len(targets) {
		return "", fmt.Errorf("simulate: %d sources and %d targets", len(sources), len(targets))
	}
	digest := sha256.New()
	// One four byte Write per index would cost tens of millions of calls on a
	// whole-brain graph, so the indices are encoded into a block first. The
	// bytes fed to the digest are the same either way.
	block := make([]byte, 0, 4096)
	for _, group := range [][]int{sources, targets} {
		for _, node := range group {
			if node < 0 || int64(node) > math.MaxUint32 {
				return "", fmt.Errorf("simulate: node index %d does not fit the topology hash encoding", node)
			}
			block = binary.LittleEndian.AppendUint32(block, uint32(node))
			if len(block) == cap(block) {
				digest.Write(block)
				block = block[:0]
			}
		}
	}
	digest.Write(block)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// pairSet is an open addressing hash set of source-target pairs with linear
// probing. A slot holds the key plus one so that zero means empty and the pair
// 0->0 is still storable. Deletion uses the backward shift of Knuth's
// algorithm 6.4R instead of tombstones, because a rewire deletes twice per
// applied swap and tombstones would fill the table.
type pairSet struct {
	slots []uint64
	mask  uint64
	count int
}

// pairSlots is the table size: the next power of two at or above twice the
// edge count, and at least two. A rewire holds at most one key per edge, so the
// table never passes half load and a probe always reaches an empty slot.
func pairSlots(edges int) (uint64, error) {
	if edges < 0 {
		return 0, fmt.Errorf("simulate: negative edge count %d", edges)
	}
	want := uint64(edges) * 2
	slots := uint64(2)
	for slots < want {
		if slots > math.MaxUint64/2 {
			return 0, fmt.Errorf("%w: a pair table for %d edges does not fit", ErrCapacity, edges)
		}
		slots <<= 1
	}
	return slots, nil
}

func newPairSet(edges int) (*pairSet, error) {
	slots, err := pairSlots(edges)
	if err != nil {
		return nil, err
	}
	if slots > math.MaxInt64/8 {
		return nil, fmt.Errorf("%w: a pair table of %d slots does not fit", ErrCapacity, slots)
	}
	return &pairSet{slots: make([]uint64, slots), mask: slots - 1}, nil
}

func pairKey(source, target int) uint64 {
	return uint64(uint32(source))<<32 | uint64(uint32(target))
}

// mixPair is the splitmix64 finaliser. It spreads the structured pair keys,
// whose low bits are dense target indices, across the table.
func mixPair(key uint64) uint64 {
	key ^= key >> 33
	key *= 0xff51afd7ed558ccd
	key ^= key >> 33
	key *= 0xc4ceb9fe1a85ec53
	key ^= key >> 33
	return key
}

func (s *pairSet) find(key uint64) (uint64, bool) {
	stored := key + 1
	i := mixPair(key) & s.mask
	for {
		switch s.slots[i] {
		case 0:
			return i, false
		case stored:
			return i, true
		}
		i = (i + 1) & s.mask
	}
}

func (s *pairSet) has(key uint64) bool {
	_, found := s.find(key)
	return found
}

// add stores a key and reports whether it was new.
func (s *pairSet) add(key uint64) bool {
	i, found := s.find(key)
	if found {
		return false
	}
	s.slots[i] = key + 1
	s.count++
	return true
}

// remove deletes a key and closes the probe chain behind it.
func (s *pairSet) remove(key uint64) bool {
	i, found := s.find(key)
	if !found {
		return false
	}
	s.count--
	j := i
	for {
		s.slots[i] = 0
		for {
			j = (j + 1) & s.mask
			if s.slots[j] == 0 {
				return true
			}
			home := mixPair(s.slots[j]-1) & s.mask
			if !cyclicallyWithin(i, j, home) {
				break
			}
		}
		s.slots[i] = s.slots[j]
		i = j
	}
}

// cyclicallyWithin reports whether home lies in the cyclic interval (i, j]. An
// element whose home slot is inside that interval cannot move back to i.
func cyclicallyWithin(i, j, home uint64) bool {
	if i <= j {
		return i < home && home <= j
	}
	return i < home || home <= j
}
