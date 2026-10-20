package connectome

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/signal"
)

func nanValue() float64 { return math.NaN() }

func TestManifestJSONRoundTripIsStrict(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeManifest(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Namespace != manifest.Namespace || len(decoded.Files) != 3 || !reflect.DeepEqual(decoded.Selection, manifest.Selection) || decoded.Identity != manifest.Identity || decoded.Files[0] != manifest.Files[0] {
		t.Fatalf("decoded = %#v", decoded)
	}
	hashA, err := manifest.Hash()
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := decoded.Hash()
	if err != nil || hashA != hashB || len(hashA) != 64 {
		t.Fatalf("manifest hashes %q %q, err=%v", hashA, hashB, err)
	}
	moved := manifest
	moved.Files = append([]SourceFile(nil), manifest.Files...)
	moved.Files[0].Path = "/elsewhere/weights.feather"
	if hashC, err := moved.Hash(); err != nil || hashC != hashA {
		t.Fatalf("local path changed the manifest hash: %q vs %q, err=%v", hashC, hashA, err)
	}
	moved.Files[0].SHA256 = strings.Repeat("1", 64)
	if hashD, _ := moved.Hash(); hashD == hashA {
		t.Fatal("source fingerprint did not change the manifest hash")
	}

	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatal(err)
	}
	generic["unexpected"] = true
	withUnknown, _ := json.Marshal(generic)
	if _, err := DecodeManifest(bytes.NewReader(withUnknown)); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := DecodeManifest(bytes.NewReader(append(encoded, []byte(" {}")...))); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	if _, err := DecodeManifest(strings.NewReader(`{"schema_version":"x"}`)); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("invalid manifest: %v", err)
	}
	if _, err := DecodeManifest(nil); err == nil {
		t.Fatal("nil reader accepted")
	}
	bad := manifest
	bad.AcquiredAt = "yesterday"
	if err := bad.Validate(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("acquired_at: %v", err)
	}
	bad = manifest
	bad.Files = append([]SourceFile(nil), manifest.Files...)
	bad.Files[0].HashStatus = "guessed"
	if err := bad.Validate(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("hash status: %v", err)
	}
	bad = manifest
	bad.Files = append([]SourceFile(nil), manifest.Files...)
	bad.Files[0].SHA256 = "abc"
	if err := bad.Validate(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("short sha: %v", err)
	}
	bad = manifest
	bad.Namespace = " "
	if err := bad.Validate(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("namespace: %v", err)
	}
	bad = manifest
	bad.TransformHistory = nil
	if err := bad.Validate(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("transform history: %v", err)
	}
	bad = manifest
	bad.CoordinateUnit = ""
	if err := bad.Validate(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("coordinate unit: %v", err)
	}
}

func TestSelectionPredicateCanonicalFormAndHash(t *testing.T) {
	p := SelectionPredicate{Source: RoleAnnotations, Field: "status", Equals: "Traced", Label: "engineering selection"}
	canonical, err := p.Canonical()
	if err != nil || canonical != `annotations.status == "Traced"` {
		t.Fatalf("canonical = %q, %v", canonical, err)
	}
	hash, err := p.Hash()
	if err != nil || len(hash) != 64 {
		t.Fatalf("hash = %q, %v", hash, err)
	}
	relabeled := p
	relabeled.Label = "different label"
	otherHash, _ := relabeled.Hash()
	if otherHash != hash {
		t.Fatal("label must not change the predicate hash")
	}
	quoted := SelectionPredicate{Source: RoleAnnotations, Field: "status", Equals: `Tra"ced`, Label: "x"}
	canonical, err = quoted.Canonical()
	if err != nil || canonical != `annotations.status == "Tra\"ced"` {
		t.Fatalf("quoted canonical = %q, %v", canonical, err)
	}
	for _, bad := range []SelectionPredicate{
		{}, {Source: RoleAnnotations, Field: "status", Label: "x"}, {Source: RoleWeights, Field: "weight", Equals: "1", Label: "x"},
		{Source: RoleAnnotations, Field: "status", Equals: "Traced"}, {Source: RoleAnnotations, Field: "bad field", Equals: "x", Label: "x"},
	} {
		if _, err := bad.Canonical(); !errors.Is(err, ErrInvalidSelection) {
			t.Fatalf("predicate %#v: %v", bad, err)
		}
	}
}

func TestSelectionPredicateAnyIsAnOrderFreeUnion(t *testing.T) {
	p := SelectionPredicate{Source: RoleAnnotations, Field: "superclass", Any: []string{"visual_projection", "central"}, Label: "two regions"}
	canonical, err := p.Canonical()
	if err != nil || canonical != `annotations.superclass in ["central", "visual_projection"]` {
		t.Fatalf("canonical = %q, %v", canonical, err)
	}
	reordered := p
	reordered.Any = []string{"central", "visual_projection"}
	a, _ := p.Hash()
	b, _ := reordered.Hash()
	if a != b {
		t.Fatal("value order changed the predicate hash")
	}
	single := SelectionPredicate{Source: RoleAnnotations, Field: "superclass", Equals: "central", Label: "x"}
	c, _ := single.Hash()
	if c == a {
		t.Fatal("a union hashed the same as one of its values")
	}
	for value, want := range map[string]bool{"central": true, "visual_projection": true, "optic": false, "": false} {
		if p.Matches(value) != want {
			t.Fatalf("Matches(%q) = %v, want %v", value, !want, want)
		}
	}
	if !single.Matches("central") || single.Matches("visual_projection") {
		t.Fatal("an equals predicate matched the wrong values")
	}
	raw, err := json.Marshal(single)
	if err != nil || strings.Contains(string(raw), `"any"`) {
		t.Fatalf("equals predicate JSON = %s, %v", raw, err)
	}
	for name, bad := range map[string]SelectionPredicate{
		"both":      {Source: RoleAnnotations, Field: "f", Equals: "a", Any: []string{"b", "c"}, Label: "x"},
		"one value": {Source: RoleAnnotations, Field: "f", Any: []string{"a"}, Label: "x"},
		"empty":     {Source: RoleAnnotations, Field: "f", Any: []string{"a", ""}, Label: "x"},
		"repeat":    {Source: RoleAnnotations, Field: "f", Any: []string{"a", "a"}, Label: "x"},
	} {
		if _, err := bad.Canonical(); !errors.Is(err, ErrInvalidSelection) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestExternalIDsAreLosslessAndBounded(t *testing.T) {
	id, err := ParseExternalID(testNamespace(), "9007199254740993")
	if err != nil || id.Value != 9007199254740993 || id.NeuronID().ExternalID != "9007199254740993" {
		t.Fatalf("parse = %#v, %v", id, err)
	}
	max, err := ParseExternalID(testNamespace(), "18446744073709551615")
	if err != nil || max.Value != math.MaxUint64 || max.NeuronID().ExternalID != "18446744073709551615" {
		t.Fatalf("max parse = %#v, %v", max, err)
	}
	for _, bad := range []string{"", " 1", "01", "-1", "+1", "1.0", "18446744073709551616", "0x10", "abc"} {
		if _, err := ParseExternalID(testNamespace(), bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	if _, err := ParseExternalID("", "1"); err == nil {
		t.Fatal("accepted empty namespace")
	}
	zero, err := ParseExternalID(testNamespace(), "0")
	if err != nil || zero.Value != 0 {
		t.Fatalf("zero = %#v, %v", zero, err)
	}
	keyA := CanonicalKey(testNamespace(), 9)
	keyB := CanonicalKey(testNamespace(), 10)
	if !(keyA < keyB) || !strings.HasPrefix(keyA, testNamespace()+"/") || !strings.HasSuffix(keyA, "00000000000000000009") {
		t.Fatalf("canonical keys %q %q are not numerically ordered", keyA, keyB)
	}
	if _, err := signalID(signal.NeuronID{Namespace: testNamespace(), ExternalID: "12"}, testNamespace()); err != nil {
		t.Fatal(err)
	}
	if _, err := signalID(signal.NeuronID{Namespace: "other", ExternalID: "12"}, testNamespace()); err == nil {
		t.Fatal("foreign namespace accepted")
	}
}

func TestDecodeManifestBoundsNestingDepthWithoutQuadraticWork(t *testing.T) {
	nested := strings.Repeat("[", 200000) + strings.Repeat("]", 200000)
	start := time.Now()
	_, err := DecodeManifest(strings.NewReader(nested))
	if err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("deeply nested JSON: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("deep nesting took %v", elapsed)
	}
	object := strings.Repeat(`{"k":`, 100) + "1" + strings.Repeat("}", 100)
	if _, err := DecodeManifest(strings.NewReader(object)); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("nested objects beyond the limit: %v", err)
	}
	shallow := strings.Repeat(`{"k":`, 10) + "1" + strings.Repeat("}", 10)
	if _, err := DecodeManifest(strings.NewReader(shallow)); err == nil || strings.Contains(err.Error(), "nesting") {
		t.Fatalf("shallow nesting must fail validation, not the depth guard: %v", err)
	}
	duplicate := `{"schema_version":"x","Schema_Version":"y"}`
	if _, err := DecodeManifest(strings.NewReader(duplicate)); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("case-folded duplicate key: %v", err)
	}
}
