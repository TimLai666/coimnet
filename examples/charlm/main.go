// charlm trains a sparse, wide continuous core as a byte-level language model
// (EXAMPLE ONLY): one byte per step is fed as a one-hot row, and the readout
// after every step is scored with the softmax cross-entropy against the next
// byte (learning.LossSoftmaxCrossEntropyEveryStep). The core is the one
// examples/seqkmnist uses: in-degree edges from uniformly drawn sources, with
// the optional current gate and softsign activation. By default every node
// receives the encoded byte: a readout node that only hears the input through
// k hops answers k steps late, which a last-step classifier never notices but
// a next-byte model pays for at every position.
//
// The text is a UTF-8 .txt file, or a .jsonl file whose lines carry a "text"
// field (one document each, such as TinyStories). Documents are split by
// index: every 100th goes to test, the one after it to validation, the rest
// to training; a .txt file is one document cut into 90/5/5 contiguous parts.
// Training draws random windows of --window bytes; validation and test score
// the first --eval-windows non-overlapping windows of their streams. The
// reported metric is bits per byte, the mean cross-entropy over every scored
// position divided by ln 2, so scripts/research/charlm.py can be compared on
// the same split.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/TimLai666/coimnet/bench"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

type settings struct {
	Text         string  `json:"text"`
	TextSHA256   string  `json:"text_sha256"`
	MaxBytes     int     `json:"max_bytes"`
	Nodes        int     `json:"nodes"`
	InputNodes   int     `json:"input_nodes"`
	ReadoutNodes int     `json:"readout_nodes"`
	InDegree     int     `json:"in_degree"`
	Window       int     `json:"window"`
	Updates      int     `json:"updates"`
	Batch        int     `json:"batch"`
	LearningRate float64 `json:"learning_rate"`
	EvalEvery    int     `json:"eval_every"`
	EvalWindows  int     `json:"eval_windows"`
	Seed         uint64  `json:"seed"`
	Workers      int     `json:"workers"`
	Gate         bool    `json:"gate,omitempty"`
	GateSquash   string  `json:"gate_squash,omitempty"`
	Activation   string  `json:"activation,omitempty"`
	GPU          bool    `json:"gpu,omitempty"`
}

type evalPoint struct {
	Updates      int     `json:"updates"`
	TrainLoss    float64 `json:"train_batch_loss"`
	ValBitsPerB  float64 `json:"val_bits_per_byte"`
	TrainSeconds float64 `json:"train_seconds"`
}

type report struct {
	Settings     settings    `json:"settings"`
	Vocabulary   int         `json:"vocabulary"`
	TrainBytes   int         `json:"train_bytes"`
	ValBytes     int         `json:"val_bytes"`
	TestBytes    int         `json:"test_bytes"`
	Parameters   int64       `json:"parameters"`
	Edges        int         `json:"edges"`
	Curve        []evalPoint `json:"curve"`
	TestBitsPerB float64     `json:"test_bits_per_byte"`
	TrainSeconds float64     `json:"train_seconds"`
	Sample       string      `json:"sample,omitempty"`
}

// checkpoint is the state after Updates updates. Resuming replays the window
// draws of those updates from the seed, so a resumed run trains on the same
// windows and ends with the same trainer state as an uninterrupted one.
type checkpoint struct {
	Settings     settings                  `json:"settings"`
	Updates      int                       `json:"updates"`
	Curve        []evalPoint               `json:"curve"`
	TrainSeconds float64                   `json:"train_seconds"`
	Trainer      learning.TrainingSnapshot `json:"trainer"`
}

func main() {
	var s settings
	flag.StringVar(&s.Text, "text", "", "UTF-8 .txt file, or .jsonl file with a \"text\" field per line")
	flag.IntVar(&s.MaxBytes, "max-bytes", 32<<20, "bytes of text read at most")
	flag.IntVar(&s.Nodes, "nodes", 512, "core nodes")
	flag.IntVar(&s.InputNodes, "input-nodes", 0, "nodes receiving the encoded byte (the first ones); 0 means every node")
	flag.IntVar(&s.ReadoutNodes, "readout-nodes", 64, "nodes read by the next-byte readout (the last ones)")
	flag.IntVar(&s.InDegree, "in-degree", 22, "incoming edges per node, sources drawn uniformly without repetition")
	flag.IntVar(&s.Window, "window", 64, "bytes per training and evaluation window")
	flag.IntVar(&s.Updates, "updates", 2000, "parameter updates")
	flag.IntVar(&s.Batch, "batch", 64, "windows per update")
	flag.Float64Var(&s.LearningRate, "lr", 1e-2, "peak AdamW learning rate (10% linear warmup, cosine decay)")
	flag.IntVar(&s.EvalEvery, "eval-every", 250, "updates between validation scores and checkpoints")
	flag.IntVar(&s.EvalWindows, "eval-windows", 256, "windows scored from the start of the validation and test streams")
	flag.Uint64Var(&s.Seed, "seed", 0, "initialization and window seed")
	flag.IntVar(&s.Workers, "workers", runtime.NumCPU(), "goroutines for gradients and evaluation")
	flag.BoolVar(&s.Gate, "gate", false, "add the per-neuron input-current gate (dynamics.GateCurrent)")
	flag.StringVar(&s.GateSquash, "gate-squash", "", `with --gate, "softsign" selects dynamics.GateCurrentSoftsign`)
	flag.StringVar(&s.Activation, "activation", "", "core activation: tanh (default), softsign or softplus")
	flag.BoolVar(&s.GPU, "gpu", false, "train each minibatch's core on the GPU (learning.NewGPUBatchTrainer)")
	reportPath := flag.String("report", "", "new file for the JSON training report")
	savePath := flag.String("save", "", "new file for the trained learning.TrainingSnapshot JSON")
	checkpointPath := flag.String("checkpoint", "", "file rewritten at every validation; a run started with an existing checkpoint of the same settings resumes from it")
	flag.Parse()
	if s.Text == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./examples/charlm --text FILE [--gpu] [--report NEW.json]")
		os.Exit(2)
	}
	if err := run(s, *reportPath, *savePath, *checkpointPath); err != nil {
		fmt.Fprintln(os.Stderr, "charlm:", err)
		os.Exit(1)
	}
}

func run(s settings, reportPath, savePath, checkpointPath string) error {
	ctx := context.Background()
	if s.InputNodes == 0 {
		s.InputNodes = s.Nodes
	}
	if s.InputNodes > s.Nodes || s.ReadoutNodes > s.Nodes || s.InDegree > s.Nodes {
		return errors.New("input/readout nodes and in-degree must not exceed nodes")
	}
	if s.Window < 2 || s.Batch < 1 || s.Updates < 1 || s.EvalEvery < 1 || s.EvalWindows < 1 {
		return errors.New("window must be at least 2; batch, updates, eval-every and eval-windows must be positive")
	}
	if (s.GateSquash != "" && s.GateSquash != "softsign") || (s.GateSquash != "" && !s.Gate) {
		return errors.New(`--gate-squash takes "softsign" and needs --gate`)
	}
	splits, sum, err := readText(s.Text, s.MaxBytes)
	if err != nil {
		return err
	}
	s.TextSHA256 = sum
	v := newVocabulary(splits.train)
	train, val, test := v.encode(splits.train), v.encode(splits.val), v.encode(splits.test)
	for name, stream := range map[string][]int{"train": train, "validation": val, "test": test} {
		if len(stream) < s.Window+1 {
			return fmt.Errorf("the %s stream has %d bytes, fewer than one window", name, len(stream))
		}
	}
	cfg, params := build(s, v.size())
	o := learning.DefaultOptions()
	o.LearningRate = s.LearningRate
	o.Loss = learning.LossSoftmaxCrossEntropyEveryStep
	o.AccumulateSteps = s.Batch
	o.Trainable.Gate = s.Gate
	warmup := uint64(s.Updates / 10)
	o.Schedule = &learning.Schedule{Kind: learning.ScheduleCosine, WarmupUpdates: warmup, DecayUpdates: uint64(s.Updates) - warmup, FinalFactor: 1e-3}
	rep := report{Settings: s, Vocabulary: v.size(), TrainBytes: len(train), ValBytes: len(val), TestBytes: len(test), Parameters: bench.CountParameters(params), Edges: len(cfg.Dynamics.Sources)}
	resumed, err := readCheckpoint(checkpointPath, s)
	if err != nil {
		return err
	}
	var tr *learning.Trainer
	switch {
	case resumed != nil && s.GPU:
		tr, err = learning.RestoreGPUBatchTrainer(ctx, resumed.Trainer)
	case resumed != nil:
		tr, err = learning.RestoreTrainer(resumed.Trainer)
	case s.GPU:
		tr, err = learning.NewGPUBatchTrainer(ctx, cfg, params, o)
	default:
		tr, err = learning.NewTrainer(cfg, params, o)
	}
	if err != nil {
		return err
	}
	defer tr.Close()
	done, elapsed := 0, 0.0
	if resumed != nil {
		done, elapsed, rep.Curve = resumed.Updates, resumed.TrainSeconds, resumed.Curve
		fmt.Printf("resuming from %s after %d updates\n", checkpointPath, done)
	}
	if info, ok := tr.GPUAdapterInfo(); ok {
		fmt.Printf("training the core on %s (%s, %s)\n", info.Forward.Adapter, info.Forward.Backend, info.Forward.DeviceType)
	}
	fmt.Printf("vocabulary %d train %d val %d test %d bytes; nodes %d edges %d parameters %d\n", v.size(), len(train), len(val), len(test), s.Nodes, rep.Edges, rep.Parameters)
	rng := rand.New(rand.NewPCG(s.Seed, 99))
	draw := func() []int {
		starts := make([]int, s.Batch)
		for i := range starts {
			starts[i] = rng.IntN(len(train) - s.Window)
		}
		return starts
	}
	for range done {
		draw()
	}
	valWindows, testWindows := evalWindows(val, s.Window, s.EvalWindows), evalWindows(test, s.Window, s.EvalWindows)
	start := time.Now().Add(-time.Duration(elapsed * float64(time.Second)))
	var loss float64
	for u := done; u < s.Updates; u++ {
		starts := draw()
		inputs := make([][][]float64, s.Batch)
		targets := make([][]float64, s.Batch)
		for i, at := range starts {
			inputs[i], targets[i] = v.window(train[at : at+s.Window+1])
		}
		results, err := tr.StepBatch(ctx, inputs, targets, s.Workers)
		if err != nil {
			return fmt.Errorf("update %d: %w", u, err)
		}
		loss = 0
		for _, r := range results {
			loss += r.Loss / float64(len(results))
		}
		if (u+1)%s.EvalEvery != 0 && u+1 != s.Updates {
			continue
		}
		bits, err := bitsPerByte(ctx, tr, v, val, valWindows, s.Window, s.Workers)
		if err != nil {
			return err
		}
		point := evalPoint{Updates: u + 1, TrainLoss: loss, ValBitsPerB: bits, TrainSeconds: time.Since(start).Seconds()}
		rep.Curve = append(rep.Curve, point)
		fmt.Printf("update %d batch loss %.3f val %.4f bits/byte (%.0fs)\n", u+1, loss, bits, point.TrainSeconds)
		if checkpointPath != "" {
			c := checkpoint{Settings: s, Updates: u + 1, Curve: rep.Curve, TrainSeconds: point.TrainSeconds, Trainer: tr.Snapshot()}
			if err := writeAtomic(checkpointPath, c); err != nil {
				return err
			}
		}
	}
	rep.TrainSeconds = time.Since(start).Seconds()
	if rep.TestBitsPerB, err = bitsPerByte(ctx, tr, v, test, testWindows, s.Window, s.Workers); err != nil {
		return err
	}
	fmt.Printf("test %.4f bits/byte\n", rep.TestBitsPerB)
	if rep.Sample, err = sample(ctx, tr, v, test[:s.Window], 200, s.Seed); err != nil {
		return err
	}
	fmt.Printf("sample after %q: %q\n", v.decode(test[:s.Window]), rep.Sample)
	if reportPath != "" {
		if err := writeNew(reportPath, rep); err != nil {
			return err
		}
	}
	if savePath != "" {
		return writeNew(savePath, tr.Snapshot())
	}
	return nil
}

// build lays out the uniform in-degree topology and the seeded initialization
// of examples/seqkmnist: each weight N(0, 1/in-degree), encoder N(0, 1) (one
// active input per step), readout U(-1/sqrt(r), 1/sqrt(r)), zero bias and
// tau = 2. The readout runs every step.
func build(s settings, vocab int) (learning.Config, learning.Parameters) {
	rng := rand.New(rand.NewPCG(s.Seed, 1))
	var sources, targets []int
	for t := 0; t < s.Nodes; t++ {
		for _, src := range rng.Perm(s.Nodes)[:s.InDegree] {
			sources, targets = append(sources, src), append(targets, t)
		}
	}
	inputNodes := make([]int, s.InputNodes)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, s.ReadoutNodes)
	for i := range readoutNodes {
		readoutNodes[i] = s.Nodes - s.ReadoutNodes + i
	}
	activation := s.Activation
	if activation == "" {
		activation = "tanh"
	}
	cfg := learning.Config{
		Dynamics:         dynamics.Config{Nodes: s.Nodes, Sources: sources, Targets: targets, DT: 1, Activation: activation},
		InputSize:        vocab,
		OutputSize:       vocab,
		InputNodes:       inputNodes,
		ReadoutNodes:     readoutNodes,
		ReadoutEveryStep: true,
	}
	encoder := make([]float64, vocab*s.InputNodes)
	for i := range encoder {
		encoder[i] = rng.NormFloat64()
	}
	readout := make([]float64, s.ReadoutNodes*vocab)
	bound := 1 / math.Sqrt(float64(s.ReadoutNodes))
	for i := range readout {
		readout[i] = (2*rng.Float64() - 1) * bound
	}
	weights := make([]float64, len(sources))
	for e := range weights {
		weights[e] = rng.NormFloat64() / math.Sqrt(float64(s.InDegree))
	}
	logTau := make([]float64, s.Nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: weights, Bias: make([]float64, s.Nodes), LogTau: logTau},
		Encoder: encoder,
		Readout: readout,
	}
	if s.Gate {
		cfg.Dynamics.Gate = dynamics.GateCurrent
		if s.GateSquash == "softsign" {
			cfg.Dynamics.Gate = dynamics.GateCurrentSoftsign
		}
		p.Core.GateGain, p.Core.GateBias = make([]float64, s.Nodes), make([]float64, s.Nodes)
	}
	return cfg, p
}

type textSplits struct{ train, val, test []byte }

// readText reads at most limit bytes of path and splits them; it returns the
// sha256 of the bytes read.
func readText(path string, limit int) (textSplits, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return textSplits{}, "", err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, int64(limit)))
	if err != nil {
		return textSplits{}, "", err
	}
	digest := sha256.Sum256(raw)
	sum := hex.EncodeToString(digest[:])
	if !strings.HasSuffix(path, ".jsonl") {
		n := len(raw)
		a, b := n*90/100, n*95/100
		return textSplits{train: raw[:a], val: raw[a:b], test: raw[b:]}, sum, nil
	}
	var out textSplits
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	scanner.Buffer(make([]byte, 1<<20), 16<<20)
	for i := 0; scanner.Scan(); i++ {
		var doc struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &doc); err != nil {
			// The limit can cut the last line short.
			break
		}
		text := append([]byte(strings.TrimSpace(doc.Text)), '\n')
		switch i % 100 {
		case 0:
			out.test = append(out.test, text...)
		case 1:
			out.val = append(out.val, text...)
		default:
			out.train = append(out.train, text...)
		}
	}
	return out, sum, scanner.Err()
}

// vocabulary maps the bytes seen in training to ids 0..n-1 in byte order; id
// n stands for every byte training never saw.
type vocabulary struct {
	ids   [256]int
	bytes []byte
}

func newVocabulary(train []byte) *vocabulary {
	var seen [256]bool
	for _, b := range train {
		seen[b] = true
	}
	v := &vocabulary{}
	for b := range 256 {
		if seen[b] {
			v.bytes = append(v.bytes, byte(b))
		}
	}
	sort.Slice(v.bytes, func(i, j int) bool { return v.bytes[i] < v.bytes[j] })
	for b := range v.ids {
		v.ids[b] = len(v.bytes)
	}
	for id, b := range v.bytes {
		v.ids[b] = id
	}
	return v
}

func (v *vocabulary) size() int { return len(v.bytes) + 1 }

func (v *vocabulary) encode(text []byte) []int {
	out := make([]int, len(text))
	for i, b := range text {
		out[i] = v.ids[b]
	}
	return out
}

func (v *vocabulary) decode(ids []int) string {
	out := make([]byte, len(ids))
	for i, id := range ids {
		out[i] = '?'
		if id < len(v.bytes) {
			out[i] = v.bytes[id]
		}
	}
	return string(out)
}

// window turns ids (one longer than the window) into one-hot input rows and
// the per-step target: row t predicts ids[t+1], and every row is scored.
func (v *vocabulary) window(ids []int) ([][]float64, []float64) {
	n, width := len(ids)-1, v.size()
	inputs := make([][]float64, n)
	target := make([]float64, n*width)
	for t := range n {
		inputs[t] = make([]float64, width)
		inputs[t][ids[t]] = 1
		target[t*width+ids[t+1]] = 1
	}
	return inputs, target
}

// evalWindows returns the starts of the first count non-overlapping windows.
func evalWindows(stream []int, window, count int) []int {
	var starts []int
	for at := 0; at+window+1 <= len(stream) && len(starts) < count; at += window {
		starts = append(starts, at)
	}
	return starts
}

// bitsPerByte is the mean next-byte cross-entropy over every position of the
// windows, in bits, computed in float64 from the trainer's per-step logits.
func bitsPerByte(ctx context.Context, tr *learning.Trainer, v *vocabulary, stream, starts []int, window, workers int) (float64, error) {
	snapshot := tr.Snapshot()
	n, err := learning.NewNetwork(snapshot.Config)
	if err != nil {
		return 0, err
	}
	nll := make([]float64, len(starts))
	errs := make(chan error, workers)
	for w := range workers {
		go func(w int) {
			for i := w; i < len(starts); i += workers {
				ids := stream[starts[i] : starts[i]+window+1]
				inputs, _ := v.window(ids)
				logits, err := n.PredictAll(ctx, snapshot.Parameters, inputs)
				if err != nil {
					errs <- err
					return
				}
				for t, row := range logits {
					nll[i] += logSumExp(row) - row[ids[t+1]]
				}
			}
			errs <- nil
		}(w)
	}
	for range workers {
		if err := <-errs; err != nil {
			return 0, err
		}
	}
	var sum float64
	for _, x := range nll {
		sum += x
	}
	return sum / float64(len(starts)*window) / math.Ln2, nil
}

func logSumExp(row []float64) float64 {
	m := math.Inf(-1)
	for _, z := range row {
		m = math.Max(m, z)
	}
	var s float64
	for _, z := range row {
		s += math.Exp(z - m)
	}
	return m + math.Log(s)
}

// sample continues prompt by count bytes drawn from the softmax, rerunning
// the whole context (at most one window) each step.
func sample(ctx context.Context, tr *learning.Trainer, v *vocabulary, prompt []int, count int, seed uint64) (string, error) {
	rng := rand.New(rand.NewPCG(seed, 7))
	ids := append([]int(nil), prompt...)
	window := len(prompt)
	for range count {
		context := ids[len(ids)-window:]
		inputs := make([][]float64, len(context))
		for t, id := range context {
			inputs[t] = make([]float64, v.size())
			inputs[t][id] = 1
		}
		logits, err := tr.Predict(ctx, inputs)
		if err != nil {
			return "", err
		}
		norm := logSumExp(logits)
		r, next := rng.Float64(), len(logits)-1
		for k, z := range logits {
			if r -= math.Exp(z - norm); r < 0 {
				next = k
				break
			}
		}
		ids = append(ids, next)
	}
	return v.decode(ids[window:]), nil
}

// readCheckpoint returns nil when path is empty or missing, and refuses a
// checkpoint written with other settings.
func readCheckpoint(path string, s settings) (*checkpoint, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c checkpoint
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("checkpoint %s: %w", path, err)
	}
	want, got := s, c.Settings
	// Workers only schedules a batch's gradients and GPU only places the core.
	want.Workers, got.Workers, want.GPU, got.GPU = 0, 0, false, false
	if got != want {
		return nil, fmt.Errorf("checkpoint %s was written with settings %+v, not %+v", path, c.Settings, s)
	}
	if c.Updates < 1 || c.Updates > s.Updates {
		return nil, fmt.Errorf("checkpoint %s records %d updates", path, c.Updates)
	}
	return &c, nil
}

// writeAtomic replaces path through a synced temporary file and a rename, so a
// crash leaves either the previous checkpoint or the new one.
func writeAtomic(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeNew(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
