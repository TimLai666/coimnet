package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
)

const sourceURL = "https://cdn.elifesciences.org/articles/62953/elife-62953-fig3-data1-v3.xlsx"
const sourceSHA256 = "39e207baafd868e44c4fdac8ecea327c2456cda3191fdec49cd4b734dc62a622"
const articleURL = "https://doi.org/10.7554/eLife.62953"
const mappingVersion = "fendl-2020-to-malecns-v1.0:type/v1"

// observation retains source column E as a puncta count, not a response
// coefficient. F and G are unlabeled and deliberately remain uninterpreted.
type observation struct {
	SourceRow int    `json:"source_row"`
	SampleID  string `json:"sample_id"`
	CellType  string `json:"cell_type"`
	Marker    string `json:"reported_marker"`
	Subtype   string `json:"subtype_group"`
	Puncta    int    `json:"puncta_count"`
}

type mappedCell struct {
	Index int             `json:"index"`
	ID    signal.NeuronID `json:"id"`
	Type  string          `json:"type"`
}

type mappingReport struct {
	Kind            string       `json:"kind"`
	Version         string       `json:"version"`
	Mapped          int          `json:"mapped"`
	ExcludedUnclear int          `json:"excluded_unclear"`
	Examples        []mappedCell `json:"examples"`
}

// observationsFromCSV reads the fixed six-column transcription of the source
// table. It rejects nil readers, incorrect/missing fields, duplicate sample or
// row IDs, invalid counts, and unsupported types/markers/subtype groups.
func observationsFromCSV(r io.Reader) ([]observation, error) {
	if nilReader(r) {
		return nil, fmt.Errorf("receptorlocation: nil CSV reader")
	}

	reader := csv.NewReader(r)
	reader.FieldsPerRecord = 6
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("receptorlocation: read CSV header: %w", err)
	}
	wantHeader := []string{"source_row", "sample_id", "cell_type", "reported_marker", "subtype_group", "puncta_count"}
	if !sameStrings(header, wantHeader) {
		return nil, fmt.Errorf("receptorlocation: unexpected CSV header %q", header)
	}

	var observations []observation
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receptorlocation: read CSV row: %w", err)
		}
		if len(row) != len(wantHeader) {
			return nil, fmt.Errorf("receptorlocation: CSV row has %d fields, want %d", len(row), len(wantHeader))
		}

		parsed, err := parseObservationRow(row)
		if err != nil {
			return nil, err
		}
		observations = append(observations, parsed)
	}
	if _, _, err := validateObservations(observations); err != nil {
		return nil, err
	}
	return observations, nil
}

// mapObservations makes a declared cell_type_inferred mapping. Only T4a-d and
// T5a-d in malecns-v1.0 are candidates; T4_unclear stays unmapped. Sample IDs
// never become body IDs. Returned receptors have unknown response, zero Kd/N,
// and dendritic Location in model region 1; counts do not scale sensitivity.
// Nodes require unique ascending indices within totalNodes. Empty source,
// unsupported namespace, malformed nodes, and no mapped cells are refused.
func mapObservations(observations []observation, nodes []connectome.NodeRecord, totalNodes int) (modulation.Receptors, mappingReport, error) {
	var zeroReceptors modulation.Receptors
	var zeroReport mappingReport

	hasT4, hasT5, err := validateObservations(observations)
	if err != nil {
		return zeroReceptors, zeroReport, err
	}
	if !hasT4 || !hasT5 {
		return zeroReceptors, zeroReport, fmt.Errorf("receptorlocation: source must contain at least one T4 and one T5 observation")
	}
	if err := validateMappingNodes(nodes, totalNodes); err != nil {
		return zeroReceptors, zeroReport, err
	}

	t4Cells := make([]int, 0)
	t5Cells := make([]int, 0)
	excludedUnclear := 0
	for _, node := range nodes {
		cell := int(node.Index)
		switch node.Type.Value {
		case "T4a", "T4b", "T4c", "T4d":
			t4Cells = append(t4Cells, cell)
		case "T5a", "T5b", "T5c", "T5d":
			t5Cells = append(t5Cells, cell)
		case "T4_unclear":
			excludedUnclear++
		}
	}
	if len(t4Cells) == 0 && len(t5Cells) == 0 {
		return zeroReceptors, zeroReport, fmt.Errorf("receptorlocation: source candidates contain no supported MaleCNS cells")
	}

	records := make([]modulation.Receptor, 0, 2)
	if len(t4Cells) > 0 {
		records = append(records, newMappedReceptor("T4", t4Cells))
	}
	if len(t5Cells) > 0 {
		records = append(records, newMappedReceptor("T5", t5Cells))
	}
	receptors := modulation.Receptors{
		Records: records,
	}
	report := mappingReport{
		Kind:            "cell_type_inferred",
		Version:         mappingVersion,
		Mapped:          len(t4Cells) + len(t5Cells),
		ExcludedUnclear: excludedUnclear,
		Examples:        mappingExamples(nodes),
	}
	return receptors, report, nil
}

type controlReport struct {
	LocatedMeanOffset float64 `json:"located_mean_offset"`
	LegacyMeanOffset  float64 `json:"legacy_mean_offset"`
	AssumedResponses  int     `json:"assumed_responses"`
}

// locationControl holds the graph, concentration and engineering coefficients
// fixed while comparing explicit region 1 against nil Location/NodeRegion 0.
// Kd=N=1 and BetaScale=-.2 are declared engineering values, never fitted from
// puncta counts. It owns its candidates and leaves the unknown source mapping
// unchanged. Invalid declarations are refused rather than partially reported.
func locationControl(receptors modulation.Receptors, nodes int) (controlReport, error) {
	var zero controlReport
	mappedCells, err := validateControlDeclaration(receptors, nodes)
	if err != nil {
		return zero, err
	}

	explicit := cloneControlReceptors(receptors, true)
	legacy := cloneControlReceptors(receptors, false)
	for i := range explicit.Records {
		explicit.Records[i].Kd = 1
		explicit.Records[i].N = 1
		legacy.Records[i].Kd = 1
		legacy.Records[i].N = 1
	}
	explicit.AllowAssumedCoefficients = true
	legacy.AllowAssumedCoefficients = true

	state := modulation.ChemistryState{Concentration: [][]float64{{0}, {1}}}
	regionOf := make([]int, nodes)
	effects := make([]modulation.Effect, len(explicit.Records))
	for i := range effects {
		effects[i] = modulation.Effect{
			Kind:      modulation.EffectSensitivity,
			Receptor:  i,
			BetaScale: -0.2,
		}
	}

	explicitOccupancies, explicitSummary, err := explicit.Occupancies(state, regionOf)
	if err != nil {
		return zero, fmt.Errorf("receptorlocation: explicit-location occupancies: %w", err)
	}
	legacyOccupancies, legacySummary, err := legacy.Occupancies(state, regionOf)
	if err != nil {
		return zero, fmt.Errorf("receptorlocation: nil-location occupancies: %w", err)
	}
	if explicitSummary.Assumed != mappedCells || legacySummary.Assumed != mappedCells {
		return zero, fmt.Errorf("receptorlocation: assumed occupancy count changed: explicit=%d legacy=%d mapped=%d", explicitSummary.Assumed, legacySummary.Assumed, mappedCells)
	}

	_, explicitOffset, _, _, err := modulation.ApplyEffects(effects, explicitOccupancies, explicit.Mix, nodes)
	if err != nil {
		return zero, fmt.Errorf("receptorlocation: explicit-location effects: %w", err)
	}
	_, legacyOffset, _, _, err := modulation.ApplyEffects(effects, legacyOccupancies, legacy.Mix, nodes)
	if err != nil {
		return zero, fmt.Errorf("receptorlocation: nil-location effects: %w", err)
	}

	locatedMean, err := mappedMeanOffset(explicitOffset, explicit.Records, mappedCells)
	if err != nil {
		return zero, err
	}
	legacyMean, err := mappedMeanOffset(legacyOffset, legacy.Records, mappedCells)
	if err != nil {
		return zero, err
	}
	return controlReport{
		LocatedMeanOffset: locatedMean,
		LegacyMeanOffset:  legacyMean,
		AssumedResponses:  explicitSummary.Assumed,
	}, nil
}

func nilReader(r io.Reader) bool {
	if r == nil {
		return true
	}
	v := reflect.ValueOf(r)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func parseObservationRow(row []string) (observation, error) {
	if len(row) != 6 {
		return observation{}, fmt.Errorf("receptorlocation: observation row has %d fields, want 6", len(row))
	}
	for i, field := range row {
		if strings.TrimSpace(field) == "" {
			return observation{}, fmt.Errorf("receptorlocation: observation field %d is empty", i)
		}
	}
	sourceRow, err := strconv.Atoi(row[0])
	if err != nil {
		return observation{}, fmt.Errorf("receptorlocation: invalid source_row %q: %w", row[0], err)
	}
	puncta, err := strconv.Atoi(row[5])
	if err != nil {
		return observation{}, fmt.Errorf("receptorlocation: invalid puncta_count %q: %w", row[5], err)
	}
	return observation{
		SourceRow: sourceRow,
		SampleID:  row[1],
		CellType:  row[2],
		Marker:    row[3],
		Subtype:   row[4],
		Puncta:    puncta,
	}, nil
}

func validateObservation(value observation) error {
	if value.SourceRow < 2 {
		return fmt.Errorf("receptorlocation: source_row %d is below data row 2", value.SourceRow)
	}
	if strings.TrimSpace(value.SampleID) == "" {
		return fmt.Errorf("receptorlocation: sample_id must not be empty")
	}
	if value.CellType != "T4" && value.CellType != "T5" {
		return fmt.Errorf("receptorlocation: unsupported cell_type %q", value.CellType)
	}
	if value.Marker != "RDL" {
		return fmt.Errorf("receptorlocation: unsupported reported_marker %q", value.Marker)
	}
	if value.Subtype != "ab" && value.Subtype != "cd" {
		return fmt.Errorf("receptorlocation: unsupported subtype_group %q", value.Subtype)
	}
	if value.Puncta < 0 {
		return fmt.Errorf("receptorlocation: puncta_count %d is negative", value.Puncta)
	}
	return nil
}

func validateObservations(observations []observation) (hasT4, hasT5 bool, err error) {
	if len(observations) == 0 {
		return false, false, fmt.Errorf("receptorlocation: no observations")
	}
	rows := make(map[int]struct{}, len(observations))
	samples := make(map[string]struct{}, len(observations))
	for i, value := range observations {
		if err := validateObservation(value); err != nil {
			return false, false, fmt.Errorf("receptorlocation: observation %d: %w", i, err)
		}
		if _, exists := rows[value.SourceRow]; exists {
			return false, false, fmt.Errorf("receptorlocation: duplicate source_row %d", value.SourceRow)
		}
		rows[value.SourceRow] = struct{}{}
		if _, exists := samples[value.SampleID]; exists {
			return false, false, fmt.Errorf("receptorlocation: duplicate sample_id %q", value.SampleID)
		}
		samples[value.SampleID] = struct{}{}
		switch value.CellType {
		case "T4":
			hasT4 = true
		case "T5":
			hasT5 = true
		}
	}
	return hasT4, hasT5, nil
}

func validateMappingNodes(nodes []connectome.NodeRecord, totalNodes int) error {
	if totalNodes < 1 {
		return fmt.Errorf("receptorlocation: totalNodes must be positive, got %d", totalNodes)
	}
	var previous uint64
	for i, node := range nodes {
		if i > 0 && node.Index <= previous {
			return fmt.Errorf("receptorlocation: node indices must be strictly ascending at node %d", i)
		}
		if node.Index >= uint64(totalNodes) {
			return fmt.Errorf("receptorlocation: node index %d is outside [0, %d)", node.Index, totalNodes)
		}
		if node.ID.Namespace != "malecns-v1.0" {
			return fmt.Errorf("receptorlocation: node %d has namespace %q, want malecns-v1.0", i, node.ID.Namespace)
		}
		if err := node.ID.Validate(); err != nil {
			return fmt.Errorf("receptorlocation: node %d has invalid ID: %w", i, err)
		}
		if !node.Type.Valid {
			return fmt.Errorf("receptorlocation: node %d has no valid cell type", i)
		}
		previous = node.Index
	}
	return nil
}

func newMappedReceptor(cellType string, cells []int) modulation.Receptor {
	copiedCells := append([]int(nil), cells...)
	evidence := "Fendl, Vieira & Borst (2020) Rdl localization source data: " + sourceURL
	return modulation.Receptor{
		Cells:           copiedCells,
		CellType:        cellType,
		Signal:          "GABA",
		Channel:         0,
		Status:          modulation.StatusUnknown,
		Kd:              0,
		N:               0,
		Evidence:        evidence,
		MeasurementKind: "measured Rdl puncta localization in T4/T5 dendrites",
		MappingVersion:  mappingVersion,
		Location: &modulation.ReceptorLocation{
			Region:             1,
			Compartment:        "dendrite",
			AnatomicalLocation: cellType + " dendrites",
			Evidence:           evidence,
			MeasurementKind:    "measured dendritic Rdl localization",
			MappingVersion:     mappingVersion,
		},
	}
}

func mappingExamples(nodes []connectome.NodeRecord) []mappedCell {
	examples := make([]mappedCell, 0, 8)
	seenTypes := make(map[string]struct{}, 8)
	for _, node := range nodes {
		cellType := node.Type.Value
		switch cellType {
		case "T4a", "T4b", "T4c", "T4d", "T5a", "T5b", "T5c", "T5d":
			if _, seen := seenTypes[cellType]; seen {
				continue
			}
			seenTypes[cellType] = struct{}{}
			examples = append(examples, mappedCell{Index: int(node.Index), ID: node.ID, Type: cellType})
			if len(examples) == 8 {
				break
			}
		}
	}
	return examples
}

func cloneControlReceptors(source modulation.Receptors, keepLocation bool) modulation.Receptors {
	clone := modulation.Receptors{
		Records:                  make([]modulation.Receptor, len(source.Records)),
		Mix:                      source.Mix,
		AllowAssumedCoefficients: source.AllowAssumedCoefficients,
	}
	for i, record := range source.Records {
		clone.Records[i] = record
		clone.Records[i].Cells = append([]int(nil), record.Cells...)
		if keepLocation && record.Location != nil {
			location := *record.Location
			clone.Records[i].Location = &location
		} else {
			clone.Records[i].Location = nil
		}
	}
	return clone
}

func validateControlDeclaration(receptors modulation.Receptors, nodes int) (int, error) {
	if nodes < 1 {
		return 0, fmt.Errorf("receptorlocation: control needs at least one node, got %d", nodes)
	}
	if len(receptors.Records) == 0 {
		return 0, fmt.Errorf("receptorlocation: control needs at least one receptor")
	}
	if err := receptors.Validate(nodes, 1); err != nil {
		return 0, fmt.Errorf("receptorlocation: invalid control receptor declaration: %w", err)
	}
	mappedCells := 0
	for i, record := range receptors.Records {
		if record.Status != modulation.StatusUnknown {
			return 0, fmt.Errorf("receptorlocation: control receptor %d has status %q, want unknown", i, record.Status)
		}
		if record.Location == nil {
			return 0, fmt.Errorf("receptorlocation: control receptor %d has no explicit location", i)
		}
		if record.Location.Region < 0 || record.Location.Region >= 2 {
			return 0, fmt.Errorf("receptorlocation: control receptor %d location region %d is outside [0, 2)", i, record.Location.Region)
		}
		if len(record.Cells) == 0 {
			return 0, fmt.Errorf("receptorlocation: control receptor %d names no cells", i)
		}
		mappedCells += len(record.Cells)
	}
	if mappedCells == 0 {
		return 0, fmt.Errorf("receptorlocation: control has no mapped cells")
	}
	return mappedCells, nil
}

func mappedMeanOffset(offset []float64, records []modulation.Receptor, mappedCells int) (float64, error) {
	if mappedCells < 1 {
		return 0, fmt.Errorf("receptorlocation: cannot compute a mapped-cell mean without mapped cells")
	}
	var total float64
	for _, record := range records {
		for _, cell := range record.Cells {
			if cell < 0 || cell >= len(offset) {
				return 0, fmt.Errorf("receptorlocation: effect output has no cell %d", cell)
			}
			total += offset[cell]
		}
	}
	return total / float64(mappedCells), nil
}
