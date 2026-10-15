package asr_test

import (
	"bytes"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

type recognizerDimensionCase struct {
	name   string
	mels   int
	hidden int
}

type tinyRecognizerDimensionCase struct {
	name    string
	mels    int
	hidden  int
	nodes   int
	edges   int
	encoder int
	readout int
}

func recognizerDimensionConfig(mels, hidden int) asr.RecognizerConfig {
	return asr.RecognizerConfig{
		SampleRate: 16000,
		FrontEnd: audio.FrontEndConfig{
			Window: 16,
			Hop:    8,
			Mels:   mels,
			FMin:   0,
			FMax:   8000,
			Floor:  1e-10,
		},
		Hidden:       hidden,
		Alphabet:     []rune{'A'},
		LearningRate: 0.01,
		Seed:         7,
	}
}

func recognizerMaxInt() int {
	return int(^uint(0) >> 1)
}

func exactIntegerSqrt(n int) int {
	x := new(big.Int).SetInt64(int64(n))
	return int(new(big.Int).Sqrt(x).Int64())
}

// denseEdges is deliberately the same signed-int expression as the public
// constructor. The first five cases must prove its wrapped result is negative
// before NewRecognizer is called.
func denseEdges(mels, hidden int) int {
	return mels*hidden + hidden*hidden
}

func marshalRecognizerConfig(t *testing.T, config asr.RecognizerConfig) []byte {
	t.Helper()
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal recognizer config: %v", err)
	}
	return data
}

func newRecognizerSafely(config asr.RecognizerConfig) (recognizer *asr.Recognizer, err error, recovered any) {
	defer func() {
		recovered = recover()
	}()
	recognizer, err = asr.NewRecognizer(config)
	return
}

func assertFloat64CapacityBounds(t *testing.T, mels, hidden int) {
	t.Helper()
	limit := big.NewInt(int64(recognizerMaxInt() / 8))
	m := big.NewInt(int64(mels))
	h := big.NewInt(int64(hidden))
	classes := big.NewInt(2) // one alphabet rune plus the CTC blank
	nodes := new(big.Int).Add(new(big.Int).Set(m), new(big.Int).Set(h))
	edges := new(big.Int).Add(
		new(big.Int).Mul(new(big.Int).Set(m), new(big.Int).Set(h)),
		new(big.Int).Mul(new(big.Int).Set(h), new(big.Int).Set(h)),
	)
	encoder := new(big.Int).Mul(new(big.Int).Set(m), new(big.Int).Set(m))
	readout := new(big.Int).Mul(new(big.Int).Set(h), classes)
	for _, value := range []struct {
		name  string
		value *big.Int
	}{
		{name: "nodes", value: nodes},
		{name: "edges", value: edges},
		{name: "encoder", value: encoder},
		{name: "readout", value: readout},
	} {
		if value.value.Sign() < 0 || value.value.Cmp(limit) > 0 {
			t.Errorf("%s=%s exceeds the exact float64-element bound %s", value.name, value.value, limit)
		}
	}
}

func TestRecognizerDimensionRepresentability(t *testing.T) {
	maxInt := recognizerMaxInt()
	sqrtMaxInt := exactIntegerSqrt(maxInt)
	sqrtMaxIntOver8 := exactIntegerSqrt(maxInt / 8)

	invalidConstructorCases := []recognizerDimensionCase{
		{name: "invalid_mels1_hiddenMaxInt", mels: 1, hidden: maxInt},
		{name: "invalid_mels2_hiddenMaxInt", mels: 2, hidden: maxInt},
		{name: "invalid_mels4_hiddenMaxInt", mels: 4, hidden: maxInt},
		{name: "invalid_mels8_hiddenMaxInt", mels: 8, hidden: maxInt},
		{name: "invalid_mels2_hiddenHalfMaxIntPlus1", mels: 2, hidden: maxInt/2 + 1},
	}
	for _, tc := range invalidConstructorCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			config := recognizerDimensionConfig(tc.mels, tc.hidden)
			before := marshalRecognizerConfig(t, config)
			if err := config.Validate(); err == nil {
				t.Errorf("Validate accepted dimensions mels=%d hidden=%d", tc.mels, tc.hidden)
			}
			if after := marshalRecognizerConfig(t, config); !bytes.Equal(before, after) {
				t.Errorf("Validate changed caller config JSON\nbefore=%s\nafter=%s", before, after)
			}

			edges := denseEdges(tc.mels, tc.hidden)
			if edges >= 0 {
				t.Fatalf("constructor probe is unsafe: wrapped edges=%d is not negative", edges)
			}
			recognizer, err, recovered := newRecognizerSafely(config)
			if recovered != nil {
				t.Errorf("NewRecognizer panicked for negative-capacity dimensions: %v", recovered)
				return
			}
			if recognizer != nil {
				t.Errorf("NewRecognizer returned a recognizer for dimensions mels=%d hidden=%d", tc.mels, tc.hidden)
			}
			if err == nil {
				t.Errorf("NewRecognizer returned nil error for dimensions mels=%d hidden=%d", tc.mels, tc.hidden)
			}
			if after := marshalRecognizerConfig(t, config); !bytes.Equal(before, after) {
				t.Errorf("NewRecognizer changed caller config JSON\nbefore=%s\nafter=%s", before, after)
			}
		})
	}

	invalidValidateOnlyCases := []recognizerDimensionCase{
		{name: "invalid_mels1_hiddenSqrtMaxIntPlus1", mels: 1, hidden: sqrtMaxInt + 1},
		{name: "invalid_mels1_hiddenSqrtMaxIntOver8Plus1", mels: 1, hidden: sqrtMaxIntOver8 + 1},
		{name: "invalid_melsSqrtMaxIntPlus1_hidden2", mels: sqrtMaxInt + 1, hidden: 2},
		{name: "invalid_melsSqrtMaxIntOver8Plus1_hidden2", mels: sqrtMaxIntOver8 + 1, hidden: 2},
		{name: "invalid_mels1_hiddenMaxIntOver8Plus1", mels: 1, hidden: maxInt/8 + 1},
		{name: "invalid_melsMaxInt_hidden2", mels: maxInt, hidden: 2},
	}
	for _, tc := range invalidValidateOnlyCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			config := recognizerDimensionConfig(tc.mels, tc.hidden)
			before := marshalRecognizerConfig(t, config)
			if err := config.Validate(); err == nil {
				t.Errorf("Validate accepted dimensions mels=%d hidden=%d", tc.mels, tc.hidden)
			}
			if after := marshalRecognizerConfig(t, config); !bytes.Equal(before, after) {
				t.Errorf("Validate changed caller config JSON\nbefore=%s\nafter=%s", before, after)
			}
		})
	}

	validLargeCases := []recognizerDimensionCase{
		{name: "valid_large_melsSqrtMaxIntOver8_hidden2", mels: sqrtMaxIntOver8, hidden: 2},
		{name: "valid_large_mels1_hiddenSqrtMaxIntOver8Minus1", mels: 1, hidden: sqrtMaxIntOver8 - 1},
	}
	for _, tc := range validLargeCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertFloat64CapacityBounds(t, tc.mels, tc.hidden)
			config := recognizerDimensionConfig(tc.mels, tc.hidden)
			before := marshalRecognizerConfig(t, config)
			if err := config.Validate(); err != nil {
				t.Errorf("Validate rejected representable dimensions mels=%d hidden=%d: %v", tc.mels, tc.hidden, err)
			}
			if after := marshalRecognizerConfig(t, config); !bytes.Equal(before, after) {
				t.Errorf("Validate changed caller config JSON\nbefore=%s\nafter=%s", before, after)
			}
		})
	}

	tinyCases := []tinyRecognizerDimensionCase{
		{name: "valid_tiny_mels1_hidden2", mels: 1, hidden: 2, nodes: 3, edges: 6, encoder: 1, readout: 4},
		{name: "valid_tiny_mels2_hidden2", mels: 2, hidden: 2, nodes: 4, edges: 8, encoder: 4, readout: 4},
		{name: "valid_tiny_mels4_hidden2", mels: 4, hidden: 2, nodes: 6, edges: 12, encoder: 16, readout: 4},
	}
	for _, tc := range tinyCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			config := recognizerDimensionConfig(tc.mels, tc.hidden)
			before := marshalRecognizerConfig(t, config)
			if err := config.Validate(); err != nil {
				t.Errorf("Validate rejected tiny dimensions mels=%d hidden=%d: %v", tc.mels, tc.hidden, err)
				return
			}
			if after := marshalRecognizerConfig(t, config); !bytes.Equal(before, after) {
				t.Errorf("Validate changed caller config JSON\nbefore=%s\nafter=%s", before, after)
			}

			recognizer, err, recovered := newRecognizerSafely(config)
			if recovered != nil {
				t.Errorf("NewRecognizer panicked for tiny dimensions: %v", recovered)
				return
			}
			if err != nil {
				t.Errorf("NewRecognizer rejected tiny dimensions: %v", err)
				return
			}
			if recognizer == nil {
				t.Error("NewRecognizer returned a nil recognizer without an error")
				return
			}

			snapshot := recognizer.Snapshot()
			checks := []struct {
				name string
				got  int
				want int
			}{
				{name: "nodes", got: snapshot.Config.Dynamics.Nodes, want: tc.nodes},
				{name: "source count", got: len(snapshot.Config.Dynamics.Sources), want: tc.edges},
				{name: "target count", got: len(snapshot.Config.Dynamics.Targets), want: tc.edges},
				{name: "weight count", got: len(snapshot.Parameters.Core.Weights), want: tc.edges},
				{name: "bias count", got: len(snapshot.Parameters.Core.Bias), want: tc.nodes},
				{name: "log tau count", got: len(snapshot.Parameters.Core.LogTau), want: tc.nodes},
				{name: "encoder count", got: len(snapshot.Parameters.Encoder), want: tc.encoder},
				{name: "readout count", got: len(snapshot.Parameters.Readout), want: tc.readout},
			}
			for _, check := range checks {
				if check.got != check.want {
					t.Errorf("snapshot %s=%d, want %d", check.name, check.got, check.want)
				}
			}
		})
	}
}
