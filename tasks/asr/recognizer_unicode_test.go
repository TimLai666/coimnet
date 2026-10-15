package asr_test

import (
	"fmt"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

func unicodeASRConfig(alphabet []rune) asr.RecognizerConfig {
	return asr.RecognizerConfig{
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
		Alphabet:     append([]rune(nil), alphabet...),
		LearningRate: 0.01,
		Seed:         7,
	}
}

func TestRecognizerAlphabetUnicodeScalar(t *testing.T) {
	invalid := []rune{-1, 0xd800, 0xdfff, 0x110000, 0x7fffffff}
	for i, bad := range invalid {
		bad := bad
		for position, alphabet := range [][]rune{{bad, 'A'}, {'A', bad}} {
			position, alphabet := position, alphabet
			t.Run(fmt.Sprintf("invalid_%d_position_%d", i, position), func(t *testing.T) {
				config := unicodeASRConfig(alphabet)
				if err := config.Validate(); err == nil {
					t.Fatal("Validate accepted an invalid Unicode scalar")
				}
				got, err := asr.NewRecognizer(config)
				if err == nil || got != nil {
					t.Fatalf("NewRecognizer = (%v, %v), want (nil, error)", got, err)
				}
			})
		}
	}

	legal := []rune{'A', 0, 0xd7ff, 0xe000, 0xfffd, 0x10ffff}
	for i, scalar := range legal {
		scalar := scalar
		t.Run(fmt.Sprintf("legal_%d", i), func(t *testing.T) {
			config := unicodeASRConfig([]rune{scalar})
			if err := config.Validate(); err != nil {
				t.Fatalf("Validate rejected U+%04X: %v", scalar, err)
			}
			got, err := asr.NewRecognizer(config)
			if err != nil || got == nil {
				t.Fatalf("NewRecognizer = (%v, %v), want recognizer", got, err)
			}
		})
	}
}
