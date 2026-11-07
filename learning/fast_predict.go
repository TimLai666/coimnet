package learning

import (
	"context"
	"fmt"
	"math"

	"github.com/HazelnutParadise/insyra/nn"
	"github.com/TimLai666/coimnet/dynamics"
)

// FastPredictor is an opt-in float32 prediction path for a last-step network
// whose core is an undelayed gated continuous core (any gate) with the
// softsign or tanh activation and no chemistry. The encoder and readout stay Insyra
// float32 products with no tape; the core runs dynamics.Infer32. Its logits
// differ from Network.Predict by float32 rounding in the core, so it is meant
// for latency measurement and deployment, not as the reference result.
//
// A FastPredictor reuses its buffers and is not safe for concurrent use; build
// one per goroutine.
type FastPredictor struct {
	inputSize, outputSize int
	inputNodes            []int
	readoutNodes          []int
	encoder, readout      *nn.Tensor
	core                  *dynamics.Infer32
	x, row, initial, sel  []float32
}

// CompileFastPredict packs p for FastPredictor. It refuses networks that the
// float32 path does not reproduce: other cores, vector nodes, a readout of
// every step, delayed edges, chemistry, or another gate or activation.
func (n *Network) CompileFastPredict(p Parameters) (*FastPredictor, error) {
	if n == nil {
		return nil, fmt.Errorf("nil network")
	}
	cpu, ok := n.core.(continuousCore)
	if !ok {
		return nil, fmt.Errorf("fast predict needs a continuous core")
	}
	if n.config.Dynamics.StateDimension > 1 {
		return nil, fmt.Errorf("fast predict needs scalar nodes")
	}
	if n.config.ReadoutEveryStep {
		return nil, fmt.Errorf("fast predict reads the last step only")
	}
	if len(p.ThetaRaw) != 0 {
		return nil, fmt.Errorf("theta_raw requires a LIF core")
	}
	core, err := n.coreParameters(p)
	if err != nil {
		return nil, err
	}
	f, err := cpu.model.CompileInfer32(core.Core)
	if err != nil {
		return nil, err
	}
	width := inputWidth(n.config)
	encoder, err := tensor([]int{n.config.InputSize, width}, p.Encoder)
	if err != nil {
		return nil, fmt.Errorf("encoder: %w", err)
	}
	readout, err := tensor([]int{len(n.config.ReadoutNodes), n.config.OutputSize}, p.Readout)
	if err != nil {
		return nil, fmt.Errorf("readout: %w", err)
	}
	nodes := f.Nodes()
	return &FastPredictor{
		inputSize: n.config.InputSize, outputSize: n.config.OutputSize,
		inputNodes: append([]int(nil), n.config.InputNodes...), readoutNodes: append([]int(nil), n.config.ReadoutNodes...),
		encoder: encoder, readout: readout, core: f,
		row: make([]float32, nodes), initial: make([]float32, nodes), sel: make([]float32, len(n.config.ReadoutNodes)),
	}, nil
}

// Predict starts an independent episode at zero voltage and returns the
// last-step readout, like Network.Predict.
func (fp *FastPredictor) Predict(ctx context.Context, input [][]float64) ([]float64, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(input) == 0 {
		return nil, fmt.Errorf("empty input sequence")
	}
	fp.x = fp.x[:0]
	for t, r := range input {
		if len(r) != fp.inputSize {
			return nil, fmt.Errorf("input[%d] width %d, want %d", t, len(r), fp.inputSize)
		}
		for i, v := range r {
			if !finite(v) || math.Abs(v) > math.MaxFloat32 {
				return nil, fmt.Errorf("input[%d][%d] cannot be represented as finite float32", t, i)
			}
			fp.x = append(fp.x, float32(v))
		}
	}
	x, err := nn.NewTensor([]int{len(input), fp.inputSize}, fp.x)
	if err != nil {
		return nil, err
	}
	z, err := nn.MatMul(x, fp.encoder)
	if err != nil {
		return nil, err
	}
	encoded, err := z.Float32Data()
	if err != nil {
		return nil, err
	}
	if err := fp.core.Reset(fp.initial); err != nil {
		return nil, err
	}
	width := len(encoded) / len(input)
	for t := range input {
		enc := encoded[t*width : (t+1)*width]
		in := enc
		if fp.inputNodes != nil {
			in = fp.row
			for i, id := range fp.inputNodes {
				in[id] = enc[i]
			}
		}
		if err := fp.core.Step(in); err != nil {
			return nil, fmt.Errorf("step %d: %w", t, err)
		}
		if t%8 == 7 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
	}
	y := fp.core.Output()
	for i, id := range fp.readoutNodes {
		fp.sel[i] = y[id]
	}
	h, err := nn.NewTensor([]int{len(fp.sel)}, fp.sel)
	if err != nil {
		return nil, err
	}
	pred, err := nn.MatMul(h, fp.readout)
	if err != nil {
		return nil, err
	}
	out, err := pred.Float32Data()
	if err != nil {
		return nil, err
	}
	logits := make([]float64, len(out))
	for i, v := range out {
		if !finite(float64(v)) {
			return nil, fmt.Errorf("non-finite prediction")
		}
		logits[i] = float64(v)
	}
	return logits, nil
}
