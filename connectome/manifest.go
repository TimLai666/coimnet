// Package connectome builds an immutable, reproducible graph view from
// read-only official connectome source files. The MaleCNS v1.0 Feather files
// are the first explicit adapter; every dataset-specific choice (field names,
// endpoint identity, selection rule) is declared in a DatasetManifest and
// recorded in the report instead of being implied by the builder.
package connectome

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/TimLai666/coimnet/internal/jsonkey"
)

const (
	// ManifestSchemaVersion identifies the DatasetManifest JSON contract.
	ManifestSchemaVersion = "coimnet-dataset-manifest/v1"
	// ReportSchemaVersion identifies the GraphReport JSON contract.
	ReportSchemaVersion = "coimnet-graph-report/v1"
	// ConverterVersion identifies the normalization rules of this builder.
	// Any change to ordering, exclusion reasons or report semantics must bump it.
	ConverterVersion = "coimnet-connectome-builder/v1"

	// HashUpstreamVerified means the source checksum was compared with the
	// provider's published value. HashLocallyRecorded means it was only
	// computed locally after acquisition.
	HashUpstreamVerified = "upstream_verified"
	HashLocallyRecorded  = "locally_recorded"

	// MaxManifestBytes bounds manifest JSON accepted by DecodeManifest.
	MaxManifestBytes int64 = 1 << 20
)

// FileRole names the three official source files consumed by the builder.
type FileRole string

const (
	RoleWeights           FileRole = "weights"
	RoleAnnotations       FileRole = "annotations"
	RoleNeurotransmitters FileRole = "neurotransmitters"
)

// DuplicateSemantics declares what repeated (source, target) rows in the
// weights file mean. Only AdditivePartitions permits pair aggregation.
type DuplicateSemantics string

const (
	DuplicateSemanticsUnknown             DuplicateSemantics = "unknown"
	DuplicateSemanticsAdditivePartitions  DuplicateSemantics = "additive_partitions"
	DuplicateSemanticsTotalWithPartitions DuplicateSemantics = "total_with_partitions"
)

var (
	// ErrInvalidManifest wraps manifest structure and value errors.
	ErrInvalidManifest = errors.New("connectome: invalid dataset manifest")
	// ErrInvalidSelection wraps selection predicate errors.
	ErrInvalidSelection = errors.New("connectome: invalid selection predicate")
	// ErrIdentityMapping is returned when the manifest does not declare that
	// weights endpoints are annotation IDs; the annotated view is refused.
	ErrIdentityMapping = errors.New("connectome: weights endpoint identity mapping is not declared")
	// ErrSourceChanged is returned when a source file does not match the
	// manifest fingerprint before, during or after reading.
	ErrSourceChanged = errors.New("connectome: source file does not match its manifest fingerprint")
	// ErrAggregationEvidence is returned when an aggregated pair view is
	// requested without additive duplicate semantics in the manifest.
	ErrAggregationEvidence = errors.New("connectome: duplicate pair aggregation has no source evidence")
	// ErrCapacity wraps every memory, temporary space, run count or width
	// limit failure. The builder never downsizes the graph instead.
	ErrCapacity = errors.New("connectome: capacity limit exceeded")
)

// License records the source license attribution.
type License struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// SourceFile declares one read-only source with its expected fingerprint.
type SourceFile struct {
	Role       FileRole `json:"role"`
	Path       string   `json:"path"`
	Bytes      int64    `json:"bytes"`
	SHA256     string   `json:"sha256"`
	HashStatus string   `json:"hash_status"`
	URL        string   `json:"url,omitempty"`
	ETag       string   `json:"etag,omitempty"`
}

// WeightsFields maps the official weights columns.
type WeightsFields struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Value  string `json:"value"`
}

// AnnotationFields maps official annotation columns. Only ID is required;
// an empty name leaves that NodeRecord field unknown.
type AnnotationFields struct {
	ID           string `json:"id"`
	Status       string `json:"status,omitempty"`
	StatusLabel  string `json:"status_label,omitempty"`
	Class        string `json:"class,omitempty"`
	Superclass   string `json:"superclass,omitempty"`
	Subclass     string `json:"subclass,omitempty"`
	Type         string `json:"type,omitempty"`
	Instance     string `json:"instance,omitempty"`
	SomaSide     string `json:"soma_side,omitempty"`
	ReceptorType string `json:"receptor_type,omitempty"`
}

// NeurotransmitterFields maps official prediction columns.
type NeurotransmitterFields struct {
	ID         string `json:"id"`
	Consensus  string `json:"consensus,omitempty"`
	Predicted  string `json:"predicted,omitempty"`
	Confidence string `json:"confidence,omitempty"`
}

// FieldMapping holds the official column names for every role.
type FieldMapping struct {
	Weights           WeightsFields          `json:"weights"`
	Annotations       AnnotationFields       `json:"annotations"`
	Neurotransmitters NeurotransmitterFields `json:"neurotransmitters"`
}

// IdentityMapping declares, with cited evidence, that endpoint IDs in the
// weights and neurotransmitter files are the annotation IDs. The builder
// never infers this from column names.
type IdentityMapping struct {
	WeightsEndpointsAreAnnotationIDs    bool   `json:"weights_endpoints_are_annotation_ids"`
	NeurotransmitterIDsAreAnnotationIDs bool   `json:"neurotransmitter_ids_are_annotation_ids"`
	Evidence                            string `json:"evidence"`
}

// SelectionPredicate is the explicit, serializable node selection rule:
// annotation rows whose Field equals Equals are selected.
type SelectionPredicate struct {
	Source FileRole `json:"source"`
	Field  string   `json:"field"`
	Equals string   `json:"equals"`
	Label  string   `json:"label"`
}

var fieldNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (p SelectionPredicate) validate() error {
	if p.Source != RoleAnnotations {
		return fmt.Errorf("%w: source %q is not supported; only %q", ErrInvalidSelection, p.Source, RoleAnnotations)
	}
	if !fieldNamePattern.MatchString(p.Field) {
		return fmt.Errorf("%w: field %q is not a plain column name", ErrInvalidSelection, p.Field)
	}
	if p.Equals == "" {
		return fmt.Errorf("%w: equals value must not be empty", ErrInvalidSelection)
	}
	if strings.TrimSpace(p.Label) == "" {
		return fmt.Errorf("%w: label must state what the selection means", ErrInvalidSelection)
	}
	return nil
}

// Canonical returns the single normalized form of the predicate, which is
// the input of Hash. The label is descriptive and excluded.
func (p SelectionPredicate) Canonical() (string, error) {
	if err := p.validate(); err != nil {
		return "", err
	}
	return string(p.Source) + "." + p.Field + " == " + strconv.Quote(p.Equals), nil
}

// Hash returns the SHA-256 of the canonical form as lowercase hex.
func (p SelectionPredicate) Hash() (string, error) {
	canonical, err := p.Canonical()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:]), nil
}

// TransformStep records one step of the provenance chain of the sources.
type TransformStep struct {
	Step        string `json:"step"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

// DatasetManifest declares everything the builder needs to reproduce a
// graph: sources with fingerprints, official field names, endpoint identity
// evidence, the selection rule and provenance.
type DatasetManifest struct {
	SchemaVersion      string             `json:"schema_version"`
	Dataset            string             `json:"dataset"`
	Namespace          string             `json:"namespace"`
	SourceVersion      string             `json:"source_version"`
	License            License            `json:"license"`
	AcquiredAt         string             `json:"acquired_at"`
	Files              []SourceFile       `json:"files"`
	FieldMapping       FieldMapping       `json:"field_mapping"`
	Identity           IdentityMapping    `json:"identity"`
	Selection          SelectionPredicate `json:"selection"`
	DuplicateSemantics DuplicateSemantics `json:"duplicate_semantics"`
	CoordinateUnit     string             `json:"coordinate_unit"`
	TransformHistory   []TransformStep    `json:"transform_history"`
}

// Validate checks structure and values. It does not touch the filesystem.
func (m DatasetManifest) Validate() error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidManifest, fmt.Sprintf(format, args...))
	}
	if m.SchemaVersion != ManifestSchemaVersion {
		return invalid("schema_version %q, want %q", m.SchemaVersion, ManifestSchemaVersion)
	}
	for _, field := range [][2]string{{"dataset", m.Dataset}, {"namespace", m.Namespace}, {"source_version", m.SourceVersion}, {"license.url", m.License.URL}, {"coordinate_unit", m.CoordinateUnit}} {
		if strings.TrimSpace(field[1]) == "" {
			return invalid("%s must not be empty", field[0])
		}
	}
	if strings.ContainsAny(m.Namespace, "/ \t\r\n") {
		return invalid("namespace %q must not contain '/' or whitespace", m.Namespace)
	}
	if _, err := time.Parse(time.RFC3339Nano, m.AcquiredAt); err != nil {
		return invalid("acquired_at %q is not RFC 3339: %v", m.AcquiredAt, err)
	}
	if len(m.Files) != 3 {
		return invalid("files has %d entries, want exactly one per role", len(m.Files))
	}
	seen := map[FileRole]bool{}
	for i, file := range m.Files {
		switch file.Role {
		case RoleWeights, RoleAnnotations, RoleNeurotransmitters:
		default:
			return invalid("files[%d] role %q is unknown", i, file.Role)
		}
		if seen[file.Role] {
			return invalid("files[%d] duplicates role %q", i, file.Role)
		}
		seen[file.Role] = true
		if strings.TrimSpace(file.Path) == "" {
			return invalid("files[%d] path must not be empty", i)
		}
		if file.Bytes <= 0 {
			return invalid("files[%d] bytes %d must be positive", i, file.Bytes)
		}
		if !isLowerHex(file.SHA256, sha256.Size*2) {
			return invalid("files[%d] sha256 must be %d lowercase hexadecimal characters", i, sha256.Size*2)
		}
		if file.HashStatus != HashUpstreamVerified && file.HashStatus != HashLocallyRecorded {
			return invalid("files[%d] hash_status %q must be %q or %q", i, file.HashStatus, HashUpstreamVerified, HashLocallyRecorded)
		}
	}
	for _, field := range [][2]string{{"weights.source", m.FieldMapping.Weights.Source}, {"weights.target", m.FieldMapping.Weights.Target}, {"weights.value", m.FieldMapping.Weights.Value}, {"annotations.id", m.FieldMapping.Annotations.ID}, {"neurotransmitters.id", m.FieldMapping.Neurotransmitters.ID}} {
		if !fieldNamePattern.MatchString(field[1]) {
			return invalid("field_mapping.%s %q must be a plain column name", field[0], field[1])
		}
	}
	for _, field := range m.optionalColumns() {
		if field[1] != "" && !fieldNamePattern.MatchString(field[1]) {
			return invalid("field_mapping.%s %q must be a plain column name", field[0], field[1])
		}
	}
	if (m.Identity.WeightsEndpointsAreAnnotationIDs || m.Identity.NeurotransmitterIDsAreAnnotationIDs) && strings.TrimSpace(m.Identity.Evidence) == "" {
		return invalid("identity.evidence must cite the source of the endpoint identity claim")
	}
	if err := m.Selection.validate(); err != nil {
		return err
	}
	switch m.DuplicateSemantics {
	case DuplicateSemanticsUnknown, DuplicateSemanticsAdditivePartitions, DuplicateSemanticsTotalWithPartitions:
	default:
		return invalid("duplicate_semantics %q must be %q, %q or %q", m.DuplicateSemantics, DuplicateSemanticsUnknown, DuplicateSemanticsAdditivePartitions, DuplicateSemanticsTotalWithPartitions)
	}
	if len(m.TransformHistory) == 0 {
		return invalid("transform_history must record at least the acquisition step")
	}
	for i, step := range m.TransformHistory {
		if strings.TrimSpace(step.Step) == "" {
			return invalid("transform_history[%d].step must not be empty", i)
		}
	}
	return nil
}

// optionalColumns lists the optional column mappings in a fixed order so
// validation errors are deterministic.
func (m DatasetManifest) optionalColumns() [][2]string {
	a, n := m.FieldMapping.Annotations, m.FieldMapping.Neurotransmitters
	return [][2]string{
		{"annotations.status", a.Status}, {"annotations.status_label", a.StatusLabel}, {"annotations.class", a.Class},
		{"annotations.superclass", a.Superclass}, {"annotations.subclass", a.Subclass}, {"annotations.type", a.Type},
		{"annotations.instance", a.Instance}, {"annotations.soma_side", a.SomaSide}, {"annotations.receptor_type", a.ReceptorType},
		{"neurotransmitters.consensus", n.Consensus}, {"neurotransmitters.predicted", n.Predicted}, {"neurotransmitters.confidence", n.Confidence},
	}
}

// File returns the declared source for role.
func (m DatasetManifest) File(role FileRole) (SourceFile, error) {
	for _, file := range m.Files {
		if file.Role == role {
			return file, nil
		}
	}
	return SourceFile{}, fmt.Errorf("%w: role %q is not declared", ErrInvalidManifest, role)
}

// Hash returns the SHA-256 of the canonical JSON encoding of the validated
// manifest as lowercase hex. Local file paths are excluded so the hash is
// portable; the source fingerprints identify the inputs.
func (m DatasetManifest) Hash() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	portable := m
	portable.Files = make([]SourceFile, len(m.Files))
	for i, file := range m.Files {
		file.Path = ""
		portable.Files[i] = file
	}
	encoded, err := json.Marshal(portable)
	if err != nil {
		return "", fmt.Errorf("connectome: encode manifest: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// DecodeManifest reads exactly one strict JSON manifest and validates it.
func DecodeManifest(r io.Reader) (DatasetManifest, error) {
	var manifest DatasetManifest
	if err := decodeStrict(r, MaxManifestBytes, &manifest); err != nil {
		return DatasetManifest{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if err := manifest.Validate(); err != nil {
		return DatasetManifest{}, err
	}
	return manifest, nil
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// decodeStrict decodes one JSON value, rejecting unknown fields, duplicate
// keys (compared case-insensitively, as encoding/json matches fields) and
// trailing data.
func decodeStrict(r io.Reader, maxBytes int64, dst any) error {
	if r == nil || (reflect.ValueOf(r).Kind() == reflect.Pointer && reflect.ValueOf(r).IsNil()) {
		return errors.New("JSON reader must not be nil")
	}
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maxBytes {
		return fmt.Errorf("JSON input exceeds %d bytes", maxBytes)
	}
	if err := rejectDuplicateKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON data")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}

// maxJSONDepth bounds nesting during the duplicate-key walk; json.Decoder.Token
// itself has no nesting limit.
const maxJSONDepth = 64

func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var path []string
	return scanJSONValue(decoder, &path)
}

// scanJSONValue walks one JSON value. The path is kept as a stack and only
// formatted when an error is reported, so the walk allocates per key, not
// per key times depth.
func scanJSONValue(decoder *json.Decoder, path *[]string) error {
	if len(*path) > maxJSONDepth {
		return fmt.Errorf("JSON nesting exceeds %d levels at %s", maxJSONDepth, formatJSONPath(*path))
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key at %s is not a string", formatJSONPath(*path))
			}
			folded := jsonkey.Fold(key)
			if _, exists := seen[folded]; exists {
				return fmt.Errorf("duplicate JSON key %q at %s", key, formatJSONPath(*path))
			}
			seen[folded] = struct{}{}
			*path = append(*path, key)
			if err := scanJSONValue(decoder, path); err != nil {
				return err
			}
			*path = (*path)[:len(*path)-1]
		}
		_, err = decoder.Token()
		return err
	case '[':
		for i := 0; decoder.More(); i++ {
			*path = append(*path, strconv.Itoa(i))
			if err := scanJSONValue(decoder, path); err != nil {
				return err
			}
			*path = (*path)[:len(*path)-1]
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %v at %s", delim, formatJSONPath(*path))
	}
}

func formatJSONPath(path []string) string {
	if len(path) == 0 {
		return "$"
	}
	return "$." + strings.Join(path, ".")
}
