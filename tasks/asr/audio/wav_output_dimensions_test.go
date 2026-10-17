package audio

import (
	"math/big"
	"strconv"
	"testing"
)

func TestWAVOutputBytesExactLimits(t *testing.T) {
	nativeMaxInt := int(^uint(0) >> 1)
	maxNative := new(big.Int).SetUint64(uint64(nativeMaxInt))
	maxUint16 := new(big.Int).SetUint64(uint64(^uint16(0)))
	maxUint32 := new(big.Int).SetUint64(uint64(^uint32(0)))

	maxRIFF := new(big.Int).Sub(new(big.Int).Set(maxUint32), big.NewInt(36))
	maxFile := new(big.Int).Sub(new(big.Int).Set(maxNative), big.NewInt(44))
	maxFrames := new(big.Int).Set(maxRIFF)
	if maxFile.Cmp(maxFrames) < 0 {
		maxFrames.Set(maxFile)
	}
	maxFrames.Div(maxFrames, big.NewInt(2))
	oneFrameTooMany := new(big.Int).Add(new(big.Int).Set(maxFrames), big.NewInt(1))
	nativeBufferOverflow := new(big.Int).Div(new(big.Int).Set(maxNative), big.NewInt(2))
	nativeBufferOverflow.Add(nativeBufferOverflow, big.NewInt(1))

	cases := []struct {
		name       string
		frames     int
		channels   int
		sampleRate uint64
		wantError  bool
		requires64 bool
	}{
		{name: "zero_frames", frames: 0, channels: 1, sampleRate: 8000},
		{name: "ordinary", frames: 2, channels: 2, sampleRate: 8000},
		{name: "largest_riff_or_native", frames: int(maxFrames.Int64()), channels: 1, sampleRate: 1},
		{name: "one_frame_too_many", frames: int(oneFrameTooMany.Int64()), channels: 1, sampleRate: 1, wantError: true},
		{name: "native_buffer_overflow", frames: int(nativeBufferOverflow.Int64()), channels: 1, sampleRate: 1, wantError: true},
		{name: "total_product_overflow", frames: nativeMaxInt, channels: 32767, sampleRate: 1, wantError: true},
		{name: "wide_byte_rate", frames: 0, channels: 2, sampleRate: 1073741824, wantError: true},
		{name: "bad_block_align", frames: 0, channels: 32768, sampleRate: 8000, wantError: true},
		{name: "sample_rate_overflow_native64", frames: 0, channels: 1, sampleRate: (uint64(1) << 32) + 8000, wantError: true, requires64: true},
		{name: "max_legal_block", frames: 0, channels: 32767, sampleRate: 8000},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.requires64 && strconv.IntSize != 64 {
				t.Skip("sample-rate case requires a 64-bit int")
			}

			frames := big.NewInt(int64(tc.frames))
			channels := big.NewInt(int64(tc.channels))
			sampleRate := new(big.Int).SetUint64(tc.sampleRate)
			stride := new(big.Int).Mul(new(big.Int).Set(channels), big.NewInt(2))
			payload := new(big.Int).Mul(new(big.Int).Set(frames), channels)
			payload.Mul(payload, big.NewInt(2))
			byteRate := new(big.Int).Mul(new(big.Int).Set(sampleRate), channels)
			byteRate.Mul(byteRate, big.NewInt(2))
			riffBytes := new(big.Int).Add(new(big.Int).Set(payload), big.NewInt(36))
			fileBytes := new(big.Int).Add(new(big.Int).Set(payload), big.NewInt(44))

			oracleError := frames.Sign() < 0 || channels.Sign() <= 0 || sampleRate.Sign() <= 0 ||
				channels.Cmp(maxUint16) > 0 || stride.Cmp(maxUint16) > 0 ||
				sampleRate.Cmp(maxUint32) > 0 || byteRate.Cmp(maxUint32) > 0 ||
				payload.Cmp(maxUint32) > 0 || riffBytes.Cmp(maxUint32) > 0 ||
				fileBytes.Cmp(maxNative) > 0
			if oracleError != tc.wantError {
				t.Fatalf("independent oracle for %s = error %v, contract expects %v", tc.name, oracleError, tc.wantError)
			}

			var (
				got        int
				err        error
				panicValue interface{}
			)
			func() {
				defer func() {
					panicValue = recover()
				}()
				got, err = wavOutputBytes(tc.frames, tc.channels, int(tc.sampleRate))
			}()
			if panicValue != nil {
				t.Fatalf("wavOutputBytes panicked: %v", panicValue)
			}

			if tc.wantError {
				if err == nil {
					t.Fatalf("wavOutputBytes(%d,%d,%d) = %d, nil error; want 0 and an error", tc.frames, tc.channels, tc.sampleRate, got)
				}
				if got != 0 {
					t.Fatalf("wavOutputBytes(%d,%d,%d) = %d, want 0 on error", tc.frames, tc.channels, tc.sampleRate, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("wavOutputBytes(%d,%d,%d) error = %v, want nil", tc.frames, tc.channels, tc.sampleRate, err)
			}
			want := int(payload.Int64())
			if got != want {
				t.Fatalf("wavOutputBytes(%d,%d,%d) = %d, want exact payload %d", tc.frames, tc.channels, tc.sampleRate, got, want)
			}
		})
	}
}
