// seqkmnist trains a sparse, wide continuous core on column-sequential KMNIST
// (EXAMPLE ONLY): each image is read one column per step, 28 steps of 28
// pixels, and the class is read from the readout nodes after the last step.
// Every target node receives in-degree edges from uniformly drawn sources, so
// the parameter count grows with edges rather than with nodes squared.
//
// It trains with softmax cross-entropy and minibatches (AccumulateSteps plus
// Trainer.StepBatch), evaluates the official test split and, with --bench,
// writes a bench.Result measured one image per call on one thread, so it can be
// compared with scripts/research/seqlab.py on the same machine.
package main

import (
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/TimLai666/coimnet/bench"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const (
	side    = 28
	classes = 10
)

type settings struct {
	DataDir      string  `json:"-"`
	Nodes        int     `json:"nodes"`
	InputNodes   int     `json:"input_nodes"`
	ReadoutNodes int     `json:"readout_nodes"`
	InDegree     int     `json:"in_degree"`
	Epochs       int     `json:"epochs"`
	Batch        int     `json:"batch"`
	LearningRate float64 `json:"learning_rate"`
	TrainCount   int     `json:"train_count"`
	TestCount    int     `json:"test_count"`
	Seed         uint64  `json:"seed"`
	Workers      int     `json:"workers"`
	Gate         bool    `json:"gate,omitempty"`
}

type report struct {
	Settings      settings  `json:"settings"`
	Parameters    int64     `json:"parameters"`
	Edges         int       `json:"edges"`
	EpochAccuracy []float64 `json:"epoch_test_accuracy"`
	TrainSeconds  float64   `json:"train_seconds"`
	Accuracy      float64   `json:"test_accuracy"`
}

// checkpoint is the state after a whole epoch. Resuming replays the earlier
// epochs' shuffles from the seed, so a resumed run takes the same batches and
// ends with the same trainer state as an uninterrupted one.
type checkpoint struct {
	Settings      settings                  `json:"settings"`
	Epochs        int                       `json:"epochs_done"`
	EpochAccuracy []float64                 `json:"epoch_test_accuracy"`
	TrainSeconds  float64                   `json:"train_seconds"`
	Trainer       learning.TrainingSnapshot `json:"trainer"`
}

func main() {
	var s settings
	flag.StringVar(&s.DataDir, "data-dir", "", "directory with the official KMNIST IDX gzip files")
	flag.IntVar(&s.Nodes, "nodes", 512, "core nodes")
	flag.IntVar(&s.InputNodes, "input-nodes", 128, "nodes receiving the encoded column (the first ones)")
	flag.IntVar(&s.ReadoutNodes, "readout-nodes", 128, "nodes read by the classifier (the last ones)")
	flag.IntVar(&s.InDegree, "in-degree", 24, "incoming edges per node, sources drawn uniformly without repetition")
	flag.IntVar(&s.Epochs, "epochs", 8, "training epochs")
	flag.IntVar(&s.Batch, "batch", 128, "images per update")
	flag.Float64Var(&s.LearningRate, "lr", 3e-3, "peak AdamW learning rate (30% linear warmup, cosine decay)")
	flag.IntVar(&s.TrainCount, "train-count", 60000, "training images, from the start of the official train split")
	flag.IntVar(&s.TestCount, "test-count", 10000, "test images, from the start of the official test split")
	flag.Uint64Var(&s.Seed, "seed", 0, "initialization and shuffling seed")
	flag.IntVar(&s.Workers, "workers", runtime.NumCPU(), "goroutines computing gradients of one batch")
	flag.BoolVar(&s.Gate, "gate", false, "add the per-neuron input-current gate (dynamics.GateCurrent), initialized to the plain integrator")
	reportPath := flag.String("report", "", "new file for the JSON training report")
	benchPath := flag.String("bench", "", "new file for a bench.Result measured after training")
	savePath := flag.String("save", "", "new file for the trained learning.TrainingSnapshot JSON")
	checkpointPath := flag.String("checkpoint", "", "file rewritten after every epoch; a run started with an existing checkpoint of the same settings resumes from it")
	loadPath := flag.String("load", "", "skip training: load this TrainingSnapshot JSON, evaluate it and (with --bench) measure it in this fresh process")
	machine := flag.String("machine", "", "machine name for the bench result (default: host name)")
	cpuProfile := flag.String("cpuprofile", "", "write a CPU profile of the run to this new file")
	flag.Parse()
	if *cpuProfile != "" {
		f, err := os.OpenFile(*cpuProfile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			err = pprof.StartCPUProfile(f)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "seqkmnist:", err)
			os.Exit(1)
		}
		defer f.Close()
		defer pprof.StopCPUProfile()
	}
	if s.DataDir == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./examples/seqkmnist --data-dir DIR [--report NEW.json] [--bench NEW.json]")
		os.Exit(2)
	}
	var err error
	if *loadPath != "" {
		err = runLoaded(s, *loadPath, *benchPath, *machine)
	} else {
		err = run(s, *reportPath, *benchPath, *savePath, *checkpointPath, *machine)
	}
	if err != nil {
		pprof.StopCPUProfile()
		fmt.Fprintln(os.Stderr, "seqkmnist:", err)
		os.Exit(1)
	}
}

// runLoaded evaluates a saved model in a process that never trained, so the
// measured peak memory covers loading the model and the test images only.
func runLoaded(s settings, loadPath, benchPath, machine string) error {
	ctx := context.Background()
	testX, testY, err := readSplit(s.DataDir, "t10k", s.TestCount)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(loadPath)
	if err != nil {
		return err
	}
	var snapshot learning.TrainingSnapshot
	if err := json.Unmarshal(b, &snapshot); err != nil {
		return err
	}
	tr, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		return err
	}
	defer tr.Close()
	acc, err := evaluate(ctx, tr, testX, testY, s.Workers)
	if err != nil {
		return err
	}
	fmt.Printf("loaded %s: test %.4f\n", loadPath, acc)
	if benchPath == "" {
		return nil
	}
	c := snapshot.Config.Dynamics
	s.Nodes, s.Gate = c.Nodes, c.Gate != ""
	if c.Nodes > 0 {
		s.InDegree = len(c.Sources) / c.Nodes
	}
	return measure(ctx, tr, s, testX, testY, acc, machine, benchPath)
}

func run(s settings, reportPath, benchPath, savePath, checkpointPath, machine string) error {
	ctx := context.Background()
	if s.InputNodes > s.Nodes || s.ReadoutNodes > s.Nodes || s.InDegree > s.Nodes || s.Batch < 1 || s.Epochs < 1 {
		return errors.New("input/readout nodes and in-degree must not exceed nodes; batch and epochs must be positive")
	}
	trainX, trainY, err := readSplit(s.DataDir, "train", s.TrainCount)
	if err != nil {
		return err
	}
	testX, testY, err := readSplit(s.DataDir, "t10k", s.TestCount)
	if err != nil {
		return err
	}
	cfg, params := build(s)
	steps := s.Epochs * (len(trainX) / s.Batch)
	o := learning.DefaultOptions()
	o.LearningRate = s.LearningRate
	o.Loss = learning.LossSoftmaxCrossEntropy
	o.AccumulateSteps = s.Batch
	o.Trainable.Gate = s.Gate
	warmup := uint64(steps * 3 / 10)
	o.Schedule = &learning.Schedule{Kind: learning.ScheduleCosine, WarmupUpdates: warmup, DecayUpdates: uint64(steps) - warmup, FinalFactor: 1e-3}
	rep := report{Settings: s, Parameters: bench.CountParameters(params), Edges: len(cfg.Dynamics.Sources)}
	resumed, err := readCheckpoint(checkpointPath, s)
	if err != nil {
		return err
	}
	var tr *learning.Trainer
	done, elapsed := 0, 0.0
	if resumed != nil {
		tr, err = learning.RestoreTrainer(resumed.Trainer)
		done, elapsed, rep.EpochAccuracy = resumed.Epochs, resumed.TrainSeconds, resumed.EpochAccuracy
		fmt.Printf("resuming from %s after epoch %d\n", checkpointPath, done-1)
	} else {
		tr, err = learning.NewTrainer(cfg, params, o)
	}
	if err != nil {
		return err
	}
	defer tr.Close()
	fmt.Printf("nodes %d edges %d parameters %d updates %d\n", s.Nodes, rep.Edges, rep.Parameters, steps)
	rng := rand.New(rand.NewPCG(s.Seed, 99))
	order := make([]int, len(trainX))
	for i := range order {
		order[i] = i
	}
	shuffle := func() { rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] }) }
	for range done {
		shuffle()
	}
	start := time.Now().Add(-time.Duration(elapsed * float64(time.Second)))
	for epoch := done; epoch < s.Epochs; epoch++ {
		shuffle()
		var loss float64
		for b := 0; b+s.Batch <= len(order); b += s.Batch {
			inputs := make([][][]float64, s.Batch)
			targets := make([][]float64, s.Batch)
			for i := range inputs {
				k := order[b+i]
				inputs[i] = trainX[k]
				targets[i] = oneHot(trainY[k])
			}
			results, err := tr.StepBatch(ctx, inputs, targets, s.Workers)
			if err != nil {
				return err
			}
			loss = 0
			for _, r := range results {
				loss += r.Loss / float64(len(results))
			}
		}
		acc, err := evaluate(ctx, tr, testX, testY, s.Workers)
		if err != nil {
			return err
		}
		rep.EpochAccuracy = append(rep.EpochAccuracy, acc)
		fmt.Printf("epoch %d batch loss %.3f test %.4f (%.0fs)\n", epoch, loss, acc, time.Since(start).Seconds())
		if checkpointPath != "" {
			c := checkpoint{Settings: s, Epochs: epoch + 1, EpochAccuracy: rep.EpochAccuracy, TrainSeconds: time.Since(start).Seconds(), Trainer: tr.Snapshot()}
			if err := writeAtomic(checkpointPath, c); err != nil {
				return err
			}
		}
	}
	rep.TrainSeconds = time.Since(start).Seconds()
	rep.Accuracy = rep.EpochAccuracy[len(rep.EpochAccuracy)-1]
	if reportPath != "" {
		if err := writeNew(reportPath, rep); err != nil {
			return err
		}
	}
	if savePath != "" {
		if err := writeNew(savePath, tr.Snapshot()); err != nil {
			return err
		}
	}
	if benchPath != "" {
		return measure(ctx, tr, s, testX, testY, rep.Accuracy, machine, benchPath)
	}
	return nil
}

// build lays out the sparse topology and the seeded initialization: weights
// N(0, 1/in-degree), encoder N(0, 1/28), readout U(-1/sqrt(r), 1/sqrt(r)),
// zero bias and tau = 2.
func build(s settings) (learning.Config, learning.Parameters) {
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
	cfg := learning.Config{
		Dynamics:     dynamics.Config{Nodes: s.Nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:    side,
		OutputSize:   classes,
		InputNodes:   inputNodes,
		ReadoutNodes: readoutNodes,
	}
	normal := func(n int, std float64) []float64 {
		v := make([]float64, n)
		for i := range v {
			v[i] = rng.NormFloat64() * std
		}
		return v
	}
	readout := make([]float64, s.ReadoutNodes*classes)
	bound := 1 / math.Sqrt(float64(s.ReadoutNodes))
	for i := range readout {
		readout[i] = (2*rng.Float64() - 1) * bound
	}
	logTau := make([]float64, s.Nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: normal(len(sources), 1/math.Sqrt(float64(s.InDegree))), Bias: make([]float64, s.Nodes), LogTau: logTau},
		Encoder: normal(side*s.InputNodes, 1/math.Sqrt(side)),
		Readout: readout,
	}
	if s.Gate {
		cfg.Dynamics.Gate = dynamics.GateCurrent
		p.Core.GateGain, p.Core.GateBias = make([]float64, s.Nodes), make([]float64, s.Nodes)
	}
	return cfg, p
}

func oneHot(label int) []float64 {
	v := make([]float64, classes)
	v[label] = 1
	return v
}

func argmax(v []float64) int {
	best := 0
	for i, x := range v {
		if x > v[best] {
			best = i
		}
	}
	return best
}

func evaluate(ctx context.Context, tr *learning.Trainer, xs [][][]float64, ys []int, workers int) (float64, error) {
	snapshot := tr.Snapshot()
	n, err := learning.NewNetwork(snapshot.Config)
	if err != nil {
		return 0, err
	}
	correct := make([]bool, len(xs))
	errs := make(chan error, workers)
	for w := 0; w < workers; w++ {
		go func(w int) {
			for i := w; i < len(xs); i += workers {
				logits, err := n.Predict(ctx, snapshot.Parameters, xs[i])
				if err != nil {
					errs <- err
					return
				}
				correct[i] = argmax(logits) == ys[i]
			}
			errs <- nil
		}(w)
	}
	for w := 0; w < workers; w++ {
		if err := <-errs; err != nil {
			return 0, err
		}
	}
	count := 0
	for _, c := range correct {
		if c {
			count++
		}
	}
	return float64(count) / float64(len(xs)), nil
}

// measure times Trainer.Predict one image per call on one thread, the same
// protocol as scripts/research/seqlab.py --bench.
func measure(ctx context.Context, tr *learning.Trainer, s settings, xs [][][]float64, ys []int, accuracy float64, machine, path string) error {
	runtime.GOMAXPROCS(1)
	samples := min(2000, len(xs))
	latency, err := bench.Measure(ctx, 50, samples, func(ctx context.Context, i int) error {
		_, err := tr.Predict(ctx, xs[i])
		return err
	})
	if err != nil {
		return err
	}
	snapshot := tr.Snapshot()
	result := bench.Result{
		SchemaVersion: bench.SchemaVersion, Framework: "coimnet",
		Model: modelName(s),
		Task:  "kmnist-t10k-column-sequence", Split: fmt.Sprintf("first:%d", len(xs)),
		Host:       bench.CurrentHost(machine, ""),
		Parameters: bench.CountParameters(snapshot.Parameters),
		Trainable:  bench.CountTrainable(snapshot.Parameters, snapshot.Options),
		// After --load the peak covers loading and evaluation only; after
		// training in the same process it is an upper bound.
		PeakRSSBytes: bench.PeakRSS(),
		Device:       "cpu", Threads: 1, Latency: latency,
		Quality: map[string]float64{"accuracy": accuracy},
		Higher:  []string{"accuracy"},
		Notes:   []string{"one image per call (28 column steps); peak RSS covers this process (training too unless --load)"},
	}
	if err := result.Validate(); err != nil {
		return err
	}
	return writeNew(path, result)
}

func modelName(s settings) string {
	name := fmt.Sprintf("sparse-core-n%d-k%d", s.Nodes, s.InDegree)
	if s.Gate {
		name += "-gate"
	}
	return name
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
	// Workers only changes how a batch's gradients are scheduled, and the data
	// directory is not recorded.
	want.Workers, got.Workers, want.DataDir = 0, 0, ""
	if got != want {
		return nil, fmt.Errorf("checkpoint %s was written with settings %+v, not %+v", path, c.Settings, s)
	}
	if c.Epochs < 1 || c.Epochs > s.Epochs || len(c.EpochAccuracy) != c.Epochs {
		return nil, fmt.Errorf("checkpoint %s records %d epochs", path, c.Epochs)
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

// readSplit reads the first limit images of a split as [column][row] pixel
// sequences scaled to [0, 1], with their labels.
func readSplit(dir, split string, limit int) ([][][]float64, []int, error) {
	labels, err := readIDX(filepath.Join(dir, split+"-labels-idx1-ubyte.gz"), 8)
	if err != nil {
		return nil, nil, err
	}
	pixels, err := readIDX(filepath.Join(dir, split+"-images-idx3-ubyte.gz"), 16)
	if err != nil {
		return nil, nil, err
	}
	count := min(limit, len(labels))
	if len(pixels) < count*side*side {
		return nil, nil, fmt.Errorf("%s images are shorter than %d labels", split, count)
	}
	xs := make([][][]float64, count)
	ys := make([]int, count)
	for i := range xs {
		img := pixels[i*side*side : (i+1)*side*side]
		xs[i] = make([][]float64, side)
		for col := range xs[i] {
			xs[i][col] = make([]float64, side)
			for row := 0; row < side; row++ {
				xs[i][col][row] = float64(img[row*side+col]) / 255
			}
		}
		ys[i] = int(labels[i])
	}
	return xs, ys, nil
}

func readIDX(path string, header int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	b, err := io.ReadAll(io.LimitReader(gz, 64<<20))
	if err != nil {
		return nil, err
	}
	if len(b) < header || binary.BigEndian.Uint32(b[:4])>>8 != 0x08 {
		return nil, fmt.Errorf("%s is not an unsigned-byte IDX file", path)
	}
	return b[header:], nil
}
