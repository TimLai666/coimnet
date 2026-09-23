package fullgraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

// runOptions is one short full-graph run: how many rows of stimulus the
// forward covers, how many more a continuation covers, which edges get the
// local hebbian rule, whether the chemical layer is enabled and how many model
// cells one Advandee call may touch at most.
type runOptions struct {
	Steps        int  `json:"steps"`         // ≥ 2
	ContinueRows int  `json:"continue_rows"` // ≥ 1
	PlasticEdges int  `json:"plastic_edges"` // first K edges whose target is a readout node; 0 off
	Chemistry    bool `json:"chemistry"`
	MaxCells     int  `json:"max_cells"` // 0 uses dynamics.MaxStateValues
}

// artifact records one bundle shortTraining wrote. Bytes is the combined size
// of document.json and arrays.bin; SHA256 hashes manifest.json, which records
// the checksums of both data files and every array in the bundle.
type artifact struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// runReport records one short run: the shape the forward ran in, how long each
// phase took, what the single training step did to the parameters and how many
// plastic and chemical rows actually ran, and the three artifacts written.
type runReport struct {
	Rows              int            `json:"rows"`
	Chunk             int            `json:"chunk"`
	ForwardCalls      int            `json:"forward_calls"`
	ForwardMS         float64        `json:"forward_ms"`
	BackwardMS        float64        `json:"backward_ms"`
	SaveMS            float64        `json:"save_ms"`
	Loss              float64        `json:"loss"`
	GradientNorm      float64        `json:"gradient_norm"`
	UpdateNorm        float64        `json:"update_norm"`
	Updates           uint64         `json:"updates"`
	ParametersChanged map[string]int `json:"parameters_changed"`
	PlasticEdges      int            `json:"plastic_edges"`
	PlasticSteps      int            `json:"plastic_steps"`
	ChemistrySteps    int            `json:"chemistry_steps"`
	Artifacts         []artifact     `json:"artifacts"`
	// ForwardDigest is the SHA-256 of the state the forward reached right
	// before the gradient update, so a chunked and an unchunked run can be
	// shown to agree bit for bit.
	ForwardDigest string `json:"forward_digest"`
}

// gateMode picks which forward call the stepwise path uses. It mirrors what
// the enabled mechanisms can observe of the call: nothing for a plain advance,
// a receptor-driven gate for a chemistry-gated rule, and an open gate for a
// rule without one.
type gateMode int

const (
	gatePlain gateMode = iota
	gateOnes
	gateZeros
)

// shortTraining is the third-stage protocol on one whole graph: build the
// persistent individual, enable chemistry then plasticity, run the declared
// stimulus one forward chunk at a time, take one gradient update that covers
// every trainable group, and atomically write the model package, the trained
// individual and its training snapshot into dir. The individual is returned
// with the persistent state the forward left it in, before the continuation
// reads it; the report tells the evidence what actually ran.
func shortTraining(ctx context.Context, c learning.Config, p learning.Parameters, o learning.Options, r runOptions, dir string) (*learning.Individual, runReport, error) {
	var rep runReport
	if ctx == nil {
		return nil, rep, fmt.Errorf("fullgraph: short training needs a context")
	}
	if err := validateRun(r); err != nil {
		return nil, rep, err
	}
	if err := ctx.Err(); err != nil {
		return nil, rep, err
	}
	if dir == "" {
		return nil, rep, fmt.Errorf("fullgraph: short training needs a directory")
	}
	ind, err := learning.NewIndividual(c, p, o, make([]float64, c.Dynamics.Nodes))
	if err != nil {
		return nil, rep, err
	}
	mode := gatePlain
	if r.Chemistry {
		if err := ind.EnableChemistry(chemistryDeclaration(c)); err != nil {
			return nil, rep, err
		}
		gain := learning.ExpressionGain{Nodes: append([]int(nil), c.ReadoutNodes...), Receptor: 0, Scale: -0.5, Min: 0.5, Max: 1}
		if err := ind.SetExpressionGain(gain); err != nil {
			return nil, rep, err
		}
	}
	if r.PlasticEdges > 0 {
		edges, err := readoutEdges(c, r.PlasticEdges)
		if err != nil {
			return nil, rep, err
		}
		rule := plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .01}
		if r.Chemistry {
			receptor := 0
			rule.GateReceptor, rule.GateScale = &receptor, 1
			mode = gateZeros
		} else {
			mode = gateOnes
		}
		if err := ind.EnablePlasticity(plasticity.Config{Rule: rule, Edges: edges}); err != nil {
			return nil, rep, err
		}
	}
	chunk := chunkFor(r, c.Dynamics.Nodes)
	rep.Rows, rep.Chunk = r.Steps, chunk
	input := buildInput(r.Steps, c.InputSize)
	start := time.Now()
	outputs, calls, plasticSteps, chemistrySteps, err := forwardStepwise(ctx, ind, input, chunk, mode)
	if err != nil {
		return nil, rep, err
	}
	rep.ForwardCalls, rep.PlasticSteps, rep.ChemistrySteps = calls, plasticSteps, chemistrySteps
	rep.ForwardMS = time.Since(start).Seconds() * 1000
	if len(outputs) != r.Steps {
		return nil, rep, fmt.Errorf("fullgraph: the forward produced %d rows, want %d", len(outputs), r.Steps)
	}
	digest, err := neuralDigest(ind.Snapshot().Neural)
	if err != nil {
		return nil, rep, err
	}
	rep.ForwardDigest = digest
	before := ind.Snapshot().Parameters
	start = time.Now()
	result, err := ind.TrainEpisode(ctx, input, trainTarget(c.OutputSize))
	if err != nil {
		return nil, rep, err
	}
	rep.BackwardMS = time.Since(start).Seconds() * 1000
	rep.Loss, rep.GradientNorm, rep.UpdateNorm, rep.Updates = result.Loss, result.GradientNorm, result.UpdateNorm, result.Updates
	rep.ParametersChanged = diffParameters(before, ind.Snapshot().Parameters)
	rep.PlasticEdges = r.PlasticEdges
	start = time.Now()
	arts, err := saveArtifacts(dir, ind, c, p)
	if err != nil {
		return nil, rep, err
	}
	rep.SaveMS = time.Since(start).Seconds() * 1000
	rep.Artifacts = arts
	return ind, rep, nil
}

// saveArtifacts writes the three bundles of one trained individual into dir:
// model.coimbundle holds the untrained topology and parameters,
// individual.coimbundle the trained persistent individual, and
// training.coimbundle the trainer state. Every existing path is refused, so a
// second run over the same directory fails instead of overwriting evidence.
func saveArtifacts(dir string, ind *learning.Individual, c learning.Config, p learning.Parameters) ([]artifact, error) {
	if ind == nil {
		return nil, fmt.Errorf("fullgraph: save artifacts needs an individual")
	}
	if dir == "" {
		return nil, fmt.Errorf("fullgraph: save artifacts needs a directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := func(kind string) string { return filepath.Join(dir, kind) }
	ctx := context.Background()

	pkg, err := checkpoint.NewModelPackage(c, p, checkpoint.Units{TimeStep: "model_step", TimeConstant: "model_step"}, nil)
	if err != nil {
		return nil, err
	}
	if err := checkpoint.SaveModelPackageBundle(ctx, path("model.coimbundle"), pkg); err != nil {
		return nil, err
	}
	if err := checkpoint.SaveIndividualBundle(ctx, path("individual.coimbundle"), ind.Snapshot()); err != nil {
		return nil, err
	}
	training, err := trainingSnapshot(ind)
	if err != nil {
		return nil, err
	}
	if err := checkpoint.SaveTrainingBundle(ctx, path("training.coimbundle"), training); err != nil {
		return nil, err
	}
	arts := make([]artifact, 0, 3)
	for _, kind := range []string{"model.coimbundle", "individual.coimbundle", "training.coimbundle"} {
		p := path(kind)
		manifest, err := checkpoint.ReadBundleManifest(ctx, p)
		if err != nil {
			return nil, err
		}
		manifestPath := filepath.Join(p, "manifest.json")
		manifestSHA, err := sha256File(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("fullgraph: hash bundle manifest %q: %w", manifestPath, err)
		}
		arts = append(arts, artifact{
			Kind:   kind,
			Path:   p,
			Bytes:  manifest.Document.Bytes + manifest.Arrays.Bytes,
			SHA256: manifestSHA,
		})
	}
	return arts, nil
}

// trainingSnapshot rebuilds the trainer from the snapshot's optimizer part
// and validates it with RestoreTrainer before bundle encoding.
func trainingSnapshot(ind *learning.Individual) (learning.TrainingSnapshot, error) {
	s := ind.Snapshot()
	ts := learning.TrainingSnapshot{
		SchemaVersion: checkpoint.TrainingSchemaVersion,
		Config:        s.Config,
		Parameters:    s.Parameters,
		Options:       s.Optimizer.Options,
		Optimizer:     s.Optimizer.State,
		Updates:       s.Optimizer.Updates,
		Accumulator:   s.Optimizer.Accumulator,
	}
	if _, err := learning.RestoreTrainer(ts); err != nil {
		return learning.TrainingSnapshot{}, err
	}
	return ts, nil
}

// continueRows runs rows more rows of the stimulus on a trained individual,
// split into chunk-sized pieces whose call boundaries never change the
// row-by-row order. The continuation uses the plain advance, which for the
// receptor-gated rule of this protocol is bit-identical to the zero-gated
// forward shortTraining ran, and keeps every persistent state the forward
// left behind, so it is the continuation an exact resume has to reproduce.
func continueRows(ctx context.Context, ind *learning.Individual, rows, chunk int) ([][]float64, error) {
	if ctx == nil {
		return nil, fmt.Errorf("fullgraph: continue rows needs a context")
	}
	if ind == nil {
		return nil, fmt.Errorf("fullgraph: continue rows needs an individual")
	}
	if rows < 1 {
		return nil, fmt.Errorf("fullgraph: continue rows needs at least one row, got %d", rows)
	}
	if chunk < 1 {
		return nil, fmt.Errorf("fullgraph: continue rows needs a positive chunk, got %d", chunk)
	}
	width := ind.Snapshot().Config.InputSize
	input := buildInput(rows, width)
	outputs, _, _, _, err := forwardStepwise(ctx, ind, input, chunk, gatePlain)
	return outputs, err
}

// resumeAndContinue loads dir/individual.coimbundle in a fresh process, restores the
// individual with every persistent part (including the enabled plasticity and
// chemistry) and continues rows more rows in chunk-sized pieces. The returned
// snapshot is the state right after that continuation.
func resumeAndContinue(ctx context.Context, dir string, rows, chunk int) ([][]float64, learning.IndividualSnapshot, error) {
	var zero learning.IndividualSnapshot
	if ctx == nil {
		return nil, zero, fmt.Errorf("fullgraph: resume needs a context")
	}
	saved, err := checkpoint.LoadIndividualBundle(ctx, filepath.Join(dir, "individual.coimbundle"))
	if err != nil {
		return nil, zero, err
	}
	ind, err := learning.RestoreIndividual(saved)
	if err != nil {
		return nil, zero, err
	}
	outputs, err := continueRows(ctx, ind, rows, chunk)
	if err != nil {
		return nil, zero, err
	}
	return outputs, ind.Snapshot(), nil
}

// validateRun rejects a run the protocol would not know how to run.
func validateRun(r runOptions) error {
	if r.Steps < 2 {
		return fmt.Errorf("fullgraph: steps must be at least 2, got %d", r.Steps)
	}
	if r.ContinueRows < 1 {
		return fmt.Errorf("fullgraph: continue_rows must be at least 1, got %d", r.ContinueRows)
	}
	if r.PlasticEdges < 0 {
		return fmt.Errorf("fullgraph: plastic_edges must not be negative, got %d", r.PlasticEdges)
	}
	return nil
}

// chunkFor bounds one forward call to at most MaxCells cells; 0 means the
// framework cap dynamics.MaxStateValues. The largest input a call may cover is
// the minimum of that row budget and the whole run.
func chunkFor(r runOptions, nodes int) int {
	cap := r.MaxCells
	if cap <= 0 {
		cap = dynamics.MaxStateValues
	}
	chunk := r.Steps
	if q := cap / nodes; q >= 1 && q < chunk {
		chunk = q
	}
	if chunk < 1 {
		chunk = 1
	}
	return chunk
}

// buildInput holds the first input at 1 on every row. The loss is taken on the last row and the gradient reaches back only Options.Truncation rows, so an input confined to the first row would give the encoder no gradient whenever the run is longer than the truncation window.
func buildInput(rows, inputSize int) [][]float64 {
	out := make([][]float64, rows)
	for t := range out {
		row := make([]float64, inputSize)
		if inputSize > 0 {
			row[0] = 1
		}
		out[t] = row
	}
	return out
}

// trainTarget is the single learned value the short run optimizes toward: the
// readout returns one number and the target keeps that shape.
func trainTarget(outputSize int) []float64 {
	target := make([]float64, outputSize)
	if len(target) > 0 {
		target[0] = 0.1
	}
	return target
}

// forwardStepwise runs the whole input through the individual, chunk rows per
// call, and sums the rows each enabled mechanism reported. A call boundary is
// a checkpoint, never a recomputation: neural, plastic and chemical state are
// committed between calls, so the order of rows is the only thing that matters.
func forwardStepwise(ctx context.Context, ind *learning.Individual, input [][]float64, chunk int, mode gateMode) ([][]float64, int, int, int, error) {
	outputs := make([][]float64, 0, len(input))
	calls, plasticSteps, chemistrySteps := 0, 0, 0
	for start := 0; start < len(input); {
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, 0, err
		}
		end := start + chunk
		if end > len(input) {
			end = len(input)
		}
		rows := input[start:end]
		var (
			out    [][]float64
			report learning.PlasticReport
			err    error
		)
		switch mode {
		case gatePlain:
			out, err = ind.Advance(ctx, rows)
		case gateZeros:
			out, report, err = ind.AdvanceGated(ctx, rows, make([]float64, len(rows)))
		default:
			gate := make([]float64, len(rows))
			for i := range gate {
				gate[i] = 1
			}
			out, report, err = ind.AdvanceGated(ctx, rows, gate)
		}
		if err != nil {
			return nil, 0, 0, 0, err
		}
		plasticSteps += report.Steps
		chemistrySteps += ind.ChemistryReport().Steps
		outputs = append(outputs, out...)
		calls++
		start = end
	}
	return outputs, calls, plasticSteps, chemistrySteps, nil
}

// readoutEdges picks the first k edges of the graph whose target is a readout
// node, in the variant's edge order, which is the edge order the whole graph
// was built with. Fewer than k such edges is an error, because the protocol
// declared more plastic edges than the readout can see.
func readoutEdges(c learning.Config, k int) ([]int, error) {
	inReadout := make(map[int]bool, len(c.ReadoutNodes))
	for _, id := range c.ReadoutNodes {
		inReadout[id] = true
	}
	edges := make([]int, 0, k)
	for i, target := range c.Dynamics.Targets {
		if inReadout[target] {
			edges = append(edges, i)
			if len(edges) == k {
				return edges, nil
			}
		}
	}
	return nil, fmt.Errorf("fullgraph: %d plastic edges requested, only %d edges reach a readout node", k, len(edges))
}

// chemistryDeclaration is the one-region, one-channel chemical layer the short
// runs use: an external timeline releasing 1 on row 1, one hypothesized
// receptor on every readout node, declared like the continual controls. The
// readout gain is declared separately with SetExpressionGain.
func chemistryDeclaration(c learning.Config) modulation.ChemistryConfig {
	region := make([]int, c.Dynamics.Nodes)
	records := []modulation.Receptor{{
		Cells:           append([]int(nil), c.ReadoutNodes...),
		Signal:          "octopamine",
		Channel:         0,
		Status:          modulation.StatusHypothesized,
		Kd:              0.5,
		N:               1,
		Evidence:        "fixture",
		MeasurementKind: "declared",
		MappingVersion:  "chem-fixture/v1",
	}}
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}},
		}},
		Receptors: modulation.Receptors{Records: records},
		Regions:   modulation.RegionAssignment{NodeRegion: region},
	}
}

// diffParameters counts, group by group, how many parameter values moved
// between two snapshots of the same individual, comparing bit patterns so a
// store-then-restore round trip is exactly zero.
func diffParameters(before, after learning.Parameters) map[string]int {
	counts := make(map[string]int)
	count := func(name string, x, y []float64) {
		n := len(x)
		if len(y) < n {
			n = len(y)
		}
		for i := 0; i < n; i++ {
			if math.Float64bits(x[i]) != math.Float64bits(y[i]) {
				counts[name]++
			}
		}
	}
	count("weights", before.Core.Weights, after.Core.Weights)
	count("bias", before.Core.Bias, after.Core.Bias)
	count("log_tau", before.Core.LogTau, after.Core.LogTau)
	count("encoder", before.Encoder, after.Encoder)
	count("readout", before.Readout, after.Readout)
	return counts
}

// sha256File streams the file through SHA-256, so hashing the store, the
// parameter set or a whole-graph snapshot never holds the file in memory. It
// is the production twin of the test helper of the same purpose, kept
// separate so saveArtifacts does not depend on a test file.
func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// neuralDigest is the SHA-256 of one neural state's canonical JSON.
func neuralDigest(s learning.NeuralState) (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// refuseExistingWrite writes data to path only if nothing is there, so a
// rerun over the same directory cannot silently replace saved evidence.
func refuseExistingWrite(path string, data []byte) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("fullgraph: %s already exists; nothing was written", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
