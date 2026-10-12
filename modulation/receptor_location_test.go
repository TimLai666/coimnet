package modulation

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func receptorLocationFixture(region int) Receptor {
	return Receptor{
		Cells: []int{0}, Signal: "octopamine", Channel: 0,
		Status: StatusHypothesized, Kd: 1, N: 1,
		Evidence: "location fixture", MeasurementKind: "receptor imaging", MappingVersion: "location/v1",
		Location: &ReceptorLocation{
			Region: region, Compartment: "dendrite", AnatomicalLocation: "mushroom body",
			Evidence: "location fixture", MeasurementKind: "receptor imaging", MappingVersion: "location/v1",
		},
	}
}

func validLocationConfig(t *testing.T) ChemistryConfig {
	t.Helper()
	c := fixtureConfig(t)
	c.Receptors.Records[0].Location = &ReceptorLocation{
		Region: 0, Compartment: "soma", AnatomicalLocation: "fixture soma",
		Evidence: "location fixture", MeasurementKind: "receptor imaging", MappingVersion: "location/v1",
	}
	c.Receptors.Records[1].Location = &ReceptorLocation{
		Region: 1, Compartment: "axon", AnatomicalLocation: "fixture axon",
		Evidence: "location fixture", MeasurementKind: "receptor imaging", MappingVersion: "location/v1",
	}
	return c
}

func maxLocationRegion() int {
	return int(^uint(0) >> 1)
}

func TestReceptorLocationUsesExplicitRegionForOccupancyAndEffects(t *testing.T) {
	state := ChemistryState{Concentration: [][]float64{{0}, {1}}}
	receptors := &Receptors{Records: []Receptor{receptorLocationFixture(1)}, Mix: MixSum}

	occupancies, summary, err := receptors.Occupancies(state, []int{0})
	if err != nil {
		t.Fatalf("Occupancies() error = %v", err)
	}
	if len(occupancies) != 1 {
		t.Fatalf("Occupancies() returned %d records, want 1", len(occupancies))
	}
	if occupancies[0].Region != 1 {
		t.Fatalf("occupancy region = %d, want explicit receptor region 1", occupancies[0].Region)
	}
	if occupancies[0].Occupancy != 0.5 {
		t.Fatalf("occupancy = %.17g, want 0.5 from concentration == Kd", occupancies[0].Occupancy)
	}
	if summary != (OccupancySummary{}) {
		t.Fatalf("occupancy summary = %+v, want zero summary", summary)
	}

	_, offset, _, clamped, err := ApplyEffects(
		[]Effect{{Kind: EffectSensitivity, Receptor: 0, BetaScale: 2}},
		occupancies, MixSum, 1,
	)
	if err != nil {
		t.Fatalf("ApplyEffects() error = %v", err)
	}
	if len(offset) != 1 || offset[0] != 1 {
		t.Fatalf("effect offset = %v, want [1] from BetaScale 2 * occupancy .5", offset)
	}
	if clamped != (ClampReport{}) {
		t.Fatalf("effect clamp report = %+v, want zero report", clamped)
	}
}

func TestReceptorLocationKeepsReceptorsOnOneCellIndependent(t *testing.T) {
	state := ChemistryState{Concentration: [][]float64{{1}, {2}}}
	first := receptorLocationFixture(0)
	second := receptorLocationFixture(1)
	receptors := &Receptors{Records: []Receptor{first, second}, Mix: MixSum}

	occupancies, _, err := receptors.Occupancies(state, []int{0})
	if err != nil {
		t.Fatalf("Occupancies() error = %v", err)
	}
	if len(occupancies) != 2 {
		t.Fatalf("Occupancies() returned %d records, want 2", len(occupancies))
	}
	want := []struct {
		region    int
		occupancy float64
	}{
		{region: 0, occupancy: 0.5},
		{region: 1, occupancy: 2.0 / 3.0},
	}
	for i, expected := range want {
		if occupancies[i].Region != expected.region {
			t.Errorf("occupancy %d region = %d, want %d", i, occupancies[i].Region, expected.region)
		}
		if math.Abs(occupancies[i].Occupancy-expected.occupancy) > 1e-15 {
			t.Errorf("occupancy %d = %.17g, want %.17g", i, occupancies[i].Occupancy, expected.occupancy)
		}
	}

	_, offset, _, _, err := ApplyEffects([]Effect{
		{Kind: EffectSensitivity, Receptor: 0, BetaScale: 2, BetaAbsMax: 4},
		{Kind: EffectSensitivity, Receptor: 1, BetaScale: 2, BetaAbsMax: 4},
	}, occupancies, MixSum, 1)
	if err != nil {
		t.Fatalf("ApplyEffects() error = %v", err)
	}
	wantOffset := 2 * (0.5 + 2.0/3.0)
	if math.Abs(offset[0]-wantOffset) > 1e-15 {
		t.Fatalf("combined offset = %.17g, want %.17g", offset[0], wantOffset)
	}

	legacy := first
	legacy.Location = nil
	legacyOccupancies, _, err := (&Receptors{Records: []Receptor{legacy}}).Occupancies(state, []int{1})
	if err != nil {
		t.Fatalf("legacy nil-location Occupancies() error = %v", err)
	}
	if len(legacyOccupancies) != 1 || math.Abs(legacyOccupancies[0].Occupancy-2.0/3.0) > 1e-15 {
		t.Fatalf("nil location occupancy = %+v, want legacy node-region concentration 2", legacyOccupancies)
	}
}

func TestReceptorLocationPreservesUnknownAndUnresponsiveStatuses(t *testing.T) {
	unknown := receptorLocationFixture(1)
	unknown.Status = StatusUnknown
	unresponsive := receptorLocationFixture(1)
	unresponsive.Status = StatusUnresponsive
	receptors := &Receptors{Records: []Receptor{unknown, unresponsive}, Mix: MixSum}

	got, summary, err := receptors.Occupancies(ChemistryState{Concentration: [][]float64{{0}, {1}}}, []int{0})
	if err != nil {
		t.Fatalf("Occupancies() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Occupancies() returned %d records, want 2", len(got))
	}
	if got[0].Status != StatusUnknown || got[0].Occupancy != 0 || !got[0].Skipped || got[0].Assumed {
		t.Fatalf("unknown receptor changed semantics: %+v", got[0])
	}
	if got[1].Status != StatusUnresponsive || got[1].Occupancy != 0 || got[1].Skipped || got[1].Assumed {
		t.Fatalf("unresponsive receptor changed semantics: %+v", got[1])
	}
	if summary != (OccupancySummary{UnknownSkipped: 1, Unresponsive: 1}) {
		t.Fatalf("summary = %+v, want one skipped and one unresponsive record", summary)
	}
}

func TestReceptorLocationValidationRejectsInvalidFieldsAndBounds(t *testing.T) {
	compartments := []string{"unknown", "soma", "dendrite", "axon", "presynaptic", "postsynaptic"}
	for _, compartment := range compartments {
		t.Run("accepts "+compartment, func(t *testing.T) {
			c := validLocationConfig(t)
			c.Receptors.Records[0].Location.Compartment = compartment
			if err := c.Validate(3); err != nil {
				t.Fatalf("Validate() rejected compartment %q: %v", compartment, err)
			}
		})
	}

	for _, tc := range []struct {
		name string
		edit func(*ReceptorLocation)
	}{
		{name: "unsupported compartment", edit: func(location *ReceptorLocation) { location.Compartment = "cell_body" }},
		{name: "missing evidence", edit: func(location *ReceptorLocation) { location.Evidence = "" }},
		{name: "missing measurement kind", edit: func(location *ReceptorLocation) { location.MeasurementKind = "" }},
		{name: "missing mapping version", edit: func(location *ReceptorLocation) { location.MappingVersion = "" }},
		{name: "missing anatomical location", edit: func(location *ReceptorLocation) { location.AnatomicalLocation = "" }},
		{name: "whitespace evidence", edit: func(location *ReceptorLocation) { location.Evidence = " \t\n" }},
		{name: "whitespace measurement kind", edit: func(location *ReceptorLocation) { location.MeasurementKind = " \t\n" }},
		{name: "whitespace mapping version", edit: func(location *ReceptorLocation) { location.MappingVersion = " \t\n" }},
		{name: "whitespace anatomical location", edit: func(location *ReceptorLocation) { location.AnatomicalLocation = " \t\n" }},
		{name: "negative region", edit: func(location *ReceptorLocation) { location.Region = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := validLocationConfig(t)
			tc.edit(c.Receptors.Records[0].Location)
			if err := c.Validate(3); err == nil {
				t.Fatalf("Validate() accepted %s", tc.name)
			}
		})
	}
	// Receptors.Validate has no declared region count, so it can only reject
	// negative regions. ChemistryConfig.Validate owns the upper-bound check.
	high := receptorLocationFixture(maxLocationRegion())
	if err := (&Receptors{Records: []Receptor{high}}).Validate(1, 1); err != nil {
		t.Fatalf("Receptors.Validate() added an unsupported upper-bound check: %v", err)
	}
	for _, tc := range []struct {
		name   string
		region int
	}{
		{name: "declared region count", region: 2},
		{name: "integer maximum", region: maxLocationRegion()},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			c := validLocationConfig(t)
			c.Receptors.Records[0].Location.Region = tc.region
			if err := c.Validate(3); err == nil {
				t.Fatalf("Validate() accepted location region %d", tc.region)
			}
		})
	}

	var location ReceptorLocation
	if err := json.Unmarshal([]byte(`{"region":9223372036854775808,"compartment":"soma","anatomical_location":"x","evidence":"e","measurement_kind":"m","mapping_version":"v"}`), &location); err == nil {
		t.Fatal("JSON integer overflow was accepted for ReceptorLocation.Region")
	}
}

func TestReceptorLocationOccupanciesRejectsInvalidStateWithEmptyResult(t *testing.T) {
	receptors := &Receptors{Records: []Receptor{receptorLocationFixture(1)}, Mix: MixSum}
	for _, tc := range []struct {
		name  string
		state ChemistryState
	}{
		{name: "no concentration rows", state: ChemistryState{}},
		{name: "location region outside state", state: ChemistryState{Concentration: [][]float64{{0}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, summary, err := receptors.Occupancies(tc.state, []int{0})
			if err == nil {
				t.Fatalf("Occupancies() accepted %s", tc.name)
			}
			if got != nil {
				t.Fatalf("Occupancies() returned records on error: %+v", got)
			}
			if summary != (OccupancySummary{}) {
				t.Fatalf("Occupancies() summary on error = %+v, want zero summary", summary)
			}
		})
	}
}

func TestReceptorLocationNilKeepsLegacyJSONAndOptionalPointer(t *testing.T) {
	record := Receptor{
		Cells: []int{0}, Signal: "octopamine", Channel: 0,
		Status: StatusHypothesized, Kd: 1, N: 1,
		Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "v1",
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want := `{"cells":[0],"signal":"octopamine","channel":0,"status":"hypothesized","engineering_kd":1,"engineering_n":1,"evidence":"fixture","measurement_kind":"declared","mapping_version":"v1"}`
	if string(encoded) != want {
		t.Fatalf("nil-location JSON changed:\n got  %s\n want %s", encoded, want)
	}
	var decoded Receptor
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.Location != nil {
		t.Fatalf("legacy JSON decoded a non-nil location: %+v", decoded.Location)
	}
}

func TestChemistryConfigCloneOwnsReceptorLocationPointers(t *testing.T) {
	original := validLocationConfig(t)
	clone := original.Clone()
	if len(clone.Receptors.Records) != len(original.Receptors.Records) {
		t.Fatalf("Clone() changed receptor count: got %d, want %d", len(clone.Receptors.Records), len(original.Receptors.Records))
	}
	for i := range original.Receptors.Records {
		want, got := original.Receptors.Records[i].Location, clone.Receptors.Records[i].Location
		if want == nil || got == nil {
			t.Fatalf("receptor %d location lost across Clone(): original=%+v clone=%+v", i, want, got)
		}
		if got == want {
			t.Fatalf("receptor %d Clone() shares its location pointer", i)
		}
	}
	clone.Receptors.Records[0].Location.Region = 1
	clone.Receptors.Records[0].Location.Evidence = "clone only"
	if original.Receptors.Records[0].Location.Region != 0 || original.Receptors.Records[0].Location.Evidence != "location fixture" {
		t.Fatalf("mutating cloned location changed original: %+v", original.Receptors.Records[0].Location)
	}
	original.Receptors.Records[1].Location.Region = 0
	if clone.Receptors.Records[1].Location.Region != 1 {
		t.Fatalf("mutating original location changed clone: %+v", clone.Receptors.Records[1].Location)
	}
}

func TestReceptorLocationJSONIncludesEveryRequiredFieldWhenPresent(t *testing.T) {
	encoded, err := json.Marshal(receptorLocationFixture(1))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	var location map[string]json.RawMessage
	if err := json.Unmarshal(object["location"], &location); err != nil {
		t.Fatalf("location is not an object: %v", err)
	}
	for _, field := range []string{"region", "compartment", "anatomical_location", "evidence", "measurement_kind", "mapping_version"} {
		if _, ok := location[field]; !ok {
			t.Errorf("location JSON omitted required field %q", field)
		}
	}
	for field, value := range location {
		if strings.TrimSpace(string(value)) == "null" {
			t.Errorf("location JSON wrote null for %q", field)
		}
	}
}
