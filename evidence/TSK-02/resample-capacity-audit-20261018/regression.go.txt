package audio_test

import (
	"math/big"
	"reflect"
	"testing"
	"unsafe"

	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

func TestResampleRejectsUnrepresentableBuffer(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	elementBytes := unsafe.Sizeof(float64(0))
	requestedBytes := new(big.Int).Mul(
		new(big.Int).SetUint64(uint64(maxInt)),
		new(big.Int).SetUint64(uint64(elementBytes)),
	)
	uintptrMax := new(big.Int).SetUint64(uint64(^uintptr(0)))
	if requestedBytes.Cmp(uintptrMax) <= 0 {
		t.Fatalf("test request bytes = %s, want more than uintptr max %s", requestedBytes, uintptrMax)
	}

	got, report, err, panicValue := resampleWithoutPanic([]float64{0}, 1, maxInt)
	if panicValue != nil {
		t.Fatalf("Resample panicked for unrepresentable byte request: %v", panicValue)
	}
	if err == nil {
		t.Fatal("Resample = nil error for unrepresentable byte request, want an error")
	}
	if got != nil {
		t.Fatalf("Resample output = %v, want nil on error", got)
	}
	if report != (audio.ResampleReport{}) {
		t.Fatalf("Resample report = %+v, want zero report on error", report)
	}
}

func TestResampleRetainsNormalAndCountOnly(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		input := []float64{0.25, -0.75}
		inputBefore := append([]float64(nil), input...)
		got, report, err := audio.Resample(input, 1, 2)
		if err != nil {
			t.Fatalf("Resample: %v", err)
		}
		want := []float64{0.25, -0.25, -0.75, -0.75}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Resample output = %v, want %v", got, want)
		}
		wantReport := audio.ResampleReport{
			Method:       "linear",
			From:         1,
			To:           2,
			InputLength:  len(input),
			OutputLength: len(want),
		}
		if report != wantReport {
			t.Fatalf("Resample report = %+v, want %+v", report, wantReport)
		}
		if !reflect.DeepEqual(input, inputBefore) {
			t.Fatalf("Resample mutated its input: %v", input)
		}
	})

	t.Run("count_only", func(t *testing.T) {
		maxInt := int(^uint(0) >> 1)
		got, err := audio.ResampleOutputLength(1, 1, maxInt)
		if err != nil {
			t.Fatalf("ResampleOutputLength: %v", err)
		}
		if got != maxInt {
			t.Fatalf("ResampleOutputLength(1, 1, MaxInt) = %d, want %d", got, maxInt)
		}
	})
}
