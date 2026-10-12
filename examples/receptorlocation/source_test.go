package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
)

// The spreadsheet is the source artifact named by sourceSHA256. Keeping this
// check independent of the CSV transcription prevents a plausible-looking
// transcription from becoming the only source validation.
//
//go:embed testdata/elife-62953-fig3-data1-v3.xlsx
var receptorLocationSourceXLSX []byte

const receptorLocationCSVHeader = "source_row,sample_id,cell_type,reported_marker,subtype_group,puncta_count"

func TestObservationsFromCSVReadsTheEmbeddedSourceTranscription(t *testing.T) {
	observations, err := observationsFromCSV(bytes.NewReader(measuredCSV))
	if err != nil {
		t.Fatalf("observationsFromCSV() error = %v", err)
	}
	if len(observations) != 28 {
		t.Fatalf("observationsFromCSV() returned %d rows, want 28", len(observations))
	}

	var t4Count, t4Puncta, t5Count, t5Puncta int
	for _, observation := range observations {
		switch observation.CellType {
		case "T4":
			t4Count++
			t4Puncta += observation.Puncta
		case "T5":
			t5Count++
			t5Puncta += observation.Puncta
		default:
			t.Fatalf("unsupported parsed cell type %q", observation.CellType)
		}
	}
	if t4Count != 18 || t4Puncta != 728 {
		t.Fatalf("T4 observations = %d rows, %d puncta; want 18 rows, 728 puncta", t4Count, t4Puncta)
	}
	if t5Count != 10 || t5Puncta != 422 {
		t.Fatalf("T5 observations = %d rows, %d puncta; want 10 rows, 422 puncta", t5Count, t5Puncta)
	}

	wantFirst := observation{SourceRow: 2, SampleID: "T4ab_c1", CellType: "T4", Marker: "RDL", Subtype: "ab", Puncta: 32}
	wantLast := observation{SourceRow: 30, SampleID: "T5cd_c8", CellType: "T5", Marker: "RDL", Subtype: "cd", Puncta: 44}
	if !reflect.DeepEqual(observations[0], wantFirst) {
		t.Fatalf("first observation = %+v, want %+v", observations[0], wantFirst)
	}
	if !reflect.DeepEqual(observations[len(observations)-1], wantLast) {
		t.Fatalf("last observation = %+v, want %+v", observations[len(observations)-1], wantLast)
	}
}
func TestReceptorLocationSourceSHA256MatchesTheOriginalXLSX(t *testing.T) {
	digest := sha256.Sum256(receptorLocationSourceXLSX)
	if got := hex.EncodeToString(digest[:]); got != sourceSHA256 {
		t.Fatalf("source XLSX SHA-256 = %s, want %s", got, sourceSHA256)
	}
}

func TestObservationsFromCSVRejectsMalformedOrUnsupportedRowsWithNoPartialResult(t *testing.T) {
	validRow := "2,T4ab_c1,T4,RDL,ab,32"
	cases := []struct {
		name string
		data string
	}{
		{name: "empty input", data: ""},
		{name: "header only", data: receptorLocationCSVHeader + "\n"},
		{name: "missing column in header", data: "source_row,sample_id,cell_type,reported_marker,subtype_group\n" + validRow},
		{name: "unknown column in header", data: receptorLocationCSVHeader + ",extra\n" + validRow + ",ignored"},
		{name: "reordered header", data: "sample_id,source_row,cell_type,reported_marker,subtype_group,puncta_count\nT4ab_c1,2,T4,RDL,ab,32"},
		{name: "wrong header spelling", data: "source_row,sample_id,cell_type,reported_marker,subtype,puncta_count\n" + validRow},
		{name: "unsupported cell type", data: receptorLocationCSVHeader + "\n2,T4ab_c1,T6,RDL,ab,32"},
		{name: "unsupported marker", data: receptorLocationCSVHeader + "\n2,T4ab_c1,T4,GABA,ab,32"},
		{name: "unsupported subtype", data: receptorLocationCSVHeader + "\n2,T4ab_c1,T4,RDL,ef,32"},
		{name: "source row below two", data: receptorLocationCSVHeader + "\n1,T4ab_c1,T4,RDL,ab,32"},
		{name: "duplicate source row", data: receptorLocationCSVHeader + "\n2,T4ab_c1,T4,RDL,ab,32\n2,T4ab_c2,T4,RDL,ab,33"},
		{name: "duplicate sample ID", data: receptorLocationCSVHeader + "\n2,T4ab_c1,T4,RDL,ab,32\n3,T4ab_c1,T4,RDL,ab,33"},
		{name: "negative puncta", data: receptorLocationCSVHeader + "\n2,T4ab_c1,T4,RDL,ab,-1"},
		{name: "non-integer puncta", data: receptorLocationCSVHeader + "\n2,T4ab_c1,T4,RDL,ab,1.5"},
		{name: "puncta integer overflow", data: receptorLocationCSVHeader + "\n2,T4ab_c1,T4,RDL,ab,999999999999999999999999999999999999"},
		{name: "malformed CSV", data: receptorLocationCSVHeader + "\n2,T4ab_c1,T4,\"RDL,ab,32"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observations, err := observationsFromCSV(strings.NewReader(tc.data))
			if err == nil {
				t.Fatalf("observationsFromCSV() accepted malformed input and returned %+v", observations)
			}
			if observations != nil {
				t.Fatalf("observationsFromCSV() returned partial rows on error: %+v", observations)
			}
		})
	}
}
func TestObservationsFromCSVRejectsNilReaderAndEmptyFields(t *testing.T) {
	observations, err := observationsFromCSV(nil)
	if err == nil {
		t.Fatalf("observationsFromCSV(nil) returned nil error and %+v", observations)
	}
	if observations != nil {
		t.Fatalf("observationsFromCSV(nil) returned rows: %+v", observations)
	}

	fields := []string{"2", "T4ab_c1", "T4", "RDL", "ab", "32"}
	fieldNames := []string{"source_row", "sample_id", "cell_type", "reported_marker", "subtype_group", "puncta_count"}
	for i, fieldName := range fieldNames {
		t.Run("empty "+fieldName, func(t *testing.T) {
			row := append([]string(nil), fields...)
			row[i] = ""
			data := receptorLocationCSVHeader + "\n" + strings.Join(row, ",")
			got, err := observationsFromCSV(strings.NewReader(data))
			if err == nil {
				t.Fatalf("observationsFromCSV() accepted an empty %s and returned %+v", fieldName, got)
			}
			if got != nil {
				t.Fatalf("observationsFromCSV() returned rows for empty %s: %+v", fieldName, got)
			}
		})
	}
}

func TestObservationsFromCSVRejectsNilReaderBeforeReading(t *testing.T) {
	var reader *bytes.Reader
	observations, err := observationsFromCSV(reader)
	if err == nil {
		t.Fatalf("observationsFromCSV(typed nil reader) returned nil error and %+v", observations)
	}
	if observations != nil {
		t.Fatalf("observationsFromCSV(typed nil reader) returned rows: %+v", observations)
	}
}

func sourceTestObservations() []observation {
	return []observation{
		{SourceRow: 2, SampleID: "T4ab_c1", CellType: "T4", Marker: "RDL", Subtype: "ab", Puncta: 32},
		{SourceRow: 3, SampleID: "T5cd_c1", CellType: "T5", Marker: "RDL", Subtype: "cd", Puncta: 43},
	}
}

func sourceTestNode(index uint64, externalID, cellType string) connectome.NodeRecord {
	return connectome.NodeRecord{
		Index: index,
		ID:    signal.NeuronID{Namespace: "malecns-v1.0", ExternalID: externalID},
		Type:  connectome.NullString{Valid: true, Value: cellType},
	}
}

func sourceTestNodes() []connectome.NodeRecord {
	return []connectome.NodeRecord{
		sourceTestNode(0, "body-4001", "T4a"),
		sourceTestNode(1, "body-4002", "T4b"),
		sourceTestNode(2, "body-4003", "T4_unclear"),
		sourceTestNode(3, "body-5001", "T5c"),
		sourceTestNode(4, "body-5002", "T5d"),
		sourceTestNode(5, "body-9001", "OL"),
	}
}

func TestMapObservationsInfersOnlySupportedMaleCNSTypesAndKeepsSourceIDs(t *testing.T) {
	observations := sourceTestObservations()
	originalObservations := append([]observation(nil), observations...)
	nodes := sourceTestNodes()
	originalNodes := append([]connectome.NodeRecord(nil), nodes...)

	receptors, report, err := mapObservations(observations, nodes, 8)
	if err != nil {
		t.Fatalf("mapObservations() error = %v", err)
	}
	if !reflect.DeepEqual(observations, originalObservations) {
		t.Fatalf("mapObservations() mutated observations:\n got %+v\nwant %+v", observations, originalObservations)
	}
	if !reflect.DeepEqual(nodes, originalNodes) {
		t.Fatalf("mapObservations() mutated nodes:\n got %+v\nwant %+v", nodes, originalNodes)
	}

	if report.Kind != "cell_type_inferred" {
		t.Fatalf("mapping report kind = %q, want cell_type_inferred", report.Kind)
	}
	if report.Version != mappingVersion {
		t.Fatalf("mapping report version = %q, want %q", report.Version, mappingVersion)
	}
	if report.Mapped != 4 {
		t.Fatalf("mapping report mapped = %d, want 4 matched nodes", report.Mapped)
	}
	if report.ExcludedUnclear != 1 {
		t.Fatalf("mapping report excluded_unclear = %d, want 1", report.ExcludedUnclear)
	}
	if len(report.Examples) == 0 || len(report.Examples) > 8 {
		t.Fatalf("mapping report examples length = %d, want 1..8", len(report.Examples))
	}

	allowedExamples := map[string]struct{}{
		"body-4001": {}, "body-4002": {}, "body-5001": {}, "body-5002": {},
	}
	haveExampleT4, haveExampleT5 := false, false
	for _, example := range report.Examples {
		if example.ID.Namespace != "malecns-v1.0" {
			t.Errorf("example %q has namespace %q, want malecns-v1.0", example.ID.ExternalID, example.ID.Namespace)
		}
		if _, ok := allowedExamples[example.ID.ExternalID]; !ok {
			t.Errorf("example ID %q is not an original matched body ID", example.ID.ExternalID)
		}
		if example.Type == "T4a" || example.Type == "T4b" {
			haveExampleT4 = true
		}
		if example.Type == "T5c" || example.Type == "T5d" {
			haveExampleT5 = true
		}
		if example.Type == "T4_unclear" || example.Type == "OL" {
			t.Errorf("example includes an excluded node type %q", example.Type)
		}
	}
	if !haveExampleT4 || !haveExampleT5 {
		t.Fatalf("mapping examples must show both observed types: T4=%v T5=%v", haveExampleT4, haveExampleT5)
	}

	if len(receptors.Records) != 2 {
		t.Fatalf("mapped receptor record count = %d, want one record for T4 and one for T5", len(receptors.Records))
	}
	wantCells := map[string][]int{"T4": []int{0, 1}, "T5": []int{3, 4}}
	seenTypes := make(map[string]bool, len(receptors.Records))
	for _, receptor := range receptors.Records {
		if seenTypes[receptor.CellType] {
			t.Fatalf("mapped receptor type %q appears more than once", receptor.CellType)
		}
		seenTypes[receptor.CellType] = true
		if !reflect.DeepEqual(receptor.Cells, wantCells[receptor.CellType]) {
			t.Errorf("%s receptor cells = %v, want %v", receptor.CellType, receptor.Cells, wantCells[receptor.CellType])
		}
		if receptor.Signal != "GABA" {
			t.Errorf("%s receptor signal = %q, want GABA", receptor.CellType, receptor.Signal)
		}
		if receptor.Status != modulation.StatusUnknown {
			t.Errorf("%s receptor status = %q, want unknown", receptor.CellType, receptor.Status)
		}
		if receptor.Kd != 0 || receptor.N != 0 {
			t.Errorf("%s receptor coefficients = Kd %v N %v, want zero unknown coefficients", receptor.CellType, receptor.Kd, receptor.N)
		}
		if strings.TrimSpace(receptor.Evidence) == "" || !strings.Contains(receptor.Evidence, sourceURL) {
			t.Errorf("%s receptor evidence = %q, want source URL", receptor.CellType, receptor.Evidence)
		}
		if strings.TrimSpace(receptor.MeasurementKind) == "" {
			t.Errorf("%s receptor has no measurement kind", receptor.CellType)
		}
		if receptor.MappingVersion != mappingVersion {
			t.Errorf("%s receptor mapping version = %q, want %q", receptor.CellType, receptor.MappingVersion, mappingVersion)
		}
		if receptor.Location == nil {
			t.Fatalf("%s receptor has nil location", receptor.CellType)
		}
		if receptor.Location.Region != 1 || receptor.Location.Compartment != "dendrite" {
			t.Errorf("%s receptor location = %+v, want region 1 dendrite", receptor.CellType, *receptor.Location)
		}
		if strings.TrimSpace(receptor.Location.AnatomicalLocation) == "" {
			t.Errorf("%s receptor has empty anatomical location", receptor.CellType)
		}
		if strings.TrimSpace(receptor.Location.Evidence) == "" || !strings.Contains(receptor.Location.Evidence, sourceURL) {
			t.Errorf("%s receptor location evidence = %q, want source URL", receptor.CellType, receptor.Location.Evidence)
		}
		if strings.TrimSpace(receptor.Location.MeasurementKind) == "" {
			t.Errorf("%s receptor location has no measurement kind", receptor.CellType)
		}
		if receptor.Location.MappingVersion != mappingVersion {
			t.Errorf("%s receptor location mapping version = %q, want %q", receptor.CellType, receptor.Location.MappingVersion, mappingVersion)
		}
	}
	if !seenTypes["T4"] || !seenTypes["T5"] {
		t.Fatalf("mapped receptor types = %v, want T4 and T5", seenTypes)
	}
}

func TestMapObservationsAcceptsAOneTypeGraphWithoutInventingAnEmptyRecord(t *testing.T) {
	cases := []struct {
		name      string
		nodes     []connectome.NodeRecord
		wantType  string
		wantCells []int
	}{
		{
			name: "T4 only",
			nodes: []connectome.NodeRecord{
				sourceTestNode(0, "body-t4a-1", "T4a"),
				sourceTestNode(1, "body-t4b-1", "T4b"),
			},
			wantType: "T4", wantCells: []int{0, 1},
		},
		{
			name: "T5 only",
			nodes: []connectome.NodeRecord{
				sourceTestNode(0, "body-t5a-1", "T5a"),
				sourceTestNode(1, "body-t5b-1", "T5b"),
			},
			wantType: "T5", wantCells: []int{0, 1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observations := sourceTestObservations()
			seenSourceTypes := make(map[string]bool, len(observations))
			for _, observation := range observations {
				seenSourceTypes[observation.CellType] = true
			}
			if !seenSourceTypes["T4"] || !seenSourceTypes["T5"] {
				t.Fatalf("test source observations must contain both T4 and T5, got %v", seenSourceTypes)
			}

			receptors, report, err := mapObservations(observations, tc.nodes, 2)
			if err != nil {
				t.Fatalf("mapObservations() error = %v", err)
			}
			if report.Mapped != 2 {
				t.Fatalf("mapping report mapped = %d, want 2", report.Mapped)
			}
			if len(receptors.Records) != 1 {
				t.Fatalf("mapped receptor records = %d, want one record for the observed graph type", len(receptors.Records))
			}
			receptor := receptors.Records[0]
			if receptor.CellType != tc.wantType {
				t.Fatalf("mapped receptor type = %q, want %q", receptor.CellType, tc.wantType)
			}
			if !reflect.DeepEqual(receptor.Cells, tc.wantCells) {
				t.Fatalf("mapped %s cells = %v, want %v", receptor.CellType, receptor.Cells, tc.wantCells)
			}
			if len(receptor.Cells) == 0 {
				t.Fatal("mapped receptor has empty Cells")
			}
		})
	}
}

func TestMapObservationsReportsEachExactSubtypeAtMostOnce(t *testing.T) {
	nodes := []connectome.NodeRecord{
		sourceTestNode(0, "body-t4a-1", "T4a"),
		sourceTestNode(1, "body-t4a-2", "T4a"),
		sourceTestNode(2, "body-t4b-1", "T4b"),
		sourceTestNode(3, "body-t4b-2", "T4b"),
		sourceTestNode(4, "body-t4c-1", "T4c"),
		sourceTestNode(5, "body-t4c-2", "T4c"),
		sourceTestNode(6, "body-t4d-1", "T4d"),
		sourceTestNode(7, "body-t4d-2", "T4d"),
		sourceTestNode(8, "body-t5a-1", "T5a"),
		sourceTestNode(9, "body-t5a-2", "T5a"),
		sourceTestNode(10, "body-t5b-1", "T5b"),
		sourceTestNode(11, "body-t5b-2", "T5b"),
		sourceTestNode(12, "body-t5c-1", "T5c"),
		sourceTestNode(13, "body-t5c-2", "T5c"),
		sourceTestNode(14, "body-t5d-1", "T5d"),
		sourceTestNode(15, "body-t5d-2", "T5d"),
	}
	wantTypes := map[string]map[string]struct{}{
		"T4a": map[string]struct{}{"body-t4a-1": struct{}{}, "body-t4a-2": struct{}{}},
		"T4b": map[string]struct{}{"body-t4b-1": struct{}{}, "body-t4b-2": struct{}{}},
		"T4c": map[string]struct{}{"body-t4c-1": struct{}{}, "body-t4c-2": struct{}{}},
		"T4d": map[string]struct{}{"body-t4d-1": struct{}{}, "body-t4d-2": struct{}{}},
		"T5a": map[string]struct{}{"body-t5a-1": struct{}{}, "body-t5a-2": struct{}{}},
		"T5b": map[string]struct{}{"body-t5b-1": struct{}{}, "body-t5b-2": struct{}{}},
		"T5c": map[string]struct{}{"body-t5c-1": struct{}{}, "body-t5c-2": struct{}{}},
		"T5d": map[string]struct{}{"body-t5d-1": struct{}{}, "body-t5d-2": struct{}{}},
	}

	receptors, report, err := mapObservations(sourceTestObservations(), nodes, len(nodes))
	if err != nil {
		t.Fatalf("mapObservations() error = %v", err)
	}
	if report.Mapped != 16 {
		t.Fatalf("mapping report mapped = %d, want 16", report.Mapped)
	}
	if len(report.Examples) != 8 {
		t.Fatalf("mapping report examples = %d, want exactly 8", len(report.Examples))
	}

	seenExampleTypes := make(map[string]bool, len(report.Examples))
	for _, example := range report.Examples {
		if seenExampleTypes[example.Type] {
			t.Errorf("mapping report contains more than one example for exact subtype %q", example.Type)
		}
		seenExampleTypes[example.Type] = true
		allowedIDs, ok := wantTypes[example.Type]
		if !ok {
			t.Errorf("mapping report example has unexpected exact subtype %q", example.Type)
			continue
		}
		if example.ID.Namespace != "malecns-v1.0" {
			t.Errorf("example %q namespace = %q, want malecns-v1.0", example.ID.ExternalID, example.ID.Namespace)
		}
		if _, ok := allowedIDs[example.ID.ExternalID]; !ok {
			t.Errorf("example %s/%s is not an original body ID for subtype %s", example.ID.Namespace, example.ID.ExternalID, example.Type)
		}
		if example.Index < 0 || example.Index >= len(nodes) {
			t.Errorf("example %s index = %d, outside node fixture", example.ID.ExternalID, example.Index)
		}
	}
	if len(seenExampleTypes) != len(wantTypes) {
		t.Errorf("mapping report example subtypes = %v, want one each of %v", seenExampleTypes, wantTypes)
	}

	wantCells := map[string][]int{
		"T4": []int{0, 1, 2, 3, 4, 5, 6, 7},
		"T5": []int{8, 9, 10, 11, 12, 13, 14, 15},
	}
	if len(receptors.Records) != 2 {
		t.Fatalf("mapped receptor records = %d, want T4 and T5", len(receptors.Records))
	}
	for _, receptor := range receptors.Records {
		if !reflect.DeepEqual(receptor.Cells, wantCells[receptor.CellType]) {
			t.Errorf("mapped %s cells = %v, want %v", receptor.CellType, receptor.Cells, wantCells[receptor.CellType])
		}
	}
}

func TestMapObservationsRejectsEmptyOrInvalidInputsWithZeroOutputs(t *testing.T) {
	baseObservations := sourceTestObservations()
	baseNodes := sourceTestNodes()
	cases := []struct {
		name         string
		observations []observation
		nodes        []connectome.NodeRecord
		totalNodes   int
	}{
		{name: "nil observations", observations: nil, nodes: baseNodes, totalNodes: 8},
		{name: "empty observations", observations: []observation{}, nodes: baseNodes, totalNodes: 8},
		{name: "only T4 source", observations: []observation{baseObservations[0]}, nodes: baseNodes, totalNodes: 8},
		{name: "only T5 source", observations: []observation{baseObservations[1]}, nodes: baseNodes, totalNodes: 8},
		{name: "unsupported source type", observations: []observation{{SourceRow: 2, SampleID: "T6_c1", CellType: "T6", Marker: "RDL", Subtype: "ab", Puncta: 1}, baseObservations[1]}, nodes: baseNodes, totalNodes: 8},
		{name: "invalid marker", observations: []observation{{SourceRow: 2, SampleID: "T4_c1", CellType: "T4", Marker: "wrong", Subtype: "ab", Puncta: 1}, baseObservations[1]}, nodes: baseNodes, totalNodes: 8},
		{name: "invalid subtype", observations: []observation{{SourceRow: 2, SampleID: "T4_c1", CellType: "T4", Marker: "RDL", Subtype: "ef", Puncta: 1}, baseObservations[1]}, nodes: baseNodes, totalNodes: 8},
		{name: "no matching nodes", observations: baseObservations, nodes: []connectome.NodeRecord{sourceTestNode(0, "body-9001", "OL")}, totalNodes: 1},
		{name: "zero total nodes", observations: baseObservations, nodes: baseNodes, totalNodes: 0},
		{name: "negative total nodes", observations: baseObservations, nodes: baseNodes, totalNodes: -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			receptors, report, err := mapObservations(tc.observations, tc.nodes, tc.totalNodes)
			if err == nil {
				t.Fatalf("mapObservations() accepted invalid input and returned %+v, %+v", receptors, report)
			}
			if !reflect.DeepEqual(receptors, modulation.Receptors{}) {
				t.Fatalf("mapObservations() receptors on error = %+v, want zero Receptors", receptors)
			}
			if !reflect.DeepEqual(report, mappingReport{}) {
				t.Fatalf("mapObservations() report on error = %+v, want zero report", report)
			}
		})
	}
}

func TestMapObservationsRejectsMalformedNodeDeclarationsWithZeroOutputs(t *testing.T) {
	baseObservations := sourceTestObservations()
	cases := []struct {
		name   string
		mutate func([]connectome.NodeRecord) []connectome.NodeRecord
	}{
		{name: "duplicate index", mutate: func(nodes []connectome.NodeRecord) []connectome.NodeRecord {
			nodes[1].Index = nodes[0].Index
			return nodes
		}},
		{name: "unsorted index", mutate: func(nodes []connectome.NodeRecord) []connectome.NodeRecord {
			nodes[1].Index = 0
			nodes[0].Index = 1
			return nodes
		}},
		{name: "index outside total nodes", mutate: func(nodes []connectome.NodeRecord) []connectome.NodeRecord {
			nodes[0].Index = 8
			return nodes
		}},
		{name: "wrong namespace", mutate: func(nodes []connectome.NodeRecord) []connectome.NodeRecord {
			nodes[0].ID.Namespace = "flywire"
			return nodes
		}},
		{name: "empty external ID", mutate: func(nodes []connectome.NodeRecord) []connectome.NodeRecord {
			nodes[0].ID.ExternalID = ""
			return nodes
		}},
		{name: "invalid node ID namespace", mutate: func(nodes []connectome.NodeRecord) []connectome.NodeRecord {
			nodes[0].ID.Namespace = " \t\n"
			return nodes
		}},
		{name: "missing node type", mutate: func(nodes []connectome.NodeRecord) []connectome.NodeRecord {
			nodes[0].Type.Valid = false
			return nodes
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodes := tc.mutate(append([]connectome.NodeRecord(nil), sourceTestNodes()...))
			receptors, report, err := mapObservations(baseObservations, nodes, 8)
			if err == nil {
				t.Fatalf("mapObservations() accepted malformed nodes and returned %+v, %+v", receptors, report)
			}
			if !reflect.DeepEqual(receptors, modulation.Receptors{}) || !reflect.DeepEqual(report, mappingReport{}) {
				t.Fatalf("mapObservations() returned non-zero outputs on malformed nodes: receptors=%+v report=%+v", receptors, report)
			}
		})
	}
}

func TestMapObservationsExcludesUnclearAndNonCandidateTypesWithoutMappingThem(t *testing.T) {
	nodes := append(sourceTestNodes(),
		sourceTestNode(6, "body-4004", "T4"),
		sourceTestNode(7, "body-4005", "T4e"),
	)
	receptors, report, err := mapObservations(sourceTestObservations(), nodes, 8)
	if err != nil {
		t.Fatalf("mapObservations() error = %v", err)
	}
	if report.Mapped != 4 {
		t.Fatalf("mapping report mapped = %d, want only four T4a/b and T5c/d nodes", report.Mapped)
	}
	if report.ExcludedUnclear != 1 {
		t.Fatalf("mapping report excluded_unclear = %d, want one T4_unclear node", report.ExcludedUnclear)
	}
	for _, receptor := range receptors.Records {
		for _, cell := range receptor.Cells {
			if cell == 2 || cell >= 5 {
				t.Errorf("receptor %s mapped excluded/non-candidate cell %d", receptor.CellType, cell)
			}
		}
	}
}

func TestMapObservationsAcceptsAscendingNonContiguousNodeIndices(t *testing.T) {
	nodes := []connectome.NodeRecord{
		sourceTestNode(0, "body-4001", "T4a"),
		sourceTestNode(2, "body-4002", "T4b"),
		sourceTestNode(4, "body-5001", "T5c"),
		sourceTestNode(6, "body-4003", "T4_unclear"),
		sourceTestNode(7, "body-5002", "T5d"),
	}
	receptors, report, err := mapObservations(sourceTestObservations(), nodes, 8)
	if err != nil {
		t.Fatalf("mapObservations() rejected valid non-contiguous indices: %v", err)
	}
	if report.Mapped != 4 || report.ExcludedUnclear != 1 {
		t.Fatalf("mapping report = %+v, want mapped 4 and excluded_unclear 1", report)
	}
	wantCells := map[string][]int{"T4": []int{0, 2}, "T5": []int{4, 7}}
	for _, receptor := range receptors.Records {
		if !reflect.DeepEqual(receptor.Cells, wantCells[receptor.CellType]) {
			t.Errorf("%s receptor cells = %v, want %v", receptor.CellType, receptor.Cells, wantCells[receptor.CellType])
		}
	}
}

func TestMappedReceptorsFeedTheLocationControlWithTheOriginalNodeCount(t *testing.T) {
	receptors, _, err := mapObservations(sourceTestObservations(), sourceTestNodes(), 8)
	if err != nil {
		t.Fatalf("mapObservations() error = %v", err)
	}
	original := receptors
	original.Records = append([]modulation.Receptor(nil), receptors.Records...)
	for i := range original.Records {
		original.Records[i].Cells = append([]int(nil), receptors.Records[i].Cells...)
		if receptors.Records[i].Location != nil {
			location := *receptors.Records[i].Location
			original.Records[i].Location = &location
		}
	}

	control, err := locationControl(receptors, 8)
	if err != nil {
		t.Fatalf("locationControl(mapped receptors) error = %v", err)
	}
	if math.Abs(control.LocatedMeanOffset-(-0.1)) > 1e-12 || math.Abs(control.LegacyMeanOffset) > 1e-12 {
		t.Fatalf("mapped control = %+v, want located -0.1 and legacy 0", control)
	}
	if control.AssumedResponses != 4 {
		t.Fatalf("mapped control assumed responses = %d, want 4", control.AssumedResponses)
	}
	if !reflect.DeepEqual(receptors, original) {
		t.Fatalf("locationControl() mutated mapped receptors:\n got %+v\nwant %+v", receptors, original)
	}
}

func sourceTestControlReceptors(cellCount int) modulation.Receptors {
	cells := make([]int, cellCount)
	for i := range cells {
		cells[i] = i
	}
	return modulation.Receptors{
		Records: []modulation.Receptor{{
			Cells: cells, CellType: "T4", Signal: "GABA", Channel: 0,
			Status: modulation.StatusUnknown, Kd: 0, N: 0,
			Evidence: sourceURL, MeasurementKind: "receptor imaging", MappingVersion: mappingVersion,
			Location: &modulation.ReceptorLocation{
				Region: 1, Compartment: "dendrite", AnatomicalLocation: "T4 dendrite",
				Evidence: sourceURL, MeasurementKind: "receptor imaging", MappingVersion: mappingVersion,
			},
		}},
	}
}

func TestLocationControlUsesDeclaredCoefficientsAndLeavesSourceRecordsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cellCount int
	}{
		{name: "one cell", cellCount: 1},
		{name: "three cells", cellCount: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cellCount := tc.cellCount
			receptors := sourceTestControlReceptors(cellCount)
			original := receptors
			original.Records = append([]modulation.Receptor(nil), receptors.Records...)
			original.Records[0].Cells = append([]int(nil), receptors.Records[0].Cells...)
			if receptors.Records[0].Location != nil {
				location := *receptors.Records[0].Location
				original.Records[0].Location = &location
			}

			control, err := locationControl(receptors, cellCount)
			if err != nil {
				t.Fatalf("locationControl() error = %v", err)
			}
			if math.Abs(control.LocatedMeanOffset-(-0.1)) > 1e-12 {
				t.Fatalf("located mean offset = %.17g, want -0.1", control.LocatedMeanOffset)
			}
			if math.Abs(control.LegacyMeanOffset) > 1e-12 {
				t.Fatalf("legacy mean offset = %.17g, want 0", control.LegacyMeanOffset)
			}
			if control.AssumedResponses != cellCount {
				t.Fatalf("assumed responses = %d, want %d mapped cells", control.AssumedResponses, cellCount)
			}
			if !reflect.DeepEqual(receptors, original) {
				t.Fatalf("locationControl() mutated source receptors:\n got %+v\nwant %+v", receptors, original)
			}
		})
	}
}

func TestLocationControlDoesNotFitBetaOrCoefficientsToPunctaOrCellCount(t *testing.T) {
	one := sourceTestControlReceptors(1)
	three := sourceTestControlReceptors(3)
	three.Records[0].Cells = []int{0, 1, 2}

	oneReport, err := locationControl(one, 1)
	if err != nil {
		t.Fatalf("locationControl(one) error = %v", err)
	}
	threeReport, err := locationControl(three, 3)
	if err != nil {
		t.Fatalf("locationControl(three) error = %v", err)
	}
	// Summing the same IEEE-754 offset three times and dividing rounds
	// differently from the one-cell result. Compare numerical coefficients,
	// not a cell-count-dependent reduction's final bit.
	if math.Abs(oneReport.LocatedMeanOffset-threeReport.LocatedMeanOffset) > 1e-12 || math.Abs(oneReport.LegacyMeanOffset-threeReport.LegacyMeanOffset) > 1e-12 {
		t.Fatalf("control coefficients depend on cell count: one=%+v three=%+v", oneReport, threeReport)
	}
}

func TestLocationControlRejectsInvalidDeclarationsWithZeroReport(t *testing.T) {
	valid := sourceTestControlReceptors(1)
	cases := []struct {
		name      string
		receptors modulation.Receptors
		nodes     int
	}{
		{name: "zero nodes", receptors: valid, nodes: 0},
		{name: "negative nodes", receptors: valid, nodes: -1},
		{name: "no receptors", receptors: modulation.Receptors{}, nodes: 1},
		{name: "empty cells", receptors: func() modulation.Receptors {
			out := sourceTestControlReceptors(1)
			out.Records[0].Cells = nil
			return out
		}(), nodes: 1},
		{name: "cell outside nodes", receptors: func() modulation.Receptors {
			out := sourceTestControlReceptors(1)
			out.Records[0].Cells = []int{1}
			return out
		}(), nodes: 1},
		{name: "invalid location region", receptors: func() modulation.Receptors {
			out := sourceTestControlReceptors(1)
			out.Records[0].Location.Region = 2
			return out
		}(), nodes: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := locationControl(tc.receptors, tc.nodes)
			if err == nil {
				t.Fatalf("locationControl() accepted invalid declaration and returned %+v", got)
			}
			if got != (controlReport{}) {
				t.Fatalf("locationControl() report on error = %+v, want zero report", got)
			}
		})
	}
}
