package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
)

func TestTrainResumeAndObservationOnlyPrediction(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	second := filepath.Join(dir, "second.json")
	whole := filepath.Join(dir, "whole.json")
	run := func(args ...string) []byte {
		t.Helper()
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), args, &out, &stderr); err != nil {
			t.Fatalf("%v: %v %s", args, err, &stderr)
		}
		return out.Bytes()
	}
	run("train", "delayed", "--steps", "12", "--checkpoint", first)
	run("resume", "--checkpoint", first, "--steps", "8", "--out", second)
	run("train", "delayed", "--steps", "20", "--checkpoint", whole)
	a, err := checkpoint.Load(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	b, err := checkpoint.Load(context.Background(), whole)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("CLI resume differs from uninterrupted training")
	}
	input := filepath.Join(dir, "input.json")
	if err = os.WriteFile(input, []byte(`[[0.7],[0],[0],[0],[0]]`), 0600); err != nil {
		t.Fatal(err)
	}
	result := run("predict", "--checkpoint", second, "--input", input)
	var pred struct {
		Output []float64 `json:"output"`
	}
	if err = json.Unmarshal(result, &pred); err != nil || len(pred.Output) != 1 {
		t.Fatalf("prediction: %s %v", result, err)
	}
	var out, stderr bytes.Buffer
	if Run(context.Background(), []string{"train", "delayed", "--steps", "1", "--checkpoint", first}, &out, &stderr) == nil {
		t.Fatal("overwrote checkpoint")
	}
	if err = os.WriteFile(input, []byte(`{"input":[[0.7]],"target":[0.28]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if Run(context.Background(), []string{"predict", "--checkpoint", second, "--input", input}, &out, &stderr) == nil {
		t.Fatal("accepted target in inference input")
	}
	for _, args := range [][]string{{"train", "delayed", "--help"}, {"resume", "--help"}, {"predict", "--help"}} {
		run(args...)
	}
	for _, args := range [][]string{{"train", "delayed"}, {"train", "delayed", "--steps", "0", "--checkpoint", first}, {"resume", "--out", second}, {"predict", "--checkpoint", second}, {"resume", "--checkpoint", first, "--steps", "-1", "--out", second}} {
		if Run(context.Background(), args, &out, &stderr) == nil {
			t.Fatalf("accepted invalid command %v", args)
		}
	}
}
