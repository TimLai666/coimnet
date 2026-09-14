package checkpoint_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// ExampleSaveIndividual is a small, deterministic SDK round trip that can be
// run directly with `go test ./checkpoint -run ExampleSaveIndividual -count=1 -v`.
func ExampleSaveIndividual() {
	dir, err := os.MkdirTemp("", "coimnet-individual-example-")
	if err != nil {
		fmt.Println("create temp:", err)
		return
	}
	defer os.RemoveAll(dir)

	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      2,
			Sources:    []int{0},
			Targets:    []int{1},
			Delays:     []int{1},
			DT:         .5,
			Activation: "tanh",
		},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.25}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		Encoder: []float64{.5, .2},
		Readout: []float64{.8},
	}
	individual, err := learning.NewIndividual(config, parameters, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		fmt.Println("create individual:", err)
		return
	}
	first, err := individual.Advance(context.Background(), [][]float64{{1}, {0}})
	if err != nil {
		fmt.Println("advance:", err)
		return
	}

	path := filepath.Join(dir, "individual.json")
	if err := checkpoint.SaveIndividual(context.Background(), path, individual.Snapshot()); err != nil {
		fmt.Println("save:", err)
		return
	}
	loaded, err := checkpoint.LoadIndividual(context.Background(), path)
	if err != nil {
		fmt.Println("load:", err)
		return
	}
	restored, err := learning.RestoreIndividual(loaded)
	if err != nil {
		fmt.Println("restore:", err)
		return
	}
	second, err := restored.Advance(context.Background(), [][]float64{{0}})
	if err != nil {
		fmt.Println("resume:", err)
		return
	}
	fmt.Println(len(first), len(second), loaded.Profile == learning.IndividualProfile)
	// Output:
	// 2 1 true
}
