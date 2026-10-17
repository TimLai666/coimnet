package audio_test

import (
	"math"
	"math/big"
	"reflect"
	"strconv"
	"testing"
	"unsafe"

	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

const melCapacityRate = 8000

func melCapacityBaseConfig() audio.FrontEndConfig {
	return audio.FrontEndConfig{
		Window: 16,
		Hop:    8,
		Mels:   1,
		FMin:   0,
		FMax:   4000,
		Floor:  1e-10,
	}
}

func melCapacityMaxInt() int {
	return int(^uint(0) >> 1)
}

func melCapacityMaxIntBig() *big.Int {
	return new(big.Int).SetUint64(uint64(melCapacityMaxInt()))
}

func melCapacityMaxUintptrBig() *big.Int {
	return new(big.Int).SetUint64(uint64(^uintptr(0)))
}

func melCapacityFloat64BytesBig() *big.Int {
	return new(big.Int).SetUint64(uint64(unsafe.Sizeof(float64(0))))
}

func melCapacityProveEdgeCountAboveMaxInt(t *testing.T, mels int) {
	t.Helper()
	edges := new(big.Int).Add(
		new(big.Int).SetInt64(int64(mels)),
		big.NewInt(2),
	)
	if edges.Cmp(melCapacityMaxIntBig()) <= 0 {
		t.Fatalf("independent edge count proof = %s, want > MaxInt %s", edges, melCapacityMaxIntBig())
	}
}

func melCapacityProveEdgeFloatBytesAboveUintptr(t *testing.T, mels int) {
	t.Helper()
	edges := new(big.Int).Add(
		new(big.Int).SetInt64(int64(mels)),
		big.NewInt(2),
	)
	bytes := new(big.Int).Mul(edges, melCapacityFloat64BytesBig())
	if bytes.Cmp(melCapacityMaxUintptrBig()) <= 0 {
		t.Fatalf("independent edge float64 bytes proof = %s, want > uintptr Max %s", bytes, melCapacityMaxUintptrBig())
	}
}

func melCapacityProveFrequencyFloatBytesAboveUintptr(t *testing.T, window int) {
	t.Helper()
	bins := new(big.Int).Add(
		new(big.Int).SetInt64(int64(window/2)),
		big.NewInt(1),
	)
	bytes := new(big.Int).Mul(bins, melCapacityFloat64BytesBig())
	if bytes.Cmp(melCapacityMaxUintptrBig()) <= 0 {
		t.Fatalf("independent frequency float64 bytes proof = %s, want > uintptr Max %s", bytes, melCapacityMaxUintptrBig())
	}
}

func melCapacityRequireValidConfig(t *testing.T, c audio.FrontEndConfig) {
	t.Helper()
	if err := c.Validate(melCapacityRate); err != nil {
		t.Fatalf("basic FrontEndConfig.Validate = %v, want nil for capacity-only case", err)
	}
}

type melCapacityCallResult struct {
	output     [][]float64
	report     audio.FrontEndReport
	err        error
	panicked   bool
	panicValue interface{}
}

func melCapacityCapture(fn func() ([][]float64, audio.FrontEndReport, error)) (result melCapacityCallResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result.panicked = true
			result.panicValue = recovered
		}
	}()
	result.output, result.report, result.err = fn()
	return result
}

func melCapacityRequireRejected(t *testing.T, result melCapacityCallResult, zeroReport bool) {
	t.Helper()
	if result.panicked {
		t.Fatalf("public API panicked: %v", result.panicValue)
	}
	if result.err == nil {
		t.Fatal("public API returned nil error for an unrepresentable buffer")
	}
	if result.output != nil {
		t.Fatalf("public API returned non-nil output on error: %#v", result.output)
	}
	if zeroReport && !reflect.DeepEqual(result.report, audio.FrontEndReport{}) {
		t.Fatalf("LogMel report on error = %+v, want zero report", result.report)
	}
}

func TestMelFilterbankRejectsUnrepresentableBuffers(t *testing.T) {
	base := melCapacityBaseConfig()
	maxInt := melCapacityMaxInt()

	t.Run("edge_add_bank", func(t *testing.T) {
		c := base
		c.Mels = maxInt
		melCapacityProveEdgeCountAboveMaxInt(t, c.Mels)
		melCapacityRequireValidConfig(t, c)
		result := melCapacityCapture(func() ([][]float64, audio.FrontEndReport, error) {
			bank, err := audio.MelFilterbank(melCapacityRate, c)
			return bank, audio.FrontEndReport{}, err
		})
		melCapacityRequireRejected(t, result, false)
	})

	t.Run("edge_add_logmel", func(t *testing.T) {
		c := base
		c.Mels = maxInt
		melCapacityProveEdgeCountAboveMaxInt(t, c.Mels)
		melCapacityRequireValidConfig(t, c)
		result := melCapacityCapture(func() ([][]float64, audio.FrontEndReport, error) {
			return audio.LogMel(make([]float64, 16), melCapacityRate, c)
		})
		melCapacityRequireRejected(t, result, true)
	})

	t.Run("edge_bytes_bank", func(t *testing.T) {
		c := base
		c.Mels = maxInt - 2
		melCapacityProveEdgeFloatBytesAboveUintptr(t, c.Mels)
		melCapacityRequireValidConfig(t, c)
		result := melCapacityCapture(func() ([][]float64, audio.FrontEndReport, error) {
			bank, err := audio.MelFilterbank(melCapacityRate, c)
			return bank, audio.FrontEndReport{}, err
		})
		melCapacityRequireRejected(t, result, false)
	})

	t.Run("edge_bytes_logmel", func(t *testing.T) {
		c := base
		c.Mels = maxInt - 2
		melCapacityProveEdgeFloatBytesAboveUintptr(t, c.Mels)
		melCapacityRequireValidConfig(t, c)
		result := melCapacityCapture(func() ([][]float64, audio.FrontEndReport, error) {
			return audio.LogMel(make([]float64, 16), melCapacityRate, c)
		})
		melCapacityRequireRejected(t, result, true)
	})

	t.Run("frequency_bytes_bank", func(t *testing.T) {
		c := base
		c.Window = 1 << (strconv.IntSize - 2)
		melCapacityProveFrequencyFloatBytesAboveUintptr(t, c.Window)
		melCapacityRequireValidConfig(t, c)
		result := melCapacityCapture(func() ([][]float64, audio.FrontEndReport, error) {
			bank, err := audio.MelFilterbank(melCapacityRate, c)
			return bank, audio.FrontEndReport{}, err
		})
		melCapacityRequireRejected(t, result, false)
	})

	t.Run("frequency_bytes_logmel", func(t *testing.T) {
		c := base
		c.Window = 1 << (strconv.IntSize - 2)
		melCapacityProveFrequencyFloatBytesAboveUintptr(t, c.Window)
		melCapacityRequireValidConfig(t, c)
		result := melCapacityCapture(func() ([][]float64, audio.FrontEndReport, error) {
			return audio.LogMel(make([]float64, 16), melCapacityRate, c)
		})
		melCapacityRequireRejected(t, result, true)
	})
}

func TestMelFilterbankRetainsNormalAndFraming(t *testing.T) {
	base := melCapacityBaseConfig()

	t.Run("normal_bank", func(t *testing.T) {
		bank, err := audio.MelFilterbank(melCapacityRate, base)
		if err != nil {
			t.Fatalf("MelFilterbank: %v", err)
		}
		if len(bank) != 1 || len(bank[0]) != 9 {
			t.Fatalf("MelFilterbank shape = %#v, want [1][9]", bank)
		}

		// Independent HTK hand calculation: the one-filter edges are at
		// 0 Hz, 1113.8357147217055 Hz, and 4000 Hz, then linearly taper
		// the FFT-bin frequencies 0, 500, ..., 4000 Hz.
		want := []float64{
			0,
			0.448899234771733,
			0.897798469543466,
			0.8662015578087375,
			0.6929612462469901,
			0.5197209346852426,
			0.3464806231234952,
			0.17324031156174777,
			0,
		}
		rowSum := 0.0
		for k, got := range bank[0] {
			if math.Abs(got-want[k]) > 1e-14 {
				t.Errorf("bank[0][%d] = %.17g, want independent HTK reference %.17g", k, got, want[k])
			}
			rowSum += got
		}
		if !(rowSum > 0) {
			t.Fatalf("bank row sum = %.17g, want positive", rowSum)
		}
	})

	t.Run("normal_logmel", func(t *testing.T) {
		input := make([]float64, 16)
		before := append([]float64(nil), input...)
		features, report, err := audio.LogMel(input, melCapacityRate, base)
		if err != nil {
			t.Fatalf("LogMel: %v", err)
		}
		if !reflect.DeepEqual(features, [][]float64{{-10}}) {
			t.Fatalf("LogMel(zero input) = %#v, want {{-10}}", features)
		}
		wantReport := audio.FrontEndReport{
			Version:    "coimnet-logmel/v1",
			SampleRate: melCapacityRate,
			Config:     base,
			Frames:     1,
		}
		if !reflect.DeepEqual(report, wantReport) {
			t.Fatalf("LogMel report = %+v, want %+v", report, wantReport)
		}
		if !reflect.DeepEqual(input, before) {
			t.Fatalf("LogMel modified input: got %#v, want %#v", input, before)
		}
	})

	t.Run("unused_mels_stft", func(t *testing.T) {
		c := base
		c.Mels = melCapacityMaxInt()
		melCapacityRequireValidConfig(t, c)
		power, err := audio.STFT(make([]float64, 16), c)
		if err != nil {
			t.Fatalf("STFT with unused Mels = %d: %v", c.Mels, err)
		}
		if len(power) != 1 || len(power[0]) != 9 {
			t.Fatalf("STFT shape = %#v, want [1][9]", power)
		}
		for k, got := range power[0] {
			if got != 0 {
				t.Errorf("STFT zero input bin %d = %.17g, want 0", k, got)
			}
		}
	})
}
