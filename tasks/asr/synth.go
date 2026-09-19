package asr

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
)

// SynthConfig fixes the synthetic voice, so the fixture carries no audio file
// of its own. Character Alphabet[k-1] is CTC class k and is rendered as a pure
// tone of 300 + 150*k Hz held for DurationMS milliseconds; consecutive
// characters are separated by GapMS milliseconds of silence, and uniform noise
// of amplitude Noise is added to the whole signal last. The tone amplitude is
// 1-Noise, so the result stays inside [-1, 1] without clipping.
type SynthConfig struct {
	SampleRate int     `json:"sample_rate"`
	DurationMS int     `json:"duration_ms"`
	GapMS      int     `json:"gap_ms"`
	Noise      float64 `json:"noise"`
	Alphabet   []rune  `json:"alphabet"`
}

// Validate reports the first field that cannot render a signal: a non-positive
// sample rate, a character duration that is not at least one whole sample, a
// negative gap, a noise amplitude outside [0, 1] (NaN included), or an empty
// or repeating alphabet.
func (c SynthConfig) Validate() error {
	if c.SampleRate < 1 {
		return fmt.Errorf("asr: sample rate %d must be positive", c.SampleRate)
	}
	if c.DurationMS < 1 {
		return fmt.Errorf("asr: character duration %d ms must be at least 1 ms", c.DurationMS)
	}
	if c.GapMS < 0 {
		return fmt.Errorf("asr: gap %d ms must not be negative", c.GapMS)
	}
	if c.durationSamples() < 1 {
		return fmt.Errorf("asr: %d ms at %d Hz is less than one sample", c.DurationMS, c.SampleRate)
	}
	// Written as a negated conjunction so a NaN amplitude is rejected too.
	if !(c.Noise >= 0 && c.Noise <= 1) {
		return fmt.Errorf("asr: noise amplitude %v outside [0, 1]", c.Noise)
	}
	if len(c.Alphabet) == 0 {
		return errors.New("asr: alphabet is empty")
	}
	seen := make(map[rune]bool, len(c.Alphabet))
	for i, r := range c.Alphabet {
		if seen[r] {
			return fmt.Errorf("asr: alphabet repeats %q at position %d", r, i)
		}
		seen[r] = true
	}
	return nil
}

// durationSamples is how many samples one character occupies, truncated to a
// whole sample.
func (c SynthConfig) durationSamples() int { return c.SampleRate * c.DurationMS / 1000 }

// gapSamples is how many samples of silence separate two characters,
// truncated to a whole sample.
func (c SynthConfig) gapSamples() int { return c.SampleRate * c.GapMS / 1000 }

// Synthesize renders text (every rune must be in Alphabet) as one mono signal
// in [-1, 1]. The result has len(text)*DurationMS + (len(text)-1)*GapMS worth
// of samples, each tone starting at phase zero, and the noise is drawn from
// rand.NewPCG(seed, 0), so two calls with the same arguments agree bit for
// bit. Empty text is an error: there would be nothing to recognise.
func Synthesize(c SynthConfig, text string, seed uint64) ([]float64, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return nil, errors.New("asr: synthesize needs at least one character")
	}
	index := make(map[rune]int, len(c.Alphabet))
	for i, r := range c.Alphabet {
		index[r] = i + 1
	}
	classes := make([]int, len(runes))
	for i, r := range runes {
		k, ok := index[r]
		if !ok {
			return nil, fmt.Errorf("asr: %q is not in the alphabet", r)
		}
		classes[i] = k
	}

	duration, gap := c.durationSamples(), c.gapSamples()
	out := make([]float64, len(runes)*duration+(len(runes)-1)*gap)
	amplitude := 1 - c.Noise
	at := 0
	for _, k := range classes {
		frequency := 300 + 150*float64(k)
		step := 2 * math.Pi * frequency / float64(c.SampleRate)
		for n := 0; n < duration; n++ {
			out[at+n] = amplitude * math.Sin(step*float64(n))
		}
		// The gap after the last character is not part of the utterance, so
		// skipping past it simply runs off the end of the slice.
		at += duration + gap
	}
	rng := rand.New(rand.NewPCG(seed, 0))
	for i := range out {
		out[i] += (rng.Float64()*2 - 1) * c.Noise
	}
	return out, nil
}

// Utterance is one synthetic recording: the reference transcript, the samples
// (never serialised, since a fixture ships no audio) and the speaker and
// session a split can hold apart.
type Utterance struct {
	Text    string    `json:"text"`
	Samples []float64 `json:"-"`
	Speaker string    `json:"speaker"`
	Session string    `json:"session"`
}

// Generate draws n utterances of length in [minLen, maxLen] from the alphabet
// with rand.NewPCG(seed, 1); speaker/session are "spk<seed%4>"/"ses<seed>" so
// a split can hold them apart, and utterance i is synthesised with noise seed
// seed+i. Deterministic.
func Generate(c SynthConfig, n, minLen, maxLen int, seed uint64) ([]Utterance, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, fmt.Errorf("asr: utterance count %d must not be negative", n)
	}
	if minLen < 1 {
		return nil, fmt.Errorf("asr: minimum length %d must be at least 1", minLen)
	}
	if maxLen < minLen {
		return nil, fmt.Errorf("asr: maximum length %d is below the minimum %d", maxLen, minLen)
	}
	rng := rand.New(rand.NewPCG(seed, 1))
	speaker := fmt.Sprintf("spk%d", seed%4)
	session := fmt.Sprintf("ses%d", seed)
	out := make([]Utterance, 0, n)
	for i := 0; i < n; i++ {
		rs := make([]rune, minLen+rng.IntN(maxLen-minLen+1))
		for j := range rs {
			rs[j] = c.Alphabet[rng.IntN(len(c.Alphabet))]
		}
		text := string(rs)
		samples, err := Synthesize(c, text, seed+uint64(i))
		if err != nil {
			return nil, err
		}
		out = append(out, Utterance{Text: text, Samples: samples, Speaker: speaker, Session: session})
	}
	return out, nil
}
