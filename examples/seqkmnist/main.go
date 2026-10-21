// seqkmnist trains a sparse, wide continuous core on column-sequential KMNIST
// (EXAMPLE ONLY): each image is read one column per step, 28 steps of 28
// pixels, and the class is read from the readout nodes after the last step.
// Every target node receives in-degree edges from uniformly drawn sources, so
// the parameter count grows with edges rather than with nodes squared. With
// --store the edges come from a connectome subgraph store instead, and
// --rewire trains a same-size null model of them.
//
// It trains with softmax cross-entropy and minibatches (AccumulateSteps plus
// Trainer.StepBatch; with --gpu the core of each minibatch runs on the device
// through learning.NewGPUBatchTrainer), evaluates the official test split
// (and, with --val-count, a validation split held out of the training images,
// for choosing hyperparameters without the test split) and, with --bench,
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
	"strings"
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
	// ValCount holds out the last ValCount of the TrainCount training images
	// as a validation split scored after every epoch, so hyperparameters can
	// be chosen without looking at the test split; zero trains on them all.
	ValCount int    `json:"val_count,omitempty"`
	Seed     uint64 `json:"seed"`
	Workers  int    `json:"workers"`
	Gate     bool   `json:"gate,omitempty"`
	// GateSquash "softsign" selects dynamics.GateCurrentSoftsign; empty keeps
	// the sigmoid gate.
	GateSquash string `json:"gate_squash,omitempty"`
	// Activation is the core activation; empty means tanh.
	Activation string `json:"activation,omitempty"`
	// GPU trains with learning.NewGPUBatchTrainer: each minibatch's core
	// forward and backward run on the device, the encoder, readout and AdamW
	// on the host. Evaluation and --bench stay on the CPU.
	GPU bool `json:"gpu,omitempty"`
	// Cols groups this many adjacent image columns into one step (28/Cols
	// steps of 28*Cols pixels); zero means one column per step.
	Cols int `json:"cols,omitempty"`
	// Store is a connectome.Save subgraph store whose annotated edges replace
	// the uniform random topology; Nodes then is its node count.
	Store string `json:"store,omitempty"`
	// Rewire, with Store, replaces the store's edges by a null model of the
	// same size: "in-degree" or "swap" (see rewire).
	Rewire string `json:"rewire,omitempty"`
	// Tie, with Store, ties the core parameters by annotated cell type
	// ("type", see tieByType); the gate parameters stay per node.
	// "type-shuffled" ties by the same labels permuted across the nodes
	// (shuffleTypes), a control with the same group sizes but no relation
	// between a node's group and its real type.
	Tie string `json:"tie,omitempty"`
	// Freeze lists parameter groups kept at their initial values: encoder,
	// weights, bias, tau, gate and readout (comma separated). Freezing
	// weights, bias, tau and gate leaves a fixed reservoir.
	Freeze string `json:"freeze,omitempty"`
	// Delays draws every edge's delay uniformly from 0..Delays steps; zero
	// keeps every edge undelayed. The GPU core refuses delayed edges.
	Delays int `json:"delays,omitempty"`
	// Dale fixes every edge's sign by its source node: a node is inhibitory
	// with probability Inhibitory and all its outgoing edges are negative,
	// otherwise positive (learning.Config.EdgeSigns, log-magnitude weights).
	// The GPU core trains them too; the host converts its weight gradient.
	Dale       bool    `json:"dale,omitempty"`
	Inhibitory float64 `json:"inhibitory,omitempty"`
	// TauMax, when above 2, draws each node's initial time constant
	// log-uniformly from [2, TauMax] instead of setting every tau to 2.
	TauMax float64 `json:"tau_max,omitempty"`
	// Core "lif" swaps the continuous core for the spiking dynamics.LIF on
	// the same edges and initial weights, with a trainable base threshold in
	// [0.05, 1], v_reset 0, tau_syn 2 and the fast-sigmoid surrogate of
	// scale SurrogateScale. Adapt adds short-term threshold adaptation
	// (tau_adapt 20, beta 0.2) and Homeostasis the slow stabiliser (tau_rate
	// 50, target rate 0.1, eta 0.01, h_max 0.5). These are engineering values,
	// not measured ones. The spiking core is CPU-only and takes no gate.
	Core           string  `json:"core,omitempty"`
	SurrogateScale float64 `json:"surrogate_scale,omitempty"`
	Adapt          bool    `json:"adapt,omitempty"`
	Homeostasis    bool    `json:"homeostasis,omitempty"`
	// ChemChannels adds a trainable dynamics.Chemistry block of this many
	// channels (zero keeps the model without it). The other Chem fields are
	// zeroed while it is zero, so a run without chemistry records the same
	// settings as before the flags existed.
	ChemChannels int `json:"chem_channels,omitempty"`
	// ChemTau is every channel's concentration time constant in steps.
	ChemTau float64 `json:"chem_tau,omitempty"`
	// ChemKd is the receptors' dissociation constant; zero means
	// ChemTau*log(2), the steady concentration of a zero release gain.
	ChemKd float64 `json:"chem_kd,omitempty"`
	// ChemHill is the receptors' Hill exponent.
	ChemHill float64 `json:"chem_hill,omitempty"`
	// ChemSource names the nodes whose mean output releases each channel:
	// "all", "split" (K contiguous blocks, one per channel), "input",
	// "readout" or "hidden" (neither input nor readout).
	ChemSource string `json:"chem_source,omitempty"`
	// ChemReceptors is the fraction of (node, channel) pairs given a
	// receptor, drawn from the chemistry stream; 1 gives every node every
	// channel.
	ChemReceptors float64 `json:"chem_receptors,omitempty"`
	// ChemInit is the standard deviation of the initial receptor gains and
	// offsets; release gains start at 1 plus a tenth of it times N(0, 1).
	ChemInit float64 `json:"chem_init,omitempty"`
}

// columns is the number of image columns fed per step.
func (s settings) columns() int { return max(s.Cols, 1) }

type report struct {
	Settings   settings `json:"settings"`
	Parameters int64    `json:"parameters"`
	Edges      int      `json:"edges"`
	// TrainedParameters counts each tied group once; it equals Parameters
	// when nothing is tied.
	TrainedParameters int64     `json:"trained_parameters"`
	EpochAccuracy     []float64 `json:"epoch_test_accuracy"`
	// EpochValAccuracy and ValAccuracy score the --val-count split.
	EpochValAccuracy []float64 `json:"epoch_val_accuracy,omitempty"`
	TrainSeconds     float64   `json:"train_seconds"`
	Accuracy         float64   `json:"test_accuracy"`
	ValAccuracy      float64   `json:"val_accuracy,omitempty"`
}

// checkpoint is the state after a whole epoch. Resuming replays the earlier
// epochs' shuffles from the seed, so a resumed run takes the same batches and
// ends with the same trainer state as an uninterrupted one.
type checkpoint struct {
	Settings         settings                  `json:"settings"`
	Epochs           int                       `json:"epochs_done"`
	EpochAccuracy    []float64                 `json:"epoch_test_accuracy"`
	EpochValAccuracy []float64                 `json:"epoch_val_accuracy,omitempty"`
	TrainSeconds     float64                   `json:"train_seconds"`
	Trainer          learning.TrainingSnapshot `json:"trainer"`
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
	flag.IntVar(&s.ValCount, "val-count", 0, "hold out the last N training images as a validation split scored every epoch; 0 trains on all of them")
	flag.Uint64Var(&s.Seed, "seed", 0, "initialization and shuffling seed")
	flag.IntVar(&s.Workers, "workers", runtime.NumCPU(), "goroutines computing gradients of one batch")
	flag.BoolVar(&s.Gate, "gate", false, "add the per-neuron input-current gate (dynamics.GateCurrent), initialized to the plain integrator")
	flag.StringVar(&s.GateSquash, "gate-squash", "", `with --gate, "softsign" squashes the gate without an exponential (dynamics.GateCurrentSoftsign)`)
	flag.StringVar(&s.Activation, "activation", "", "core activation: tanh (default), softsign or softplus")
	flag.IntVar(&s.Cols, "cols", 0, "adjacent image columns fed per step (28/cols steps); 0 means 1")
	flag.StringVar(&s.Store, "store", "", "connectome.Save subgraph store whose edges replace the random topology (--nodes and --in-degree are then ignored)")
	flag.StringVar(&s.Tie, "tie", "", `with --store, tie the core weights, biases and time constants by annotated cell type: "type", or "type-shuffled" for the same labels permuted across nodes`)
	flag.StringVar(&s.Rewire, "rewire", "", `with --store, train on a null model of its edges: "in-degree" keeps each target's in-degree, "swap" keeps every in- and out-degree`)
	flag.StringVar(&s.Freeze, "freeze", "", "parameter groups kept at their initial values: encoder, weights, bias, tau, gate, theta, readout (comma separated)")
	flag.IntVar(&s.Delays, "delays", 0, "draw each edge's delay uniformly from 0..N steps (CPU core only)")
	flag.BoolVar(&s.Dale, "dale", false, "fix each edge's sign by its source node (Dale's law)")
	flag.Float64Var(&s.Inhibitory, "inhibitory", 0.5, "with --dale, the probability that a node is inhibitory")
	flag.Float64Var(&s.TauMax, "tau-max", 0, "draw each node's initial tau log-uniformly from [2, N] (0 keeps tau = 2)")
	flag.StringVar(&s.Core, "core", "", `"lif" trains the spiking core on the same edges (CPU only)`)
	flag.Float64Var(&s.SurrogateScale, "surrogate-scale", 5, "with --core lif, the fast-sigmoid surrogate scale")
	flag.BoolVar(&s.Adapt, "adapt", false, "with --core lif, add short-term threshold adaptation")
	flag.BoolVar(&s.Homeostasis, "homeostasis", false, "with --core lif, add the slow activity stabiliser")
	flag.BoolVar(&s.GPU, "gpu", false, "train each minibatch's core on the GPU (learning.NewGPUBatchTrainer) and evaluate the test split there in float32 (Trainer.PredictBatch)")
	flag.IntVar(&s.ChemChannels, "chem-channels", 0, "add a trainable activity-driven chemistry block (dynamics.Chemistry) with this many channels; 0 turns it off (CPU only)")
	flag.Float64Var(&s.ChemTau, "chem-tau", 5, "with --chem-channels, every channel's concentration time constant in steps")
	flag.Float64Var(&s.ChemKd, "chem-kd", 0, "with --chem-channels, the receptor dissociation constant; 0 means chem-tau*ln 2")
	flag.Float64Var(&s.ChemHill, "chem-hill", 2, "with --chem-channels, the receptor Hill exponent")
	flag.StringVar(&s.ChemSource, "chem-source", "all", `with --chem-channels, the releasing nodes of each channel: "all", "split", "input", "readout" or "hidden"`)
	flag.Float64Var(&s.ChemReceptors, "chem-receptors", 1, "with --chem-channels, the fraction of (node, channel) pairs that get a receptor; 1 is every node")
	flag.Float64Var(&s.ChemInit, "chem-init", .01, "with --chem-channels, the standard deviation of the initial receptor gains and offsets")
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
	b, err := os.ReadFile(loadPath)
	if err != nil {
		return err
	}
	var snapshot learning.TrainingSnapshot
	if err := json.Unmarshal(b, &snapshot); err != nil {
		return err
	}
	if in := snapshot.Config.InputSize; in > side && in%side == 0 {
		s.Cols = in / side
	}
	testX, testY, err := readSplit(s.DataDir, "t10k", s.TestCount, s.columns())
	if err != nil {
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
	s.Nodes, s.Gate, s.Activation = c.Nodes, c.Gate != "", c.Activation
	if c.Chemistry != nil {
		s.ChemChannels = len(c.Chemistry.Channels)
	}
	if c.Gate == dynamics.GateCurrentSoftsign {
		s.GateSquash = "softsign"
	}
	if c.Nodes > 0 {
		s.InDegree = len(c.Sources) / c.Nodes
	}
	return measure(ctx, tr, s, testX, testY, acc, machine, benchPath)
}

func run(s settings, reportPath, benchPath, savePath, checkpointPath, machine string) error {
	ctx := context.Background()
	var topo *topology
	if s.Store != "" {
		t, err := loadStore(ctx, s.Store)
		if err != nil {
			return err
		}
		if t, err = rewire(t, s.Rewire, s.Seed); err != nil {
			return err
		}
		topo, s.Nodes, s.InDegree = &t, t.nodes, (len(t.sources)+t.nodes/2)/t.nodes
	} else if s.Rewire != "" || s.Tie != "" {
		return errors.New("--rewire and --tie need --store")
	}
	if s.Tie != "" && ((s.Tie != "type" && s.Tie != "type-shuffled") || s.Dale || s.Core != "") {
		return errors.New(`--tie takes "type" or "type-shuffled" and does not combine with --dale or --core`)
	}
	if s.InputNodes > s.Nodes || s.ReadoutNodes > s.Nodes || s.InDegree > s.Nodes || s.Batch < 1 || s.Epochs < 1 {
		return errors.New("input/readout nodes and in-degree must not exceed nodes; batch and epochs must be positive")
	}
	if (s.GateSquash != "" && s.GateSquash != "softsign") || (s.GateSquash != "" && !s.Gate) {
		return errors.New(`--gate-squash takes "softsign" and needs --gate`)
	}
	if s.Cols < 0 || side%s.columns() != 0 {
		return fmt.Errorf("--cols must divide %d", side)
	}
	if !s.Dale {
		s.Inhibitory = 0
	}
	switch s.Core {
	case "":
		if s.Adapt || s.Homeostasis {
			return errors.New("--adapt and --homeostasis need --core lif")
		}
		s.SurrogateScale = 0
	case "lif":
		if s.Gate || s.Activation != "" || s.SurrogateScale <= 0 {
			return errors.New("--core lif takes no gate or activation and needs a positive --surrogate-scale")
		}
	default:
		return fmt.Errorf(`--core takes "lif", not %q`, s.Core)
	}
	if s.Delays < 0 || s.Inhibitory < 0 || s.Inhibitory > 1 || (s.TauMax != 0 && s.TauMax <= 2) {
		return errors.New("--delays must be non-negative, --inhibitory within [0, 1] and --tau-max above 2")
	}
	trainable := learning.DefaultOptions().Trainable
	trainable.Gate, trainable.Theta = s.Gate, s.Core == "lif"
	trainable, err := freeze(trainable, s.Freeze)
	if err != nil {
		return err
	}
	s = normalizeChemistry(s)
	if err := checkChemistry(s); err != nil {
		return err
	}
	trainX, trainY, err := readSplit(s.DataDir, "train", s.TrainCount, s.columns())
	if err != nil {
		return err
	}
	testX, testY, err := readSplit(s.DataDir, "t10k", s.TestCount, s.columns())
	if err != nil {
		return err
	}
	trainX, valX, trainY, valY, err := holdOut(trainX, trainY, s.ValCount)
	if err != nil {
		return err
	}
	cfg, params, tiedAway := build(s, topo)
	steps := s.Epochs * (len(trainX) / s.Batch)
	o := learning.DefaultOptions()
	o.LearningRate = s.LearningRate
	o.Loss = learning.LossSoftmaxCrossEntropy
	o.AccumulateSteps = s.Batch
	o.Trainable = trainable
	o.Trainable.Chemistry = s.ChemChannels > 0
	warmup := uint64(steps * 3 / 10)
	o.Schedule = &learning.Schedule{Kind: learning.ScheduleCosine, WarmupUpdates: warmup, DecayUpdates: uint64(steps) - warmup, FinalFactor: 1e-3}
	rep := report{Settings: s, Parameters: bench.CountParameters(params), Edges: len(cfg.Dynamics.Sources)}
	rep.TrainedParameters = rep.Parameters - int64(tiedAway)
	if cfg.LIF != nil {
		rep.Edges = len(cfg.LIF.Sources)
	}
	resumed, err := readCheckpoint(checkpointPath, s)
	if err != nil {
		return err
	}
	var tr *learning.Trainer
	done, elapsed := 0, 0.0
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
	if resumed != nil {
		done, elapsed, rep.EpochAccuracy, rep.EpochValAccuracy = resumed.Epochs, resumed.TrainSeconds, resumed.EpochAccuracy, resumed.EpochValAccuracy
		fmt.Printf("resuming from %s after epoch %d\n", checkpointPath, done-1)
	}
	if info, ok := tr.GPUAdapterInfo(); ok {
		fmt.Printf("training the core on %s (%s, %s)\n", info.Forward.Adapter, info.Forward.Backend, info.Forward.DeviceType)
	}
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
		evalFn := evaluate
		if s.GPU {
			evalFn = evaluateBatched
		}
		acc, err := evalFn(ctx, tr, testX, testY, s.Workers)
		if err != nil {
			return err
		}
		rep.EpochAccuracy = append(rep.EpochAccuracy, acc)
		val := ""
		if len(valX) > 0 {
			vacc, err := evalFn(ctx, tr, valX, valY, s.Workers)
			if err != nil {
				return err
			}
			rep.EpochValAccuracy = append(rep.EpochValAccuracy, vacc)
			val = fmt.Sprintf(" val %.4f", vacc)
		}
		fmt.Printf("epoch %d batch loss %.3f%s test %.4f (%.0fs)\n", epoch, loss, val, acc, time.Since(start).Seconds())
		if checkpointPath != "" {
			c := checkpoint{Settings: s, Epochs: epoch + 1, EpochAccuracy: rep.EpochAccuracy, EpochValAccuracy: rep.EpochValAccuracy, TrainSeconds: time.Since(start).Seconds(), Trainer: tr.Snapshot()}
			if err := writeAtomic(checkpointPath, c); err != nil {
				return err
			}
		}
	}
	rep.TrainSeconds = time.Since(start).Seconds()
	rep.Accuracy = rep.EpochAccuracy[len(rep.EpochAccuracy)-1]
	if n := len(rep.EpochValAccuracy); n > 0 {
		rep.ValAccuracy = rep.EpochValAccuracy[n-1]
	}
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

// build lays out the sparse topology (topo, or uniform random sources when
// topo is nil) and the seeded initialization: each weight N(0, 1/in-degree of
// its target), encoder N(0, 1/28), readout U(-1/sqrt(r), 1/sqrt(r)), zero bias
// and tau = 2.
func build(s settings, topo *topology) (learning.Config, learning.Parameters, int) {
	rng := rand.New(rand.NewPCG(s.Seed, 1))
	var sources, targets []int
	if topo != nil {
		sources, targets = topo.sources, topo.targets
	} else {
		for t := 0; t < s.Nodes; t++ {
			for _, src := range rng.Perm(s.Nodes)[:s.InDegree] {
				sources, targets = append(sources, src), append(targets, t)
			}
		}
	}
	degree := make([]int, s.Nodes)
	for _, t := range targets {
		degree[t]++
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
		Dynamics:     dynamics.Config{Nodes: s.Nodes, Sources: sources, Targets: targets, DT: 1, Activation: activation(s)},
		InputSize:    side * s.columns(),
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
	weights := make([]float64, len(sources))
	for e, t := range targets {
		weights[e] = rng.NormFloat64() * (1 / math.Sqrt(float64(degree[t])))
	}
	logTau := make([]float64, s.Nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: weights, Bias: make([]float64, s.Nodes), LogTau: logTau},
		Encoder: normal(side*s.columns()*s.InputNodes, 1/math.Sqrt(float64(side*s.columns()))),
		Readout: readout,
	}
	// The optional mechanisms draw from their own streams, so turning one on
	// leaves the topology and every other initial value unchanged.
	if s.Delays > 0 {
		r := rand.New(rand.NewPCG(s.Seed, 3))
		cfg.Dynamics.Delays = make([]int, len(sources))
		for e := range cfg.Dynamics.Delays {
			cfg.Dynamics.Delays[e] = r.IntN(s.Delays + 1)
		}
	}
	if s.Dale {
		r := rand.New(rand.NewPCG(s.Seed, 4))
		sign := make([]int8, s.Nodes)
		for i := range sign {
			sign[i] = 1
			if r.Float64() < s.Inhibitory {
				sign[i] = -1
			}
		}
		cfg.EdgeSigns = make([]int8, len(sources))
		for e, src := range sources {
			cfg.EdgeSigns[e] = sign[src]
			weights[e] = math.Log(math.Abs(weights[e]))
		}
	}
	if s.TauMax > 0 {
		r := rand.New(rand.NewPCG(s.Seed, 5))
		for i := range logTau {
			logTau[i] = math.Log(2) + r.Float64()*(math.Log(s.TauMax)-math.Log(2))
		}
	}
	if s.Core == "lif" {
		d := cfg.Dynamics
		lif := dynamics.LIFConfig{
			Nodes: d.Nodes, Sources: d.Sources, Targets: d.Targets, Delays: d.Delays, DT: d.DT,
			TauSyn: 2, ThetaMin: 0.05, ThetaMax: 1, VReset: 0,
			Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: s.SurrogateScale},
		}
		if s.Adapt {
			lif.Adaptation = dynamics.LIFAdaptation{Enabled: true, TauAdapt: 20, Beta: 0.2}
		}
		if s.Homeostasis {
			lif.Homeostasis = &dynamics.LIFHomeostasis{Enabled: true, TauRate: 50, TargetRate: 0.1, Eta: 0.01, HMax: 0.5}
		}
		cfg.Dynamics, cfg.LIF = dynamics.Config{}, &lif
		p.ThetaRaw = make([]float64, s.Nodes)
	}
	if s.Gate {
		cfg.Dynamics.Gate = dynamics.GateCurrent
		if s.GateSquash == "softsign" {
			cfg.Dynamics.Gate = dynamics.GateCurrentSoftsign
		}
		p.Core.GateGain, p.Core.GateBias = make([]float64, s.Nodes), make([]float64, s.Nodes)
	}
	if s.ChemChannels > 0 {
		cfg.Dynamics.Chemistry, p.Core.ChemRelease, p.Core.ChemGain, p.Core.ChemOffset = buildChemistry(s)
	}
	tiedAway := 0
	if topo != nil && s.Tie != "" {
		types := topo.types
		if s.Tie == "type-shuffled" {
			types = shuffleTypes(types, s.Seed)
		}
		cfg.Sharing, tiedAway = tieByType(types, sources, targets, &p)
	}
	return cfg, p, tiedAway
}

// freeze turns off the training of each named parameter group.
func freeze(t learning.Trainable, list string) (learning.Trainable, error) {
	if list == "" {
		return t, nil
	}
	for _, name := range strings.Split(list, ",") {
		switch strings.TrimSpace(name) {
		case "encoder":
			t.Encoder = false
		case "weights":
			t.Weights = false
		case "bias":
			t.Bias = false
		case "tau":
			t.Tau = false
		case "gate":
			t.Gate = false
		case "theta":
			t.Theta = false
		case "readout":
			t.Readout = false
		default:
			return t, fmt.Errorf("--freeze: unknown parameter group %q", name)
		}
	}
	return t, nil
}

// normalizeChemistry zeroes the chemistry settings of a run without the
// block, so its recorded settings and checkpoints match a run made before
// the flags existed.
func normalizeChemistry(s settings) settings {
	if s.ChemChannels == 0 {
		s.ChemTau, s.ChemKd, s.ChemHill, s.ChemSource, s.ChemReceptors, s.ChemInit = 0, 0, 0, "", 0, 0
	}
	return s
}

func checkChemistry(s settings) error {
	if s.ChemChannels == 0 {
		return nil
	}
	if s.ChemChannels < 0 || s.ChemChannels > dynamics.MaxChemistryChannels {
		return fmt.Errorf("--chem-channels must be in [0, %d]", dynamics.MaxChemistryChannels)
	}
	if s.GPU {
		return errors.New("--chem-channels runs on the CPU core only; drop --gpu")
	}
	if s.Core != "" {
		return errors.New("--chem-channels needs the continuous core; drop --core")
	}
	if s.ChemTau <= 0 || s.ChemKd < 0 || s.ChemHill <= 0 || s.ChemReceptors <= 0 || s.ChemReceptors > 1 || s.ChemInit < 0 {
		return errors.New("--chem-tau and --chem-hill must be positive, --chem-kd and --chem-init nonnegative and --chem-receptors in (0, 1]")
	}
	switch s.ChemSource {
	case "all", "input", "readout":
	case "split":
		if s.ChemChannels > s.Nodes {
			return errors.New(`--chem-source "split" needs at most one channel per node`)
		}
	case "hidden":
		if s.InputNodes+s.ReadoutNodes >= s.Nodes {
			return errors.New(`--chem-source "hidden" needs nodes that are neither input nor readout`)
		}
	default:
		return fmt.Errorf("unknown --chem-source %q", s.ChemSource)
	}
	return nil
}

// buildChemistry declares the chemistry block and draws its initial values
// from a PCG stream of its own, so the base model's initialization is the
// same with and without it. The effect range is gamma in [0, 3] and
// |beta| <= 1, fixed engineering bounds.
func buildChemistry(s settings) (*dynamics.Chemistry, []float64, []float64, []float64) {
	rng := rand.New(rand.NewPCG(s.Seed, 7))
	kd := s.ChemKd
	if kd == 0 {
		kd = s.ChemTau * math.Ln2
	}
	c := &dynamics.Chemistry{GammaMin: 0, GammaMax: 3, BetaAbsMax: 1}
	readoutStart := s.Nodes - s.ReadoutNodes
	for k := 0; k < s.ChemChannels; k++ {
		var sources []int
		for j := 0; j < s.Nodes; j++ {
			var keep bool
			switch s.ChemSource {
			case "all":
				keep = true
			case "split":
				keep = j*s.ChemChannels/s.Nodes == k
			case "input":
				keep = j < s.InputNodes
			case "readout":
				keep = j >= readoutStart
			case "hidden":
				keep = j >= s.InputNodes && j < readoutStart
			}
			if keep {
				sources = append(sources, j)
			}
		}
		c.Channels = append(c.Channels, dynamics.ChemistryChannel{Tau: s.ChemTau, Kd: kd, Hill: s.ChemHill, Sources: sources})
	}
	for i := 0; i < s.Nodes; i++ {
		for k := 0; k < s.ChemChannels; k++ {
			if s.ChemReceptors >= 1 || rng.Float64() < s.ChemReceptors {
				c.Receptors = append(c.Receptors, dynamics.ChemistryReceptor{Node: i, Channel: k})
			}
		}
	}
	release := make([]float64, s.ChemChannels)
	for k := range release {
		release[k] = 1 + s.ChemInit/10*rng.NormFloat64()
	}
	gain, offset := make([]float64, len(c.Receptors)), make([]float64, len(c.Receptors))
	for e := range gain {
		gain[e], offset[e] = s.ChemInit*rng.NormFloat64(), s.ChemInit*rng.NormFloat64()
	}
	return c, release, gain, offset
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

// evaluateBatched is evaluate through Trainer.PredictBatch, so a --gpu
// trainer runs the test episodes' core on the device in float32, in chunks of
// evalChunk images.
func evaluateBatched(ctx context.Context, tr *learning.Trainer, xs [][][]float64, ys []int, workers int) (float64, error) {
	const evalChunk = 256
	count := 0
	for start := 0; start < len(xs); start += evalChunk {
		end := min(start+evalChunk, len(xs))
		logits, err := tr.PredictBatch(ctx, xs[start:end], workers)
		if err != nil {
			return 0, err
		}
		for i, l := range logits {
			if argmax(l) == ys[start+i] {
				count++
			}
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
	if s.Store != "" {
		name = fmt.Sprintf("store-%s-n%d-k%d", strings.TrimSuffix(filepath.Base(s.Store), filepath.Ext(s.Store)), s.Nodes, s.InDegree)
		if s.Rewire != "" {
			name += "-rewire-" + s.Rewire
		}
	}
	if c := s.columns(); c > 1 {
		name += fmt.Sprintf("-cols%d", c)
	}
	if a := activation(s); a != "tanh" {
		name += "-" + a
	}
	if s.Gate {
		name += "-gate"
		if s.GateSquash != "" {
			name += "-" + s.GateSquash
		}
	}
	if s.ChemChannels > 0 {
		name += fmt.Sprintf("-chem%d", s.ChemChannels)
	}
	return name
}

func activation(s settings) string {
	if s.Activation == "" {
		return "tanh"
	}
	return s.Activation
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
	// Workers only changes how a batch's gradients are scheduled, GPU only
	// where the core runs, and the data directory is not recorded.
	want.Workers, got.Workers, want.DataDir = 0, 0, ""
	want.GPU, got.GPU = false, false
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

// holdOut splits the last n images off as the validation split. The
// official train split is not ordered by class, so its tail is a fair sample.
func holdOut(xs [][][]float64, ys []int, n int) (trainX, valX [][][]float64, trainY, valY []int, err error) {
	if n < 0 || n >= len(xs) {
		return nil, nil, nil, nil, fmt.Errorf("--val-count %d must be in [0, %d)", n, len(xs))
	}
	k := len(xs) - n
	return xs[:k], xs[k:], ys[:k], ys[k:], nil
}

// readSplit reads the first limit images of a split as [column][row] pixel
// sequences scaled to [0, 1], with their labels.
// readSplit returns each image as 28/cols steps; step t holds columns
// t*cols..t*cols+cols-1, each top to bottom.
func readSplit(dir, split string, limit, cols int) ([][][]float64, []int, error) {
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
		xs[i] = make([][]float64, side/cols)
		for t := range xs[i] {
			xs[i][t] = make([]float64, side*cols)
			for c := range cols {
				col := t*cols + c
				for row := 0; row < side; row++ {
					xs[i][t][c*side+row] = float64(img[row*side+col]) / 255
				}
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
