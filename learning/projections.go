package learning

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/signal"
)

// BindProjections builds owned configuration/parameter values for a new model.
// Candidates must describe the actual dynamics graph in its index order.
// Channel widths must match Config. Input/readout coefficients are replaced,
// while core parameters are preserved. Start a new Trainer for a replacement;
// optimizer moments from the old projection must not be reused.
func BindProjections(c Config, p Parameters, candidates []signal.NeuronCandidate, input, output signal.Projection) (Config, Parameters, error) {
	fail := func(err error) (Config, Parameters, error) { return Config{}, Parameters{}, err }
	if len(candidates) != c.Dynamics.Nodes {
		return fail(fmt.Errorf("candidate graph has %d nodes, want %d", len(candidates), c.Dynamics.Nodes))
	}
	if input.Direction() != signal.ProjectionInput || output.Direction() != signal.ProjectionOutput {
		return fail(fmt.Errorf("projections must have input and output directions respectively"))
	}
	inputNodes, err := input.Bind(candidates)
	if err != nil {
		return fail(fmt.Errorf("input projection: %w", err))
	}
	outputNodes, err := output.Bind(candidates)
	if err != nil {
		return fail(fmt.Errorf("output projection: %w", err))
	}
	if input.InputSize() != c.InputSize || output.OutputSize() != c.OutputSize {
		return fail(fmt.Errorf("projection channel dimensions do not match network input/output sizes"))
	}
	if len(p.Core.Weights) != len(c.Dynamics.Sources) || len(p.Core.Bias) != c.Dynamics.Nodes || len(p.Core.LogTau) != c.Dynamics.Nodes {
		return fail(fmt.Errorf("core parameter shape mismatch"))
	}
	c.InputNodes = inputNodes
	c.ReadoutNodes = outputNodes
	n, err := NewNetwork(c)
	if err != nil {
		return fail(err)
	}
	candidate := Parameters{Core: p.Core, Encoder: input.Weights(), Readout: output.Weights()}
	// Use the same executable validation as NewTrainer, including float32
	// representability and dynamics parameter checks, before publishing values.
	if _, err := n.Predict(context.Background(), candidate, [][]float64{make([]float64, c.InputSize)}); err != nil {
		return fail(fmt.Errorf("invalid projected model: %w", err))
	}
	return n.Config(), copyParameters(candidate), nil
}
