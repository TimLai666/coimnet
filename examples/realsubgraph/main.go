package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	osSignal "os/signal"
	"strings"
	"syscall"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/signal"
)

const (
	exampleSchemaVersion = "coimnet-real-subgraph-example/v1"
	defaultProfile       = "real-subgraph"
	defaultUpdates       = 20
	maxUpdates           = 1000
	minExampleNodes      = 1
	maxExampleNodes      = 2000
	minExampleEdges      = 1
	maxExampleEdges      = 100000
)

var fixedStoreLimits = connectome.StoreLimits{
	MaxFileBytes:   64 << 20,
	MaxFooterBytes: 1 << 20,
	MaxMemoryBytes: 128 << 20,
}

type exampleReport struct {
	SchemaVersion string                  `json:"schema_version"`
	Profile       string                  `json:"profile"`
	NodeCount     uint64                  `json:"node_count"`
	EdgeCount     uint64                  `json:"edge_count"`
	PredicateHash string                  `json:"predicate_hash"`
	NeuronIDs     []signal.NeuronID       `json:"neuron_ids"`
	GraphReport   connectome.GraphReport  `json:"graph_report"`
	Store         connectome.StoreReceipt `json:"store"`
	Model         exampleModelReport      `json:"model"`
	Training      exampleTrainingReport   `json:"training"`
	Assumptions   exampleAssumptions      `json:"assumptions"`
}

type exampleModelReport struct {
	Config                     learning.Config  `json:"config"`
	Options                    learning.Options `json:"options"`
	TopologyFingerprint        string           `json:"topology_fingerprint"`
	InitialParameterHash       string           `json:"initial_parameter_hash"`
	FinalParameterHash         string           `json:"final_parameter_hash"`
	InitialWeightsHash         string           `json:"initial_weights_hash"`
	FinalWeightsHash           string           `json:"final_weights_hash"`
	InitialFrozenParameterHash string           `json:"initial_frozen_parameter_hash"`
	FinalFrozenParameterHash   string           `json:"final_frozen_parameter_hash"`
}

type exampleTrainingReport struct {
	Updates        int                   `json:"updates"`
	BeforeMeanLoss float64               `json:"before_mean_loss"`
	AfterMeanLoss  float64               `json:"after_mean_loss"`
	LossImproved   bool                  `json:"loss_improved"`
	Steps          []exampleTrainingStep `json:"steps"`
}

type exampleTrainingStep struct {
	Index           int     `json:"index"`
	PulseSign       float64 `json:"pulse_sign"`
	Target          float64 `json:"target"`
	Loss            float64 `json:"loss"`
	GradientNorm    float64 `json:"gradient_norm"`
	UpdateNorm      float64 `json:"update_norm"`
	WeightDeltaNorm float64 `json:"weight_delta_norm"`
}

type exampleAssumptions struct {
	Task          string `json:"task"`
	Normalization string `json:"normalization"`
	Delay         string `json:"delay"`
	Sign          string `json:"sign"`
}

type pulseExample struct {
	sign   float64
	target float64
	input  [][]float64
}

func main() {
	ctx, stop := osSignal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run executes the standalone real-subgraph example. It only reads the store
// and writes one JSON report to stdout; model parameters and files are never
// persisted by this example.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if ctx == nil || stdout == nil || stderr == nil {
		return errors.New("context and output writers are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fs := flag.NewFlagSet("realsubgraph", flag.ContinueOnError)
	diagnosticOutput := &errorTrackingWriter{Writer: stderr}
	fs.SetOutput(diagnosticOutput)
	usageOutput := &errorTrackingWriter{Writer: stdout}
	var storePath, expectedPredicateHash, profile string
	var updates int
	fs.StringVar(&storePath, "store", "", "required graph store file produced by data import --out-store")
	fs.StringVar(&expectedPredicateHash, "expected-predicate-hash", "", "required lowercase SHA-256 of annotations.class == \"ALIN\"")
	fs.IntVar(&updates, "updates", defaultUpdates, "alternating synthetic updates (1..1000)")
	fs.StringVar(&profile, "profile", defaultProfile, "data profile: real-subgraph (default) or fixture (offline test data)")
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: go run ./examples/realsubgraph --store FILE --expected-predicate-hash SHA256 [--updates 20] [--profile real-subgraph|fixture]")
		fmt.Fprintln(usageOutput, "Loads one validated graph store, checks the ALIN selection, MaleCNS v1.0 source fingerprints and bounded graph size, then runs a four-step synthetic signed-pulse task. The task is an engineering integration check, not a biological behavior claim.")
		fmt.Fprintln(usageOutput, "Example: go run ./examples/realsubgraph --store graph.coimgraph --expected-predicate-hash HASH")
		fmt.Fprintln(usageOutput, "Limitations: fixed CPU continuous model; DT=0.1, tau=1, tanh, zero delay; 1..2000 nodes; 1..100000 edges; 1..1000 updates; fixed store limits 64 MiB file, 1 MiB footer, 128 MiB accounted memory; no files or model parameters are written.")
		fmt.Fprintln(usageOutput, "Errors: missing or malformed options, unknown flags, extra arguments, unreadable or corrupt store, predicate mismatch, unsupported profile, duplicate pairs, null or negative weights, graph bounds, cancellation, numerical failure or output failure.")
		fmt.Fprintln(usageOutput, "Options:")
		fs.SetOutput(usageOutput)
		fs.PrintDefaults()
		fs.SetOutput(diagnosticOutput)
	}
	if len(args) == 0 {
		fs.Usage()
		if err := usageOutput.Err(); err != nil {
			return fmt.Errorf("write usage: %w", err)
		}
		return errors.New("--store and --expected-predicate-hash are required")
	}
	if err := fs.Parse(args); err != nil {
		if diagnosticErr := diagnosticOutput.Err(); diagnosticErr != nil {
			return errors.Join(err, fmt.Errorf("write flag diagnostic: %w", diagnosticErr))
		}
		if usageErr := usageOutput.Err(); usageErr != nil {
			return fmt.Errorf("write usage: %w", usageErr)
		}
		if errors.Is(err, flag.ErrHelp) {
			if fs.NArg() != 0 {
				return errors.New("unexpected positional arguments")
			}
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if storePath == "" || expectedPredicateHash == "" {
		return errors.New("--store and --expected-predicate-hash are required; use --help")
	}
	if profile != "real-subgraph" && profile != "fixture" {
		return fmt.Errorf("unsupported profile %q; choose real-subgraph or fixture", profile)
	}
	if updates < 1 || updates > maxUpdates {
		return fmt.Errorf("--updates must be between 1 and %d", maxUpdates)
	}
	if err := validateSHA256(expectedPredicateHash); err != nil {
		return fmt.Errorf("--expected-predicate-hash: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return runStore(ctx, storePath, expectedPredicateHash, profile, updates, stdout)
}

func runStore(ctx context.Context, path, expectedPredicateHash, profile string, updates int, stdout io.Writer) error {
	graph, receipt, err := connectome.LoadWithReceipt(ctx, path, fixedStoreLimits)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	nodeCount, edgeCount := graph.NodeCount(), graph.EdgeCount()
	if nodeCount < minExampleNodes || nodeCount > maxExampleNodes {
		return fmt.Errorf("graph has %d nodes; real-subgraph example requires %d..%d", nodeCount, minExampleNodes, maxExampleNodes)
	}
	if edgeCount < minExampleEdges || edgeCount > maxExampleEdges {
		return fmt.Errorf("graph has %d edges; real-subgraph example requires %d..%d", edgeCount, minExampleEdges, maxExampleEdges)
	}
	report := graph.Report()
	if report.Predicate.Hash != expectedPredicateHash {
		return fmt.Errorf("predicate hash %s does not match expected %s", report.Predicate.Hash, expectedPredicateHash)
	}
	if report.Annotated.DuplicatePairs > 0 {
		return fmt.Errorf("annotated graph contains %d duplicate pairs", report.Annotated.DuplicatePairs)
	}
	if profile == "real-subgraph" {
		if report.Dataset != "MaleCNS" || report.Namespace != "malecns-v1.0" || report.SourceVersion != "v1.0" || report.Predicate.Canonical != `annotations.class == "ALIN"` {
			return fmt.Errorf("store provenance is not the MaleCNS ALIN real-subgraph profile")
		}
		// Fixed official inputs for this example; the SDK accepts other manifests.
		expectedSources := map[connectome.FileRole]string{
			connectome.RoleWeights:           "e35da783d1c686b2b58b3b87cd6a403ae43bfcfba8bff28e08ef752c1a56afc1",
			connectome.RoleAnnotations:       "2177e246113e4cfbf1e7772ec37c6da1955ff22e8063d0b1f833101f99a9a3b2",
			connectome.RoleNeurotransmitters: "95c9289220663abeb3409f3ad9e5a7f8a53f8093f5139d15502cd08da8879621",
		}
		for _, source := range report.Sources {
			expected, ok := expectedSources[source.Role]
			if !ok || source.SHA256 != expected || source.SHA256After != expected || !source.FingerprintStable {
				return fmt.Errorf("store provenance differs from the official %s source fingerprint", source.Role)
			}
			delete(expectedSources, source.Role)
		}
		if len(expectedSources) != 0 {
			return errors.New("store provenance is missing official source fingerprints")
		}
	}

	sources := make([]int, 0, edgeCount)
	targets := make([]int, 0, edgeCount)
	rawWeights := make([]float64, 0, edgeCount)
	incoming := make([]float64, nodeCount)
	var previousSource, previousTarget uint64
	havePair := false
	err = graph.StreamAnnotatedEdges(ctx, func(edge connectome.EdgeRecord) error {
		if !edge.Weight.Valid {
			return errors.New("annotated graph contains a null weight")
		}
		if edge.Weight.Value < 0 {
			return fmt.Errorf("annotated graph contains a negative weight at absolute row %d", edge.Position.AbsoluteRow)
		}
		if edge.Source >= nodeCount || edge.Target >= nodeCount {
			return fmt.Errorf("edge endpoint exceeds graph node count")
		}
		if havePair && edge.Source == previousSource && edge.Target == previousTarget {
			return fmt.Errorf("annotated graph contains a duplicate pair (%d,%d)", edge.Source, edge.Target)
		}
		previousSource, previousTarget, havePair = edge.Source, edge.Target, true
		raw := float64(edge.Weight.Value)
		target := int(edge.Target)
		incoming[target] += raw
		if !finite(incoming[target]) {
			return errors.New("incoming raw weight sum is non-finite")
		}
		sources = append(sources, int(edge.Source))
		targets = append(targets, target)
		rawWeights = append(rawWeights, raw)
		return nil
	})
	if err != nil {
		return err
	}
	if len(sources) != int(edgeCount) {
		return fmt.Errorf("streamed %d edges, store declared %d", len(sources), edgeCount)
	}

	coreWeights := make([]float64, len(rawWeights))
	for i, raw := range rawWeights {
		denominator := incoming[targets[i]]
		if denominator != 0 {
			coreWeights[i] = .1 * raw / denominator
		}
	}
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      int(nodeCount),
			Sources:    sources,
			Targets:    targets,
			Delays:     make([]int, len(sources)),
			DT:         .1,
			Activation: "tanh",
		},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: make([]int, nodeCount),
	}
	for i := range config.ReadoutNodes {
		config.ReadoutNodes[i] = i
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: coreWeights,
			Bias:    make([]float64, nodeCount),
			LogTau:  make([]float64, nodeCount),
		},
		Encoder: make([]float64, nodeCount),
		Readout: make([]float64, nodeCount),
	}
	for i := range parameters.Encoder {
		parameters.Encoder[i] = .1
		parameters.Readout[i] = 1 / float64(nodeCount)
	}
	options := learning.DefaultOptions()
	options.Trainable = learning.Trainable{Weights: true}
	network, err := learning.NewNetwork(config)
	if err != nil {
		return err
	}
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		return err
	}
	initial := trainer.Snapshot()
	initialHash, err := hashJSON(initial.Parameters)
	if err != nil {
		return fmt.Errorf("hash initial parameters: %w", err)
	}
	topologyHash, err := hashJSON(config)
	if err != nil {
		return fmt.Errorf("hash topology: %w", err)
	}
	examples := []pulseExample{
		{sign: 1, target: .2, input: [][]float64{{.25}, {0}, {0}, {0}}},
		{sign: -1, target: -.2, input: [][]float64{{-.25}, {0}, {0}, {0}}},
	}
	beforeLoss, err := meanLoss(ctx, network, initial.Parameters, examples)
	if err != nil {
		return err
	}
	steps := make([]exampleTrainingStep, 0, updates)
	previousWeights := append([]float64(nil), initial.Parameters.Core.Weights...)
	for i := 0; i < updates; i++ {
		example := examples[i%len(examples)]
		step, err := trainer.Step(ctx, example.input, []float64{example.target})
		if err != nil {
			return err
		}
		snapshot := trainer.Snapshot()
		weightDelta, err := vectorDistance(previousWeights, snapshot.Parameters.Core.Weights)
		if err != nil {
			return fmt.Errorf("weight delta at update %d: %w", i+1, err)
		}
		if !finite(step.Loss) || !finite(step.GradientNorm) || !finite(step.UpdateNorm) || !finite(weightDelta) {
			return fmt.Errorf("non-finite training result at update %d", i+1)
		}
		steps = append(steps, exampleTrainingStep{Index: i + 1, PulseSign: example.sign, Target: example.target, Loss: step.Loss, GradientNorm: step.GradientNorm, UpdateNorm: step.UpdateNorm, WeightDeltaNorm: weightDelta})
		previousWeights = append(previousWeights[:0], snapshot.Parameters.Core.Weights...)
	}
	final := trainer.Snapshot()
	afterLoss, err := meanLoss(ctx, network, final.Parameters, examples)
	if err != nil {
		return err
	}
	finalHash, err := hashJSON(final.Parameters)
	if err != nil {
		return fmt.Errorf("hash final parameters: %w", err)
	}
	initialWeightsHash, err := hashJSON(initial.Parameters.Core.Weights)
	if err != nil {
		return fmt.Errorf("hash initial weights: %w", err)
	}
	finalWeightsHash, err := hashJSON(final.Parameters.Core.Weights)
	if err != nil {
		return fmt.Errorf("hash final weights: %w", err)
	}
	initialFrozenHash, err := hashJSON(frozenParameters(initial.Parameters))
	if err != nil {
		return fmt.Errorf("hash initial frozen parameters: %w", err)
	}
	finalFrozenHash, err := hashJSON(frozenParameters(final.Parameters))
	if err != nil {
		return fmt.Errorf("hash final frozen parameters: %w", err)
	}
	if initialFrozenHash != finalFrozenHash {
		return errors.New("frozen parameter groups changed during weights-only training")
	}
	ids := graph.NeuronIDs()
	if len(ids) != int(nodeCount) {
		return fmt.Errorf("graph returned %d neuron IDs, want %d", len(ids), nodeCount)
	}
	result := exampleReport{
		SchemaVersion: exampleSchemaVersion,
		Profile:       profile,
		NodeCount:     nodeCount,
		EdgeCount:     edgeCount,
		PredicateHash: report.Predicate.Hash,
		NeuronIDs:     ids,
		GraphReport:   report,
		Store:         receipt,
		Model: exampleModelReport{
			Config:                     config,
			Options:                    options,
			TopologyFingerprint:        topologyHash,
			InitialParameterHash:       initialHash,
			FinalParameterHash:         finalHash,
			InitialWeightsHash:         initialWeightsHash,
			FinalWeightsHash:           finalWeightsHash,
			InitialFrozenParameterHash: initialFrozenHash,
			FinalFrozenParameterHash:   finalFrozenHash,
		},
		Training: exampleTrainingReport{Updates: updates, BeforeMeanLoss: beforeLoss, AfterMeanLoss: afterLoss, LossImproved: afterLoss < beforeLoss, Steps: steps},
		Assumptions: exampleAssumptions{
			Task:          "synthetic four-step signed pulse; inputs are [+0.25,0,0,0] and [-0.25,0,0,0] with targets +0.2 and -0.2; this is not a biological behavior claim",
			Normalization: "core weight = 0.1 * non-negative raw weight / the target neuron's incoming raw-weight sum; zero denominator gives weight 0",
			Delay:         "continuous tanh core with DT=0.1, tau=1 and delay 0 on every edge",
			Sign:          "raw edge weights must be finite and non-negative for initialization; learned weights have no sign constraint, no biological sign is inferred, and pulse target signs match pulse input signs",
		},
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func meanLoss(ctx context.Context, network *learning.Network, parameters learning.Parameters, examples []pulseExample) (float64, error) {
	if len(examples) == 0 {
		return 0, errors.New("no pulse examples")
	}
	var total float64
	for _, example := range examples {
		loss, _, err := network.LossGradient(ctx, parameters, example.input, []float64{example.target}, 0)
		if err != nil {
			return 0, err
		}
		if !finite(loss) {
			return 0, errors.New("non-finite mean loss")
		}
		total += loss
	}
	return total / float64(len(examples)), nil
}

func vectorDistance(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector lengths %d and %d differ", len(a), len(b))
	}
	var distance float64
	for i, value := range a {
		if !finite(value) || !finite(b[i]) {
			return 0, fmt.Errorf("non-finite vector value at %d", i)
		}
		distance = math.Hypot(distance, b[i]-value)
	}
	return distance, nil
}

func hashJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func validateSHA256(value string) error {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return fmt.Errorf("must be %d lowercase hexadecimal characters", sha256.Size*2)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("must be lowercase hexadecimal: %w", err)
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

type frozenParameterSet struct {
	Bias    []float64 `json:"bias"`
	LogTau  []float64 `json:"log_tau"`
	Encoder []float64 `json:"encoder"`
	Readout []float64 `json:"readout"`
}

func frozenParameters(parameters learning.Parameters) frozenParameterSet {
	return frozenParameterSet{
		Bias:    parameters.Core.Bias,
		LogTau:  parameters.Core.LogTau,
		Encoder: parameters.Encoder,
		Readout: parameters.Readout,
	}
}

type errorTrackingWriter struct {
	io.Writer
	err error
}

func (w *errorTrackingWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.Writer.Write(p)
	if err != nil {
		w.err = err
	}
	return n, err
}

func (w *errorTrackingWriter) Err() error { return w.err }
