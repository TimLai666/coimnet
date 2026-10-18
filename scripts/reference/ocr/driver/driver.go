//go:build refocr

// Reference driver for the TSK-11 single-character KMNIST investigation.
// It is compiled together with unmodified copies of examples/realocr/main.go
// and model.go (only realocr's main renamed) by ../build.sh, so it reuses the
// strict LoadDataset and the public tasks/ocr SDK exactly like the frozen
// protocol driver in evidence/TSK-11/quality-20261012/final/sources.
//
// Modes:
//
//	recognizer  ocr.NewRecognizer + TrainLine/Infer (the frozen protocol path)
//	custom      the same model rebuilt directly on learning.Trainer, so the
//	            readout can optionally see every core node (--readout all)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

type model interface {
	train(ctx context.Context, s glyphs.Sample) (loss, gradNorm float64)
	logits(ctx context.Context, img glyphs.Image) []float64 // final row, all classes incl. blank
	params() learning.Parameters
}

// ---- recognizer path -------------------------------------------------------

type recModel struct {
	r *ocr.Recognizer
	t *learning.Trainer
}

func (m *recModel) train(ctx context.Context, s glyphs.Sample) (float64, float64) {
	rep, err := m.r.TrainLine(ctx, s.Image, s.Text)
	must(err)
	m.t = nil
	return rep.Loss, -1
}

func (m *recModel) logits(ctx context.Context, img glyphs.Image) []float64 {
	if m.t == nil {
		t, err := learning.RestoreTrainer(m.r.Snapshot())
		must(err)
		m.t = t
	}
	in, err := m.r.Inputs(img)
	must(err)
	all, err := m.t.PredictAll(ctx, in)
	must(err)
	return all[len(all)-1]
}

func (m *recModel) params() learning.Parameters { return m.r.Parameters() }

// ---- custom path: the NewRecognizer construction, readout selectable -------

type customModel struct {
	t       *learning.Trainer
	index   map[rune]int
	classes int
	inSize  int
}

func newCustom(height, inputSize, hidden int, alphabet []rune, lr float64, seed uint64, readoutAll bool) *customModel {
	nodes := height + hidden
	var sources, targets []int
	for s := 0; s < nodes; s++ {
		for h := 0; h < hidden; h++ {
			sources = append(sources, s)
			targets = append(targets, height+h)
		}
	}
	inputNodes := make([]int, height)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	var readoutNodes []int
	if readoutAll {
		for i := 0; i < nodes; i++ {
			readoutNodes = append(readoutNodes, i)
		}
	} else {
		for i := 0; i < hidden; i++ {
			readoutNodes = append(readoutNodes, height+i)
		}
	}
	classes := len(alphabet) + 1
	cfg := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        inputSize,
		OutputSize:       classes,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	encoder := make([]float64, inputSize*height)
	rng := rand.New(rand.NewPCG(seed, 42))
	scale := 1 / math.Sqrt(float64(inputSize))
	for i := range encoder {
		encoder[i] = (2*rng.Float64() - 1) * scale
	}
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: uniform(rand.NewPCG(seed, 0), len(sources)), Bias: make([]float64, nodes), LogTau: logTau},
		Encoder: encoder,
		Readout: uniform(rand.NewPCG(seed, 1), len(readoutNodes)*classes),
	}
	o := learning.DefaultOptions()
	o.LearningRate = lr
	o.Trainable = learning.Trainable{Encoder: true, Weights: true, Bias: true, Readout: true}
	t, err := learning.NewTrainer(cfg, p, o)
	must(err)
	idx := map[rune]int{}
	for i, r := range alphabet {
		idx[r] = i + 1
	}
	return &customModel{t: t, index: idx, classes: classes, inSize: inputSize}
}

func uniform(src *rand.PCG, n int) []float64 {
	rng := rand.New(src)
	out := make([]float64, n)
	for i := range out {
		out[i] = rng.Float64()*.6 - .3
	}
	return out
}

func (m *customModel) inputs(img glyphs.Image) [][]float64 {
	return [][]float64{append([]float64(nil), img.Pixels...), make([]float64, m.inSize)}
}

func (m *customModel) train(ctx context.Context, s glyphs.Sample) (float64, float64) {
	in := m.inputs(s.Image)
	all, err := m.t.PredictAll(ctx, in)
	must(err)
	target := m.index[[]rune(s.Text)[0]]
	loss, grad := softmaxNonBlank(all[1], target)
	up := [][]float64{make([]float64, m.classes), grad}
	res, err := m.t.StepFrom(ctx, in, up)
	must(err)
	return loss, res.GradientNorm
}

func (m *customModel) logits(ctx context.Context, img glyphs.Image) []float64 {
	all, err := m.t.PredictAll(ctx, m.inputs(img))
	must(err)
	return all[len(all)-1]
}

func (m *customModel) params() learning.Parameters { return m.t.Snapshot().Parameters }

func softmaxNonBlank(row []float64, target int) (float64, []float64) {
	mx := row[1]
	for _, v := range row[2:] {
		mx = math.Max(mx, v)
	}
	sum := 0.0
	g := make([]float64, len(row))
	for c := 1; c < len(row); c++ {
		g[c] = math.Exp(row[c] - mx)
		sum += g[c]
	}
	for c := 1; c < len(row); c++ {
		g[c] /= sum
	}
	loss := -(row[target] - mx - math.Log(sum))
	g[target]--
	return loss, g
}

// ---- scoring ---------------------------------------------------------------

type score struct {
	Accuracy         float64 `json:"accuracy"`
	MeanCrossEntropy float64 `json:"mean_cross_entropy"`
	Samples          int     `json:"samples"`
	// InferAgree counts samples where ocr.Recognizer.Infer equals the argmax
	// over classes 1..10 of the final row (only for the recognizer path).
	InferAgree int `json:"infer_agree,omitempty"`
}

func scoreSet(ctx context.Context, m model, rec *ocr.Recognizer, samples []glyphs.Sample, alphabet []rune) score {
	idx := map[rune]int{}
	for i, r := range alphabet {
		idx[r] = i + 1
	}
	out := score{Samples: len(samples)}
	correct := 0
	for _, s := range samples {
		row := m.logits(ctx, s.Image)
		want := idx[[]rune(s.Text)[0]]
		best := 1
		for c := 2; c < len(row); c++ {
			if row[c] > row[best] {
				best = c
			}
		}
		if best == want {
			correct++
		}
		l, _ := softmaxNonBlank(row, want)
		out.MeanCrossEntropy += l
		if rec != nil {
			h, err := rec.Infer(ctx, s.Image)
			must(err)
			if h == string(alphabet[best-1]) {
				out.InferAgree++
			}
		}
	}
	out.Accuracy = float64(correct) / float64(len(samples))
	out.MeanCrossEntropy /= float64(len(samples))
	return out
}

func writeJSON(path string, v any) {
	b, err := json.Marshal(v)
	must(err)
	must(os.WriteFile(path, b, 0o644))
}

func main() {
	dir := flag.String("data-dir", "data/TSK-11/kmnist", "KMNIST directory with metadata.json")
	mode := flag.String("mode", "recognizer", "recognizer or custom")
	readout := flag.String("readout", "hidden", "custom mode: hidden or all")
	seed := flag.Uint64("seed", 20260924, "model and order seed")
	hidden := flag.Int("hidden", 24, "hidden nodes")
	inputNodes := flag.Int("input-nodes", 28, "custom mode: number of encoder-driven input nodes (recognizer fixes 28)")
	trainCount := flag.Int("train-count", 6000, "balanced training samples")
	testCount := flag.Int("test-count", 2000, "final test samples after the offset")
	testOffset := flag.Int("test-offset", 200, "balanced test samples discarded first (development set)")
	epochs := flag.Int("epochs", 20, "epochs")
	lr := flag.Float64("lr", 0.002, "AdamW learning rate")
	logSteps := flag.Int("log-steps", 0, "write the pre-update loss of the first N updates")
	out := flag.String("out", "", "output directory (created)")
	dump := flag.Bool("dump", false, "dump initial parameters, selected indices and epoch orders")
	evalEvery := flag.Int("eval-every", 0, "evaluate test accuracy every K epochs (0: only at end)")
	flag.Parse()
	must(os.MkdirAll(*out, 0o755))
	ctx := context.Background()
	ds, err := LoadDataset(*dir, DatasetLimits{TrainCount: *trainCount, TestCount: *testCount + *testOffset})
	must(err)
	devIdx := map[int]bool{}
	for _, i := range ds.TestIndices[:*testOffset] {
		devIdx[i] = true
	}
	ds.Test, ds.TestIndices = ds.Test[*testOffset:], ds.TestIndices[*testOffset:]
	for _, i := range ds.TestIndices {
		if devIdx[i] {
			panic("development/final overlap")
		}
	}
	alphabet := make([]rune, len(ds.Metadata.Mapping))
	for i, m := range ds.Metadata.Mapping {
		alphabet[i] = []rune(m.Char)[0]
	}
	var m model
	var rec *ocr.Recognizer
	switch *mode {
	case "recognizer":
		cfg := ocr.RecognizerConfig{Height: 28, Hidden: *hidden, Alphabet: alphabet, LearningRate: *lr, Seed: *seed, TrainableEncoder: true, InputProfile: "single-character", CharacterWidth: 28}
		rec, err = ocr.NewRecognizer(cfg)
		must(err)
		m = &recModel{r: rec}
	case "custom":
		m = newCustom(*inputNodes, 784, *hidden, alphabet, *lr, *seed, *readout == "all")
	default:
		panic("unknown mode")
	}
	init := m.params()
	if *dump {
		labels := make([]int, len(ds.Train))
		idx := map[rune]int{}
		for i, r := range alphabet {
			idx[r] = i
		}
		for i, s := range ds.Train {
			labels[i] = idx[[]rune(s.Text)[0]]
		}
		writeJSON(filepath.Join(*out, "init.json"), map[string]any{
			"encoder": init.Encoder, "weights": init.Core.Weights, "bias": init.Core.Bias,
			"log_tau": init.Core.LogTau, "readout": init.Readout, "hidden": *hidden, "height": *inputNodes,
			"readout_all": *mode == "custom" && *readout == "all",
			"train_indices": ds.TrainIndices, "test_indices": ds.TestIndices, "train_labels": labels,
		})
		if rec != nil {
			writeJSON(filepath.Join(*out, "snapshot-config.json"), rec.Snapshot().Config)
			writeJSON(filepath.Join(*out, "snapshot-options.json"), rec.Snapshot().Options)
		}
	}
	before := scoreSet(ctx, m, rec, ds.Test, alphabet)
	order := make([]int, len(ds.Train))
	for i := range order {
		order[i] = i
	}
	rng := rand.New(rand.NewPCG(*seed, 37))
	var orders [][]int
	var stepLog []map[string]float64
	var epochLog []map[string]any
	updates := 0
	start := time.Now()
	for epoch := 0; epoch < *epochs; epoch++ {
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		if *dump {
			orders = append(orders, append([]int(nil), order...))
		}
		total := 0.0
		for _, i := range order {
			loss, gn := m.train(ctx, ds.Train[i])
			total += loss
			if updates < *logSteps {
				stepLog = append(stepLog, map[string]float64{"update": float64(updates), "loss": loss, "grad_norm": gn})
			}
			updates++
		}
		e := map[string]any{"epoch": epoch + 1, "mean_train_loss": total / float64(len(order)), "elapsed_s": time.Since(start).Seconds()}
		if *evalEvery > 0 && (epoch+1)%*evalEvery == 0 && epoch+1 != *epochs {
			e["test"] = scoreSet(ctx, m, nil, ds.Test, alphabet)
		}
		epochLog = append(epochLog, e)
		fmt.Fprintf(os.Stderr, "epoch %d loss %.4f %.0fs\n", epoch+1, total/float64(len(order)), time.Since(start).Seconds())
	}
	after := scoreSet(ctx, m, rec, ds.Test, alphabet)
	trainScore := scoreSet(ctx, m, nil, ds.Train, alphabet)
	final := m.params()
	if *dump {
		writeJSON(filepath.Join(*out, "orders.json"), orders)
		writeJSON(filepath.Join(*out, "final.json"), map[string]any{"encoder": final.Encoder, "weights": final.Core.Weights, "bias": final.Core.Bias, "log_tau": final.Core.LogTau, "readout": final.Readout})
	}
	if len(stepLog) > 0 {
		writeJSON(filepath.Join(*out, "steps.json"), stepLog)
	}
	res := map[string]any{"mode": *mode, "readout": *readout, "seed": *seed, "hidden": *hidden, "input_nodes": *inputNodes, "train_count": len(ds.Train), "test_count": len(ds.Test), "test_offset": *testOffset, "epochs": *epochs, "lr": *lr, "updates": updates,
		"params": map[string]int{"encoder": len(final.Encoder), "core_weights": len(final.Core.Weights), "bias": len(final.Core.Bias), "readout": len(final.Readout)},
		"before": before, "after": after, "train": trainScore, "epoch_log": epochLog, "train_seconds": time.Since(start).Seconds()}
	writeJSON(filepath.Join(*out, "result.json"), res)
	b, _ := json.MarshalIndent(map[string]any{"after": after, "train": trainScore, "before": before}, "", " ")
	fmt.Println(string(b))
}
