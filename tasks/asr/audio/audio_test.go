package audio_test

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

// chunkSized builds a RIFF chunk whose declared size may differ from the bytes
// it actually carries, so a truncated file can be written on purpose. The
// payload is padded to an even length, as RIFF requires.
func chunkSized(id string, size uint32, payload []byte) []byte {
	out := make([]byte, 0, 8+len(payload)+1)
	out = append(out, id...)
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], size)
	out = append(out, hdr[:]...)
	out = append(out, payload...)
	if len(payload)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

// chunk builds a RIFF chunk whose declared size matches its payload.
func chunk(id string, payload []byte) []byte {
	return chunkSized(id, uint32(len(payload)), payload)
}

// riff wraps chunks in a RIFF/WAVE container.
func riff(chunks ...[]byte) []byte {
	body := []byte("WAVE")
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := make([]byte, 0, 8+len(body))
	out = append(out, "RIFF"...)
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(body)))
	out = append(out, hdr[:]...)
	return append(out, body...)
}

// fmtChunkPayload builds the 16-byte PCM fmt chunk body.
func fmtChunkPayload(tag, channels, sampleRate, bits int) []byte {
	p := make([]byte, 16)
	binary.LittleEndian.PutUint16(p[0:2], uint16(tag))
	binary.LittleEndian.PutUint16(p[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(p[4:8], uint32(sampleRate))
	binary.LittleEndian.PutUint32(p[8:12], uint32(sampleRate*channels*bits/8))
	binary.LittleEndian.PutUint16(p[12:14], uint16(channels*bits/8))
	binary.LittleEndian.PutUint16(p[14:16], uint16(bits))
	return p
}

// pcmBytes interleaves per-channel int16 samples into little-endian bytes.
func pcmBytes(channels [][]int16) []byte {
	if len(channels) == 0 {
		return nil
	}
	out := make([]byte, 0, len(channels[0])*len(channels)*2)
	var v [2]byte
	for f := range channels[0] {
		for ch := range channels {
			binary.LittleEndian.PutUint16(v[:], uint16(channels[ch][f]))
			out = append(out, v[:]...)
		}
	}
	return out
}

// writeFixture drops raw bytes into the test's own temporary directory.
func writeFixture(t *testing.T, name string, raw []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
	return path
}

// peakAbs returns the largest absolute value in x.
func peakAbs(x []float64) float64 {
	p := 0.0
	for _, v := range x {
		if a := math.Abs(v); a > p {
			p = a
		}
	}
	return p
}

func TestWAVRoundTripIsExact(t *testing.T) {
	const (
		rate    = 16000
		frames  = 100
		fullFS  = 32768.0
		nChans  = 2
		example = "roundtrip.wav"
	)
	boundary := []int16{-32768, 32767, 0, 1, -1, 12345, -12345}
	want := make([][]float64, nChans)
	for ch := range want {
		want[ch] = make([]float64, frames)
		for i := range want[ch] {
			want[ch][i] = float64(boundary[(i+ch)%len(boundary)]) / fullFS
		}
	}

	path := filepath.Join(t.TempDir(), example)
	if err := audio.WriteWAV(path, audio.Signal{SampleRate: rate, Channels: nChans, Samples: want}); err != nil {
		t.Fatalf("WriteWAV: %v", err)
	}
	got, err := audio.ReadWAV(path)
	if err != nil {
		t.Fatalf("ReadWAV: %v", err)
	}
	if got.SampleRate != rate {
		t.Errorf("SampleRate = %d, want %d", got.SampleRate, rate)
	}
	if got.Channels != nChans {
		t.Errorf("Channels = %d, want %d", got.Channels, nChans)
	}
	if !reflect.DeepEqual(got.Samples, want) {
		for ch := range want {
			if ch >= len(got.Samples) {
				t.Fatalf("Samples has %d channels, want %d", len(got.Samples), nChans)
			}
			for i := range want[ch] {
				if i >= len(got.Samples[ch]) {
					t.Fatalf("channel %d has %d samples, want %d", ch, len(got.Samples[ch]), frames)
				}
				if got.Samples[ch][i] != want[ch][i] {
					t.Fatalf("Samples[%d][%d] = %v, want %v", ch, i, got.Samples[ch][i], want[ch][i])
				}
			}
		}
		t.Fatalf("Samples = %v, want %v", got.Samples, want)
	}

	if err := audio.WriteWAV(path, audio.Signal{SampleRate: rate, Channels: nChans, Samples: want}); err == nil {
		t.Error("WriteWAV over an existing path = nil, want an error")
	}
}

func TestReadWAVRejects(t *testing.T) {
	data := pcmBytes([][]int16{{1, -1, 32767, -32768}})
	cases := []struct {
		name string
		raw  []byte
		want string
	}{
		{"not riff", []byte("JUNK\x00\x00\x00\x00WAVEfmt "), "RIFF"},
		{"fmt chunk missing", riff(chunk("data", data)), "fmt"},
		{"float format tag", riff(chunk("fmt ", fmtChunkPayload(3, 1, 16000, 32)), chunk("data", data)), "format tag"},
		{"eight bit depth", riff(chunk("fmt ", fmtChunkPayload(1, 1, 16000, 8)), chunk("data", data)), "bit depth"},
		{"zero sample rate", riff(chunk("fmt ", fmtChunkPayload(1, 1, 0, 16)), chunk("data", data)), "sample rate"},
		{"data chunk missing", riff(chunk("fmt ", fmtChunkPayload(1, 1, 16000, 16))), "data chunk"},
		{
			"data longer than the file",
			riff(chunk("fmt ", fmtChunkPayload(1, 1, 16000, 16)), chunkSized("data", uint32(len(data)+64), data)),
			"truncated",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeFixture(t, "reject.wav", c.raw)
			_, err := audio.ReadWAV(path)
			if err == nil {
				t.Fatalf("ReadWAV = nil error, want one naming %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("ReadWAV error = %q, want it to name %q", err.Error(), c.want)
			}
		})
	}
}

func TestReadWAVSkipsExtraChunks(t *testing.T) {
	src := [][]int16{{1000, -1000, 0}}
	raw := riff(
		chunk("LIST", []byte("INFOhi!")), // odd payload: exercises the pad byte
		chunk("fmt ", fmtChunkPayload(1, 1, 8000, 16)),
		chunk("JUNK", []byte{0, 0, 0, 0}),
		chunk("data", pcmBytes(src)),
	)
	got, err := audio.ReadWAV(writeFixture(t, "extra.wav", raw))
	if err != nil {
		t.Fatalf("ReadWAV: %v", err)
	}
	want := audio.Signal{
		SampleRate: 8000,
		Channels:   1,
		Samples:    [][]float64{{1000.0 / 32768, -1000.0 / 32768, 0}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadWAV = %+v, want %+v", got, want)
	}
}

func TestMixdown(t *testing.T) {
	s := audio.Signal{
		SampleRate: 16000,
		Channels:   2,
		Samples:    [][]float64{{1, 1}, {-1, 0}},
	}
	want := []float64{0, 0.5}
	if got := audio.Mixdown(s); !reflect.DeepEqual(got, want) {
		t.Fatalf("Mixdown = %v, want %v", got, want)
	}
}

func TestResampleLengthsAndConstant(t *testing.T) {
	const constant = 0.25
	x := make([]float64, 100)
	for i := range x {
		x[i] = constant
	}

	t.Run("halved rate", func(t *testing.T) {
		got, report, err := audio.Resample(x, 16000, 8000)
		if err != nil {
			t.Fatalf("Resample: %v", err)
		}
		if len(got) != 50 {
			t.Fatalf("len(Resample) = %d, want 50", len(got))
		}
		for i, v := range got {
			if v != constant {
				t.Fatalf("Resample[%d] = %v, want the constant %v", i, v, constant)
			}
		}
		want := audio.ResampleReport{
			Method:       "linear",
			From:         16000,
			To:           8000,
			InputLength:  100,
			OutputLength: 50,
		}
		if report != want {
			t.Fatalf("report = %+v, want %+v", report, want)
		}
	})

	t.Run("fractional ratio", func(t *testing.T) {
		got, report, err := audio.Resample(x, 16000, 11025)
		if err != nil {
			t.Fatalf("Resample: %v", err)
		}
		if len(got) != 69 { // round(100*11025/16000) = round(68.90625)
			t.Fatalf("len(Resample) = %d, want 69", len(got))
		}
		if report.OutputLength != len(got) {
			t.Fatalf("report.OutputLength = %d, want %d", report.OutputLength, len(got))
		}
		for i, v := range got {
			if v != constant {
				t.Fatalf("Resample[%d] = %v, want the constant %v", i, v, constant)
			}
		}
	})

	errCases := []struct {
		name     string
		x        []float64
		from, to int
	}{
		{"zero source rate", x, 0, 8000},
		{"zero target rate", x, 16000, 0},
		{"negative source rate", x, -16000, 8000},
		{"empty input", nil, 16000, 8000},
	}
	for _, c := range errCases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := audio.Resample(c.x, c.from, c.to); err == nil {
				t.Fatal("Resample = nil error, want one")
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	t.Run("scales to the target peak", func(t *testing.T) {
		x := []float64{0.25, -0.125, 0, 0.25}
		got, report, err := audio.Normalize(x, 1)
		if err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		if p := peakAbs(got); p != 1 {
			t.Errorf("peak after Normalize = %v, want 1", p)
		}
		want := audio.NormalizeReport{Peak: 0.25, Gain: 4, Scaled: true}
		if report != want {
			t.Errorf("report = %+v, want %+v", report, want)
		}
		if !reflect.DeepEqual(x, []float64{0.25, -0.125, 0, 0.25}) {
			t.Errorf("Normalize mutated its input: %v", x)
		}
	})

	t.Run("all-zero input is unchanged", func(t *testing.T) {
		x := []float64{0, 0, 0}
		got, report, err := audio.Normalize(x, 1)
		if err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		if !reflect.DeepEqual(got, x) {
			t.Errorf("Normalize = %v, want it unchanged %v", got, x)
		}
		want := audio.NormalizeReport{Peak: 0, Gain: 1, Scaled: false}
		if report != want {
			t.Errorf("report = %+v, want %+v", report, want)
		}
	})

	for _, peak := range []float64{0, -0.5, 1.5} {
		t.Run("rejects peak", func(t *testing.T) {
			if _, _, err := audio.Normalize([]float64{0.5}, peak); err == nil {
				t.Fatalf("Normalize(peak=%v) = nil error, want one", peak)
			}
		})
	}
}
