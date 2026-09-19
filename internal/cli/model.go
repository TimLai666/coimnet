package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/resources"
)

// modelInspectDefaultSteps is the declared history window of the estimate when
// the caller does not name one; it counts model steps, not a goal length.
const modelInspectDefaultSteps = 1000

// runModel dispatches the model command group: inspect reads one model package
// or individual snapshot read-only, validate recomputes its integrity. Files
// are never written and an unknown subcommand is a usage error.
func runModel(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		_, err := fmt.Fprintln(stdout, "Usage: coimnet model inspect --path FILE [--steps 1000]\n       coimnet model validate --path FILE\nInspect or verify one model package or individual snapshot with the schema's strict decoder.\nRun 'coimnet model inspect --help' or 'coimnet model validate --help' for options, an example and the error list.")
		return err
	}
	switch args[0] {
	case "inspect":
		return runModelInspect(ctx, args[1:], stdout, stderr)
	case "validate":
		return runModelValidate(ctx, args[1:], stdout, stderr)
	}
	return &ExitError{Code: exitUsage, Err: fmt.Errorf("unknown model command %q; use coimnet model inspect or coimnet model validate", args[0])}
}

// modelEnvelope carries the two envelope fields the commands report, read
// before the strict decoder decides which loader owns the file.
type modelEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Checksum      string `json:"checksum"`
}

// peekModelEnvelope reads the envelope header of FILE. It only routes and
// reports; the strict loader re-reads and fully validates the same file.
func peekModelEnvelope(path string) (modelEnvelope, error) {
	var env modelEnvelope
	data, err := os.ReadFile(path)
	if err != nil {
		return env, err
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return env, err
	}
	return env, nil
}

// modelDocument is one loaded artefact behind the two commands: the file's
// schema version and whichever loader owned it. Exactly one of the two
// pointers is set.
type modelDocument struct {
	schemaVersion string
	pkg           *checkpoint.ModelPackage
	individual    *learning.IndividualSnapshot
}

func (d modelDocument) kind() string {
	if d.pkg != nil {
		return "model_package"
	}
	return "individual_snapshot"
}

// loadModelDocument reads FILE through the strict loader matched by the
// envelope schema, so a reader who opened the wrong kind is told which kind it
// is instead of guessing. A model package goes to checkpoint.LoadModelPackage;
// everything else goes to checkpoint.LoadIndividual, whose kind error names
// the artefact actually opened. An unreadable or unparseable file falls there
// too and reports the loader's own error unchanged. The reported schema
// version is the envelope's, because that is the version that identifies the
// file and pairs with its checksum.
func loadModelDocument(ctx context.Context, path string) (modelDocument, modelEnvelope, error) {
	env, err := peekModelEnvelope(path)
	if err != nil {
		return modelDocument{}, env, err
	}
	doc := modelDocument{schemaVersion: env.SchemaVersion}
	if env.SchemaVersion == checkpoint.ModelPackageSchemaVersion {
		pkg, loadErr := checkpoint.LoadModelPackage(ctx, path)
		doc.pkg = &pkg
		return doc, env, loadErr
	}
	snapshot, loadErr := checkpoint.LoadIndividual(ctx, path)
	doc.individual = &snapshot
	return doc, env, loadErr
}

// runModelInspect reads one file the model path declares and prints an
// indented JSON report; it never changes the file.
func runModelInspect(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var path string
	var steps int
	fs := flag.NewFlagSet("model inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&path, "path", "", "path to the model package or individual snapshot to inspect (required)")
	fs.IntVar(&steps, "steps", modelInspectDefaultSteps, "declared model steps for the memory estimate, at least 0 (default "+fmt.Sprint(modelInspectDefaultSteps)+")")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet model inspect --path FILE [--steps 1000]")
		fmt.Fprintln(usageOutput, "Reads one model package or individual snapshot with the schema's strict decoder and prints an indented JSON report: schema version, kind, topology (nodes, edges, fingerprint), per-part parameter counts and weight statistics, declared modes, the evidence registry and the Units and CompatibleVersions mapping when the file is a model package, and the memory estimate of one declared run.")
		fmt.Fprintln(usageOutput, "The report changes nothing. The estimate covers the declared arrays only and is not a resident set size; --steps is the backward history window it sizes.")
		fmt.Fprintln(usageOutput, "Example: coimnet model inspect --path model.coimpkg --steps 1000 > inspect.json")
		fmt.Fprintln(usageOutput, "Errors: missing --path, an unreadable or invalid file, an artefact kind the strict decoder refuses, a negative --steps, cancellation or output failure. Usage errors exit with status 1 and name the flag.")
		fmt.Fprintln(usageOutput, "Options and their defaults:")
		fs.SetOutput(usageOutput)
		fs.PrintDefaults()
		fs.SetOutput(stderr)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	if fs.NArg() != 0 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("model inspect takes no positional arguments; use coimnet model inspect --help")}
	}
	if path == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--path is required; use coimnet model inspect --help")}
	}
	if steps < 0 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--steps must not be negative")}
	}
	doc, _, err := loadModelDocument(ctx, path)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("model inspect --path %s: %w", path, err)}
	}
	report, err := modelInspectOf(path, doc, steps)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	return writeJSON(stdout, report)
}

// modelInspectTopology is the connectivity summary of one model or snapshot.
type modelInspectTopology struct {
	Nodes       int    `json:"nodes"`
	Edges       int    `json:"edges"`
	Fingerprint string `json:"fingerprint"`
}

// modelInspectParameters counts the learnable arrays a model declares and the
// statistics of its base weights. The count is the sum of the five listed
// parts; a model with no edges reports zero weight statistics.
type modelInspectParameters struct {
	Count   int     `json:"count"`
	Weights int     `json:"weights"`
	Bias    int     `json:"bias"`
	LogTau  int     `json:"log_tau"`
	Encoder int     `json:"encoder"`
	Readout int     `json:"readout"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Mean    float64 `json:"mean"`
}

// modelInspectModes names the mechanisms a model actually carries. Core is the
// declared core: continuous, lif, mixed, or vector when a continuous
// configuration selects nodes with more than one state value.
type modelInspectModes struct {
	Plastic  bool   `json:"plastic"`
	Chemical bool   `json:"chemical"`
	Core     string `json:"core"`
}

// modelInspectMapping carries the two declarations only a model package owns:
// the units and the artefact schema versions it can seed.
type modelInspectMapping struct {
	Units              checkpoint.Units              `json:"units"`
	CompatibleVersions checkpoint.CompatibleVersions `json:"compatible_versions"`
}

// modelInspectReport is the whole output of model inspect.
type modelInspectReport struct {
	SchemaVersion string                 `json:"schema_version"`
	Kind          string                 `json:"kind"`
	Topology      modelInspectTopology   `json:"topology"`
	Parameters    modelInspectParameters `json:"parameters"`
	Modes         modelInspectModes      `json:"modes"`
	Evidence      []string               `json:"evidence,omitempty"`
	Mapping       *modelInspectMapping   `json:"mapping,omitempty"`
	Resources     resources.Report       `json:"resources"`
}

// modelInspectOf builds the report from one loaded document and the declared
// step count. The topology and parameter statistics come from the runtime
// model; the resource estimate is resources.Estimate over a plan sized from
// the same configuration and parameters.
func modelInspectOf(path string, doc modelDocument, steps int) (modelInspectReport, error) {
	report := modelInspectReport{SchemaVersion: doc.schemaVersion, Kind: doc.kind()}
	var (
		config     learning.Config
		parameters learning.Parameters
	)
	if doc.pkg != nil {
		pkg := doc.pkg
		report.Topology = modelInspectTopology{Nodes: pkg.Topology.Nodes, Edges: pkg.Topology.Edges, Fingerprint: pkg.Topology.SHA256}
		report.Parameters = modelParameterStats(pkg.Parameters)
		report.Modes = modelModes(pkg.Config, false, false)
		report.Evidence = append([]string(nil), pkg.EvidenceRegistry...)
		report.Mapping = &modelInspectMapping{Units: pkg.Units, CompatibleVersions: pkg.CompatibleVersions}
		config = pkg.Config
		parameters = pkg.Parameters
	} else {
		snapshot := doc.individual
		report.Topology = modelInspectTopology{Nodes: modelNodesOf(snapshot.Config), Edges: modelEdgesOf(snapshot.Config), Fingerprint: snapshot.ConfigHash}
		report.Parameters = modelParameterStats(snapshot.Parameters)
		report.Modes = modelModes(snapshot.Config, snapshot.Plastic != nil, snapshot.Chemical != nil)
		config = snapshot.Config
		parameters = snapshot.Parameters
	}
	estimate, err := resources.Estimate(modelInspectPlan(config, parameters, steps))
	if err != nil {
		return report, fmt.Errorf("estimate %s: %w", path, err)
	}
	report.Resources = estimate
	return report, nil
}

// modelNodesOf and modelEdgesOf read whichever core a configuration declares,
// mirroring the checkpoint fingerprint helpers for the individual snapshot.
func modelNodesOf(c learning.Config) int {
	if c.LIF != nil {
		return c.LIF.Nodes
	}
	if c.Mixed != nil {
		return c.Mixed.Nodes
	}
	return c.Dynamics.Nodes
}

func modelEdgesOf(c learning.Config) int {
	if c.LIF != nil {
		return len(c.LIF.Sources)
	}
	if c.Mixed != nil {
		return len(c.Mixed.Sources)
	}
	return len(c.Dynamics.Sources)
}

// modelParameterStats builds the parameter breakdown of one model. Count is
// the sum of the five named parts; weight statistics over an empty weight
// array are zero.
func modelParameterStats(p learning.Parameters) modelInspectParameters {
	weights, bias, logTau := len(p.Core.Weights), len(p.Core.Bias), len(p.Core.LogTau)
	encoder, readout := len(p.Encoder), len(p.Readout)
	min, max, mean := modelWeightStats(p.Core.Weights)
	return modelInspectParameters{
		Count:   weights + bias + logTau + encoder + readout,
		Weights: weights,
		Bias:    bias,
		LogTau:  logTau,
		Encoder: encoder,
		Readout: readout,
		Min:     min,
		Max:     max,
		Mean:    mean,
	}
}

// modelWeightStats reports the minimum, maximum and mean of one weight array.
// An empty array has no statistics and reports all zeros.
func modelWeightStats(values []float64) (min, max, mean float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	min, max, sum := values[0], values[0], 0.0
	for _, value := range values {
		if value < min {
			min = value
		}
		if value > max {
			max = value
		}
		sum += value
	}
	return min, max, sum / float64(len(values))
}

// modelModes names the mechanisms one model carries, with the core named by
// whichever configuration block is set.
func modelModes(c learning.Config, plastic, chemical bool) modelInspectModes {
	core := "continuous"
	if c.LIF != nil {
		core = "lif"
	} else if c.Mixed != nil {
		core = "mixed"
	} else if c.Dynamics.StateDimension > 1 {
		core = "vector"
	}
	return modelInspectModes{Plastic: plastic, Chemical: chemical, Core: core}
}

// modelInspectPlan is the declared run the estimate sizes: one individual on
// the CPU in float64 with no optimizer and no local rule, because inspect
// reads a model and does not configure a run, and the caller's --steps as the
// backward history window. No buffer allowance is estimated.
func modelInspectPlan(c learning.Config, p learning.Parameters, steps int) resources.Plan {
	return resources.Plan{
		Nodes:         modelNodesOf(c),
		Edges:         modelEdgesOf(c),
		StateDim:      1,
		Individuals:   1,
		HistorySteps:  steps,
		Precision:     resources.PrecisionF64,
		Optimizer:     resources.OptimizerNone,
		MaxDelay:      maxDelayOf(c),
		EncoderValues: len(p.Encoder),
		ReadoutValues: len(p.Readout),
	}
}

// runModelValidate recomputes the schema checks, the SHA-256 envelope checksum
// and the topology fingerprint of one file and prints the verdict. A file
// that does not verify is a command failure, not a usage mistake, so its error
// is returned without the usage status.
func runModelValidate(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var path string
	fs := flag.NewFlagSet("model validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&path, "path", "", "path to the model package or individual snapshot to verify (required)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet model validate --path FILE")
		fmt.Fprintln(usageOutput, "Rereads one model package or individual snapshot with the schema's strict decoder, which verifies the JSON shape, the SHA-256 envelope checksum and the topological fingerprint, and prints one indented JSON verdict: valid, schema_version and checksum. A version this build does not know is refused rather than read optimistically.")
		fmt.Fprintln(usageOutput, "The command changes nothing. Any schema difference from the envelope is an error, so the reported checksum is the one the file already claimed after that claim was verified.")
		fmt.Fprintln(usageOutput, "Example: coimnet model validate --path model.coimpkg > valid.json")
		fmt.Fprintln(usageOutput, "Errors: missing --path, an unreadable file, an invalid document, a checksum or fingerprint mismatch, an incompatible artefact kind, cancellation or output failure. A failed validation exits nonzero; only usage errors name the flag.")
		fmt.Fprintln(usageOutput, "Options and their defaults:")
		fs.SetOutput(usageOutput)
		fs.PrintDefaults()
		fs.SetOutput(stderr)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	if fs.NArg() != 0 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("model validate takes no positional arguments; use coimnet model validate --help")}
	}
	if path == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--path is required; use coimnet model validate --help")}
	}
	doc, envelope, err := loadModelDocument(ctx, path)
	if err != nil {
		return fmt.Errorf("model validate --path %s: %w", path, err)
	}
	verdict := struct {
		Valid         bool   `json:"valid"`
		SchemaVersion string `json:"schema_version"`
		Checksum      string `json:"checksum"`
	}{
		Valid:         true,
		SchemaVersion: doc.schemaVersion,
		Checksum:      envelope.Checksum,
	}
	return writeJSON(stdout, verdict)
}
