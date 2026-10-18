// benchocr measures a saved tasks/ocr.Recognizer on the KMNIST test split and
// writes a bench.Result, so it can be compared with a model from another
// framework measured on the same machine and the same images (see
// scripts/bench/kmnist_mlp.py). Train the model first with examples/realocr
// --model-output; run this in a fresh process so the memory peak covers only
// loading and evaluation.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/TimLai666/coimnet/bench"
	"github.com/TimLai666/coimnet/examples/internal/kmnist"
	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

// task names the shared evaluation; the split hash pins the exact images.
const task = "kmnist-t10k-single-character"

func main() {
	var (
		dataDir   = flag.String("data-dir", "", "directory with the official KMNIST t10k IDX gzip files")
		modelDir  = flag.String("model", "", "task-model bundle saved by examples/realocr --model-output")
		name      = flag.String("name", "", "model name for the report (default: the bundle directory name)")
		testCount = flag.Int("test-count", 2000, "balanced test images to evaluate")
		warmup    = flag.Int("warmup", 50, "untimed calls before measuring")
		threads   = flag.Int("threads", 1, "GOMAXPROCS for the measurement")
		machine   = flag.String("machine", "", "machine name for the report (default: host name)")
		gpu       = flag.String("gpu", "", "accelerator name to record, if any")
		out       = flag.String("out", "", "new file to write the bench.Result JSON to")
	)
	flag.Parse()
	if *dataDir == "" || *modelDir == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./examples/benchocr --data-dir DIR --model BUNDLE --out NEW.json")
		os.Exit(2)
	}
	if *threads < 1 {
		fatal(errors.New("threads must be positive"))
	}
	runtime.GOMAXPROCS(*threads)
	if err := run(*dataDir, *modelDir, *name, *testCount, *warmup, *threads, *machine, *gpu, *out); err != nil {
		fatal(err)
	}
}

func run(dataDir, modelDir, name string, testCount, warmup, threads int, machine, gpu, out string) error {
	ctx := context.Background()
	test, err := kmnist.Load(dataDir, "t10k")
	if err != nil {
		return err
	}
	indices, err := test.Balanced(testCount)
	if err != nil {
		return err
	}
	images := make([]glyphs.Image, len(indices))
	for i, idx := range indices {
		images[i] = test.Image(idx, false)
	}
	idle := bench.PeakRSS()
	model, err := ocr.LoadRecognizer(ctx, modelDir)
	if err != nil {
		return err
	}
	alphabet := model.Config().Alphabet
	if len(alphabet) != kmnist.Classes {
		return fmt.Errorf("model has %d classes, KMNIST has %d", len(alphabet), kmnist.Classes)
	}
	// The timed pass overwrites every prediction the warmup calls left behind.
	predictions := make([]string, len(images))
	latency, err := bench.Measure(ctx, warmup, len(images), func(ctx context.Context, i int) error {
		var err error
		predictions[i], err = model.Infer(ctx, images[i])
		return err
	})
	if err != nil {
		return err
	}
	correct := 0
	for i, text := range predictions {
		if text == string(alphabet[test.Labels[indices[i]]]) {
			correct++
		}
	}
	snapshot := model.Snapshot()
	if name == "" {
		name = filepath.Base(filepath.Clean(modelDir))
	}
	result := bench.Result{
		SchemaVersion: bench.SchemaVersion, Framework: "coimnet", Model: name,
		Task: task, Split: kmnist.SplitID("balanced_round_robin_by_label_v1", indices), Host: bench.CurrentHost(machine, gpu),
		Parameters:   bench.CountParameters(snapshot.Parameters),
		Trainable:    bench.CountTrainable(snapshot.Parameters, snapshot.Options),
		PeakRSSBytes: bench.PeakRSS(), IdleRSSBytes: idle,
		Device: "cpu", Threads: threads, Latency: latency,
		Quality: map[string]float64{"accuracy": float64(correct) / float64(len(images))},
		Higher:  []string{"accuracy"},
		Notes:   []string{"one image per call; latency covers tasks/ocr.Recognizer.Infer including decoding"},
	}
	if err := result.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "benchocr:", err)
	os.Exit(1)
}
