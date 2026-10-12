package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"time"

	"github.com/TimLai666/coimnet/backend/webgpu"
	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

type result struct {
	Snapshot    learning.TrainingSnapshot `json:"snapshot"`
	Prediction  []float64                 `json:"prediction"`
	Adapters    learning.GPUAdapterInfo   `json:"adapters"`
	Optimizer   webgpu.AdamWMetrics       `json:"optimizer"`
	StepSeconds []float64                 `json:"step_seconds"`
	Steps       []learning.StepResult     `json:"steps"`
}

var input = [][]float64{{.7, -.2}, {-.3, .5}, {.2, .4}, {-.1, -.6}}
var suffix = []float64{-.2, .3, -.1, .6, -.4, .2, .1}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
func write(path string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	check(err)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	check(err)
	_, err = f.Write(append(b, '\n'))
	check(err)
	check(f.Close())
}
func normalized(s learning.TrainingSnapshot) learning.TrainingSnapshot {
	// Training bundles normalize required empty delay arrays. Numeric values,
	// optimizer state and optional pointers are compared without modification.
	if s.Config.Dynamics.Delays == nil {
		s.Config.Dynamics.Delays = []int{}
	}
	return s
}
func finish(ctx context.Context, t *learning.Trainer) result {
	r := result{}
	for _, target := range suffix {
		start := time.Now()
		step, err := t.Step(ctx, input, []float64{target})
		check(err)
		r.StepSeconds = append(r.StepSeconds, time.Since(start).Seconds())
		r.Steps = append(r.Steps, step)
	}
	r.Snapshot = normalized(t.Snapshot())
	var err error
	r.Prediction, err = t.Predict(ctx, input)
	check(err)
	var ok bool
	r.Adapters, ok = t.GPUAdapterInfo()
	if !ok {
		panic("missing actual sparse GPU identities")
	}
	r.Optimizer, ok = t.GPUAdamWMetrics()
	if !ok || r.Optimizer.Dispatch == 0 {
		panic("missing actual device optimizer dispatch")
	}
	return r
}
func flat(p learning.Parameters) []float64 {
	v := append([]float64{}, p.Core.Weights...)
	v = append(v, p.Core.Bias...)
	v = append(v, p.Core.LogTau...)
	v = append(v, p.ThetaRaw...)
	v = append(v, p.Encoder...)
	return append(v, p.Readout...)
}
func maxDifference(a, b []float64) float64 {
	if len(a) != len(b) {
		panic("shape mismatch")
	}
	var m float64
	for i := range a {
		d := math.Abs(a[i] - b[i])
		if math.IsNaN(d) || math.IsInf(d, 0) {
			panic("nonfinite comparison")
		}
		m = math.Max(m, d)
	}
	return m
}
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	if len(os.Args) != 3 {
		panic("usage: bundle_flow parent OUTPUT_DIRECTORY | restore OUTPUT_DIRECTORY")
	}
	mode, dir := os.Args[1], os.Args[2]
	bundle := filepath.Join(dir, "partial-bundle")
	if mode == "restore" {
		s, err := checkpoint.LoadTrainingBundle(ctx, bundle)
		check(err)
		t, err := learning.RestoreGPUDeviceTrainer(ctx, s, 1<<20)
		check(err)
		if !reflect.DeepEqual(normalized(t.Snapshot()), s) {
			panic("restore changed saved partial state")
		}
		r := finish(ctx, t)
		check(t.Close())
		write(filepath.Join(dir, "child.json"), r)
		return
	}
	if mode != "parent" {
		panic("unknown mode")
	}
	check(os.Mkdir(dir, 0700))
	c := learning.Config{Dynamics: dynamics.Config{Nodes: 4, Sources: []int{0, 1, 2, 3, 0}, Targets: []int{1, 2, 3, 0, 2}, DT: .4, Activation: "tanh"}, InputSize: 2, OutputSize: 1, ReadoutNodes: []int{2, 3}}
	p := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{.35, -.2, .4, .15, -.1}, Bias: []float64{.03, -.02, .01, .04}, LogTau: []float64{.1, -.05, .02, .08}}, Encoder: []float64{.4, -.15, .25, -.3, .2, .1, .05, -.25}, Readout: []float64{.7, -.45}}
	o := learning.DefaultOptions()
	o.LearningRate = .023
	o.WeightDecay = .01
	o.ClipNorm = .01
	o.AccumulateSteps = 3
	o.Schedule = &learning.Schedule{Kind: learning.ScheduleConstant, WarmupUpdates: 1}
	o.Masks = &learning.UpdateMasks{Edges: []bool{true, false, true, false, true}, Nodes: []bool{true, false, true, false}}
	started := time.Now()
	t, err := learning.NewGPUDeviceTrainer(ctx, c, p, o, 1<<20)
	check(err)
	construction := time.Since(started).Seconds()
	initial := t.Snapshot()
	cpu, err := learning.NewTrainer(initial.Config, initial.Parameters, initial.Options)
	check(err)
	before, err := t.Predict(ctx, input)
	check(err)
	prefix, err := t.Step(ctx, input, []float64{.4})
	check(err)
	if prefix.Applied || prefix.Accumulated != 1 {
		panic("prefix did not preserve partial accumulation")
	}
	_, err = cpu.Step(ctx, input, []float64{.4})
	check(err)
	saved := normalized(t.Snapshot())
	write(filepath.Join(dir, "saved.json"), saved)
	check(checkpoint.SaveTrainingBundle(ctx, bundle, saved))
	loaded, err := checkpoint.LoadTrainingBundle(ctx, bundle)
	check(err)
	if !reflect.DeepEqual(loaded, saved) {
		panic("actual bundle load changed saved full state")
	}
	continuous := finish(ctx, t)
	for _, target := range suffix {
		_, err = cpu.Step(ctx, input, []float64{target})
		check(err)
	}
	write(filepath.Join(dir, "continuous.json"), continuous)
	check(t.Close())
	check(cpu.Close())
	exe, err := os.Executable()
	check(err)
	cmd := exec.CommandContext(ctx, exe, "restore", dir)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("child failed: %v\n%s", err, output))
	}
	var child result
	b, err := os.ReadFile(filepath.Join(dir, "child.json"))
	check(err)
	check(json.Unmarshal(b, &child))
	if !reflect.DeepEqual(child.Snapshot, continuous.Snapshot) || !reflect.DeepEqual(child.Prediction, continuous.Prediction) || !reflect.DeepEqual(child.Steps, continuous.Steps) {
		panic("new-process actual bundle continuation diverged")
	}
	if reflect.DeepEqual(before, continuous.Prediction) {
		panic("prediction did not use new parameters")
	}
	difference := maxDifference(flat(cpu.Snapshot().Parameters), flat(continuous.Snapshot.Parameters))
	if difference > 7e-5 {
		panic(fmt.Sprintf("CPU parameter difference exceeds fixture tolerance: %g", difference))
	}
	write(filepath.Join(dir, "verification.json"), map[string]any{"passed": true, "scope": "deterministic 4-node sparse fixture; actual training bundle in separate Metal process; not fullbrain or useful-model evidence", "construction_seconds": construction, "updates": continuous.Snapshot.Updates, "final_accumulated": continuous.Snapshot.Accumulator.Count, "cpu_parameter_max_abs_error": difference, "same_snapshot": true, "same_prediction": true, "same_steps": true, "optimizer": continuous.Optimizer, "adapters": continuous.Adapters})
	fmt.Println("actual bundle/new-process GPU continuation passed")
}
