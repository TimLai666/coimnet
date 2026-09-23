package learning

// CapacityReport separates the size of a model from its node count: two models with the same node count can differ by an
// order of magnitude in what they can store. FreeParameterCount excludes entries an update can never change (masked or
// disabled groups).
type CapacityReport struct {
	Nodes              int `json:"nodes"`
	Edges              int `json:"edges"`
	StateDimension     int `json:"state_dimension"` // 1 for scalar cores
	ParameterCount     int `json:"parameter_count"` // every learnable array together (weights, bias, log_tau, theta_raw, encoder, readout)
	FreeParameterCount int `json:"free_parameter_count"`
	MultAddsPerStep    int `json:"mult_adds_per_step"` // E·C·C (matrix edges) or E·C (scalar or C = 1) plus N·C for the leak
}

// Capacity reports what this network can store and what one step costs, from
// the topology alone plus the parameter arrays the caller passes. Every
// parameter counts as free here: per-item masks and disabled groups only
// exist inside a Trainer, whose Capacity narrows the free count accordingly.
// A nil or zero-value Network reports the zero report.
func (n *Network) Capacity(p Parameters) CapacityReport {
	if n == nil || n.core == nil {
		return CapacityReport{}
	}
	report := n.capacity(p)
	report.FreeParameterCount = report.ParameterCount
	return report
}

// Capacity is the network's report with FreeParameterCount narrowed to the
// entries whose parameterMask is true: masked items and disabled groups are
// stored but an update can never change them. A nil or zero-value Trainer
// reports the zero report.
func (tr *Trainer) Capacity() CapacityReport {
	if tr == nil || tr.network == nil || tr.network.core == nil {
		return CapacityReport{}
	}
	report := tr.network.capacity(tr.parameters)
	mask := parameterMask(tr.parameters, tr.options, tr.network.core.thetaNodes(), tr.network.core)
	free := 0
	for _, enabled := range mask {
		if enabled {
			free++
		}
	}
	report.FreeParameterCount = free
	return report
}

// capacity builds the shared fields of the report: ParameterCount is every
// learnable array laid out flat, and MultAddsPerStep is one multiply-add per
// weight value an edge integrates plus one leak multiply-add per state
// component, so a scalar core costs E + N and a matrix core E*C*C + N*C.
func (n *Network) capacity(p Parameters) CapacityReport {
	core := n.core
	dim := core.stateDim()
	perEdge := dim
	if core.matrixEdges() {
		perEdge = dim * dim
	}
	return CapacityReport{
		Nodes:           core.nodes(),
		Edges:           core.edges(),
		StateDimension:  dim,
		ParameterCount:  len(flatParameters(p)),
		MultAddsPerStep: core.edges()*perEdge + core.nodes()*dim,
	}
}
