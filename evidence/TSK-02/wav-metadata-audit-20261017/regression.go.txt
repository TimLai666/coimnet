package audio_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

// oneFrameSamples reuses one read-only frame backing slice for every channel.
// This keeps the large-channel cases small while preserving len(Samples).
func oneFrameSamples(channels int) [][]float64 {
	frame := []float64{0.25}
	samples := make([][]float64, channels)
	for i := range samples {
		samples[i] = frame
	}
	return samples
}

// writeWAVRecover turns an unexpected panic into an assertion result so each
// invalid metadata case can also check that no output path was created.
func writeWAVRecover(path string, s audio.Signal) (err error, panicValue interface{}) {
	defer func() {
		panicValue = recover()
	}()
	err = audio.WriteWAV(path, s)
	return
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("WriteWAV created %q after rejecting metadata", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat rejected output %q: %v", path, err)
	}
}

func decodeGolden(t *testing.T, encoded string) []byte {
	t.Helper()
	want, err := hex.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode golden WAV bytes: %v", err)
	}
	return want
}

func assertWAVInfo(t *testing.T, raw []byte, want audio.WAVInfo) {
	t.Helper()
	got, err := audio.InspectWAV(raw)
	if err != nil {
		t.Fatalf("InspectWAV: %v", err)
	}
	if got != want {
		t.Fatalf("InspectWAV = %+v, want %+v", got, want)
	}
}

func TestWriteWAVRejectsUnrepresentableMetadata(t *testing.T) {
	cases := []struct {
		name       string
		rate       uint64
		channels   int
		frames     int
		requires64 bool
	}{
		{
			name:       "sample_rate_overflow",
			rate:       (uint64(1) << 32) + 8000,
			channels:   1,
			frames:     1,
			requires64: true,
		},
		{
			name:     "byte_rate_overflow",
			rate:     (uint64(1) << 30) + 8000,
			channels: 2,
			frames:   1,
		},
		{
			name:     "channel_count_overflow",
			rate:     8000,
			channels: 65536,
			frames:   1,
		},
		{
			name:     "block_align_overflow",
			rate:     8000,
			channels: 32768,
			frames:   1,
		},
		{
			name:     "empty_channel_metadata_overflow",
			rate:     8000,
			channels: 32768,
			frames:   0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.requires64 && strconv.IntSize != 64 {
				t.Skip("sample-rate case requires a 64-bit int")
			}

			var samples [][]float64
			if tc.frames == 0 {
				samples = make([][]float64, tc.channels)
			} else {
				samples = oneFrameSamples(tc.channels)
			}
			path := filepath.Join(t.TempDir(), tc.name+".wav")
			err, panicValue := writeWAVRecover(path, audio.Signal{
				SampleRate: int(tc.rate),
				Channels:   1,
				Samples:    samples,
			})
			if panicValue != nil {
				t.Fatalf("WriteWAV panicked for %s: %v", tc.name, panicValue)
			}
			if err == nil {
				t.Fatalf("WriteWAV(%s) = nil error, want unrepresentable metadata rejection", tc.name)
			}
			assertPathAbsent(t, path)
		})
	}
}

func TestWriteWAVNormalMetadataAndBytes(t *testing.T) {
	t.Run("mono_golden", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "mono.wav")
		if err := audio.WriteWAV(path, audio.Signal{
			SampleRate: 8000,
			Channels:   99,
			Samples:    [][]float64{{0.25}},
		}); err != nil {
			t.Fatalf("WriteWAV: %v", err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		want := decodeGolden(t, "524946462600000057415645666d74201000000001000100401f0000803e00000200100064617461020000000020")
		if !bytes.Equal(raw, want) {
			t.Fatalf("mono WAV bytes = %x, want %x", raw, want)
		}
		assertWAVInfo(t, raw, audio.WAVInfo{SampleRate: 8000, Channels: 1, Frames: 1})
		got, err := audio.ReadWAV(path)
		if err != nil {
			t.Fatalf("ReadWAV: %v", err)
		}
		if got.SampleRate != 8000 || got.Channels != 1 {
			t.Fatalf("ReadWAV metadata = rate %d, channels %d, want 8000/1", got.SampleRate, got.Channels)
		}
		if len(got.Samples) != 1 || len(got.Samples[0]) != 1 || got.Samples[0][0] != 0.25 {
			t.Fatalf("ReadWAV samples = %#v, want one sample 0.25", got.Samples)
		}
	})

	t.Run("stereo_golden", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "stereo.wav")
		if err := audio.WriteWAV(path, audio.Signal{
			SampleRate: 8000,
			Channels:   1,
			Samples:    [][]float64{{0.25}, {-0.25}},
		}); err != nil {
			t.Fatalf("WriteWAV: %v", err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		want := decodeGolden(t, "524946462800000057415645666d74201000000001000200401f0000007d0000040010006461746104000000002000e0")
		if !bytes.Equal(raw, want) {
			t.Fatalf("stereo WAV bytes = %x, want %x", raw, want)
		}
		assertWAVInfo(t, raw, audio.WAVInfo{SampleRate: 8000, Channels: 2, Frames: 1})
		got, err := audio.ReadWAV(path)
		if err != nil {
			t.Fatalf("ReadWAV: %v", err)
		}
		if got.SampleRate != 8000 || got.Channels != 2 {
			t.Fatalf("ReadWAV metadata = rate %d, channels %d, want 8000/2", got.SampleRate, got.Channels)
		}
		if len(got.Samples) != 2 || len(got.Samples[0]) != 1 || len(got.Samples[1]) != 1 || got.Samples[0][0] != 0.25 || got.Samples[1][0] != -0.25 {
			t.Fatalf("ReadWAV samples = %#v, want one frame 0.25/-0.25", got.Samples)
		}
	})

	t.Run("maximum_block_align_empty", func(t *testing.T) {
		const channels = 32767
		path := filepath.Join(t.TempDir(), "maximum-block-align-empty.wav")
		if err := audio.WriteWAV(path, audio.Signal{
			SampleRate: 8000,
			Channels:   1,
			Samples:    make([][]float64, channels),
		}); err != nil {
			t.Fatalf("WriteWAV: %v", err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if len(raw) != 44 {
			t.Fatalf("empty WAV length = %d, want 44", len(raw))
		}
		assertWAVInfo(t, raw, audio.WAVInfo{SampleRate: 8000, Channels: channels, Frames: 0})
		if got := binary.LittleEndian.Uint16(raw[22:24]); got != channels {
			t.Fatalf("header channels = %d, want %d", got, channels)
		}
		if got := binary.LittleEndian.Uint32(raw[28:32]); got != 8000*channels*2 {
			t.Fatalf("header byte rate = %d, want %d", got, 8000*channels*2)
		}
		if got := binary.LittleEndian.Uint16(raw[32:34]); got != channels*2 {
			t.Fatalf("header block align = %d, want %d", got, channels*2)
		}
		if got := binary.LittleEndian.Uint32(raw[40:44]); got != 0 {
			t.Fatalf("header data size = %d, want 0", got)
		}
	})
}

func TestWriteWAVExistingPathPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.wav")
	sentinel := []byte("existing path must remain unchanged")
	if err := os.WriteFile(path, sentinel, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := audio.WriteWAV(path, audio.Signal{
		SampleRate: 8000,
		Channels:   7,
		Samples:    [][]float64{{0.25}},
	}); err == nil {
		t.Fatal("WriteWAV over an existing path = nil error, want an error")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, sentinel) {
		t.Fatalf("existing path bytes = %q, want %q", got, sentinel)
	}
}
