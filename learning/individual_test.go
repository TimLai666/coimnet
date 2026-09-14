package learning_test

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestIndividualContinuousMatchesEpisodeAndSplit(t *testing.T) {
	n, p := network(t)
	c := n.Config()
	// Keep the fixture edge count while exercising delays across calls.
	c.Dynamics.Delays = make([]int, len(c.Dynamics.Sources))
	for e := range c.Dynamics.Delays {
		c.Dynamics.Delays[e] = e % 3
	}
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, c.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	b, err := learning.RestoreIndividual(a.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	x := [][]float64{{.7}, {.2}, {0}, {-.1}, {0}, {.3}}
	all, err := a.Advance(context.Background(), x)
	if err != nil {
		t.Fatal(err)
	}
	var split [][]float64
	for _, part := range [][][]float64{x[:1], x[1:3], x[3:]} {
		y, err := b.Advance(context.Background(), part)
		if err != nil {
			t.Fatal(err)
		}
		split = append(split, y...)
	}
	if !reflect.DeepEqual(all, split) || !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("split persistent evolution diverged")
	}
	want, err := n.Predict(context.Background(), p, x)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(all[len(all)-1], want) {
		t.Fatalf("episode boundary differs: %v != %v", all, want)
	}
}

func TestIndividualFourWaySeparationAndResets(t *testing.T) {
	n, p := network(t)
	c := n.Config()
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, c.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	b, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, c.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	untouched := b.Snapshot()
	initial := a.Snapshot()
	if _, err = a.Advance(context.Background(), [][]float64{{.7}, {0}}); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err = a.TrainEpisode(context.Background(), [][]float64{{.7}, {0}, {0}}, []float64{.4}); err != nil {
			t.Fatal(err)
		}
	}
	learned := a.Snapshot()
	if reflect.DeepEqual(initial.Parameters, learned.Parameters) || learned.Optimizer.Updates != 10 {
		t.Fatal("learning did not change this individual's parameters")
	}
	if !reflect.DeepEqual(b.Snapshot(), untouched) || !reflect.DeepEqual(learned.Config, initial.Config) || learned.ConfigHash != initial.ConfigHash {
		t.Fatal("learning altered another individual or anatomy")
	}
	rebuilt := learning.IndividualSnapshot{SchemaVersion: learned.SchemaVersion, Profile: learned.Profile, ConfigHash: learned.ConfigHash}
	dir := t.TempDir()
	for _, part := range []struct {
		name                string
		source, destination any
	}{
		{"anatomy", learned.Config, &rebuilt.Config}, {"parameters", learned.Parameters, &rebuilt.Parameters}, {"neural", learned.Neural, &rebuilt.Neural}, {"optimizer", learned.Optimizer, &rebuilt.Optimizer},
	} {
		data, err := json.Marshal(part.source)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, part.name+".json")
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		saved, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(saved, part.destination); err != nil {
			t.Fatal(err)
		}
	}
	restored, err := learning.RestoreIndividual(rebuilt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(learned, restored.Snapshot()) {
		t.Fatal("separately persisted parts did not reconstruct")
	}

	if err = a.ResetNeural(context.Background(), make([]float64, c.Dynamics.Nodes)); err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot()
	if !reflect.DeepEqual(s.Parameters, learned.Parameters) || !reflect.DeepEqual(s.Optimizer, learned.Optimizer) || !reflect.DeepEqual(s.Config, learned.Config) || s.Neural.Steps != 0 {
		t.Fatal("neural reset crossed ownership")
	}
	if err = a.ResetParameters(context.Background(), initial.Parameters); err != nil {
		t.Fatal(err)
	}
	q := a.Snapshot()
	if !reflect.DeepEqual(q.Parameters, initial.Parameters) || !reflect.DeepEqual(q.Optimizer, s.Optimizer) || !reflect.DeepEqual(q.Neural, s.Neural) {
		t.Fatal("parameter reset crossed ownership")
	}
	if err = a.ResetOptimizer(context.Background(), learning.DefaultOptions()); err != nil {
		t.Fatal(err)
	}
	r := a.Snapshot()
	if r.Optimizer.Updates != 0 || !reflect.DeepEqual(r.Parameters, q.Parameters) || !reflect.DeepEqual(r.Neural, q.Neural) || !reflect.DeepEqual(r.Config, q.Config) {
		t.Fatal("optimizer reset crossed ownership")
	}
	if !reflect.DeepEqual(r.Optimizer, initial.Optimizer) {
		t.Fatal("optimizer moments did not reset")
	}
}

func TestIndividualOwnsAllBuffers(t *testing.T) {
	n, p := network(t)
	c := n.Config()
	v := make([]float64, c.Dynamics.Nodes)
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), v)
	if err != nil {
		t.Fatal(err)
	}
	before := a.Snapshot()
	c.Dynamics.Sources[0] = 99
	c.ReadoutNodes[0] = 99
	p.Encoder[0] = 99
	p.Core.Weights[0] = 99
	v[0] = 99
	if !reflect.DeepEqual(before, a.Snapshot()) {
		t.Fatal("constructor retained aliases")
	}
	b, err := learning.RestoreIndividual(before)
	if err != nil {
		t.Fatal(err)
	}
	before.Config.ReadoutNodes[0] = 99
	before.Parameters.Readout[0] = 99
	before.Neural.Voltage[0] = 99
	before.Neural.History[0][0] = 99
	before.Optimizer.State.First[0] = 99
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("restore retained aliases")
	}
	y, err := a.Advance(context.Background(), [][]float64{{.5}})
	if err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot()
	y[0][0] = 99
	if !reflect.DeepEqual(s, a.Snapshot()) {
		t.Fatal("output aliases state")
	}
}

func TestIndividualFailureAtomicityAndVersions(t *testing.T) {
	n, p := network(t)
	c := n.Config()
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, c.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	before := a.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, x := range [][][]float64{nil, {{math.NaN()}}, {{1, 2}}} {
		if _, err = a.Advance(context.Background(), x); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if _, err = a.Advance(ctx, [][]float64{{1}}); err == nil {
		t.Fatal("cancel accepted")
	}
	if _, err = a.Advance(nil, [][]float64{{1}}); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err = a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{math.Inf(1)}); err == nil {
		t.Fatal("invalid target accepted")
	}
	if err = a.ResetNeural(context.Background(), nil); err == nil {
		t.Fatal("invalid initial accepted")
	}
	bad := p
	bad.Encoder = nil
	if err = a.ResetParameters(context.Background(), bad); err == nil {
		t.Fatal("bad parameters accepted")
	}
	opts := learning.DefaultOptions()
	opts.Beta1 = 1
	if err = a.ResetOptimizer(context.Background(), opts); err == nil {
		t.Fatal("bad optimizer accepted")
	}
	if !reflect.DeepEqual(before, a.Snapshot()) {
		t.Fatal("failed operation changed state")
	}
	for _, mutate := range []func(*learning.IndividualSnapshot){func(s *learning.IndividualSnapshot) { s.SchemaVersion = "unknown" }, func(s *learning.IndividualSnapshot) { s.Profile = "lif" }, func(s *learning.IndividualSnapshot) { s.ConfigHash = "bad" }, func(s *learning.IndividualSnapshot) { s.Config.ReadoutNodes[0] = 0 }, func(s *learning.IndividualSnapshot) { s.Neural.History = nil }, func(s *learning.IndividualSnapshot) { s.Optimizer.State.First[0] = 1 }} {
		s := a.Snapshot()
		mutate(&s)
		if _, err = learning.RestoreIndividual(s); err == nil {
			t.Fatal("bad snapshot accepted")
		}
	}
	var zero learning.Individual
	if _, err = zero.Advance(context.Background(), [][]float64{{1}}); err == nil {
		t.Fatal("zero individual accepted")
	}
	if _, err = zero.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{1}); err == nil {
		t.Fatal("zero trainer accepted")
	}
	if err = zero.ResetNeural(context.Background(), nil); err == nil {
		t.Fatal("zero reset accepted")
	}
}

func TestIndividualConcurrentIsolationAndSyncSnapshots(t *testing.T) {
	n, p := network(t)
	c := n.Config()
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, c.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	b, err := learning.RestoreIndividual(a.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	baseline := b.Snapshot()
	var wg sync.WaitGroup
	for mode := 0; mode < 3; mode++ {
		wg.Add(1)
		go func(mode int) {
			defer wg.Done()
			for range 20 {
				switch mode {
				case 0:
					if _, e := a.Advance(context.Background(), [][]float64{{.3}, {0}}); e != nil {
						t.Error(e)
					}
				case 1:
					if _, e := a.TrainEpisode(context.Background(), [][]float64{{.3}, {0}}, []float64{.2}); e != nil {
						t.Error(e)
					}
				case 2:
					if _, e := learning.RestoreIndividual(a.Snapshot()); e != nil {
						t.Error(e)
					}
					if !reflect.DeepEqual(b.Snapshot(), baseline) {
						t.Error("parallel snapshot observed learning from another individual")
					}
				}
			}
		}(mode)
	}
	wg.Wait()
	if !reflect.DeepEqual(b.Snapshot(), baseline) {
		t.Fatal("concurrent learning crossed individuals")
	}
	s := a.Snapshot()
	if s.Neural.Steps != 40 || s.Optimizer.Updates != 20 {
		t.Fatalf("lost/partial update: %d/%d", s.Neural.Steps, s.Optimizer.Updates)
	}
}

func TestIndividualReadoutFailurePreservesWholeCall(t *testing.T) {
	n, p := network(t)
	c := n.Config()
	c.ReadoutNodes = []int{0, 1}
	p.Core.Bias = []float64{0, 0}
	p.Encoder = []float64{1, 1}
	p.Readout = []float64{math.MaxFloat32, math.MaxFloat32}
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	before := a.Snapshot()
	if out, err := a.Advance(context.Background(), [][]float64{{0}, {100}}); err == nil || out != nil {
		t.Fatalf("accepted overflowing final readout: %v %v", out, err)
	}
	if !reflect.DeepEqual(before, a.Snapshot()) {
		t.Fatal("readout failure committed core steps")
	}
}

func TestIndividualSelectedInputsPersist(t *testing.T) {
	n, p := network(t)
	c := n.Config()
	c.InputNodes = []int{1}
	p.Encoder = []float64{.5}
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	input := [][]float64{{.4}, {0}, {-.2}}
	want, err := n.Predict(context.Background(), p, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range input[:2] {
		if _, err = a.Advance(context.Background(), [][]float64{row}); err != nil {
			t.Fatal(err)
		}
	}
	b, err := learning.RestoreIndividual(a.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.Advance(context.Background(), input[2:])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Fatalf("selected input restore: %v != %v", got, want)
	}
}
