package audio_test

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

// frontEndRate is the sample rate every front-end test analyses at.
const frontEndRate = 16000

// defaultFrontEnd is the analysis the front-end tests use unless they say
// otherwise: a 512-sample Hann window, a 128-sample hop and 20 HTK mel filters
// spanning the whole 16 kHz band.
func defaultFrontEnd() audio.FrontEndConfig {
	return audio.FrontEndConfig{Window: 512, Hop: 128, Mels: 20, FMin: 0, FMax: 8000, Floor: 1e-10}
}

// editFrontEnd copies base, applies edit to the copy and returns it, so a table
// can name one broken field without restating the rest.
func editFrontEnd(base audio.FrontEndConfig, edit func(*audio.FrontEndConfig)) audio.FrontEndConfig {
	edit(&base)
	return base
}

// sine builds n samples of a unit-amplitude sine at hz.
func sine(hz float64, rate, n int) []float64 {
	x := make([]float64, n)
	for i := range x {
		x[i] = math.Sin(2 * math.Pi * hz * float64(i) / float64(rate))
	}
	return x
}

// argMax returns the index of the largest value in row, the lowest such index
// when several tie. An empty row yields -1.
func argMax(row []float64) int {
	best := -1
	for i, v := range row {
		if best < 0 || v > row[best] {
			best = i
		}
	}
	return best
}

// binHz is the centre frequency of FFT bin k.
func binHz(k, rate, window int) float64 {
	return float64(k) * float64(rate) / float64(window)
}

func TestSTFTPeaksAtTheSineFrequency(t *testing.T) {
	c := defaultFrontEnd()
	const tone = 440.0
	want := int(math.Round(tone * float64(c.Window) / float64(frontEndRate)))
	if want != 14 {
		t.Fatalf("expected peak bin for %g Hz = %d, want 14; the test's own arithmetic is wrong", tone, want)
	}

	spec, err := audio.STFT(sine(tone, frontEndRate, frontEndRate), c)
	if err != nil {
		t.Fatalf("STFT: %v", err)
	}
	if len(spec) == 0 {
		t.Fatal("STFT returned no frames")
	}
	for f, row := range spec {
		if got := argMax(row); got != want {
			t.Errorf("frame %d peaks at bin %d (%.2f Hz), want bin %d (%.2f Hz)",
				f, got, binHz(got, frontEndRate, c.Window), want, binHz(want, frontEndRate, c.Window))
		}
	}
	t.Logf("peak bin = %d (%.2f Hz) in all %d frames; frame 0 power: bin 13 = %.6g, bin 14 = %.6g, bin 15 = %.6g",
		want, binHz(want, frontEndRate, c.Window), len(spec), spec[0][13], spec[0][14], spec[0][15])
}

func TestSTFTFrameCount(t *testing.T) {
	c := defaultFrontEnd()
	// 1 + (16000-512)/128 = 122 frames of 512/2+1 = 257 bins.
	const (
		wantFrames = 122
		wantBins   = 257
	)
	spec, err := audio.STFT(make([]float64, frontEndRate), c)
	if err != nil {
		t.Fatalf("STFT: %v", err)
	}
	if len(spec) != wantFrames {
		t.Errorf("STFT over %d samples returned %d frames, want %d", frontEndRate, len(spec), wantFrames)
	}
	for f, row := range spec {
		if len(row) != wantBins {
			t.Fatalf("frame %d has %d bins, want %d", f, len(row), wantBins)
		}
	}

	if _, err := audio.STFT(make([]float64, 100), c); err == nil {
		t.Error("STFT over 100 samples = nil error, want one naming the window")
	}
	t.Logf("frames = %d, bins = %d", len(spec), len(spec[0]))
}

func TestHannWindowEndpoints(t *testing.T) {
	c := defaultFrontEnd()
	x := make([]float64, c.Window)
	for i := range x {
		x[i] = 1
	}

	sum := 0.0
	for n := 0; n < c.Window; n++ {
		sum += 0.5 - 0.5*math.Cos(2*math.Pi*float64(n)/float64(c.Window))
	}
	// The periodic Hann window starts at exactly 0 and sums to Window/2; the
	// symmetric variant (denominator Window-1) does not, so this pins which one
	// STFT applies.
	if math.Abs(sum-float64(c.Window)/2) > 1e-9 {
		t.Fatalf("sum of the Hann window = %v, want %v; the test's own window is wrong", sum, float64(c.Window)/2)
	}

	spec, err := audio.STFT(x, c)
	if err != nil {
		t.Fatalf("STFT: %v", err)
	}
	if len(spec) != 1 {
		t.Fatalf("STFT over exactly one window returned %d frames, want 1", len(spec))
	}
	want := sum * sum
	got := spec[0][0]
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("bin 0 power of a constant-1 signal = %.17g, want (sum w)^2 = %.17g (difference %.3g)",
			got, want, got-want)
	}
	t.Logf("sum w = %.17g, bin 0 power = %.17g, (sum w)^2 = %.17g, difference = %.3g", sum, got, want, got-want)
}

func TestMelFilterbankRowsArePositiveAndCover(t *testing.T) {
	c := defaultFrontEnd()
	bank, err := audio.MelFilterbank(frontEndRate, c)
	if err != nil {
		t.Fatalf("MelFilterbank: %v", err)
	}
	if len(bank) != c.Mels {
		t.Fatalf("MelFilterbank returned %d rows, want %d", len(bank), c.Mels)
	}

	bins := c.Window/2 + 1
	covered := make([]bool, bins)
	for m, row := range bank {
		if len(row) != bins {
			t.Fatalf("filter %d has %d weights, want %d", m, len(row), bins)
		}
		sum := 0.0
		for k, v := range row {
			if v < 0 || math.IsNaN(v) {
				t.Errorf("filter %d weight %d = %v, want a non-negative number", m, k, v)
			}
			if v > 0 {
				covered[k] = true
			}
			sum += v
		}
		if !(sum > 0) {
			t.Errorf("filter %d sums to %v, want a positive total", m, sum)
		}
	}

	for k, ok := range covered {
		f := binHz(k, frontEndRate, c.Window)
		if f <= c.FMin || f >= c.FMax {
			continue // bins sitting on the band limits carry weight 0 by construction
		}
		if !ok {
			t.Errorf("bin %d (%.2f Hz) lies inside [%v, %v] Hz but no filter covers it", k, f, c.FMin, c.FMax)
		}
	}
}

func TestLogMelShapeAndDeterminism(t *testing.T) {
	c := defaultFrontEnd()
	const wantFrames = 122
	x := sine(440, frontEndRate, frontEndRate)

	got, report, err := audio.LogMel(x, frontEndRate, c)
	if err != nil {
		t.Fatalf("LogMel: %v", err)
	}
	if len(got) != wantFrames {
		t.Fatalf("LogMel returned %d frames, want %d", len(got), wantFrames)
	}
	for f, row := range got {
		if len(row) != c.Mels {
			t.Fatalf("frame %d has %d mel bands, want %d", f, len(row), c.Mels)
		}
		for m, v := range row {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("LogMel[%d][%d] = %v, want a finite number", f, m, v)
			}
		}
	}

	if report.Version != audio.FrontEndVersion {
		t.Errorf("report.Version = %q, want %q", report.Version, audio.FrontEndVersion)
	}
	if audio.FrontEndVersion != "coimnet-logmel/v1" {
		t.Errorf("FrontEndVersion = %q, want %q", audio.FrontEndVersion, "coimnet-logmel/v1")
	}
	if report.SampleRate != frontEndRate {
		t.Errorf("report.SampleRate = %d, want %d", report.SampleRate, frontEndRate)
	}
	if report.Config != c {
		t.Errorf("report.Config = %+v, want %+v", report.Config, c)
	}
	if report.Frames != len(got) {
		t.Errorf("report.Frames = %d, want %d", report.Frames, len(got))
	}

	again, _, err := audio.LogMel(x, frontEndRate, c)
	if err != nil {
		t.Fatalf("LogMel (second run): %v", err)
	}
	if !reflect.DeepEqual(got, again) {
		t.Error("two LogMel runs over the same input differ, want identical output")
	}
	t.Logf("shape = [%d][%d], report = %+v, frame 0 band 0 = %.6g", len(got), len(got[0]), report, got[0][0])
}

func TestFrontEndConfigValidate(t *testing.T) {
	base := defaultFrontEnd()
	if err := base.Validate(frontEndRate); err != nil {
		t.Fatalf("Validate on the default config = %v, want nil", err)
	}

	cases := []struct {
		name string
		c    audio.FrontEndConfig
		want string
	}{
		{
			"window not a power of two",
			editFrontEnd(base, func(c *audio.FrontEndConfig) { c.Window = 500 }),
			"power of two",
		},
		{"hop zero", editFrontEnd(base, func(c *audio.FrontEndConfig) { c.Hop = 0 }), "hop"},
		{"no mel filters", editFrontEnd(base, func(c *audio.FrontEndConfig) { c.Mels = 0 }), "mel filter"},
		{"fmax above nyquist", editFrontEnd(base, func(c *audio.FrontEndConfig) { c.FMax = 9000 }), "band"},
		{"floor zero", editFrontEnd(base, func(c *audio.FrontEndConfig) { c.Floor = 0 }), "floor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.c.Validate(frontEndRate)
			if err == nil {
				t.Fatalf("Validate = nil error, want one naming %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate error = %q, want it to name %q", err.Error(), tc.want)
			}
		})
	}
}
