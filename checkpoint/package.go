package checkpoint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"reflect"
	"strings"

	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/TimLai666/coimnet/learning"
)

const (
	// ModelPackageSchemaVersion identifies the envelope of a model package: the
	// third artefact kind, alongside the individual snapshot and the episode
	// training checkpoint. A package declares a model. It carries no persistent
	// neural state, no optimizer moments and no data cursor, so it can seed a
	// new individual but can never restore a running one.
	ModelPackageSchemaVersion = "coimnet-model-package/v1"

	// TrainingSchemaVersion is the training snapshot a package can seed. It is
	// learning's own snapshot version, not the episode checkpoint envelope.
	TrainingSchemaVersion = "coimnet-episode-training/v1"
)

// TopologyFingerprint identifies the edge arrays a package was built from.
// SHA256 is the digest of every source index followed by every target index,
// each encoded as a little-endian uint32. That encoding is identical to the
// topology hash simulate writes into its run reports, so the same graph
// produces the same value in both places; it is reimplemented privately here
// rather than shared, because checkpoint must not depend on simulate.
//
// It fingerprints the stored edge arrays, not the abstract graph: the same
// edges in a different order hash differently. It is a computational identity,
// not biological provenance; keep the source connectome.Graph and its receipts.
type TopologyFingerprint struct {
	Nodes  int    `json:"nodes"`
	Edges  int    `json:"edges"`
	SHA256 string `json:"sha256"`
}

// Units declares what one time step and one time constant are counted in.
// The strings are declarations only: "model_step" means the model advances in
// its own discrete steps and makes no claim about milliseconds or about any
// measured biological timescale. Nothing in the framework converts them.
type Units struct {
	TimeStep     string `json:"time_step"`
	TimeConstant string `json:"time_constant"`
}

// CompatibleVersions lists the artefact schema versions this package can seed.
// Individual holds individual-snapshot versions, Training holds training
// snapshot versions. A package that names a version this build does not know is
// rejected rather than read optimistically.
type CompatibleVersions struct {
	Individual []string `json:"individual"`
	Training   []string `json:"training"`
}

// ModelPackage is the shareable model declaration: anatomy fingerprint,
// computational configuration, base parameters, declared units, the evidence
// paths the publisher stands behind, and the artefact versions it can seed.
// It is deliberately not a recovery source: a reader that needs persistent
// neural state, optimizer moments or an update count must load an individual
// snapshot instead.
type ModelPackage struct {
	SchemaVersion      string              `json:"schema_version"`
	Topology           TopologyFingerprint `json:"topology"`
	Config             learning.Config     `json:"config"`
	Parameters         learning.Parameters `json:"parameters"`
	Units              Units               `json:"units"`
	EvidenceRegistry   []string            `json:"evidence_registry"`
	CompatibleVersions CompatibleVersions  `json:"compatible_versions"`
}

// NewModelPackage validates one model declaration and returns the package that
// SaveModelPackage publishes. The configuration and parameters are validated
// together through the trainer, so a package can never carry a model that
// cannot be built. Evidence paths are the caller's own relative record paths;
// they are stored verbatim and are never opened here.
func NewModelPackage(c learning.Config, p learning.Parameters, units Units, evidence []string) (ModelPackage, error) {
	return canonicalModelPackage(ModelPackage{
		SchemaVersion:      ModelPackageSchemaVersion,
		Config:             c,
		Parameters:         p,
		Units:              units,
		EvidenceRegistry:   evidence,
		CompatibleVersions: supportedCompatibleVersions(),
		Topology:           TopologyFingerprint{Nodes: configNodes(c), Edges: configEdges(c), SHA256: declaredTopologyDigest(c)},
	})
}

// SaveModelPackage validates and publishes one model package. The caller's
// package is never retained, and an existing path is never replaced.
func SaveModelPackage(ctx context.Context, path string, pkg ModelPackage) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("model package path must not be empty")
	}
	owned, err := canonicalModelPackage(pkg)
	if err != nil {
		return fmt.Errorf("invalid model package: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	payload, err := json.Marshal(owned)
	if err != nil {
		return fmt.Errorf("marshal model package payload: %w", err)
	}
	// A new package has no nullable fields. Optional arrays are omitted rather
	// than written as null, so a reader never has to interpret one.
	if err := checkUniqueJSONRejectNull(payload); err != nil {
		return fmt.Errorf("marshal model package payload: %w", err)
	}
	document, err := marshalEnvelopeFor(ModelPackageSchemaVersion, payload)
	if err != nil {
		return fmt.Errorf("marshal model package envelope: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	return publishDocument(ctx, path, document)
}

// LoadModelPackage reads one model package. It validates the raw JSON before
// encoding/json can replace invalid Unicode or turn null values into Go zero
// values, then rebuilds the model through the trainer and recomputes the
// topology fingerprint, so a document whose fingerprint no longer describes its
// configuration is refused instead of being trusted.
func LoadModelPackage(ctx context.Context, path string) (ModelPackage, error) {
	var empty ModelPackage
	if err := contextError(ctx); err != nil {
		return empty, err
	}
	if path == "" {
		return empty, fmt.Errorf("model package path must not be empty")
	}
	data, err := fileio.ReadRegular(ctx, path, maxCheckpointBytes)
	if err != nil {
		return empty, fmt.Errorf("read model package: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return empty, err
	}
	pkg, err := decodeModelPackageDocument(data)
	if err != nil {
		return empty, err
	}
	if err := contextError(ctx); err != nil {
		return empty, err
	}
	return pkg, nil
}

// NewIndividualFromPackage seeds a fresh individual from a model package: new
// persistent neural state at the given initial voltage, a new optimizer, and
// the package's own parameters. It is equivalent to learning.NewIndividual with
// the package's configuration and parameters, and it is the only supported way
// a package becomes a running model; a package can never continue an existing
// trajectory, because it carries none.
func NewIndividualFromPackage(pkg ModelPackage, o learning.Options, initial []float64) (*learning.Individual, error) {
	owned, err := canonicalModelPackage(pkg)
	if err != nil {
		return nil, fmt.Errorf("invalid model package: %w", err)
	}
	return learning.NewIndividual(owned.Config, owned.Parameters, o, initial)
}

func supportedCompatibleVersions() CompatibleVersions {
	return CompatibleVersions{
		Individual: []string{IndividualSchemaVersion},
		Training:   []string{TrainingSchemaVersion},
	}
}

// canonicalModelPackage validates every part and returns an owned copy whose
// optional arrays follow the wire representation, so a package built in memory
// and the same package read back from a file compare equal.
func canonicalModelPackage(pkg ModelPackage) (ModelPackage, error) {
	var empty ModelPackage
	if pkg.SchemaVersion != ModelPackageSchemaVersion {
		return empty, fmt.Errorf("unsupported model package schema %q", pkg.SchemaVersion)
	}
	if err := validateUnits(pkg.Units); err != nil {
		return empty, err
	}
	if err := validateCompatibleVersions(pkg.CompatibleVersions); err != nil {
		return empty, err
	}
	evidence, err := ownedEvidenceRegistry(pkg.EvidenceRegistry)
	if err != nil {
		return empty, err
	}
	// The trainer is the cheapest complete validator of a configuration and its
	// parameters together: it builds the core, checks every parameter shape and
	// runs one prediction. The optimizer it creates is discarded, and its
	// options are a local validation choice that the package never stores.
	trainer, err := learning.NewTrainer(pkg.Config, pkg.Parameters, learning.DefaultOptions())
	if err != nil {
		return empty, err
	}
	snapshot := trainer.Snapshot()
	owned := ModelPackage{
		SchemaVersion:      ModelPackageSchemaVersion,
		Config:             snapshot.Config,
		Parameters:         snapshot.Parameters,
		Units:              pkg.Units,
		EvidenceRegistry:   evidence,
		CompatibleVersions: CompatibleVersions{Individual: append([]string(nil), pkg.CompatibleVersions.Individual...), Training: append([]string(nil), pkg.CompatibleVersions.Training...)},
	}
	fingerprint, err := topologyFingerprint(owned.Config)
	if err != nil {
		return empty, err
	}
	if pkg.Topology != fingerprint {
		return empty, fmt.Errorf("model package topology fingerprint %+v does not describe its configuration %+v", pkg.Topology, fingerprint)
	}
	owned.Topology = fingerprint
	return normalizeModelPackage(owned), nil
}

func validateUnits(u Units) error {
	if strings.TrimSpace(u.TimeStep) == "" {
		return fmt.Errorf("model package must declare a time step unit")
	}
	if strings.TrimSpace(u.TimeConstant) == "" {
		return fmt.Errorf("model package must declare a time constant unit")
	}
	return nil
}

func validateCompatibleVersions(v CompatibleVersions) error {
	supported := supportedCompatibleVersions()
	if err := validateVersionList("individual", v.Individual, supported.Individual); err != nil {
		return err
	}
	return validateVersionList("training", v.Training, supported.Training)
}

func validateVersionList(kind string, declared, supported []string) error {
	if len(declared) == 0 {
		return fmt.Errorf("model package declares no compatible %s version", kind)
	}
	seen := make(map[string]bool, len(declared))
	for _, version := range declared {
		if seen[version] {
			return fmt.Errorf("model package lists compatible %s version %q twice", kind, version)
		}
		seen[version] = true
		var known bool
		for _, candidate := range supported {
			if candidate == version {
				known = true
			}
		}
		if !known {
			return fmt.Errorf("model package claims to seed unknown %s schema %q", kind, version)
		}
	}
	return nil
}

// ownedEvidenceRegistry copies the caller's evidence paths after checking they
// are usable as repository-relative record paths. The framework never opens
// them; the check exists so a published package cannot carry a path that points
// outside the record set it claims to reference.
func ownedEvidenceRegistry(entries []string) ([]string, error) {
	owned := make([]string, 0, len(entries))
	for i, entry := range entries {
		if strings.TrimSpace(entry) == "" {
			return nil, fmt.Errorf("evidence_registry[%d] must not be empty", i)
		}
		if path.IsAbs(entry) || strings.HasPrefix(entry, "/") || strings.Contains(entry, `\`) {
			return nil, fmt.Errorf("evidence_registry[%d] %q must be a relative path with forward slashes", i, entry)
		}
		for _, part := range strings.Split(entry, "/") {
			if part == ".." {
				return nil, fmt.Errorf("evidence_registry[%d] %q must not leave the record set", i, entry)
			}
		}
		owned = append(owned, entry)
	}
	return owned, nil
}

// configNodes and configEdges read whichever core a configuration declares.
// learning keeps its own private versions; these exist because checkpoint has
// to fingerprint a configuration before the trainer has validated it.
func configNodes(c learning.Config) int {
	if c.LIF != nil {
		return c.LIF.Nodes
	}
	return c.Dynamics.Nodes
}

func configEdges(c learning.Config) int {
	if c.LIF != nil {
		return len(c.LIF.Sources)
	}
	return len(c.Dynamics.Sources)
}

func configEdgeArrays(c learning.Config) (sources, targets []int) {
	if c.LIF != nil {
		return c.LIF.Sources, c.LIF.Targets
	}
	return c.Dynamics.Sources, c.Dynamics.Targets
}

func topologyFingerprint(c learning.Config) (TopologyFingerprint, error) {
	sources, targets := configEdgeArrays(c)
	digest, err := topologyDigest(sources, targets)
	if err != nil {
		return TopologyFingerprint{}, err
	}
	return TopologyFingerprint{Nodes: configNodes(c), Edges: len(sources), SHA256: digest}, nil
}

// declaredTopologyDigest is the fingerprint of a configuration that has not
// been validated yet. An encoding failure is left for canonicalModelPackage to
// report through the mismatch it then sees.
func declaredTopologyDigest(c learning.Config) string {
	sources, targets := configEdgeArrays(c)
	digest, err := topologyDigest(sources, targets)
	if err != nil {
		return ""
	}
	return digest
}

// topologyDigest is the SHA-256 of every source index followed by every target
// index, each encoded as a little-endian uint32. The encoding is identical to
// simulate's topology hash, so a graph fingerprinted here and the same graph
// fingerprinted by a simulation run produce the same digest.
func topologyDigest(sources, targets []int) (string, error) {
	if len(sources) != len(targets) {
		return "", fmt.Errorf("model package has %d sources and %d targets", len(sources), len(targets))
	}
	digest := sha256.New()
	block := make([]byte, 0, 4096)
	for _, group := range [][]int{sources, targets} {
		for _, node := range group {
			if node < 0 || int64(node) > math.MaxUint32 {
				return "", fmt.Errorf("node index %d does not fit the topology fingerprint encoding", node)
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

// normalizeModelPackage changes nil required arrays to empty arrays so their
// JSON representation is [] rather than null. InputNodes keeps its documented
// meaning, where nil is "the full core receives encoded observations".
func normalizeModelPackage(pkg ModelPackage) ModelPackage {
	pkg.Config.Dynamics.Sources = nonNilInts(pkg.Config.Dynamics.Sources)
	pkg.Config.Dynamics.Targets = nonNilInts(pkg.Config.Dynamics.Targets)
	pkg.Config.Dynamics.Delays = nonNilInts(pkg.Config.Dynamics.Delays)
	if pkg.Config.LIF != nil {
		lif := *pkg.Config.LIF
		lif.Sources = nonNilInts(lif.Sources)
		lif.Targets = nonNilInts(lif.Targets)
		lif.Delays = nonNilInts(lif.Delays)
		pkg.Config.LIF = &lif
	}
	pkg.Parameters.Core.Weights = nonNilFloats(pkg.Parameters.Core.Weights)
	pkg.Parameters.Core.Bias = nonNilFloats(pkg.Parameters.Core.Bias)
	pkg.Parameters.Core.LogTau = nonNilFloats(pkg.Parameters.Core.LogTau)
	pkg.Parameters.Encoder = nonNilFloats(pkg.Parameters.Encoder)
	pkg.Parameters.Readout = nonNilFloats(pkg.Parameters.Readout)
	if pkg.EvidenceRegistry == nil {
		pkg.EvidenceRegistry = []string{}
	}
	return pkg
}

func decodeModelPackageDocument(data []byte) (ModelPackage, error) {
	var empty ModelPackage
	if err := validateJSONUnicode(data); err != nil {
		return empty, fmt.Errorf("decode model package: %w", err)
	}
	if err := checkUniqueJSONRejectNull(data); err != nil {
		return empty, err
	}
	var raw envelope
	if err := decodeStrict(data, &raw); err != nil {
		return empty, fmt.Errorf("decode model package envelope: %w", err)
	}
	// Which artefact this file is has to be settled before the package specific
	// presence walk, so a reader who opened the wrong kind is told which kind it
	// is instead of receiving a missing-field path.
	if raw.SchemaVersion != ModelPackageSchemaVersion {
		return empty, modelPackageKindError(raw.SchemaVersion)
	}
	if err := requireModelPackageFields(data); err != nil {
		return empty, err
	}
	payload := bytes.TrimSpace(raw.Payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) {
		return empty, fmt.Errorf("model package payload must be an object")
	}
	if len(raw.Checksum) != sha256.Size*2 {
		return empty, fmt.Errorf("invalid model package checksum encoding")
	}
	got, err := hex.DecodeString(raw.Checksum)
	if err != nil {
		return empty, fmt.Errorf("invalid model package checksum encoding: %w", err)
	}
	want := sha256.Sum256(raw.Payload)
	if !bytes.Equal(got, want[:]) {
		return empty, fmt.Errorf("model package payload checksum mismatch")
	}
	var pkg ModelPackage
	if err := decodeStrict(raw.Payload, &pkg); err != nil {
		return empty, fmt.Errorf("decode model package payload: %w", err)
	}
	owned, err := canonicalModelPackage(pkg)
	if err != nil {
		return empty, fmt.Errorf("invalid model package: %w", err)
	}
	return owned, nil
}

// requireModelPackageFields closes the gap between strict decoding and presence
// validation: encoding/json cannot distinguish an omitted scalar from a present
// zero, so every required value is checked in the raw object first.
func requireModelPackageFields(data []byte) error {
	outer, err := requiredObject(data, "$", "schema_version", "payload", "checksum")
	if err != nil {
		return err
	}
	return checkRequiredFields(outer["payload"], reflect.TypeOf(ModelPackage{}), "$.payload")
}

// modelPackageKindError names the artefact kind a reader opened by mistake.
func modelPackageKindError(schema string) error {
	switch schema {
	case IndividualSchemaVersion:
		return fmt.Errorf("%q is an individual snapshot, which carries persistent neural state and optimizer moments, not a model package", schema)
	case SchemaVersion:
		return fmt.Errorf("%q is an episode training checkpoint, which carries trainer state and a data cursor, not a model package", schema)
	}
	return fmt.Errorf("unsupported model package schema %q", schema)
}

// individualKindError explains which artefact a reader opened when it is not an
// individual snapshot. The model package deserves the longest explanation
// because it is the one that could be mistaken for a complete recovery source:
// it holds only configuration and parameters, so restoring from it would
// silently invent a neural trajectory, an optimizer and an update count.
func individualKindError(schema string) error {
	switch schema {
	case ModelPackageSchemaVersion:
		return fmt.Errorf("%q is a model package, which declares configuration and parameters only and is not an individual snapshot: seed a new individual with NewIndividualFromPackage instead of restoring one", schema)
	case SchemaVersion:
		return fmt.Errorf("%q is an episode training checkpoint, which carries no persistent neural state, and is not an individual snapshot", schema)
	}
	return fmt.Errorf("unsupported individual checkpoint schema %q", schema)
}
