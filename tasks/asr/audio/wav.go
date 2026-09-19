package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
)

// fullScale is the divisor that maps int16 PCM onto [-1, 1]: -32768 becomes
// exactly -1 and 32767 becomes just under 1.
const fullScale = 32768.0

// ReadWAV decodes a RIFF/WAVE file with PCM 16-bit little-endian samples
// (format tag 1). Any other format tag, bit depth, missing fmt/data chunk,
// truncated data (data chunk shorter than declared) or a zero sample rate is
// an error naming the problem. Extra chunks (LIST, etc.) are skipped. Samples
// are int16/32768.
func ReadWAV(path string) (Signal, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Signal{}, err
	}
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return Signal{}, fmt.Errorf("audio: %s is not a RIFF/WAVE file", path)
	}

	var (
		haveFmt, haveData         bool
		formatTag, channels, bits int
		sampleRate                int
		data                      []byte
	)
	for off := 12; off+8 <= len(raw); {
		id := string(raw[off : off+4])
		size := int64(binary.LittleEndian.Uint32(raw[off+4 : off+8]))
		end := int64(off+8) + size
		if end > int64(len(raw)) {
			return Signal{}, fmt.Errorf("audio: %s chunk truncated: declares %d bytes, file holds %d", id, size, int64(len(raw))-int64(off+8))
		}
		payload := raw[off+8 : int(end)]
		switch id {
		case "fmt ":
			if len(payload) < 16 {
				return Signal{}, fmt.Errorf("audio: fmt chunk is %d bytes, PCM needs at least 16", len(payload))
			}
			formatTag = int(binary.LittleEndian.Uint16(payload[0:2]))
			channels = int(binary.LittleEndian.Uint16(payload[2:4]))
			sampleRate = int(binary.LittleEndian.Uint32(payload[4:8]))
			bits = int(binary.LittleEndian.Uint16(payload[14:16]))
			haveFmt = true
		case "data":
			data = payload
			haveData = true
		}
		off = int(end)
		if size%2 == 1 { // RIFF pads odd-sized chunks to an even boundary.
			off++
		}
	}

	switch {
	case !haveFmt:
		return Signal{}, errors.New("audio: missing fmt chunk")
	case formatTag != 1:
		return Signal{}, fmt.Errorf("audio: format tag %d is not PCM (1); compressed audio needs an explicit decoder", formatTag)
	case bits != 16:
		return Signal{}, fmt.Errorf("audio: bit depth %d is not the supported 16", bits)
	case sampleRate <= 0:
		return Signal{}, errors.New("audio: sample rate is zero")
	case channels <= 0:
		return Signal{}, errors.New("audio: channel count is zero")
	case !haveData:
		return Signal{}, errors.New("audio: missing data chunk")
	}

	frameBytes := channels * 2
	if len(data)%frameBytes != 0 {
		return Signal{}, fmt.Errorf("audio: data chunk of %d bytes is truncated mid-frame; a frame is %d bytes", len(data), frameBytes)
	}
	frames := len(data) / frameBytes
	samples := make([][]float64, channels)
	for ch := range samples {
		samples[ch] = make([]float64, frames)
	}
	for f := 0; f < frames; f++ {
		for ch := 0; ch < channels; ch++ {
			at := (f*channels + ch) * 2
			samples[ch][f] = float64(int16(binary.LittleEndian.Uint16(data[at:at+2]))) / fullScale
		}
	}
	return Signal{SampleRate: sampleRate, Channels: channels, Samples: samples}, nil
}

// WriteWAV writes s as PCM 16-bit (rounded, clipped to [-1, 1]) so fixtures
// can be produced without any external tool. The channel count comes from
// len(s.Samples); a NaN sample is written as silence so the output stays
// deterministic.
// Errors: no channels, ragged channels, sample rate <= 0, existing path.
func WriteWAV(path string, s Signal) error {
	channels := len(s.Samples)
	if channels == 0 {
		return errors.New("audio: signal has no channels")
	}
	frames := len(s.Samples[0])
	for ch, c := range s.Samples {
		if len(c) != frames {
			return fmt.Errorf("audio: channel %d holds %d samples, channel 0 holds %d", ch, len(c), frames)
		}
	}
	if s.SampleRate <= 0 {
		return fmt.Errorf("audio: sample rate %d is not positive", s.SampleRate)
	}

	dataBytes := frames * channels * 2
	buf := bytes.NewBuffer(make([]byte, 0, 44+dataBytes))
	buf.WriteString("RIFF")
	writeU32(buf, uint32(36+dataBytes))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	writeU32(buf, 16)
	writeU16(buf, 1) // PCM
	writeU16(buf, uint16(channels))
	writeU32(buf, uint32(s.SampleRate))
	writeU32(buf, uint32(s.SampleRate*channels*2)) // byte rate
	writeU16(buf, uint16(channels*2))              // block align
	writeU16(buf, 16)                              // bits per sample
	buf.WriteString("data")
	writeU32(buf, uint32(dataBytes))
	for f := 0; f < frames; f++ {
		for ch := 0; ch < channels; ch++ {
			writeU16(buf, uint16(pcm16(s.Samples[ch][f])))
		}
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// pcm16 converts one sample in [-1, 1] to int16, rounding to nearest and
// clipping anything outside the range; NaN becomes silence.
func pcm16(v float64) int16 {
	switch {
	case math.IsNaN(v):
		return 0
	case v >= 1:
		return math.MaxInt16
	case v <= -1:
		return math.MinInt16
	}
	n := math.Round(v * fullScale)
	if n > math.MaxInt16 {
		return math.MaxInt16
	}
	return int16(n)
}

// writeU16 appends a little-endian uint16.
func writeU16(buf *bytes.Buffer, v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	buf.Write(b[:])
}

// writeU32 appends a little-endian uint32.
func writeU32(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}
