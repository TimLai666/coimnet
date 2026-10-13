package audio_test

import (
	"math"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

func frontendFiniteInput(sample float64) []float64 {
	x := make([]float64, defaultFrontEnd().Window)
	x[len(x)/4] = sample
	return x
}

func frontendOverflowInput() []float64 {
	x := make([]float64, defaultFrontEnd().Window)
	start := len(x) / 4
	for i := 0; i < 4; i++ {
		x[start+i] = math.MaxFloat64
	}
	return x
}

func frontendFiniteAssertRows(t *testing.T, name string, rows [][]float64) {
	t.Helper()
	if len(rows) == 0 {
		t.Fatalf("%s returned no rows", name)
	}
	for i, row := range rows {
		if len(row) == 0 {
			t.Fatalf("%s row %d is empty", name, i)
		}
		for j, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("%s[%d][%d] = %v, want a finite feature", name, i, j, value)
			}
		}
	}
}

func TestAudioFrontEndRejectsNonFiniteSamplesAndFiniteOverflow(t *testing.T) {
	c := defaultFrontEnd()
	for _, tc := range []struct {
		name  string
		input func() []float64
	}{
		{name: "NaN", input: func() []float64 { return frontendFiniteInput(math.NaN()) }},
		{name: "positive infinity", input: func() []float64 { return frontendFiniteInput(math.Inf(1)) }},
		{name: "negative infinity", input: func() []float64 { return frontendFiniteInput(math.Inf(-1)) }},
		{name: "finite arithmetic overflow", input: frontendOverflowInput},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			x := tc.input()
			stft, err := audio.STFT(x, c)
			if err == nil {
				t.Error("STFT accepted an input that should be rejected")
			}
			if stft != nil {
				t.Error("STFT returned features after rejecting input")
			}

			mel, report, err := audio.LogMel(x, frontEndRate, c)
			if err == nil {
				t.Error("LogMel accepted an input that should be rejected")
			}
			if mel != nil {
				t.Error("LogMel returned features after rejecting input")
			}
			if report != (audio.FrontEndReport{}) {
				t.Fatalf("LogMel report after rejecting input = %+v, want zero report", report)
			}
		})
	}
}

func TestAudioFrontEndAcceptsFiniteSamples(t *testing.T) {
	c := defaultFrontEnd()
	x := make([]float64, c.Window)
	for i := range x {
		x[i] = 0.25 * math.Sin(2*math.Pi*440*float64(i)/float64(frontEndRate))
	}

	stft, err := audio.STFT(x, c)
	if err != nil {
		t.Fatalf("STFT(finite input): %v", err)
	}
	frontendFiniteAssertRows(t, "STFT", stft)

	mel, report, err := audio.LogMel(x, frontEndRate, c)
	if err != nil {
		t.Fatalf("LogMel(finite input): %v", err)
	}
	frontendFiniteAssertRows(t, "LogMel", mel)
	if report.Frames != len(mel) {
		t.Fatalf("LogMel report frames = %d, want %d", report.Frames, len(mel))
	}
}

func TestAudioFrontEndRejectsNonFiniteFloor(t *testing.T) {
	c := defaultFrontEnd()
	c.Floor = math.Inf(1)
	if err := c.Validate(frontEndRate); err == nil {
		t.Error("Validate accepted an infinite floor")
	}
	features, report, err := audio.LogMel(make([]float64, c.Window), frontEndRate, c)
	if err == nil || features != nil || report != (audio.FrontEndReport{}) {
		t.Fatalf("LogMel infinite floor: error=%v report=%+v", err, report)
	}
}
