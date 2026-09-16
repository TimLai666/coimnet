package modulation

import (
	"fmt"
	"math"
)

// The three receptor statuses are three different answers and are never merged.
// An unresponsive receptor was reported not to respond, so its occupancy is a
// declared zero rather than a measured one; an unknown receptor has no usable
// coefficients unless the run declares that assumed ones may be used; a
// hypothesized receptor carries a declared engineering curve.
const (
	StatusUnresponsive = "unresponsive"
	StatusUnknown      = "unknown"
	StatusHypothesized = "hypothesized"
)

// Mixing rules for several receptors acting on the same cell through the same
// kind of effect. An empty rule means MixSum.
const (
	MixSum = "sum"
	MixMax = "max"
)

// Receptor declares which cells carry a receptor for one modulation channel and
// how strongly they respond. Kd and N are engineering coefficients: their JSON
// names say so, and no report may describe them as measured. CellType is a
// label only. Evidence, MeasurementKind and MappingVersion are required, so a
// record always names where it came from and which mapping produced it.
type Receptor struct {
	Cells           []int   `json:"cells"`
	CellType        string  `json:"cell_type,omitempty"`
	Signal          string  `json:"signal"`
	Channel         int     `json:"channel"`
	Status          string  `json:"status"`
	Kd              float64 `json:"engineering_kd"`
	N               float64 `json:"engineering_n"`
	Evidence        string  `json:"evidence"`
	MeasurementKind string  `json:"measurement_kind"`
	MappingVersion  string  `json:"mapping_version"`
}

// Receptors is a declared receptor set. AllowAssumedCoefficients is the single
// switch that lets an unknown receptor be computed from coefficients nobody
// measured; every record it produces is marked assumed.
type Receptors struct {
	Records                  []Receptor `json:"records"`
	Mix                      string     `json:"mix,omitempty"`
	AllowAssumedCoefficients bool       `json:"allow_assumed_coefficients"`
}

// OccupancyRecord is one receptor acting on one cell in one step. Skipped marks
// an unknown receptor that was not computed; Assumed marks one that was, from
// coefficients that are an engineering assumption.
type OccupancyRecord struct {
	Receptor  int     `json:"receptor"`
	Cell      int     `json:"cell"`
	Region    int     `json:"region"`
	Channel   int     `json:"channel"`
	Status    string  `json:"status"`
	Occupancy float64 `json:"occupancy"`
	Assumed   bool    `json:"assumed"`
	Skipped   bool    `json:"skipped"`
}

// OccupancySummary counts the occupancy records of one call by how they were
// produced, so a report can state how much of it rests on assumptions. The unit
// is one receptor acting on one cell, which is exactly one OccupancyRecord.
type OccupancySummary struct {
	Assumed        int `json:"assumed"`
	UnknownSkipped int `json:"unknown_skipped"`
	Unresponsive   int `json:"unresponsive"`
}

// Occupancy is the Hill curve c^n/(Kd^n + c^n) evaluated in the log domain as
// 1/(1 + exp(n*(ln Kd - ln c))), which stays finite where the direct form
// overflows: c = 1e300 with n = 8 answers 1 and c = 1e-300 answers 0, both
// without a NaN or an infinity. A zero concentration is zero occupancy, and
// c = Kd is exactly 0.5 for every n.
//
// The function has no error return, so arguments outside its declared domain
// (a negative or non-finite c, Kd <= 0, n < 1) answer NaN rather than a number
// that could be mistaken for an occupancy. Receptors.Validate is the gate that
// keeps such a record from ever reaching it.
func Occupancy(c, kd, n float64) float64 {
	if !finite(c) || !finite(kd) || !finite(n) || c < 0 || kd <= 0 || n < 1 {
		return math.NaN()
	}
	if c == 0 {
		return 0
	}
	return 1 / (1 + math.Exp(n*(math.Log(kd)-math.Log(c))))
}

// Validate checks a receptor set against a node count and a channel count. It
// requires ascending in-range cells, a known status, a provenance on every
// record, and valid coefficients on every record whose status will compute one.
// An unknown record that declares no coefficient at all is valid and will be
// skipped; an unresponsive record never needs coefficients.
func (r *Receptors) Validate(nodes, channels int) error {
	if r == nil {
		return fmt.Errorf("modulation: nil receptor set")
	}
	if nodes < 1 {
		return fmt.Errorf("modulation: a receptor set needs at least one node, got %d", nodes)
	}
	if channels < 1 {
		return fmt.Errorf("modulation: a receptor set needs at least one channel, got %d", channels)
	}
	switch r.Mix {
	case "", MixSum, MixMax:
	default:
		return fmt.Errorf("modulation: unsupported receptor mix %q", r.Mix)
	}
	for i, record := range r.Records {
		if len(record.Cells) == 0 {
			return fmt.Errorf("modulation: receptor %d names no cell", i)
		}
		previous := -1
		for _, cell := range record.Cells {
			if cell < 0 || cell >= nodes {
				return fmt.Errorf("modulation: receptor %d cell %d is outside [0, %d)", i, cell, nodes)
			}
			if cell <= previous {
				return fmt.Errorf("modulation: receptor %d cells must be ascending, %d follows %d", i, cell, previous)
			}
			previous = cell
		}
		if record.Channel < 0 || record.Channel >= channels {
			return fmt.Errorf("modulation: receptor %d channel %d is outside [0, %d)", i, record.Channel, channels)
		}
		switch record.Status {
		case StatusUnresponsive, StatusUnknown, StatusHypothesized:
		default:
			return fmt.Errorf("modulation: receptor %d has unsupported status %q", i, record.Status)
		}
		if record.Evidence == "" {
			return fmt.Errorf("modulation: receptor %d has no evidence", i)
		}
		if record.MeasurementKind == "" {
			return fmt.Errorf("modulation: receptor %d has no measurement_kind", i)
		}
		if record.MappingVersion == "" {
			return fmt.Errorf("modulation: receptor %d has no mapping_version", i)
		}
		// Coefficients are checked only where they will be used. A half
		// declared unknown record is a mistake, not a skip, so it is refused
		// as soon as the run allows assumed coefficients at all.
		switch record.Status {
		case StatusHypothesized:
			if err := checkedCoefficients(i, record); err != nil {
				return err
			}
		case StatusUnknown:
			if r.AllowAssumedCoefficients && (record.Kd != 0 || record.N != 0) {
				if err := checkedCoefficients(i, record); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkedCoefficients(i int, record Receptor) error {
	if !finite(record.Kd) || record.Kd <= 0 {
		return fmt.Errorf("modulation: receptor %d engineering_kd must be finite and positive, got %v", i, record.Kd)
	}
	if !finite(record.N) || record.N < 1 {
		return fmt.Errorf("modulation: receptor %d engineering_n must be finite and at least 1, got %v", i, record.N)
	}
	return nil
}

func usableCoefficients(record Receptor) bool { return checkedCoefficients(0, record) == nil }

// Occupancies evaluates every receptor against the concentration of the region
// its cells sit in. regionOf names one region per node, so its length is the
// node count. The result is in declaration order, one record per receptor per
// cell, together with the counts a report needs.
//
// A refused call returns no record and an empty summary.
func (r *Receptors) Occupancies(state ChemistryState, regionOf []int) ([]OccupancyRecord, OccupancySummary, error) {
	var empty OccupancySummary
	if r == nil {
		return nil, empty, fmt.Errorf("modulation: nil receptor set")
	}
	regions, channels, err := concentrationShape(state.Concentration)
	if err != nil {
		return nil, empty, err
	}
	if len(regionOf) == 0 {
		return nil, empty, fmt.Errorf("modulation: the region map names no node, so no cell has a region")
	}
	for cell, region := range regionOf {
		if region < 0 || region >= regions {
			return nil, empty, fmt.Errorf("modulation: node %d is mapped to region %d, outside [0, %d)", cell, region, regions)
		}
	}
	if err = r.Validate(len(regionOf), channels); err != nil {
		return nil, empty, err
	}
	total := 0
	for _, record := range r.Records {
		total += len(record.Cells)
	}
	records, summary := make([]OccupancyRecord, 0, total), empty
	for i, record := range r.Records {
		for _, cell := range record.Cells {
			region := regionOf[cell]
			out := OccupancyRecord{Receptor: i, Cell: cell, Region: region, Channel: record.Channel, Status: record.Status}
			switch record.Status {
			case StatusUnresponsive:
				// A declared absence of response, not a measured zero.
				summary.Unresponsive++
			case StatusUnknown:
				if r.AllowAssumedCoefficients && usableCoefficients(record) {
					out.Occupancy = Occupancy(state.Concentration[region][record.Channel], record.Kd, record.N)
					out.Assumed = true
					summary.Assumed++
				} else {
					out.Skipped = true
					summary.UnknownSkipped++
				}
			case StatusHypothesized:
				out.Occupancy = Occupancy(state.Concentration[region][record.Channel], record.Kd, record.N)
			}
			if !finite(out.Occupancy) {
				return nil, empty, fmt.Errorf("modulation: receptor %d cell %d reached a non-finite occupancy", i, cell)
			}
			records = append(records, out)
		}
	}
	return records, summary, nil
}

// concentrationShape validates a concentration matrix that did not come from a
// Kinetics.Step, which is the case whenever a caller hands one in directly.
func concentrationShape(grid [][]float64) (int, int, error) {
	if len(grid) == 0 {
		return 0, 0, fmt.Errorf("modulation: the state has no concentration row")
	}
	channels := len(grid[0])
	if channels == 0 {
		return 0, 0, fmt.Errorf("modulation: the concentration declares no channel")
	}
	for region, row := range grid {
		if len(row) != channels {
			return 0, 0, fmt.Errorf("modulation: concentration row %d has %d channels, want %d", region, len(row), channels)
		}
		for channel, v := range row {
			if !finite(v) {
				return 0, 0, fmt.Errorf("modulation: concentration [%d][%d] is not finite", region, channel)
			}
			if v < 0 {
				return 0, 0, fmt.Errorf("modulation: concentration [%d][%d] is negative (%v)", region, channel, v)
			}
		}
	}
	return len(grid), channels, nil
}
