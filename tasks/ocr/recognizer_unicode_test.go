package ocr_test

import (
	"fmt"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr"
)

func unicodeOCRConfig(alphabet []rune) ocr.RecognizerConfig {
	return ocr.RecognizerConfig{
		Height:       2,
		Hidden:       2,
		Alphabet:     append([]rune(nil), alphabet...),
		LearningRate: 0.01,
		Seed:         7,
	}
}

func TestRecognizerAlphabetUnicodeScalar(t *testing.T) {
	valid, err := ocr.NewRecognizer(unicodeOCRConfig([]rune{'A', 'B'}))
	if err != nil {
		t.Fatalf("NewRecognizer valid parameters: %v", err)
	}
	parameters := valid.Parameters()

	invalid := []rune{-1, 0xd800, 0xdfff, 0x110000, 0x7fffffff}
	for i, bad := range invalid {
		bad := bad
		for position, alphabet := range [][]rune{{bad, 'A'}, {'A', bad}} {
			position, alphabet := position, alphabet
			t.Run(fmt.Sprintf("invalid_%d_position_%d", i, position), func(t *testing.T) {
				config := unicodeOCRConfig(alphabet)
				if err := config.Validate(); err == nil {
					t.Fatal("Validate accepted an invalid Unicode scalar")
				}
				got, err := ocr.NewRecognizer(config)
				if err == nil || got != nil {
					t.Fatalf("NewRecognizer = (%v, %v), want (nil, error)", got, err)
				}
				got, err = ocr.NewRecognizerWith(config, parameters)
				if err == nil || got != nil {
					t.Fatalf("NewRecognizerWith = (%v, %v), want (nil, error)", got, err)
				}
			})
		}
	}

	legal := []rune{'A', 0, 0xd7ff, 0xe000, 0xfffd, 0x10ffff}
	for i, scalar := range legal {
		scalar := scalar
		t.Run(fmt.Sprintf("legal_%d", i), func(t *testing.T) {
			config := unicodeOCRConfig([]rune{scalar})
			if err := config.Validate(); err != nil {
				t.Fatalf("Validate rejected U+%04X: %v", scalar, err)
			}
			got, err := ocr.NewRecognizer(config)
			if err != nil || got == nil {
				t.Fatalf("NewRecognizer = (%v, %v), want recognizer", got, err)
			}
		})
	}
}
