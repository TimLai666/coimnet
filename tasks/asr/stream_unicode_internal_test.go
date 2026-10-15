package asr

import (
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

func TestNewStreamRejectsInvalidAlphabetRune(t *testing.T) {
	config := RecognizerConfig{
		SampleRate: 16000,
		FrontEnd: audio.FrontEndConfig{
			Window: 16,
			Hop:    8,
			Mels:   2,
			FMin:   0,
			FMax:   8000,
			Floor:  1e-10,
		},
		Hidden:       2,
		Alphabet:     []rune{'a', 'b', 'c'},
		LearningRate: 0.01,
		Seed:         7,
	}
	r, err := NewRecognizer(config)
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	r.config.Alphabet = []rune{0xd800, 'b', 'c'}
	if _, err := r.NewStream(); err == nil {
		t.Fatal("NewStream accepted an invalid Unicode alphabet rune")
	}
}
