package modulation

import (
	"fmt"
	"strings"
)

// ReceptorLocation maps a reported anatomical compartment to a chemical
// region. Region is an explicit model mapping, not an anatomical coordinate.
// Evidence and MeasurementKind describe the localization observation;
// MappingVersion identifies how it was assigned to cells and a model region.
// The scalar core pools its downstream effects at the receiving neuron.
type ReceptorLocation struct {
	Region             int    `json:"region"`
	Compartment        string `json:"compartment"`
	AnatomicalLocation string `json:"anatomical_location"`
	Evidence           string `json:"evidence"`
	MeasurementKind    string `json:"measurement_kind"`
	MappingVersion     string `json:"mapping_version"`
}

// validateReceptorLocation checks localization independently of response
// coefficients. A location never changes unknown sensitivity into a measured
// response. Expected refusals: unsupported compartment, negative/out-of-range
// region, or missing observation/mapping provenance.
func validateReceptorLocation(location *ReceptorLocation, regions int) error {
	if location == nil {
		return nil
	}
	if location.Region < 0 {
		return fmt.Errorf("modulation: receptor location region %d is negative", location.Region)
	}
	if regions > 0 && location.Region >= regions {
		return fmt.Errorf("modulation: receptor location region %d is outside [0, %d)", location.Region, regions)
	}
	switch location.Compartment {
	case "unknown", "soma", "dendrite", "axon", "presynaptic", "postsynaptic":
	default:
		return fmt.Errorf("modulation: receptor location has unsupported compartment %q", location.Compartment)
	}
	if strings.TrimSpace(location.AnatomicalLocation) == "" {
		return fmt.Errorf("modulation: receptor location has no anatomical_location")
	}
	if strings.TrimSpace(location.Evidence) == "" {
		return fmt.Errorf("modulation: receptor location has no evidence")
	}
	if strings.TrimSpace(location.MeasurementKind) == "" {
		return fmt.Errorf("modulation: receptor location has no measurement_kind")
	}
	if strings.TrimSpace(location.MappingVersion) == "" {
		return fmt.Errorf("modulation: receptor location has no mapping_version")
	}
	return nil
}

// receptorRegion selects the local chemical concentration. A nil location
// retains the original neuron-region mapping bit for bit.
func receptorRegion(record Receptor, cell, regions int, regionOf []int) (int, error) {
	if record.Location == nil {
		if cell < 0 || cell >= len(regionOf) {
			return 0, fmt.Errorf("modulation: cell %d is outside the region map", cell)
		}
		return regionOf[cell], nil
	}
	if err := validateReceptorLocation(record.Location, regions); err != nil {
		return 0, err
	}
	return record.Location.Region, nil
}
