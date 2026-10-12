package learning

import "fmt"

// Check supplied array sizes before topology or sharing buffers are allocated.
// NewNetwork still validates all core rules and connectivity afterwards.
func preflightTrainerParameters(c Config, p Parameters) error {
	nodes, edges, components := c.Dynamics.Nodes, len(c.Dynamics.Sources), 1
	matrix := false
	if c.Mixed != nil {
		nodes, edges = c.Mixed.Nodes, len(c.Mixed.Sources)
	} else if c.LIF != nil {
		nodes, edges = c.LIF.Nodes, len(c.LIF.Sources)
	} else if c.Dynamics.StateDimension > 1 {
		components = c.Dynamics.StateDimension
		matrix = c.Dynamics.EdgeShape == "matrix"
	}
	if nodes <= 0 || nodes != len(p.Core.LogTau) || c.InputSize <= 0 || c.OutputSize <= 0 {
		return fmt.Errorf("parameter shape mismatch")
	}
	maxInt := int(^uint(0) >> 1)
	product := func(a, b int) (int, bool) {
		if a < 0 || b < 0 || (a != 0 && b > maxInt/a) {
			return 0, false
		}
		return a * b, true
	}
	bias, ok := product(nodes, components)
	if !ok || bias != len(p.Core.Bias) {
		return fmt.Errorf("parameter shape mismatch")
	}
	weights := edges
	if matrix {
		weights, ok = product(weights, components)
		if ok {
			weights, ok = product(weights, components)
		}
	}
	if !ok || weights != len(p.Core.Weights) {
		return fmt.Errorf("parameter shape mismatch")
	}
	width := nodes
	if c.InputNodes != nil {
		width = len(c.InputNodes)
	}
	width, ok = product(width, components)
	if !ok {
		return fmt.Errorf("parameter dimensions overflow")
	}
	encoder, ok := product(c.InputSize, width)
	if !ok || encoder != len(p.Encoder) {
		return fmt.Errorf("parameter shape mismatch")
	}
	readout, ok := product(len(c.ReadoutNodes), components)
	if ok {
		readout, ok = product(readout, c.OutputSize)
	}
	if !ok || readout != len(p.Readout) {
		return fmt.Errorf("parameter shape mismatch")
	}
	return nil
}
