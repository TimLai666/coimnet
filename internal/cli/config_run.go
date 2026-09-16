package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/config"
	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/resources"
)

// The exit statuses this command distinguishes. A refusal is not a usage
// mistake: the configuration was understood, the report was printed, and the
// declared limits were not met.
const (
	exitOK      = 0
	exitUsage   = 1
	exitRefused = 2
)

// ExitError carries the process exit status one command asks for. Commands that
// return a plain error keep the historical status 1.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// ExitCode reports the status a caller should exit with for err.
func ExitCode(err error) int {
	if err == nil {
		return exitOK
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	return exitUsage
}

// The check statuses and verdicts of one dry run report.
const (
	statusOK      = "ok"
	statusFailed  = "failed"
	statusSkipped = "skipped"

	verdictOK      = "ok"
	verdictRefused = "refused"
)

// dryRunCheck is one named precondition and what became of it. A skipped check
// is one this stage deliberately does not perform, and its detail says why.
type dryRunCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// dryRunReport is the whole output of run --dry-run.
type dryRunReport struct {
	SchemaVersion string            `json:"schema_version"`
	Config        json.RawMessage   `json:"config"`
	Provenance    map[string]string `json:"provenance"`
	Checks        []dryRunCheck     `json:"checks"`
	Estimate      resources.Report  `json:"estimate"`
	Verdict       string            `json:"verdict"`
}

// setFlags collects repeated --set overrides in the order they were written.
type setFlags []string

func (s *setFlags) String() string { return strings.Join(*s, " ") }

func (s *setFlags) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func runConfigRun(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return runConfigRunWith(ctx, args, stdout, stderr, nil)
}

// runConfigRunWith is the entry point the tests use to hand in a teacher client
// and prove that a dry run never reaches it. The command itself passes nil,
// because no teacher client exists yet; the real one is ticket 27.
func runConfigRunWith(ctx context.Context, args []string, stdout, stderr io.Writer, teacher config.TeacherCaller) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var path string
	var dryRun bool
	var overrides setFlags
	fs.StringVar(&path, "config", "", "required path to a coimnet-config/v1 document (default none)")
	fs.BoolVar(&dryRun, "dry-run", false, "expand and check the configuration without running anything (default false)")
	fs.Var(&overrides, "set", "override one field, written section.field=value; repeatable, highest precedence (default none)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet run --config FILE [--set section.field=value]... --dry-run")
		fmt.Fprintln(usageOutput, "Reads one strictly validated coimnet-config/v1 document and expands it over four layers, in the fixed order default, file, environment (COIMNET_SECTION_FIELD), command line (--set). Unknown fields, duplicate keys, null required values and names no implementation answers to are all refused. A secret is only ever a reference of the form {\"ref\": \"env:NAME\"}; its value is never read or printed here.")
		fmt.Fprintln(usageOutput, "With --dry-run it resolves the task generator, loads the declared model package or graph store, builds the pre-launch memory estimate and checks it against resources.max_memory_mib, then prints one coimnet-dry-run/v1 JSON document holding the expanded configuration, the provenance of every leaf, every check and the estimate with its formulas. A dry run has no side effects: it writes no file, creates no output directory, changes no parameter and calls no teacher.")
		fmt.Fprintln(usageOutput, "Without --dry-run the command reports that executing a configuration is not implemented in this stage.")
		fmt.Fprintln(usageOutput, "Exit status: 0 when every check passes, 2 when the run is refused (a failed check, or an estimate above the declared limit), 1 for a usage or configuration error.")
		fmt.Fprintln(usageOutput, "Example: coimnet run --config runs/fixture.json --set resources.max_memory_mib=4096 --dry-run > dry-run.json")
		fmt.Fprintln(usageOutput, "Errors: missing or unreadable configuration, invalid or unknown configuration fields, an override that addresses no field, an unreadable model package or graph store, an estimate above the declared memory limit, cancellation or output failure.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("run takes no positional arguments; use run --help")}
	}
	if path == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("run requires --config; use run --help")}
	}
	resolved, err := config.Load(ctx, path, os.Environ(), overrides)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	if !dryRun {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("executing a configuration is not implemented in this stage: use coimnet train or coimnet simulate to run a model, and run --dry-run to expand and check this configuration")}
	}
	report, err := dryRunOf(ctx, path, resolved, teacher)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	if err := writeJSON(stdout, report); err != nil {
		return err
	}
	if report.Verdict != verdictOK {
		return &ExitError{Code: exitRefused, Err: fmt.Errorf("the declared run was refused; see the checks in the printed coimnet-dry-run/v1 report")}
	}
	return nil
}

// dryRunOf performs every check and builds the report. It opens the declared
// model file and nothing else, and it creates nothing.
func dryRunOf(ctx context.Context, path string, resolved config.Resolved, teacher config.TeacherCaller) (dryRunReport, error) {
	c := resolved.Config
	encoded, err := json.Marshal(c)
	if err != nil {
		return dryRunReport{}, fmt.Errorf("marshal the expanded configuration: %w", err)
	}
	checks := []dryRunCheck{{
		Name:   "configuration",
		Status: statusOK,
		Detail: fmt.Sprintf("%s read as %s; %d leaves resolved over the four layers", path, c.SchemaVersion, len(resolved.Provenance)),
	}}

	name := c.Task.Name + "/" + c.Task.Version
	generator, err := config.Generator(name)
	if err != nil {
		checks = append(checks, dryRunCheck{"task_generator", statusFailed, err.Error()})
	} else {
		checks = append(checks, dryRunCheck{"task_generator", statusOK, fmt.Sprintf("%s resolves to %s", name, generator.Description)})
	}

	shape, modelDetail, modelErr := loadModelShape(ctx, c)
	if modelErr != nil {
		checks = append(checks, dryRunCheck{"model", statusFailed, modelErr.Error()})
	} else {
		checks = append(checks, dryRunCheck{"model", statusOK, modelDetail})
	}

	var estimate resources.Report
	if modelErr != nil {
		checks = append(checks, dryRunCheck{"resources", statusSkipped, "the model did not load, so no plan was sized and nothing was estimated"})
	} else {
		plan := planFor(c, shape)
		estimate, err = resources.Estimate(plan)
		if err != nil {
			checks = append(checks, dryRunCheck{"resources", statusFailed, err.Error()})
		} else {
			limit, ok := limitBytes(c.Resources.MaxMemoryMiB)
			switch {
			case !ok:
				checks = append(checks, dryRunCheck{"resources", statusFailed, fmt.Sprintf("resources.max_memory_mib %d does not fit a byte count", c.Resources.MaxMemoryMiB)})
			case resources.Check(estimate, limit) != nil:
				checks = append(checks, dryRunCheck{"resources", statusFailed, resources.Check(estimate, limit).Error()})
			default:
				checks = append(checks, dryRunCheck{"resources", statusOK, fmt.Sprintf(
					"the declared plan accounts for %d bytes, within resources.max_memory_mib %d (%d bytes); the estimate covers the declared arrays only and is not a resident set size",
					estimate.TotalBytes, c.Resources.MaxMemoryMiB, limit)})
			}
		}
	}

	if c.Teacher == nil {
		checks = append(checks, dryRunCheck{"teacher", statusSkipped, "no teacher is declared, so nothing could be called"})
	} else {
		detail := fmt.Sprintf("teacher %s at %s is declared with a budget of %d requests; a dry run never called it, and its credential stays a reference to %s",
			c.Teacher.Kind, c.Teacher.Endpoint, c.Teacher.Budget.MaxRequests, c.Teacher.Secret.Env)
		if teacher == nil {
			detail += "; no client is wired in this build (the teacher package is ticket 27)"
		}
		checks = append(checks, dryRunCheck{"teacher", statusSkipped, detail})
	}

	checks = append(checks, dryRunCheck{"output", statusSkipped, fmt.Sprintf(
		"a run would write into %s; a dry run creates nothing there", c.Output.Dir)})

	verdict := verdictOK
	for _, check := range checks {
		if check.Status == statusFailed {
			verdict = verdictRefused
		}
	}
	return dryRunReport{
		SchemaVersion: "coimnet-dry-run/v1",
		Config:        encoded,
		Provenance:    resolved.Provenance,
		Checks:        checks,
		Estimate:      estimate,
		Verdict:       verdict,
	}, nil
}

// modelShape is what a dry run needs from the declared model file: the sizes
// that go into the plan. Peripheral counts are only known from a model package;
// a graph store carries topology alone and reports them as zero, which the
// model check says in its detail.
type modelShape struct {
	nodes, edges     int
	maxDelay         int
	encoder, readout int
}

// loadModelShape opens exactly the one file the configuration declares.
func loadModelShape(ctx context.Context, c config.Config) (modelShape, string, error) {
	if c.Model.Package != "" {
		pkg, err := checkpoint.LoadModelPackage(ctx, c.Model.Package)
		if err != nil {
			return modelShape{}, "", fmt.Errorf("model.package %s did not load: %w", c.Model.Package, err)
		}
		shape := modelShape{
			nodes:    pkg.Topology.Nodes,
			edges:    pkg.Topology.Edges,
			maxDelay: maxDelayOf(pkg.Config),
			encoder:  len(pkg.Parameters.Encoder),
			readout:  len(pkg.Parameters.Readout),
		}
		detail := fmt.Sprintf("model.package %s is a %s with %d nodes and %d edges, topology %s, %d encoder and %d readout parameters",
			c.Model.Package, pkg.SchemaVersion, shape.nodes, shape.edges, pkg.Topology.SHA256, shape.encoder, shape.readout)
		return shape, detail, nil
	}
	limits := connectome.StoreLimits{MaxFileBytes: 8 << 30, MaxFooterBytes: 16 << 20, MaxMemoryBytes: 8 << 30}
	graph, err := connectome.Load(ctx, c.Model.Graph, limits)
	if err != nil {
		return modelShape{}, "", fmt.Errorf("model.graph %s did not load: %w", c.Model.Graph, err)
	}
	shape := modelShape{nodes: int(graph.NodeCount()), edges: int(graph.EdgeCount())}
	detail := fmt.Sprintf("model.graph %s verified with %d nodes and %d edges; a graph store carries no encoder, readout or delay declaration, so those are counted as zero",
		c.Model.Graph, shape.nodes, shape.edges)
	return shape, detail, nil
}

// maxDelayOf reports the longest declared conduction delay of one model.
func maxDelayOf(c learning.Config) int {
	delays := c.Dynamics.Delays
	if c.LIF != nil {
		delays = c.LIF.Delays
	}
	longest := 0
	for _, delay := range delays {
		if delay > longest {
			longest = delay
		}
	}
	return longest
}

// planFor turns one configuration and one model into the plan to be estimated.
// The declared choices it makes are: this build computes in float64 on the CPU,
// one node keeps one state value, one individual advances at a time, the
// backward window is learning.options.truncation, and a local rule enables a
// fast change and an eligibility record on every edge. No buffer allowance is
// estimated, so the total is the sum of the accounted arrays and nothing else.
func planFor(c config.Config, shape modelShape) resources.Plan {
	plastic := 0
	gradient := false
	for _, rule := range c.Learning.Rules {
		switch rule {
		case config.RuleGradient:
			gradient = true
		default:
			plastic = shape.edges
		}
	}
	optimizer := resources.OptimizerNone
	if gradient && anyTrainable(c.Trainable.Trainable) {
		optimizer = resources.OptimizerAdamW
	}
	plan := resources.Plan{
		Nodes:            shape.nodes,
		Edges:            shape.edges,
		StateDim:         1,
		Individuals:      1,
		HistorySteps:     c.Learning.Options.Truncation,
		Precision:        resources.PrecisionF64,
		Optimizer:        optimizer,
		PlasticEdges:     plastic,
		EligibilityEdges: plastic,
		MaxDelay:         shape.maxDelay,
		EncoderValues:    shape.encoder,
		ReadoutValues:    shape.readout,
	}
	if c.Modulation != nil {
		plan.ModulationRegions = c.Modulation.Chemistry.Regions
		plan.ModulationChannels = c.Modulation.Chemistry.Channels
		plan.Receptors = c.Modulation.Receptors
	}
	return plan
}

func anyTrainable(t learning.Trainable) bool {
	return t.Encoder || t.Weights || t.Bias || t.Tau || t.Theta || t.Readout
}

// limitBytes converts a mebibyte limit into bytes, reporting whether it fits.
func limitBytes(mib int) (uint64, bool) {
	if mib < 0 || uint64(mib) > math.MaxUint64>>20 {
		return 0, false
	}
	return uint64(mib) << 20, true
}
