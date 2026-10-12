package checkpoint

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

func locationCheckpointConfig() modulation.ChemistryConfig {
	c := chemicalCheckpointConfig()
	c.Receptors.Records[0].Location = &modulation.ReceptorLocation{
		Region: 1, Compartment: "postsynaptic", AnatomicalLocation: "fixture neuropil",
		Evidence: "checkpoint location fixture", MeasurementKind: "receptor imaging", MappingVersion: "checkpoint-location/v1",
	}
	c.Receptors.Records[1].Location = &modulation.ReceptorLocation{
		Region: 0, Compartment: "presynaptic", AnatomicalLocation: "fixture axon terminal",
		Evidence: "checkpoint location fixture", MeasurementKind: "receptor imaging", MappingVersion: "checkpoint-location/v1",
	}
	return c
}

func newLocationCheckpointIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	individual := newCheckpointLIFIndividual(t)
	if err := individual.EnableChemistry(locationCheckpointConfig()); err != nil {
		t.Fatalf("EnableChemistry() error = %v", err)
	}
	if err := individual.SetResource("energy", 1.25); err != nil {
		t.Fatalf("SetResource() error = %v", err)
	}
	if err := individual.OfferFeedback(chemicalCheckpointFeedback(t)); err != nil {
		t.Fatalf("OfferFeedback() error = %v", err)
	}
	if _, err := individual.Advance(context.Background(), [][]float64{{1}, {0}, {1}}); err != nil {
		t.Fatalf("Advance() error = %v", err)
	}
	return individual
}

func rewriteFirstReceptorLocationPayload(t *testing.T, payload []byte, edit func(map[string]json.RawMessage)) []byte {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var chemical map[string]json.RawMessage
	if err := json.Unmarshal(object["chemical"], &chemical); err != nil {
		t.Fatalf("decode chemical part: %v", err)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(chemical["config"], &config); err != nil {
		t.Fatalf("decode chemical config: %v", err)
	}
	var receptors map[string]json.RawMessage
	if err := json.Unmarshal(config["receptors"], &receptors); err != nil {
		t.Fatalf("decode receptors: %v", err)
	}
	var records []map[string]json.RawMessage
	if err := json.Unmarshal(receptors["records"], &records); err != nil {
		t.Fatalf("decode receptor records: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("location fixture has no receptor records")
	}
	var location map[string]json.RawMessage
	if err := json.Unmarshal(records[0]["location"], &location); err != nil {
		t.Fatalf("decode receptor location: %v", err)
	}
	edit(location)
	encodedLocation, err := json.Marshal(location)
	if err != nil {
		t.Fatalf("encode receptor location: %v", err)
	}
	records[0]["location"] = encodedLocation
	encodedRecords, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("encode receptor records: %v", err)
	}
	receptors["records"] = encodedRecords
	encodedReceptors, err := json.Marshal(receptors)
	if err != nil {
		t.Fatalf("encode receptors: %v", err)
	}
	config["receptors"] = encodedReceptors
	encodedConfig, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("encode chemical config: %v", err)
	}
	chemical["config"] = encodedConfig
	encodedChemical, err := json.Marshal(chemical)
	if err != nil {
		t.Fatalf("encode chemical part: %v", err)
	}
	object["chemical"] = encodedChemical
	changed, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return changed
}

func TestSaveLoadIndividualPreservesReceptorLocationAndOwnsPointers(t *testing.T) {
	individual := newLocationCheckpointIndividual(t)
	want := individual.Snapshot()
	if want.Chemical == nil || want.Chemical.Config.Receptors.Records[0].Location == nil {
		t.Fatal("snapshot lost the enabled receptor location")
	}
	if got := want.Chemical.Config.Receptors.Records[0].Location.Region; got != 1 {
		t.Fatalf("snapshot location region = %d, want 1", got)
	}

	payload := savedIndividualPayload(t, want)
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		t.Fatalf("decode saved payload: %v", err)
	}
	var chemical map[string]json.RawMessage
	if err := json.Unmarshal(object["chemical"], &chemical); err != nil {
		t.Fatalf("decode saved chemical part: %v", err)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(chemical["config"], &config); err != nil {
		t.Fatalf("decode saved chemical config: %v", err)
	}
	var receptors map[string]json.RawMessage
	if err := json.Unmarshal(config["receptors"], &receptors); err != nil {
		t.Fatalf("decode saved receptors: %v", err)
	}
	var records []map[string]json.RawMessage
	if err := json.Unmarshal(receptors["records"], &records); err != nil {
		t.Fatalf("decode saved receptor records: %v", err)
	}
	var location map[string]json.RawMessage
	if err := json.Unmarshal(records[0]["location"], &location); err != nil {
		t.Fatalf("decode saved location: %v", err)
	}
	for _, field := range []string{"region", "compartment", "anatomical_location", "evidence", "measurement_kind", "mapping_version"} {
		if _, ok := location[field]; !ok {
			t.Errorf("saved location omitted required field %q", field)
		}
	}

	path := filepath.Join(t.TempDir(), "location-individual.json")
	if err := SaveIndividual(context.Background(), path, want); err != nil {
		t.Fatalf("SaveIndividual() error = %v", err)
	}
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatalf("LoadIndividual() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed receptor location:\n got %+v\nwant %+v", got.Chemical.Config.Receptors.Records[0].Location, want.Chemical.Config.Receptors.Records[0].Location)
	}
	if got.Chemical.Config.Receptors.Records[0].Location == want.Chemical.Config.Receptors.Records[0].Location {
		t.Fatal("LoadIndividual returned the caller's receptor location pointer")
	}

	snapshotCopy := individual.Snapshot()
	snapshotCopy.Chemical.Config.Receptors.Records[0].Location.Region = 0
	if got := individual.Snapshot().Chemical.Config.Receptors.Records[0].Location.Region; got != 1 {
		t.Fatalf("mutating Snapshot() location changed the individual: region %d", got)
	}

	restored, err := learning.RestoreIndividual(want)
	if err != nil {
		t.Fatalf("RestoreIndividual() error = %v", err)
	}
	restoredCopy := restored.Snapshot()
	restoredCopy.Chemical.Config.Receptors.Records[0].Location.Region = 0
	if got := restored.Snapshot().Chemical.Config.Receptors.Records[0].Location.Region; got != 1 {
		t.Fatalf("mutating restored Snapshot() location changed the restored individual: region %d", got)
	}
}

func TestLoadIndividualRejectsMissingOrOverflowedReceptorLocationFields(t *testing.T) {
	want := newLocationCheckpointIndividual(t).Snapshot()
	payload := savedIndividualPayload(t, want)
	fields := []string{"region", "compartment", "anatomical_location", "evidence", "measurement_kind", "mapping_version"}
	for _, field := range fields {
		t.Run("missing "+field, func(t *testing.T) {
			changed := rewriteFirstReceptorLocationPayload(t, payload, func(location map[string]json.RawMessage) {
				delete(location, field)
			})
			path := filepath.Join(t.TempDir(), "missing-location-field.json")
			writeRaw(t, path, envelopeJSON(IndividualSchemaVersion, changed, checksumHex(changed)))
			if _, err := LoadIndividual(context.Background(), path); err == nil {
				t.Fatalf("LoadIndividual() accepted missing receptor location field %q", field)
			}
		})
	}

	t.Run("region integer overflow", func(t *testing.T) {
		changed := rewriteFirstReceptorLocationPayload(t, payload, func(location map[string]json.RawMessage) {
			location["region"] = json.RawMessage(`9223372036854775808`)
		})
		path := filepath.Join(t.TempDir(), "overflowed-location-region.json")
		writeRaw(t, path, envelopeJSON(IndividualSchemaVersion, changed, checksumHex(changed)))
		if _, err := LoadIndividual(context.Background(), path); err == nil {
			t.Fatal("LoadIndividual() accepted an overflowing receptor location region")
		}
	})
}
