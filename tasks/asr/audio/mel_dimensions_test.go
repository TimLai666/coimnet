package audio

import (
	"math/big"
	"strconv"
	"testing"
	"unsafe"
)

type melFilterbankCapacityOracleResult struct {
	bins           int
	edgeBytes      *big.Int
	freqBytes      *big.Int
	bankBytes      *big.Int
	aggregateBytes *big.Int
	wantError      bool
}

func melFilterbankBinsCapacityOracle(c FrontEndConfig, nativeMaxInt int, headerBytes, floatBytes uintptr) melFilterbankCapacityOracleResult {
	maxNative := new(big.Int).SetUint64(uint64(nativeMaxInt))
	floatSize := new(big.Int).SetUint64(uint64(floatBytes))

	edgeCount := new(big.Int).Add(
		new(big.Int).SetUint64(uint64(c.Mels)),
		big.NewInt(2),
	)
	edgeBytes := new(big.Int).Mul(edgeCount, new(big.Int).Set(floatSize))

	bins := c.Window/2 + 1
	freqBytes := new(big.Int).Mul(
		new(big.Int).SetUint64(uint64(bins)),
		new(big.Int).Set(floatSize),
	)
	bankBytes := new(big.Int).Mul(
		new(big.Int).SetUint64(uint64(c.Mels)),
		new(big.Int).SetUint64(uint64(headerBytes)),
	)
	aggregateBytes := new(big.Int).Mul(
		new(big.Int).SetUint64(uint64(c.Mels)),
		new(big.Int).SetUint64(uint64(bins)),
	)
	aggregateBytes.Mul(aggregateBytes, new(big.Int).Set(floatSize))

	return melFilterbankCapacityOracleResult{
		bins:           bins,
		edgeBytes:      edgeBytes,
		freqBytes:      freqBytes,
		bankBytes:      bankBytes,
		aggregateBytes: aggregateBytes,
		wantError: edgeBytes.Cmp(maxNative) > 0 ||
			freqBytes.Cmp(maxNative) > 0 ||
			bankBytes.Cmp(maxNative) > 0,
	}
}

func assertMelFilterbankBinsCase(t *testing.T, c FrontEndConfig, wantError, proveAggregateOverflow bool, nativeMaxInt int, headerBytes, floatBytes uintptr) {
	t.Helper()
	if err := c.Validate(8000); err != nil {
		t.Fatalf("FrontEndConfig.Validate(%d) = %v, want nil", 8000, err)
	}

	oracle := melFilterbankBinsCapacityOracle(c, nativeMaxInt, headerBytes, floatBytes)
	if oracle.wantError != wantError {
		t.Fatalf("independent capacity oracle = error %v, contract expects %v", oracle.wantError, wantError)
	}
	if proveAggregateOverflow {
		maxNative := new(big.Int).SetUint64(uint64(nativeMaxInt))
		if oracle.edgeBytes.Cmp(maxNative) > 0 || oracle.freqBytes.Cmp(maxNative) > 0 || oracle.bankBytes.Cmp(maxNative) > 0 {
			t.Fatalf("independent rows do not each fit: edges=%s freq=%s bank=%s max=%s", oracle.edgeBytes, oracle.freqBytes, oracle.bankBytes, maxNative)
		}
		if oracle.aggregateBytes.Cmp(maxNative) <= 0 {
			t.Fatalf("aggregate Mels*bins*8 = %s, want more than native MaxInt %s", oracle.aggregateBytes, maxNative)
		}
		if oracle.wantError {
			t.Fatal("independent-row case unexpectedly failed an individual capacity check")
		}
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
		got, err = melFilterbankBins(c)
	}()
	if panicValue != nil {
		t.Fatalf("melFilterbankBins panicked: %v", panicValue)
	}

	if wantError {
		if err == nil {
			t.Fatalf("melFilterbankBins(%+v) = %d, nil error; want 0 and an error", c, got)
		}
		if got != 0 {
			t.Fatalf("melFilterbankBins(%+v) = %d, want 0 on error", c, got)
		}
		return
	}
	if err != nil {
		t.Fatalf("melFilterbankBins(%+v) error = %v, want nil", c, err)
	}
	if got != oracle.bins {
		t.Fatalf("melFilterbankBins(%+v) = %d, want %d", c, got, oracle.bins)
	}
}

func TestMelFilterbankBinsCapacityOracle(t *testing.T) {
	nativeMaxInt := int(^uint(0) >> 1)
	headerBytes := unsafe.Sizeof([]float64(nil))
	floatBytes := unsafe.Sizeof(float64(0))
	if floatBytes != 8 {
		t.Fatalf("float64 size = %d bytes, want 8", floatBytes)
	}

	base := FrontEndConfig{
		Window: 16,
		Hop:    8,
		Mels:   1,
		FMin:   0,
		FMax:   4000,
		Floor:  1e-10,
	}
	maxBankRows := nativeMaxInt / int(headerBytes)
	maxWindowPower := 1 << (strconv.IntSize - 4)

	t.Run("normal", func(t *testing.T) {
		assertMelFilterbankBinsCase(t, base, false, false, nativeMaxInt, headerBytes, floatBytes)
	})
	t.Run("max_bank_rows", func(t *testing.T) {
		c := base
		c.Mels = maxBankRows
		assertMelFilterbankBinsCase(t, c, false, false, nativeMaxInt, headerBytes, floatBytes)
	})
	t.Run("bank_rows_over", func(t *testing.T) {
		c := base
		c.Mels = maxBankRows + 1
		assertMelFilterbankBinsCase(t, c, true, false, nativeMaxInt, headerBytes, floatBytes)
	})
	t.Run("edge_bytes_over", func(t *testing.T) {
		c := base
		c.Mels = nativeMaxInt/8 - 1
		assertMelFilterbankBinsCase(t, c, true, false, nativeMaxInt, headerBytes, floatBytes)
	})
	t.Run("max_window_power", func(t *testing.T) {
		c := base
		c.Window = maxWindowPower
		assertMelFilterbankBinsCase(t, c, false, false, nativeMaxInt, headerBytes, floatBytes)
	})
	t.Run("window_bytes_over", func(t *testing.T) {
		c := base
		c.Window = 1 << (strconv.IntSize - 3)
		assertMelFilterbankBinsCase(t, c, true, false, nativeMaxInt, headerBytes, floatBytes)
	})
	t.Run("independent_rows_no_aggregate_cap", func(t *testing.T) {
		c := base
		c.Mels = maxBankRows
		c.Window = maxWindowPower
		assertMelFilterbankBinsCase(t, c, false, true, nativeMaxInt, headerBytes, floatBytes)
	})
}
