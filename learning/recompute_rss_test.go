package learning_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// The LRN-02 memory comparison of ticket 25 item 11 takes T = 256 steps with
// the full reverse history against Recompute{SegmentSteps: 16}, on a random
// graph of recomputeRSSFanIn in-edges per node whose first recomputeRSSPorts
// nodes receive the input and whose last recomputeRSSPorts nodes feed the
// readout.
const (
	recomputeRSSSteps   = 256
	recomputeRSSSegment = 16
	recomputeRSSFanIn   = 8
	recomputeRSSPorts   = 64
)

// recomputeRSSRun is the one line TestRecomputeRSSHelper prints, in the field
// order scripts/recompute-evidence.sh copies into each run.
type recomputeRSSRun struct {
	Core             string  `json:"core"`
	Mode             string  `json:"mode"`
	Nodes            int     `json:"nodes"`
	Edges            int     `json:"edges"`
	Steps            int     `json:"steps"`
	Segment          int     `json:"segment"`
	GradientNorm     float64 `json:"gradient_norm"`
	UpdateNorm       float64 `json:"update_norm"`
	ParametersSHA256 string  `json:"parameters_sha256"`
}

// TestRecomputeRSSHelper is the child process whose maximum resident set size
// scripts/recompute-evidence.sh measures, so it is skipped unless
// COIMNET_RECOMPUTE_RSS is set. The value must be <core>:<mode>, core
// continuous or lif and mode full or recompute. COIMNET_RECOMPUTE_RSS_SCALE
// small selects 512 nodes; any other value, or none, selects 50000 continuous
// or 20000 LIF nodes. The helper takes one StepFrom over recomputeRSSSteps rows, row t
// being {1 + sin(t/7)} and only the last upstream row nonzero, then prints one
// JSON line on stdout whose parameters_sha256 hashes the json.Marshal of the
// trained parameters.
func TestRecomputeRSSHelper(t *testing.T) {
	value, ok := os.LookupEnv("COIMNET_RECOMPUTE_RSS")
	if !ok {
		t.Skip("COIMNET_RECOMPUTE_RSS is not set; scripts/recompute-evidence.sh runs this helper")
	}
	core, mode, _ := strings.Cut(value, ":")
	if (core != "continuous" && core != "lif") || (mode != "full" && mode != "recompute") {
		t.Fatalf("COIMNET_RECOMPUTE_RSS is %q, want <core>:<mode> with core continuous or lif and mode full or recompute", value)
	}
	nodes := 512
	if os.Getenv("COIMNET_RECOMPUTE_RSS_SCALE") != "small" {
		nodes = 50000
		if core == "lif" {
			nodes = 20000
		}
	}
	config, params, options := recomputeRSSModel(core, mode, nodes)
	trainer, err := learning.NewTrainer(config, params, options)
	if err != nil {
		t.Fatal(err)
	}
	input := make([][]float64, recomputeRSSSteps)
	upstream := make([][]float64, recomputeRSSSteps)
	for i := range input {
		input[i] = []float64{1 + math.Sin(float64(i)/7)}
		upstream[i] = []float64{0}
	}
	upstream[recomputeRSSSteps-1][0] = 1
	result, err := trainer.StepFrom(context.Background(), input, upstream)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := json.Marshal(trainer.Snapshot().Parameters)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(stored)
	segment := 0
	if options.Recompute != nil {
		segment = options.Recompute.SegmentSteps
	}
	line, err := json.Marshal(recomputeRSSRun{
		Core: core, Mode: mode, Nodes: nodes, Edges: len(params.Core.Weights), Steps: recomputeRSSSteps, Segment: segment,
		GradientNorm: result.GradientNorm, UpdateNorm: result.UpdateNorm, ParametersSHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(line))
}

// recomputeRSSModel returns the helper's model. Target by target, each of the
// recomputeRSSFanIn in-edges draws a source uniformly from PCG(1, 0), again
// while it repeats a source of the same target (a self edge is allowed), and
// then its delay uniformly from {0, 1, 2} on the same stream. Weights are
// uniform in [-0.05, 0.05) from PCG(1, 1), bias is zero, log_tau is log 2,
// the encoder is all ones and the readout weighs each of its nodes 1/64. The
// LIF core adds zero theta_raw and a trainable threshold group; mode recompute
// sets Recompute{SegmentSteps: recomputeRSSSegment} on DefaultOptions.
func recomputeRSSModel(core, mode string, nodes int) (learning.Config, learning.Parameters, learning.Options) {
	graph := rand.New(rand.NewPCG(1, 0))
	edges := nodes * recomputeRSSFanIn
	sources, targets, delays := make([]int, 0, edges), make([]int, 0, edges), make([]int, 0, edges)
	for target := range nodes {
		first := len(sources)
		for range recomputeRSSFanIn {
			source := graph.IntN(nodes)
			for slices.Contains(sources[first:], source) {
				source = graph.IntN(nodes)
			}
			sources = append(sources, source)
			targets = append(targets, target)
			delays = append(delays, graph.IntN(3))
		}
	}
	draw := rand.New(rand.NewPCG(1, 1))
	weights := make([]float64, edges)
	for i := range weights {
		weights[i] = -.05 + .1*draw.Float64()
	}
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	inputNodes, readoutNodes := make([]int, recomputeRSSPorts), make([]int, recomputeRSSPorts)
	encoder, readout := make([]float64, recomputeRSSPorts), make([]float64, recomputeRSSPorts)
	for i := range recomputeRSSPorts {
		inputNodes[i] = i
		readoutNodes[i] = nodes - recomputeRSSPorts + i
		encoder[i] = 1
		readout[i] = 1. / recomputeRSSPorts
	}
	config := learning.Config{InputSize: 1, InputNodes: inputNodes, ReadoutNodes: readoutNodes, OutputSize: 1}
	params := learning.Parameters{
		Core:    dynamics.Parameters{Weights: weights, Bias: make([]float64, nodes), LogTau: logTau},
		Encoder: encoder,
		Readout: readout,
	}
	options := learning.DefaultOptions()
	options.Truncation = 0
	if core == "lif" {
		config.LIF = &dynamics.LIFConfig{
			Nodes: nodes, Sources: sources, Targets: targets, Delays: delays,
			DT: .5, TauSyn: .7, ThetaMin: .3, ThetaMax: 1.4, VReset: -.5, RefractorySteps: 1,
			Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		}
		params.ThetaRaw = make([]float64, nodes)
		options.Trainable.Theta = true
	} else {
		config.Dynamics = dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, Delays: delays, DT: 1, Activation: "tanh"}
	}
	if mode == "recompute" {
		options.Recompute = &learning.Recompute{SegmentSteps: recomputeRSSSegment}
	}
	return config, params, options
}
