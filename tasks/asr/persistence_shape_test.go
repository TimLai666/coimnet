package asr

import (
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

func TestCheckpointShapeRejectsSparseAmplificationBeforeReconstruction(t *testing.T) {
	const hidden = 50000
	config := RecognizerConfig{FrontEnd: audio.FrontEndConfig{Mels: 1}, Hidden: hidden, Alphabet: []rune{'a'}, LearningRate: 0.01}
	snapshot := learning.TrainingSnapshot{Config: learning.Config{
		Dynamics: dynamics.Config{Nodes: 1 + hidden}, InputSize: 1, OutputSize: 2,
		InputNodes: []int{0}, ReadoutNodes: make([]int, hidden),
	}}
	if err := validateASRCheckpointShape(config, snapshot); err == nil {
		t.Fatal("sparse checkpoint permits reconstruction of 2,500,050,000 dense edges")
	}
}
