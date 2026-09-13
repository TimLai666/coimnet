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

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/TimLai666/coimnet/learning"
)

func runTrain(ctx context.Context, args []string, out, stderr io.Writer, resume bool) error {
	name := "train delayed"
	if resume {
		name = "resume"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	steps := fs.Int("steps", 600, "additional training episodes (1..100000)")
	path := fs.String("checkpoint", "", "required checkpoint path")
	var output string
	var seed, dataSeed uint64
	var rate float64
	if resume {
		fs.StringVar(&output, "out", "", "required new checkpoint path")
	} else {
		fs.Uint64Var(&seed, "seed", 42, "initialization seed")
		fs.Uint64Var(&dataSeed, "data-seed", 1001, "training data seed")
		fs.Float64Var(&rate, "learning-rate", .02, "positive AdamW learning rate")
	}
	usageOutput := &outputCapture{writer: out}
	fs.Usage = func() {
		if resume {
			fmt.Fprintln(usageOutput, "Usage: coimnet resume --checkpoint OLD --out NEW [--steps 600]\nRestores episode-reset training, optimizer state, generator and sample cursor.\nExample: coimnet resume --checkpoint first.json --out second.json --steps 600")
		} else {
			fmt.Fprintln(usageOutput, "Usage: coimnet train delayed --checkpoint NEW [flags]\nTrains the three-neuron synthetic delayed pulse fixture. Each episode starts at zero neural state.\nExample: coimnet train delayed --checkpoint first.json --steps 600")
		}
		fmt.Fprintln(usageOutput, "Checkpoint paths are never overwritten. Parent directories must exist.\nErrors: invalid options, cancellation, numerical failure, invalid checkpoint or file conflict.\nOptions:")
		fs.SetOutput(usageOutput)
		fs.PrintDefaults()
		fs.SetOutput(stderr)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 || *path == "" || *steps < 1 || *steps > 100000 || (resume && output == "") {
		return fmt.Errorf("invalid or missing options; use %s --help", name)
	}
	var tr *learning.Trainer
	var err error
	var next uint64
	if resume {
		state, e := checkpoint.Load(ctx, *path)
		if e != nil {
			return e
		}
		tr, err = learning.RestoreTrainer(state.Training)
		dataSeed = state.DataSeed
		next = state.NextSample
	} else {
		tr, err = experiment.NewDelayedTrainer(seed, rate, false)
		output = *path
	}
	if err != nil {
		return err
	}
	// Avoid spending the update budget when the destination already exists.
	if _, err = os.Lstat(output); err == nil {
		return fmt.Errorf("checkpoint already exists: %s", output)
	} else if !os.IsNotExist(err) {
		return err
	}
	if next > math.MaxUint64-uint64(*steps) {
		return fmt.Errorf("sample cursor overflow")
	}
	var last learning.StepResult
	for i := 0; i < *steps; i++ {
		ep := experiment.DelayedEpisode(dataSeed, next)
		last, err = tr.Step(ctx, ep.Input, ep.Target)
		if err != nil {
			return err
		}
		next++
	}
	state, err := checkpoint.NewState(tr.Snapshot(), dataSeed, next)
	if err != nil {
		return err
	}
	if err = checkpoint.Save(ctx, output, state); err != nil {
		return err
	}
	return writeJSON(out, struct {
		SchemaVersion string              `json:"schema_version"`
		Profile       string              `json:"profile"`
		Checkpoint    string              `json:"checkpoint"`
		NextSample    uint64              `json:"next_sample"`
		LastStep      learning.StepResult `json:"last_step"`
	}{"coimnet-train-result/v1", "fixture", output, next, last})
}

func runPredict(ctx context.Context, args []string, out, stderr io.Writer) error {
	fs := flag.NewFlagSet("predict", flag.ContinueOnError)
	fs.SetOutput(stderr)
	path := fs.String("checkpoint", "", "required episode checkpoint")
	input := fs.String("input", "", "required JSON observations: [[value], [value], ...]")
	usageOutput := &outputCapture{writer: out}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet predict --checkpoint FILE --input JSON_FILE\nRuns a frozen independent episode using observations only. Input is an array of time steps, each containing numeric channels. No targets or feedback fields are accepted. Input limit: 16 MiB.\nExample: coimnet predict --checkpoint first.json --input observations.json\nErrors: malformed input, wrong shape, invalid checkpoint, cancellation or output failure.\nOptions:")
		fs.SetOutput(usageOutput)
		fs.PrintDefaults()
		fs.SetOutput(stderr)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 || *path == "" || *input == "" {
		return fmt.Errorf("checkpoint and input are required; use predict --help")
	}
	state, err := checkpoint.Load(ctx, *path)
	if err != nil {
		return err
	}
	tr, err := learning.RestoreTrainer(state.Training)
	if err != nil {
		return err
	}
	data, err := fileio.ReadRegular(ctx, *input, 16<<20)
	if err != nil {
		return err
	}
	var observation [][]float64
	if err = json.Unmarshal(data, &observation); err != nil {
		return fmt.Errorf("invalid observation array: %w", err)
	}
	prediction, err := tr.Predict(ctx, observation)
	if err != nil {
		return err
	}
	return writeJSON(out, struct {
		SchemaVersion string    `json:"schema_version"`
		Output        []float64 `json:"output"`
	}{"coimnet-prediction/v1", prediction})
}
