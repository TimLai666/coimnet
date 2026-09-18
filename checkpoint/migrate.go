package checkpoint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TimLai666/coimnet/internal/fileio"
)

// FieldChange names one way a source field changed under the target schema and
// why, so a migration report stays auditable without re-reading both files.
type FieldChange struct {
	Path string `json:"path"`
	Kind string `json:"kind"` // added|renamed|dropped|defaulted|retyped
	Note string `json:"note"`
}

// PrecisionMapping names one value whose representation changed precision in a
// migration that narrowed or widened a numeric field.
type PrecisionMapping struct {
	Path string `json:"path"`
	From string `json:"from"`
	To   string `json:"to"`
}

// MigrationReport records one completed migration: both path names, both whole
// file SHA-256 digests, both schema strings, every changed field, every
// precision retype and every piece of information the rewrite could not carry.
// NoInformationLoss states that nothing was lost; an explicit note is part of
// the report rather than inferred from an empty InformationLoss slice.
type MigrationReport struct {
	SourcePath        string             `json:"source_path"`
	TargetPath        string             `json:"target_path"`
	SourceSHA256      string             `json:"source_sha256"`
	TargetSHA256      string             `json:"target_sha256"`
	SourceSchema      string             `json:"source_schema"`
	TargetSchema      string             `json:"target_schema"`
	FieldChanges      []FieldChange      `json:"field_changes"`
	PrecisionMapping  []PrecisionMapping `json:"precision_mapping"`
	InformationLoss   []string           `json:"information_loss"`
	NoInformationLoss bool               `json:"no_information_loss"`
}

// Migrate reads the document at src with the strict decoders, rewrites it under
// the target schema and publishes dst atomically through publishDocument. dst
// must differ from src and must not exist yet: no file is ever overwritten, and
// the source bytes are never changed. The source schema is read from the
// envelope and validated by the schema's own strict decoder, so an unreadable
// envelope or a document with unknown required fields fails with the same error
// surface as the corresponding Load, and dst is not created.
//
// Two migrations are supported. An individual document whose "neural" payload
// has no "core" member is the pre-union form the released code wrote before the
// continuous-spiking union existed; migrating it to IndividualSchemaVersion
// applies upgradeIndividualNeural and reports the rename of neural into
// neural.continuous and the added neural.core declaration with no information
// loss. Any document whose schema string already equals the target is copied
// byte for byte, also with no information loss. Anything else fails with
// "unsupported migration from <schema> to <target>".
func Migrate(ctx context.Context, src, dst, target string) (MigrationReport, error) {
	if err := contextError(ctx); err != nil {
		return MigrationReport{}, err
	}
	if src == "" {
		return MigrationReport{}, fmt.Errorf("migration source path must not be empty")
	}
	if dst == "" {
		return MigrationReport{}, fmt.Errorf("migration target path must not be empty")
	}
	if target == "" {
		return MigrationReport{}, fmt.Errorf("migration target schema must not be empty")
	}

	cleanSrc := filepath.Clean(src)
	cleanDst := filepath.Clean(dst)
	if cleanDst == cleanSrc {
		return MigrationReport{}, fmt.Errorf("migration target %q is the same file as the source %q", dst, src)
	}
	if resolvedSrc, err := filepath.EvalSymlinks(cleanSrc); err == nil {
		if resolvedDst, err := filepath.EvalSymlinks(cleanDst); err == nil && resolvedDst == resolvedSrc {
			return MigrationReport{}, fmt.Errorf("migration target %q resolves to the same file as the source %q", dst, src)
		}
	}
	if _, err := os.Lstat(cleanDst); err == nil {
		return MigrationReport{}, fmt.Errorf("migration target %q already exists", dst)
	} else if !os.IsNotExist(err) {
		return MigrationReport{}, fmt.Errorf("check migration target: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return MigrationReport{}, err
	}

	data, err := fileio.ReadRegular(ctx, src, maxCheckpointBytes)
	if err != nil {
		return MigrationReport{}, fmt.Errorf("read migration source: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return MigrationReport{}, err
	}

	if err := validateJSONUnicode(data); err != nil {
		return MigrationReport{}, fmt.Errorf("decode migration source envelope: %w", err)
	}
	if err := checkUniqueJSON(data); err != nil {
		return MigrationReport{}, err
	}
	var raw envelope
	if err := decodeStrict(data, &raw); err != nil {
		return MigrationReport{}, fmt.Errorf("decode migration source envelope: %w", err)
	}
	if raw.SchemaVersion == "" {
		return MigrationReport{}, fmt.Errorf("migration source document has no schema_version")
	}

	report := MigrationReport{
		SourcePath:      src,
		TargetPath:      dst,
		SourceSHA256:    sha256Hex(data),
		SourceSchema:    raw.SchemaVersion,
		TargetSchema:    target,
		InformationLoss: []string{},
	}

	var document []byte
	switch raw.SchemaVersion {
	case IndividualSchemaVersion:
		// The strict individual decoder validates the whole artefact before any
		// rewrite, so unknown required fields, a broken envelope or a bad
		// checksum is refused exactly as LoadIndividual would refuse it.
		if _, err := decodeIndividualDocument(data); err != nil {
			return MigrationReport{}, err
		}
		if target != IndividualSchemaVersion {
			return MigrationReport{}, fmt.Errorf("unsupported migration from %q to %q", raw.SchemaVersion, target)
		}
		payload := bytes.TrimSpace(raw.Payload)
		upgraded, err := upgradeIndividualNeural(payload)
		if err != nil {
			return MigrationReport{}, err
		}
		if bytes.Equal(upgraded, payload) {
			document = data
		} else {
			document, err = marshalEnvelopeFor(IndividualSchemaVersion, upgraded)
			if err != nil {
				return MigrationReport{}, err
			}
			report.FieldChanges = migrationPreUnionFieldChanges()
		}
		report.NoInformationLoss = true
	case SchemaVersion:
		if target != SchemaVersion {
			return MigrationReport{}, fmt.Errorf("unsupported migration from %q to %q", raw.SchemaVersion, target)
		}
		if _, err := decodeDocument(data); err != nil {
			return MigrationReport{}, err
		}
		document = data
		report.NoInformationLoss = true
	case ModelPackageSchemaVersion:
		if target != ModelPackageSchemaVersion {
			return MigrationReport{}, fmt.Errorf("unsupported migration from %q to %q", raw.SchemaVersion, target)
		}
		if _, err := decodeModelPackageDocument(data); err != nil {
			return MigrationReport{}, err
		}
		document = data
		report.NoInformationLoss = true
	default:
		if raw.SchemaVersion != target {
			return MigrationReport{}, fmt.Errorf("unsupported migration from %q to %q", raw.SchemaVersion, target)
		}
		// An unregistered schema that already equals the requested target is
		// copied verbatim; its envelope was read strictly above even though no
		// artifact-specific presence walk exists for it.
		document = data
		report.NoInformationLoss = true
	}
	if err := contextError(ctx); err != nil {
		return MigrationReport{}, err
	}
	report.TargetSHA256 = sha256Hex(document)
	if err := publishDocument(ctx, dst, document); err != nil {
		return MigrationReport{}, err
	}
	return report, nil
}

// migrationPreUnionFieldChanges is the audited difference between the pre-union
// continuous individual document and the union shape of
// IndividualSchemaVersion: the continuous dynamics state moved from directly on
// payload.neural onto payload.neural.continuous, and the union added the
// "core":"continuous" declaration that names the carried half.
func migrationPreUnionFieldChanges() []FieldChange {
	return []FieldChange{
		{
			Path: "neural",
			Kind: "renamed",
			Note: "the continuous dynamics state that sat directly on payload.neural is now the payload.neural.continuous half of the union",
		},
		{
			Path: "neural.core",
			Kind: "added",
			Note: `the union adds the "core":"continuous" declaration so every persistent neural state names its half`,
		},
	}
}

// sha256Hex is the lowercase hex SHA-256 of a whole document's bytes.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
