package asr_test

import (
	"crypto/sha256"
	"encoding/json"
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr"
)

func nativeMaxInt() int { return int(^uint(0) >> 1) }

func exactMillisSamples(rate, milliseconds int) *big.Int {
	product := new(big.Int).Mul(big.NewInt(int64(rate)), big.NewInt(int64(milliseconds)))
	return product.Quo(product, big.NewInt(1000))
}

func exactShape(c asr.SynthConfig, text string) *big.Int {
	runeCount := len([]rune(text))
	duration := exactMillisSamples(c.SampleRate, c.DurationMS)
	gap := exactMillisSamples(c.SampleRate, c.GapMS)
	result := new(big.Int).Mul(big.NewInt(int64(runeCount)), duration)
	if runeCount > 1 {
		separations := new(big.Int).Mul(big.NewInt(int64(runeCount-1)), gap)
		result.Add(result, separations)
	}
	return result
}

func synthesizeWithoutPanic(c asr.SynthConfig, text string, seed uint64) (samples []float64, err error, panicValue any) {
	defer func() {
		if p := recover(); p != nil {
			panicValue = p
		}
	}()
	samples, err = asr.Synthesize(c, text, seed)
	return
}

func TestSynthConfigExactSampleLengths(t *testing.T) {
	maxInt := nativeMaxInt()
	cases := []struct {
		name       string
		rate       int
		durationMS int
		wantError  bool
	}{
		{name: "normal_truncation", rate: 11025, durationMS: 17, wantError: false},
		{name: "too_short", rate: 999, durationMS: 1, wantError: true},
		{name: "wide_product_representable_duration", rate: 1000, durationMS: maxInt/1000 + 1, wantError: false},
		{name: "wide_rate_product_representable_duration", rate: maxInt, durationMS: 2, wantError: false},
		{name: "duration_quotient_exceeds_int", rate: maxInt, durationMS: 1001, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := asr.SynthConfig{SampleRate: tc.rate, DurationMS: tc.durationMS, Alphabet: []rune{'a'}}
			expected := exactMillisSamples(tc.rate, tc.durationMS)
			representable := expected.Sign() >= 1 && expected.Cmp(big.NewInt(int64(maxInt))) <= 0
			if tc.wantError != !representable {
				t.Fatalf("contract case has inconsistent oracle: duration samples %s, wantError=%v", expected, tc.wantError)
			}
			if err := c.Validate(); (err != nil) != tc.wantError {
				t.Fatalf("Validate() error=%v, want error=%v (exact duration samples %s)", err, tc.wantError, expected)
			}
		})
	}
}

func TestSynthesizeRejectsUnrepresentableLength(t *testing.T) {
	maxInt := nativeMaxInt()
	cases := []struct {
		name string
		c    asr.SynthConfig
		text string
	}{
		{name: "gap_max_int", c: asr.SynthConfig{SampleRate: 1000, DurationMS: 1, GapMS: maxInt, Alphabet: []rune{'a'}}, text: "aa"},
		{name: "gap_max_int_minus_one", c: asr.SynthConfig{SampleRate: 1000, DurationMS: 1, GapMS: maxInt - 1, Alphabet: []rune{'a'}}, text: "aa"},
		{name: "gap_quotient_exceeds_int", c: asr.SynthConfig{SampleRate: 2000, DurationMS: 1, GapMS: maxInt, Alphabet: []rune{'a'}}, text: "aa"},
		{name: "sample_bytes_exceed_int", c: asr.SynthConfig{SampleRate: 1000, DurationMS: 1, GapMS: maxInt / 2, Alphabet: []rune{'a'}}, text: "aa"},
		{name: "total_product_exceeds_int", c: asr.SynthConfig{SampleRate: 1, DurationMS: 1000, GapMS: maxInt, Alphabet: []rune{'a'}}, text: strings.Repeat("a", 1002)},
	}
	limit := big.NewInt(int64(maxInt / 8))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := exactShape(tc.c, tc.text)
			if expected.Cmp(limit) <= 0 {
				t.Fatalf("exact shape %s does not exceed MaxInt/8 capacity guard %s", expected, limit)
			}
			samples, err, panicValue := synthesizeWithoutPanic(tc.c, tc.text, 7)
			if panicValue != nil {
				t.Fatalf("Synthesize panicked for exact shape %s: %v", expected, panicValue)
			}
			if err == nil {
				t.Fatalf("Synthesize accepted exact shape %s; want a nonnil error", expected)
			}
			if samples != nil {
				t.Fatalf("Synthesize returned %d samples with error %v; want nil output", len(samples), err)
			}
		})
	}
}

type fingerprintUtterance struct {
	Text       string   `json:"text"`
	SampleBits []uint64 `json:"sample_bits"`
	Speaker    string   `json:"speaker"`
	Session    string   `json:"session"`
}

type normalFingerprint struct {
	Rendered  fingerprintUtterance   `json:"rendered"`
	Generated []fingerprintUtterance `json:"generated"`
}

func sampleBits(samples []float64) []uint64 {
	bits := make([]uint64, len(samples))
	for i, sample := range samples {
		bits[i] = math.Float64bits(sample)
	}
	return bits
}

func fingerprint(u asr.Utterance) fingerprintUtterance {
	return fingerprintUtterance{Text: u.Text, SampleBits: sampleBits(u.Samples), Speaker: u.Speaker, Session: u.Session}
}

func TestSynthShapeNormalAndEmptyGeneration(t *testing.T) {
	maxInt := nativeMaxInt()
	extreme := asr.SynthConfig{SampleRate: 1000, DurationMS: 1, GapMS: maxInt, Alphabet: []rune{'a'}}
	one, err := asr.Synthesize(extreme, "a", 7)
	if err != nil {
		t.Fatalf("one-character synthesis rejected unused extreme gap: %v", err)
	}
	if want := exactShape(extreme, "a"); want.Cmp(big.NewInt(int64(len(one)))) != 0 || len(one) != 1 {
		t.Fatalf("one-character extreme-gap length=%d, want exact one sample (%s)", len(one), want)
	}

	empty, err := asr.Generate(extreme, 0, 1, maxInt, 7)
	if err != nil {
		t.Fatalf("zero-count Generate rejected valid declared range: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("zero-count Generate returned %#v, want a nonnil empty slice", empty)
	}

	oneGenerated, err := asr.Generate(extreme, 1, 1, 1, 7)
	if err != nil {
		t.Fatalf("one-character Generate rejected unused extreme gap: %v", err)
	}
	if len(oneGenerated) != 1 || oneGenerated[0].Text != "a" || len(oneGenerated[0].Samples) != 1 {
		t.Fatalf("one-character Generate returned %#v, want one one-sample utterance", oneGenerated)
	}

	ordinary := asr.SynthConfig{SampleRate: 11025, DurationMS: 17, GapMS: 3, Noise: 0.2, Alphabet: []rune{'a', 'b', '界'}}
	rendered, err := asr.Synthesize(ordinary, "ab界", 7)
	if err != nil {
		t.Fatalf("ordinary Synthesize: %v", err)
	}
	wantRendered := exactShape(ordinary, "ab界")
	if wantRendered.Cmp(big.NewInt(int64(len(rendered)))) != 0 {
		t.Fatalf("ordinary Synthesize length=%d, want exact %s", len(rendered), wantRendered)
	}
	repeated, err := asr.Synthesize(ordinary, "ab界", 7)
	if err != nil {
		t.Fatalf("ordinary repeated Synthesize: %v", err)
	}
	if !reflect.DeepEqual(sampleBits(rendered), sampleBits(repeated)) {
		t.Fatal("ordinary Synthesize changed Float64bits across equal seeds")
	}

	generated, err := asr.Generate(ordinary, 3, 1, 3, 7)
	if err != nil {
		t.Fatalf("ordinary Generate: %v", err)
	}
	repeatedGenerated, err := asr.Generate(ordinary, 3, 1, 3, 7)
	if err != nil {
		t.Fatalf("ordinary repeated Generate: %v", err)
	}
	if len(generated) != 3 || !reflect.DeepEqual(fingerprintList(generated), fingerprintList(repeatedGenerated)) {
		t.Fatalf("ordinary Generate is not deterministic: %#v vs %#v", fingerprintList(generated), fingerprintList(repeatedGenerated))
	}
	validRune := map[rune]bool{'a': true, 'b': true, '界': true}
	for i, u := range generated {
		if runeCount := len([]rune(u.Text)); runeCount < 1 || runeCount > 3 {
			t.Fatalf("utterance %d text length=%d, want 1..3", i, runeCount)
		}
		for _, r := range []rune(u.Text) {
			if !validRune[r] {
				t.Fatalf("utterance %d contains rune %q outside the ordinary alphabet", i, r)
			}
		}
		if u.Speaker != "spk3" || u.Session != "ses7" {
			t.Fatalf("utterance %d metadata=(%q,%q), want (spk3,ses7)", i, u.Speaker, u.Session)
		}
		if want := exactShape(ordinary, u.Text); want.Cmp(big.NewInt(int64(len(u.Samples)))) != 0 {
			t.Fatalf("utterance %d sample length=%d, want exact %s", i, len(u.Samples), want)
		}
	}

	payload := normalFingerprint{
		Rendered:  fingerprintUtterance{Text: "ab界", SampleBits: sampleBits(rendered)},
		Generated: fingerprintList(generated),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal normal payload: %v", err)
	}
	digest := sha256.Sum256(encoded)
	t.Logf("normal payload sha256=%x bytes=%d", digest, len(encoded))
}

func fingerprintList(utterances []asr.Utterance) []fingerprintUtterance {
	result := make([]fingerprintUtterance, len(utterances))
	for i, u := range utterances {
		result[i] = fingerprint(u)
	}
	return result
}
