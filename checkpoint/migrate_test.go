package checkpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMigrateRefusesSamePathAndExistingTarget(t *testing.T) {
	individual := newCheckpointIndividual(t, false)
	dir := t.TempDir()
	src := filepath.Join(dir, "source.json")
	if err := SaveIndividual(context.Background(), src, individual.Snapshot()); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Migrate(context.Background(), src, src, IndividualSchemaVersion); err == nil || !strings.Contains(err.Error(), "same") {
		t.Fatalf("same-path Migrate error = %v", err)
	}
	current, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, original) {
		t.Fatal("same-path migration changed the source bytes")
	}
	if entries, err := os.ReadDir(dir); err != nil {
		t.Fatal(err)
	} else if len(entries) != 1 {
		t.Fatalf("same-path migration left %d entries in %s: %v", len(entries), dir, dirNames(entries))
	}

	dst := filepath.Join(dir, "occupied.json")
	writeRaw(t, dst, []byte("occupied"))
	if _, err := Migrate(context.Background(), src, dst, IndividualSchemaVersion); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("existing-target Migrate error = %v", err)
	}
	current, err = os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, original) {
		t.Fatal("existing-target migration changed the source bytes")
	}
	occupied, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(occupied) != "occupied" {
		t.Fatal("existing migration target was modified")
	}
	if entries, err := os.ReadDir(dir); err != nil {
		t.Fatal(err)
	} else if len(entries) != 2 {
		t.Fatalf("existing-target migration left %d entries in %s: %v", len(entries), dir, dirNames(entries))
	}
	assertNoTemps(t, dir, filepath.Base(dst))
}

func TestMigrateSameSchemaIsAByteCopy(t *testing.T) {
	individual := newCheckpointIndividual(t, false)
	dir := t.TempDir()
	src := filepath.Join(dir, "source.json")
	if err := SaveIndividual(context.Background(), src, individual.Snapshot()); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "copy.json")
	report, err := Migrate(context.Background(), src, dst, IndividualSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	srcBytes, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dstBytes, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dstBytes, srcBytes) {
		t.Fatal("same-schema migration is not a byte-for-byte copy")
	}
	if report.SourceSHA256 != report.TargetSHA256 {
		t.Fatalf("source and target SHA-256 differ: %s vs %s", report.SourceSHA256, report.TargetSHA256)
	}
	if report.SourceSHA256 != checksumHex(srcBytes) {
		t.Fatalf("SourceSHA256 = %s, want %s", report.SourceSHA256, checksumHex(srcBytes))
	}
	if report.SourceSchema != IndividualSchemaVersion || report.TargetSchema != IndividualSchemaVersion {
		t.Fatalf("schemas = %q, %q", report.SourceSchema, report.TargetSchema)
	}
	if !report.NoInformationLoss {
		t.Fatal("same-schema migration reported information loss")
	}
	if len(report.InformationLoss) != 0 {
		t.Fatalf("InformationLoss = %v", report.InformationLoss)
	}
	if len(report.FieldChanges) != 0 {
		t.Fatalf("FieldChanges = %+v", report.FieldChanges)
	}
	if _, err := LoadIndividual(context.Background(), dst); err != nil {
		t.Fatalf("migrated copy is not loadable: %v", err)
	}
}

func TestMigrateUpgradesPreUnionIndividual(t *testing.T) {
	individual := newCheckpointIndividual(t, false)
	want := individual.Snapshot()
	dir := t.TempDir()
	saved := filepath.Join(dir, "saved.json")
	if err := SaveIndividual(context.Background(), saved, want); err != nil {
		t.Fatal(err)
	}
	savedBytes, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(dir, "old.json")
	writeRaw(t, src, preUnionIndividualDocument(t, savedBytes))
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "migrated.json")
	report, err := Migrate(context.Background(), src, dst, IndividualSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	if report.SourceSHA256 != checksumHex(before) {
		t.Fatalf("SourceSHA256 = %s, want %s", report.SourceSHA256, checksumHex(before))
	}
	if report.SourceSchema != IndividualSchemaVersion || report.TargetSchema != IndividualSchemaVersion {
		t.Fatalf("schemas = %q, %q", report.SourceSchema, report.TargetSchema)
	}
	if !report.NoInformationLoss {
		t.Fatal("pre-union migration reported information loss")
	}
	if len(report.InformationLoss) != 0 {
		t.Fatalf("InformationLoss = %v", report.InformationLoss)
	}
	if len(report.FieldChanges) != 2 {
		t.Fatalf("FieldChanges = %+v", report.FieldChanges)
	}
	renamed, added := report.FieldChanges[0], report.FieldChanges[1]
	if renamed.Path != "neural" || renamed.Kind != "renamed" {
		t.Fatalf("first FieldChange = %+v", renamed)
	}
	if added.Path != "neural.core" || added.Kind != "added" {
		t.Fatalf("second FieldChange = %+v", added)
	}
	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("pre-union migration changed the source bytes")
	}
	got, err := LoadIndividual(context.Background(), dst)
	if err != nil {
		t.Fatalf("migrated document is not loadable: %v", err)
	}
	if got.Neural.Core == "" {
		t.Fatal("migrated document has no declared neural core")
	}
	if !reflect.DeepEqual(got.Neural.Continuous, want.Neural.Continuous) {
		t.Fatalf("migrated continuous state differs:\n got %+v\nwant %+v", got.Neural.Continuous, want.Neural.Continuous)
	}
}

func TestMigrateRejectsUnknownSchemaAndBrokenSource(t *testing.T) {
	individual := newCheckpointIndividual(t, false)
	payload := mustIndividualJSON(t, individual.Snapshot())
	dir := t.TempDir()

	unknown := filepath.Join(dir, "unknown.json")
	writeRaw(t, unknown, envelopeJSON("coimnet-unknown/v9", payload, checksumHex(payload)))
	unknownDst := filepath.Join(dir, "unknown-dst.json")
	if _, err := Migrate(context.Background(), unknown, unknownDst, IndividualSchemaVersion); err == nil || !strings.Contains(err.Error(), "unsupported migration") {
		t.Fatalf("unknown-schema Migrate error = %v", err)
	}
	if _, err := os.Stat(unknownDst); !os.IsNotExist(err) {
		t.Fatal("unknown-schema migration created its target")
	}

	broken := filepath.Join(dir, "broken.json")
	writeRaw(t, broken, []byte(`{"schema_version":`))
	brokenDst := filepath.Join(dir, "broken-dst.json")
	if _, err := Migrate(context.Background(), broken, brokenDst, IndividualSchemaVersion); err == nil {
		t.Fatal("broken source was accepted")
	}
	if _, err := os.Stat(brokenDst); !os.IsNotExist(err) {
		t.Fatal("broken-source migration created its target")
	}
}

func TestMigrateHonoursContext(t *testing.T) {
	individual := newCheckpointIndividual(t, false)
	dir := t.TempDir()
	src := filepath.Join(dir, "source.json")
	if err := SaveIndividual(context.Background(), src, individual.Snapshot()); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "canceled.json")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Migrate(ctx, src, dst, IndividualSchemaVersion); !errors.Is(err, context.Canceled) {
		t.Fatalf("Migrate cancellation error = %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("cancelled migration created its target")
	}
	assertNoTemps(t, dir, filepath.Base(dst))
}

// preUnionIndividualDocument rewrites a saved union document into the shape the
// released code wrote before the neural union existed: the continuous dynamics
// state sits directly on payload.neural and there is no "core" member.
func preUnionIndividualDocument(t *testing.T, saved []byte) []byte {
	t.Helper()
	var raw testEnvelope
	if err := json.Unmarshal(saved, &raw); err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	var neural map[string]json.RawMessage
	if err := json.Unmarshal(payload["neural"], &neural); err != nil {
		t.Fatal(err)
	}
	continuous, ok := neural["continuous"]
	if !ok {
		t.Fatal("saved document carries no continuous half to extract")
	}
	if _, ok := neural["core"]; !ok {
		t.Fatal("saved document is not a union")
	}
	payload["neural"] = continuous
	prePayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	document, err := marshalEnvelopeFor(IndividualSchemaVersion, prePayload)
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func dirNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}
