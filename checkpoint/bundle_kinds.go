package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/TimLai666/coimnet/learning"
)

// SaveModelPackageBundle validates pkg exactly as SaveModelPackage does and writes it as a model_package bundle whose
// DocumentSchema is ModelPackageSchemaVersion and whose Topology is the package's validated fingerprint.
func SaveModelPackageBundle(ctx context.Context, dir string, pkg ModelPackage) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if dir == "" {
		return fmt.Errorf("model package bundle directory must not be empty")
	}
	owned, err := canonicalModelPackage(pkg)
	if err != nil {
		return fmt.Errorf("invalid model package: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	return writeBundle(ctx, dir, BundleManifest{
		Kind: BundleModelPackage, DocumentSchema: ModelPackageSchemaVersion, Topology: owned.Topology,
	}, owned)
}

// LoadModelPackageBundle reads a model_package bundle, applies to the decoded package every check LoadModelPackage applies
// after decoding (canonicalModelPackage: rebuild the model, recompute and compare the fingerprint), requires the manifest's
// Topology to equal the package's, and returns the normalized package.
func LoadModelPackageBundle(ctx context.Context, dir string) (ModelPackage, error) {
	var empty ModelPackage
	var pkg ModelPackage
	manifest, err := readBundle(ctx, dir, BundleModelPackage, &pkg)
	if err != nil {
		return empty, err
	}
	if manifest.DocumentSchema != ModelPackageSchemaVersion {
		return empty, fmt.Errorf("model package bundle document schema %q, want %q", manifest.DocumentSchema, ModelPackageSchemaVersion)
	}
	owned, err := canonicalModelPackage(pkg)
	if err != nil {
		return empty, fmt.Errorf("invalid model package: %w", err)
	}
	if manifest.Topology != owned.Topology {
		return empty, fmt.Errorf("model package bundle topology %+v does not match package topology %+v", manifest.Topology, owned.Topology)
	}
	return normalizeModelPackage(owned), nil
}

// SaveIndividualBundle validates snapshot exactly as SaveIndividual does (learning.RestoreIndividual, then the normalized
// snapshot of the restored individual) and writes that as an individual bundle (DocumentSchema IndividualSchemaVersion,
// Topology of snapshot.Config).
func SaveIndividualBundle(ctx context.Context, dir string, snapshot learning.IndividualSnapshot) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if dir == "" {
		return fmt.Errorf("checkpoint bundle directory must not be empty")
	}
	owned, err := validatedIndividualSnapshot(snapshot)
	if err != nil {
		return fmt.Errorf("invalid individual snapshot: %w", err)
	}
	topology, err := individualTopologyFingerprint(owned.Config)
	if err != nil {
		return fmt.Errorf("fingerprint individual topology: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	return writeBundle(ctx, dir, BundleManifest{
		Kind: BundleIndividual, DocumentSchema: IndividualSchemaVersion, Topology: topology,
	}, owned)
}

// LoadIndividualBundle reads an individual bundle, normalizes the decoded snapshot, requires learning.RestoreIndividual to
// accept it and the manifest Topology to equal the snapshot's, and returns the restored individual's normalized snapshot.
func LoadIndividualBundle(ctx context.Context, dir string) (learning.IndividualSnapshot, error) {
	var empty learning.IndividualSnapshot
	var snapshot learning.IndividualSnapshot
	manifest, err := readBundle(ctx, dir, BundleIndividual, &snapshot)
	if err != nil {
		return empty, err
	}
	if manifest.DocumentSchema != IndividualSchemaVersion {
		return empty, fmt.Errorf("individual bundle document schema %q, want %q", manifest.DocumentSchema, IndividualSchemaVersion)
	}
	snapshot = normalizeIndividualSnapshot(snapshot)
	individual, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		return empty, fmt.Errorf("invalid individual snapshot: %w", err)
	}
	topology, err := individualTopologyFingerprint(snapshot.Config)
	if err != nil {
		return empty, fmt.Errorf("fingerprint individual topology: %w", err)
	}
	if manifest.Topology != topology {
		return empty, fmt.Errorf("individual bundle topology %+v does not match snapshot topology %+v", manifest.Topology, topology)
	}
	return normalizeIndividualSnapshot(individual.Snapshot()), nil
}

// SaveTrainingBundle requires snapshot.SchemaVersion == TrainingSchemaVersion and learning.RestoreTrainer to accept it,
// and writes the restored trainer's snapshot as a training bundle (DocumentSchema TrainingSchemaVersion).
func SaveTrainingBundle(ctx context.Context, dir string, snapshot learning.TrainingSnapshot) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if dir == "" {
		return fmt.Errorf("training bundle directory must not be empty")
	}
	if snapshot.SchemaVersion != TrainingSchemaVersion {
		return fmt.Errorf("unsupported training snapshot %q", snapshot.SchemaVersion)
	}
	trainer, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		return fmt.Errorf("invalid training snapshot: %w", err)
	}
	owned := normalizeBundleTrainingSnapshot(trainer.Snapshot())
	topology, err := topologyFingerprint(owned.Config)
	if err != nil {
		return fmt.Errorf("fingerprint training topology: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	return writeBundle(ctx, dir, BundleManifest{
		Kind: BundleTraining, DocumentSchema: TrainingSchemaVersion, Topology: topology,
	}, owned)
}

// LoadTrainingBundle reads a training bundle, requires SchemaVersion == TrainingSchemaVersion and learning.RestoreTrainer to
// accept it, and returns the restored trainer's snapshot.
func LoadTrainingBundle(ctx context.Context, dir string) (learning.TrainingSnapshot, error) {
	var empty learning.TrainingSnapshot
	var snapshot learning.TrainingSnapshot
	manifest, err := readBundle(ctx, dir, BundleTraining, &snapshot)
	if err != nil {
		return empty, err
	}
	if manifest.DocumentSchema != TrainingSchemaVersion {
		return empty, fmt.Errorf("training bundle document schema %q, want %q", manifest.DocumentSchema, TrainingSchemaVersion)
	}
	if snapshot.SchemaVersion != TrainingSchemaVersion {
		return empty, fmt.Errorf("unsupported training snapshot %q", snapshot.SchemaVersion)
	}
	trainer, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		return empty, fmt.Errorf("invalid training snapshot: %w", err)
	}
	owned := normalizeBundleTrainingSnapshot(trainer.Snapshot())
	topology, err := topologyFingerprint(owned.Config)
	if err != nil {
		return empty, fmt.Errorf("fingerprint training topology: %w", err)
	}
	if manifest.Topology != topology {
		return empty, fmt.Errorf("training bundle manifest topology %+v does not match snapshot topology %+v", manifest.Topology, topology)
	}
	return owned, nil
}

// validatedIndividualSnapshot keeps the temporary restored individual and its references to the full snapshot inside this
// function. Only the normalized snapshot returns, so the validation instance is out of scope before gob encoding starts.
func validatedIndividualSnapshot(snapshot learning.IndividualSnapshot) (learning.IndividualSnapshot, error) {
	individual, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		return learning.IndividualSnapshot{}, err
	}
	return normalizeIndividualSnapshot(individual.Snapshot()), nil
}

// ReadBundleManifest reads and validates only dir/manifest.json, exactly as the first two steps of readBundle do (without
// the kind check and without touching the payload), and returns it, so a caller can report a bundle's payload size and
// SHA-256 without decoding it. bundle.go is read-only in this ticket, so implement the manifest read and its checks here
// (the same checks as readBundle's steps 1 and 2) instead of refactoring readBundle.
func ReadBundleManifest(ctx context.Context, dir string) (BundleManifest, error) {
	var empty BundleManifest
	if err := contextError(ctx); err != nil {
		return empty, err
	}
	manifestPath := filepath.Join(dir, bundleManifestFile)
	data, err := fileio.ReadRegular(ctx, manifestPath, maxBundleManifestBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return empty, fmt.Errorf("incomplete bundle %q: manifest is missing", dir)
		}
		return empty, fmt.Errorf("read bundle manifest %q: %w", manifestPath, err)
	}
	if err := checkUniqueJSON(data); err != nil {
		return empty, fmt.Errorf("invalid bundle manifest %q: %w", manifestPath, err)
	}
	var manifest BundleManifest
	if err := decodeStrict(data, &manifest); err != nil {
		return empty, fmt.Errorf("decode bundle manifest %q: %w", manifestPath, err)
	}
	if manifest.SchemaVersion != BundleSchemaVersion {
		return empty, fmt.Errorf("unsupported bundle schema %q", manifest.SchemaVersion)
	}
	if manifest.PayloadFile != bundlePayloadFile {
		return empty, fmt.Errorf("invalid bundle payload_file %q", manifest.PayloadFile)
	}
	if !validBundleSHA256(manifest.PayloadSHA256) {
		return empty, fmt.Errorf("invalid bundle payload SHA-256 %q", manifest.PayloadSHA256)
	}
	if manifest.PayloadBytes <= 0 {
		return empty, fmt.Errorf("bundle payload size must be positive, got %d", manifest.PayloadBytes)
	}
	if manifest.DocumentSchema == "" {
		return empty, fmt.Errorf("bundle document schema must not be empty")
	}
	return manifest, nil
}

// individualTopologyFingerprint includes mixed-core edge arrays, which the model-package fingerprint helper does not cover.
func individualTopologyFingerprint(config learning.Config) (TopologyFingerprint, error) {
	if config.Mixed != nil {
		digest, err := topologyDigest(config.Mixed.Sources, config.Mixed.Targets)
		if err != nil {
			return TopologyFingerprint{}, err
		}
		return TopologyFingerprint{Nodes: config.Mixed.Nodes, Edges: len(config.Mixed.Sources), SHA256: digest}, nil
	}
	return topologyFingerprint(config)
}

// normalizeBundleTrainingSnapshot restores required empty arrays after gob decoding collapses nil and empty slices.
func normalizeBundleTrainingSnapshot(snapshot learning.TrainingSnapshot) learning.TrainingSnapshot {
	snapshot.Config.Dynamics.Sources = nonNilInts(snapshot.Config.Dynamics.Sources)
	snapshot.Config.Dynamics.Targets = nonNilInts(snapshot.Config.Dynamics.Targets)
	snapshot.Config.Dynamics.Delays = nonNilInts(snapshot.Config.Dynamics.Delays)
	snapshot.Config.ReadoutNodes = nonNilInts(snapshot.Config.ReadoutNodes)
	if snapshot.Config.LIF != nil {
		lif := *snapshot.Config.LIF
		lif.Sources = nonNilInts(lif.Sources)
		lif.Targets = nonNilInts(lif.Targets)
		lif.Delays = nonNilInts(lif.Delays)
		snapshot.Config.LIF = &lif
	}
	if snapshot.Config.Mixed != nil {
		mixed := *snapshot.Config.Mixed
		mixed.Sources = nonNilInts(mixed.Sources)
		mixed.Targets = nonNilInts(mixed.Targets)
		mixed.Delays = nonNilInts(mixed.Delays)
		if mixed.NodeRule == nil {
			mixed.NodeRule = []uint8{}
		}
		snapshot.Config.Mixed = &mixed
	}
	snapshot.Parameters.Core.Weights = nonNilFloats(snapshot.Parameters.Core.Weights)
	snapshot.Parameters.Core.Bias = nonNilFloats(snapshot.Parameters.Core.Bias)
	snapshot.Parameters.Core.LogTau = nonNilFloats(snapshot.Parameters.Core.LogTau)
	snapshot.Parameters.Encoder = nonNilFloats(snapshot.Parameters.Encoder)
	snapshot.Parameters.Readout = nonNilFloats(snapshot.Parameters.Readout)
	snapshot.Optimizer.First = nonNilFloats(snapshot.Optimizer.First)
	snapshot.Optimizer.Second = nonNilFloats(snapshot.Optimizer.Second)
	snapshot.Optimizer.Steps = nonNilUint64(snapshot.Optimizer.Steps)
	if snapshot.Accumulator != nil {
		accumulator := *snapshot.Accumulator
		accumulator.Sum = nonNilFloats(accumulator.Sum)
		snapshot.Accumulator = &accumulator
	}
	return snapshot
}
