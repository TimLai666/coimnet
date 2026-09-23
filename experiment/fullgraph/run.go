package fullgraph

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/resources"
	"github.com/TimLai666/coimnet/simulate"
)

// ReportSchemaVersion is the schema version written into Report documents.
const ReportSchemaVersion = "coimnet-full-graph-short-training/v1"

// Options is one full-graph short-training run. Protocol is a simulate compare protocol file: its run block (continuous core,
// tanh, parameter source derived_release/v1) builds the original variant and its dt, and its sets name the input and readout
// nodes by InputSet and ReadoutSet.
type Options struct {
	Store        string  `json:"store"`
	Params       string  `json:"params"`
	Protocol     string  `json:"protocol"`
	InputSet     string  `json:"input_set"`      // default "alin"
	ReadoutSet   string  `json:"readout_set"`    // default "descending_neuron"
	Steps        int     `json:"steps"`          // default 16, ≥ 2
	Truncation   int     `json:"truncation"`     // default 8, ≥ 0
	ContinueRows int     `json:"continue_rows"`  // default 4, ≥ 1
	PlasticEdges int     `json:"plastic_edges"`  // default 4096, ≥ 0
	Chemistry    bool    `json:"chemistry"`      // default true
	LearningRate float64 `json:"learning_rate"`  // default 0.001, > 0
	MaxMemoryMiB int     `json:"max_memory_mib"` // default 12288, > 0
	MaxCells     int     `json:"max_cells"`      // default 0 (= dynamics.MaxStateValues)
	OutDir       string  `json:"out_dir"`        // must not exist; Run creates it
}

// DefaultOptions returns the declared defaults for every option. The full
// graph's backward history term 2*E*w*T puts 32 steps above the default 12,288
// MiB estimate budget, while 16 steps fits. Steps are an explicit protocol
// value, never silently reduced to fit memory.
func DefaultOptions() Options {
	return Options{
		InputSet:     "alin",
		ReadoutSet:   "descending_neuron",
		Steps:        16,
		Truncation:   8,
		ContinueRows: 4,
		PlasticEdges: 4096,
		Chemistry:    true,
		LearningRate: 0.001,
		MaxMemoryMiB: 12288,
	}
}

// Validate checks that required paths are non-empty, numeric ranges satisfy contract bounds, and InputSet != ReadoutSet.
func (o Options) Validate() error {
	for _, p := range []struct {
		name string
		path string
	}{{"store", o.Store}, {"params", o.Params}, {"protocol", o.Protocol}, {"out_dir", o.OutDir}} {
		if p.path == "" {
			return fmt.Errorf("fullgraph: %s is empty, want the path of the store file, the parameter set, the compare protocol and the new output directory", p.name)
		}
	}
	if o.Steps < 2 {
		return fmt.Errorf("fullgraph: steps is %d, want at least 2", o.Steps)
	}
	if o.Truncation < 0 {
		return fmt.Errorf("fullgraph: truncation is %d, want a non-negative value", o.Truncation)
	}
	if o.ContinueRows < 1 {
		return fmt.Errorf("fullgraph: continue_rows is %d, want at least 1", o.ContinueRows)
	}
	if o.PlasticEdges < 0 {
		return fmt.Errorf("fullgraph: plastic_edges is %d, want a non-negative value", o.PlasticEdges)
	}
	if !finite(o.LearningRate) || o.LearningRate <= 0 {
		return fmt.Errorf("fullgraph: learning_rate is %v, want a finite value above zero", o.LearningRate)
	}
	if o.MaxMemoryMiB <= 0 {
		return fmt.Errorf("fullgraph: max_memory_mib is %d, want a value above zero", o.MaxMemoryMiB)
	}
	if o.MaxCells < 0 {
		return fmt.Errorf("fullgraph: max_cells is %d, want a non-negative value", o.MaxCells)
	}
	if o.InputSet == "" || o.ReadoutSet == "" {
		return fmt.Errorf("fullgraph: input_set and readout_set must not be empty")
	}
	if o.InputSet == o.ReadoutSet {
		return fmt.Errorf("fullgraph: input_set %q and readout_set %q must name different sets", o.InputSet, o.ReadoutSet)
	}
	return nil
}

// Mechanisms is what the plastic and chemical layers actually did during the forward, read from the individual that
// shortTraining returned (before the continuation): FastNonzero and FastMaxAbs over Snapshot().Plastic.State.Plastic,
// ConcentrationMax over every value of Snapshot().Chemical.State.Concentration; zero values when a layer is off.
type Mechanisms struct {
	PlasticEdges     int     `json:"plastic_edges"`
	PlasticSteps     int     `json:"plastic_steps"`
	FastNonzero      int     `json:"fast_nonzero"`
	FastMaxAbs       float64 `json:"fast_max_abs"`
	ChemistrySteps   int     `json:"chemistry_steps"`
	ConcentrationMax float64 `json:"concentration_max"`
}

// mechanismsOf fills Mechanisms from the individual and the run report.
func mechanismsOf(ind *learning.Individual, rep runReport) Mechanisms {
	m := Mechanisms{
		PlasticEdges:   rep.PlasticEdges,
		PlasticSteps:   rep.PlasticSteps,
		ChemistrySteps: rep.ChemistrySteps,
	}
	if ind == nil {
		return m
	}
	snap := ind.Snapshot()
	if snap.Plastic != nil {
		for _, v := range snap.Plastic.State.Plastic {
			if v != 0 {
				m.FastNonzero++
			}
			abs := math.Abs(v)
			if abs > m.FastMaxAbs {
				m.FastMaxAbs = abs
			}
		}
	}
	if snap.Chemical != nil {
		for _, row := range snap.Chemical.State.Concentration {
			for _, v := range row {
				if v > m.ConcentrationMax {
					m.ConcentrationMax = v
				}
			}
		}
	}
	return m
}

// continuationDigest returns the SHA-256 hex digest of every Float64bits of
// the continuation rows, row by row in little-endian order.
func continuationDigest(rows [][]float64) string {
	h := sha256.New()
	var buf [8]byte
	for _, row := range rows {
		for _, v := range row {
			binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
			h.Write(buf[:])
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Report is what Run found and did. ContinuationDigest is the SHA-256 of every
// Float64bits in the ContinueRows output; NeuralDigest is the SHA-256 of the
// final Snapshot().Neural JSON. Resume (resume.go) loads OutDir/individual.coimbundle
// in a new process and recomputes both digests.
type Report struct {
	SchemaVersion      string                 `json:"schema_version"`
	Options            Options                `json:"options"`
	Files              map[string]string      `json:"files"` // "store", "params", "protocol" → sha256 hex of the file bytes
	Nodes              int                    `json:"nodes"`
	Edges              int                    `json:"edges"`
	Sets               []simulate.ResolvedSet `json:"sets"` // the input set then the readout set
	Model              modelReport            `json:"model"`
	Memory             resources.Report       `json:"memory"`
	Run                runReport              `json:"run"`
	Mechanisms         Mechanisms             `json:"mechanisms"`
	ContinuationDigest string                 `json:"continuation_digest"`
	NeuralDigest       string                 `json:"neural_digest"`
	Timings            map[string]float64     `json:"timings_ms"` // "load", "variant", "build", "estimate", "train", "continue"
	GoVersion          string                 `json:"go_version"`
	GOOS               string                 `json:"goos"`
	GOARCH             string                 `json:"goarch"`
	NumCPU             int                    `json:"num_cpu"`
}

// Run validates o, refuses an existing OutDir before loading anything, loads the store (connectome.Load), the parameter set
// (params.LoadWithReceipt + CheckGraph) and the compare protocol (DecodeCompareProtocol), builds the original variant, resolves
// the two named sets, builds the model with buildModel (dt from the protocol's continuous block; a non-continuous or non-tanh
// run block is an error), checks memory with memoryPlan/checkMemory before the training (refusal keeps the estimate in the
// error), runs shortTraining into OutDir, then continueRows(ContinueRows) on the in-process individual and fills the digests.
// It writes Report as indented JSON to OutDir/report.json (new file) and returns it. ctx cancellation aborts.
func Run(ctx context.Context, o Options) (Report, error) {
	if ctx == nil {
		return Report{}, errors.New("fullgraph: nil context")
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if err := o.Validate(); err != nil {
		return Report{}, err
	}
	if _, err := os.Stat(o.OutDir); err == nil {
		return Report{}, fmt.Errorf("fullgraph: output directory %q already exists; refuse to overwrite", o.OutDir)
	} else if !os.IsNotExist(err) {
		return Report{}, fmt.Errorf("fullgraph: stat out dir %q: %w", o.OutDir, err)
	}

	loaded, err := LoadModel(ctx, o)
	if err != nil {
		return Report{}, err
	}
	timings := loaded.Timings
	c, p, learnOpts := loaded.Config, loaded.Parameters, loaded.Options

	// Phase 4: estimate
	start := time.Now()
	regions, channels := 0, 0
	if o.Chemistry {
		regions, channels = 1, 1
	}
	plasticEdges := 0
	if o.PlasticEdges > 0 {
		plasticEdges = o.PlasticEdges
	}
	memReport, err := CheckModelMemory(loaded, o.Steps, plasticEdges, regions, channels, o.MaxMemoryMiB)
	timings["estimate"] = time.Since(start).Seconds() * 1000
	if err != nil {
		return Report{Memory: memReport}, err
	}

	// Phase 5: train
	start = time.Now()
	runOpt := runOptions{
		Steps:        o.Steps,
		ContinueRows: o.ContinueRows,
		PlasticEdges: o.PlasticEdges,
		Chemistry:    o.Chemistry,
		MaxCells:     o.MaxCells,
	}
	ind, runRep, err := shortTraining(ctx, c, p, learnOpts, runOpt, o.OutDir)
	if err != nil {
		return Report{}, fmt.Errorf("fullgraph: short training: %w", err)
	}
	timings["train"] = time.Since(start).Seconds() * 1000
	mechanisms := mechanismsOf(ind, runRep)

	// Phase 6: continue
	start = time.Now()
	outputs, err := continueRows(ctx, ind, o.ContinueRows, runRep.Chunk)
	if err != nil {
		return Report{}, fmt.Errorf("fullgraph: continue rows: %w", err)
	}
	timings["continue"] = time.Since(start).Seconds() * 1000

	contDigest := continuationDigest(outputs)
	neurDigest, err := neuralDigest(ind.Snapshot().Neural)
	if err != nil {
		return Report{}, fmt.Errorf("fullgraph: neural digest: %w", err)
	}

	report := Report{
		SchemaVersion:      ReportSchemaVersion,
		Options:            o,
		Files:              loaded.Files,
		Nodes:              loaded.Nodes,
		Edges:              loaded.Edges,
		Sets:               loaded.Sets,
		Model:              loaded.model,
		Memory:             memReport,
		Run:                runRep,
		Mechanisms:         mechanisms,
		ContinuationDigest: contDigest,
		NeuralDigest:       neurDigest,
		Timings:            timings,
		GoVersion:          runtime.Version(),
		GOOS:               runtime.GOOS,
		GOARCH:             runtime.GOARCH,
		NumCPU:             runtime.NumCPU(),
	}

	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return Report{}, fmt.Errorf("fullgraph: marshal report: %w", err)
	}
	data = append(data, '\n')
	if err := refuseExistingWrite(filepath.Join(o.OutDir, "report.json"), data); err != nil {
		return Report{}, fmt.Errorf("fullgraph: write report: %w", err)
	}
	return report, nil
}
