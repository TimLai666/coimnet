// Package params derives dynamics parameters for a connectome graph from the
// official MaleCNS release files under explicit, versioned rules.
//
// Nothing here measures a synapse. The release carries predicted transmitter
// probabilities per T-bar and raw connection weights; a rule file turns those
// into a sign, a confidence and a normalized strength per edge, and records
// every count and every unknown. Unknown stays unknown: the package never
// substitutes a default sign.
//
// The pipeline is bounded: every per-synapse stage goes through
// internal/extsort with fixed-width records, the memory, temporary space, run
// file, Arrow and row limits come from Limits, and a cancelled or over-budget
// run leaves no temporary file behind.
package params

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"reflect"
	"strings"

	"github.com/TimLai666/coimnet/internal/strictjson"
)

const (
	// RulesSchemaVersion identifies the derivation rules file contract.
	RulesSchemaVersion = "coimnet-derivation-rules/v1"
	// SignRuleSchemaVersion identifies the sign rule block inside it.
	SignRuleSchemaVersion = "coimnet-sign-rule/v1"
	// MaxRulesBytes bounds a rules document before it is decoded.
	MaxRulesBytes = 1 << 20

	rulesHashDomain = "coimnet-derivation-rules-hash/v1"
)

// Sign values a rule may map a transmitter to. There is no default: a
// transmitter the rule calls unknown produces an unknown edge sign.
const (
	SignPositive = "+1"
	SignNegative = "-1"
	SignUnknown  = "unknown"
)

// Strength normalizers. post_total divides by the target's body-stats post
// count, pre_total by the source's pre count, none divides by one.
const (
	NormalizerNone      = "none"
	NormalizerPostTotal = "post_total"
	NormalizerPreTotal  = "pre_total"
)

// Transmitters is the fixed index order of every probability column, every
// mapping key and the EdgeTransmitter codes 0..6.
var Transmitters = [7]string{"acetylcholine", "dopamine", "gaba", "glutamate", "histamine", "octopamine", "serotonin"}

// NoTransmitter is the EdgeTransmitter code of an edge no synapse matched.
const NoTransmitter uint8 = 255

// assumptionWords are the markers the glutamate basis must contain, because
// inhibitory glutamate in the fly is an engineering assumption, not a
// measurement in the release.
var assumptionWords = []string{"假設", "assumption"}

// SourceRef names one release file relative to the rules file directory and
// the SHA-256 it must still have.
type SourceRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Sources are the four release files the derivation reads.
type Sources struct {
	BodyStats    SourceRef `json:"body_stats"`
	Tbar         SourceRef `json:"tbar"`
	SynPartners  SourceRef `json:"syn_partners"`
	NeuprintMeta SourceRef `json:"neuprint_meta"`
}

// refs returns the four sources with the role names used in the report.
func (s Sources) refs() []struct {
	Role string
	Ref  SourceRef
} {
	return []struct {
		Role string
		Ref  SourceRef
	}{
		{"body_stats", s.BodyStats},
		{"tbar", s.Tbar},
		{"syn_partners", s.SynPartners},
		{"neuprint_meta", s.NeuprintMeta},
	}
}

// SignRule turns the winning transmitter of an edge into a sign. Mapping must
// name all seven transmitters, Basis must justify every one of them and the
// glutamate basis must say it is an assumption.
type SignRule struct {
	SchemaVersion      string            `json:"schema_version"`
	Mapping            map[string]string `json:"mapping"`
	Basis              map[string]string `json:"basis"`
	MinProbability     float64           `json:"min_probability"`
	MinMatchedFraction float64           `json:"min_matched_fraction"`
}

// StrengthRule is weight = Gain * raw_weight / normalizer.
type StrengthRule struct {
	Gain       float64 `json:"gain"`
	Normalizer string  `json:"normalizer"`
}

// Rules is one complete derivation rule document.
type Rules struct {
	SchemaVersion string       `json:"schema_version"`
	Sources       Sources      `json:"sources"`
	Sign          SignRule     `json:"sign"`
	Strength      StrengthRule `json:"strength"`
}

// DecodeRules reads one rules document under the shared strict JSON rules:
// at most MaxRulesBytes, no unknown fields, no duplicate keys under Unicode
// simple folding, no trailing data. Every value is validated before it is
// returned.
func DecodeRules(r io.Reader) (Rules, error) {
	var rules Rules
	if r == nil || (reflect.ValueOf(r).Kind() == reflect.Pointer && reflect.ValueOf(r).IsNil()) {
		return Rules{}, errors.New("params: rules reader must not be nil")
	}
	if err := strictjson.Decode(r, MaxRulesBytes, &rules); err != nil {
		return Rules{}, fmt.Errorf("params: decode rules: %w", err)
	}
	if err := rules.Validate(); err != nil {
		return Rules{}, err
	}
	return rules, nil
}

// Validate reports the first rule that cannot be used.
func (r Rules) Validate() error {
	if r.SchemaVersion != RulesSchemaVersion {
		return fmt.Errorf("params: rules schema_version %q, want %q", r.SchemaVersion, RulesSchemaVersion)
	}
	for _, source := range r.Sources.refs() {
		if err := validateSourceRef(source.Role, source.Ref); err != nil {
			return err
		}
	}
	if err := r.Sign.validate(); err != nil {
		return err
	}
	return r.Strength.validate()
}

func validateSourceRef(role string, ref SourceRef) error {
	if ref.Path == "" {
		return fmt.Errorf("params: source %s has no path", role)
	}
	if path.IsAbs(ref.Path) || strings.HasPrefix(ref.Path, "/") || strings.Contains(ref.Path, `\`) {
		return fmt.Errorf("params: source %s path %q must be relative to the rules file", role, ref.Path)
	}
	for _, element := range strings.Split(ref.Path, "/") {
		if element == ".." || element == "" {
			return fmt.Errorf("params: source %s path %q must not contain empty or parent elements", role, ref.Path)
		}
	}
	if !isLowerHex(ref.SHA256, sha256.Size*2) {
		return fmt.Errorf("params: source %s sha256 %q is not 64 lowercase hexadecimal characters", role, ref.SHA256)
	}
	return nil
}

func (s SignRule) validate() error {
	if s.SchemaVersion != SignRuleSchemaVersion {
		return fmt.Errorf("params: sign rule schema_version %q, want %q", s.SchemaVersion, SignRuleSchemaVersion)
	}
	if len(s.Mapping) != len(Transmitters) {
		return fmt.Errorf("params: sign mapping has %d entries, want all %d transmitters", len(s.Mapping), len(Transmitters))
	}
	for _, name := range Transmitters {
		value, ok := s.Mapping[name]
		if !ok {
			return fmt.Errorf("params: sign mapping has no entry for %s", name)
		}
		switch value {
		case SignPositive, SignNegative, SignUnknown:
		default:
			return fmt.Errorf("params: sign mapping %s is %q, want %q, %q or %q", name, value, SignPositive, SignNegative, SignUnknown)
		}
		basis, ok := s.Basis[name]
		if !ok || strings.TrimSpace(basis) == "" {
			return fmt.Errorf("params: sign basis for %s must state why the rule assigns that sign", name)
		}
	}
	if len(s.Basis) != len(Transmitters) {
		return fmt.Errorf("params: sign basis has %d entries, want all %d transmitters", len(s.Basis), len(Transmitters))
	}
	glutamate := s.Basis["glutamate"]
	stated := false
	for _, word := range assumptionWords {
		if strings.Contains(glutamate, word) {
			stated = true
		}
	}
	if !stated {
		return fmt.Errorf("params: the glutamate basis must contain %q or %q; inhibitory glutamate is an engineering assumption, not release evidence", assumptionWords[0], assumptionWords[1])
	}
	for name, value := range map[string]float64{"min_probability": s.MinProbability, "min_matched_fraction": s.MinMatchedFraction} {
		if math.IsNaN(value) || value < 0 || value > 1 {
			return fmt.Errorf("params: sign rule %s is %v, want a value in [0,1]", name, value)
		}
	}
	return nil
}

func (s StrengthRule) validate() error {
	if math.IsNaN(s.Gain) || math.IsInf(s.Gain, 0) || s.Gain <= 0 {
		return fmt.Errorf("params: strength gain is %v, want a finite value above zero", s.Gain)
	}
	switch s.Normalizer {
	case NormalizerNone, NormalizerPostTotal, NormalizerPreTotal:
		return nil
	default:
		return fmt.Errorf("params: strength normalizer %q, want %q, %q or %q", s.Normalizer, NormalizerNone, NormalizerPostTotal, NormalizerPreTotal)
	}
}

// Hash is the SHA-256 of the domain string, a zero byte and the canonical JSON
// encoding of the rules. encoding/json sorts map keys, so the same rule
// content always produces the same hash regardless of the document layout.
func (r Rules) Hash() string {
	canonical, err := json.Marshal(r)
	if err != nil {
		// Rules holds only strings, floats and string maps, so Marshal cannot
		// fail; hash the error text rather than pretend the rules are known.
		canonical = []byte("params: rules are not encodable: " + err.Error())
	}
	digest := sha256.New()
	digest.Write([]byte(rulesHashDomain))
	digest.Write([]byte{0})
	digest.Write(canonical)
	return hex.EncodeToString(digest.Sum(nil))
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
