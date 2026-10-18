// benchocr measures a saved tasks/ocr.Recognizer on the KMNIST test split and
// writes a bench.Result, so it can be compared with a model from another
// framework measured on the same machine and the same images (see
// scripts/bench/kmnist_mlp.py). Train the model first with examples/realocr
// --model-output; run this in a fresh process so the memory peak covers only
// loading and evaluation.
package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/TimLai666/coimnet/bench"
	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

const (
	imageSide = 28
	classes   = 10
	// Task names the shared evaluation; the split hash pins the exact images.
	task = "kmnist-t10k-single-character"
)

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
	labels, err := readLabels(filepath.Join(dataDir, "t10k-labels-idx1-ubyte.gz"))
	if err != nil {
		return err
	}
	indices, err := balancedIndices(labels, testCount)
	if err != nil {
		return err
	}
	images, err := readImages(filepath.Join(dataDir, "t10k-images-idx3-ubyte.gz"), len(labels), indices)
	if err != nil {
		return err
	}
	idle := bench.PeakRSS()
	model, err := ocr.LoadRecognizer(ctx, modelDir)
	if err != nil {
		return err
	}
	alphabet := model.Config().Alphabet
	if len(alphabet) != classes {
		return fmt.Errorf("model has %d classes, KMNIST has %d", len(alphabet), classes)
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
		if text == string(alphabet[labels[indices[i]]]) {
			correct++
		}
	}
	snapshot := model.Snapshot()
	if name == "" {
		name = filepath.Base(filepath.Clean(modelDir))
	}
	result := bench.Result{
		SchemaVersion: bench.SchemaVersion, Framework: "coimnet", Model: name,
		Task: task, Split: SplitID(indices), Host: bench.CurrentHost(machine, gpu),
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

// SplitID names the evaluated selection: the SHA-256 of the test indices
// joined by commas. scripts/bench/kmnist_mlp.py computes the same string.
func SplitID(indices []int) string {
	parts := make([]string, len(indices))
	for i, v := range indices {
		parts[i] = strconv.Itoa(v)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, ",")))
	return "balanced_round_robin_by_label_v1:" + strconv.Itoa(len(indices)) + ":" + hex.EncodeToString(sum[:])
}

// balancedIndices takes the first index of each label in turn, the same
// selection examples/realocr uses.
func balancedIndices(labels []int, limit int) ([]int, error) {
	byClass := make([][]int, classes)
	for i, l := range labels {
		if l < 0 || l >= classes {
			return nil, fmt.Errorf("label %d at index %d is outside the class count", l, i)
		}
		byClass[l] = append(byClass[l], i)
	}
	out := make([]int, 0, limit)
	for round := 0; len(out) < limit; round++ {
		progress := false
		for c := 0; c < classes && len(out) < limit; c++ {
			if round < len(byClass[c]) {
				out = append(out, byClass[c][round])
				progress = true
			}
		}
		if !progress {
			return nil, fmt.Errorf("only %d balanced examples for the requested %d", len(out), limit)
		}
	}
	return out, nil
}

func readGzip(path string, magic uint32, dims int) ([]byte, []uint32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	b, err := io.ReadAll(io.LimitReader(z, 64<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	head := 4 + 4*dims
	if len(b) < head || binary.BigEndian.Uint32(b) != magic {
		return nil, nil, fmt.Errorf("%s: not an IDX file with magic %d", path, magic)
	}
	shape := make([]uint32, dims)
	size := 1
	for i := range shape {
		shape[i] = binary.BigEndian.Uint32(b[4+4*i:])
		size *= int(shape[i])
	}
	if len(b)-head != size {
		return nil, nil, fmt.Errorf("%s: payload has %d bytes, header says %d", path, len(b)-head, size)
	}
	return b[head:], shape, nil
}

func readLabels(path string) ([]int, error) {
	b, _, err := readGzip(path, 2049, 1)
	if err != nil {
		return nil, err
	}
	out := make([]int, len(b))
	for i, v := range b {
		out[i] = int(v)
	}
	return out, nil
}

func readImages(path string, count int, indices []int) ([]glyphs.Image, error) {
	b, shape, err := readGzip(path, 2051, 3)
	if err != nil {
		return nil, err
	}
	if int(shape[0]) != count || shape[1] != imageSide || shape[2] != imageSide {
		return nil, fmt.Errorf("%s: shape %v, want %d×28×28", path, shape, count)
	}
	const n = imageSide * imageSide
	out := make([]glyphs.Image, len(indices))
	for k, idx := range indices {
		pixels := make([]float64, n)
		for j := range pixels {
			pixels[j] = float64(b[idx*n+j]) / 255
		}
		out[k] = glyphs.Image{Width: imageSide, Height: imageSide, Pixels: pixels}
	}
	return out, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "benchocr:", err)
	os.Exit(1)
}
