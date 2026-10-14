package glyphs_test

import (
	"fmt"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
	"math/big"
	"reflect"
	"testing"
	"unsafe"
)

func capacityGenerate(t *testing.T, n, min, max int) (samples []glyphs.Sample, err error) {
	t.Helper()
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("Generate panicked before returning an error: %v", p)
		}
	}()
	return glyphs.Generate([]rune{'a', 'b'}, n, min, max, glyphs.Family{Seed: 7}, glyphs.Options{Background: .25}, 9)
}
func TestGenerateRejectsUnrepresentableStorage(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	maxLine := (maxInt/64 + 1) / 9
	cases := []struct {
		name        string
		n, min, max int
	}{{"range", 1, 0, maxInt}, {"samples", maxInt, 1, 1}, {"fixed_length", 1, maxInt, maxInt}, {"pixel_bytes", 1, maxLine + 1, maxLine + 1}, {"sample_bytes", maxInt/int(unsafe.Sizeof(glyphs.Sample{})) + 1, 1, 1}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			samples, err := capacityGenerate(t, c.n, c.min, c.max)
			if err == nil || samples != nil {
				t.Errorf("unrepresentable input returned samples=%d/error=%v", len(samples), err)
			}
		})
	}
}
func TestGenerateCapacityControls(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, c := range []struct {
		name        string
		n, min, max int
	}{{"normal", 3, 1, 3}, {"empty_normal", 0, 1, 3}, {"empty_wide_range", 0, 0, maxInt}, {"empty_fixed_wide", 0, maxInt, maxInt}} {
		t.Run(c.name, func(t *testing.T) {
			a, err := capacityGenerate(t, c.n, c.min, c.max)
			if err != nil || a == nil || len(a) != c.n {
				t.Fatalf("legal control samples=%d/error=%v", len(a), err)
			}
			b, err := capacityGenerate(t, c.n, c.min, c.max)
			if err != nil || !reflect.DeepEqual(a, b) {
				t.Fatal("same arguments changed generated data")
			}
		})
	}
	for i, c := range [][3]int{{-1, 1, 1}, {1, -1, 1}, {1, 2, 1}, {1, 0, 0}} {
		t.Run(fmt.Sprint("old_error_", i), func(t *testing.T) {
			a, err := capacityGenerate(t, c[0], c[1], c[2])
			if err == nil || a != nil {
				t.Fatal("old error case changed")
			}
		})
	}
}

// The oracle multiplies the declared geometry in arbitrary-precision integers;
// no large buffers are allocated, including at the last representable value.
func TestGenerateDimensionsIndependentByteReference(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	sampleSize := int(reflect.TypeOf(glyphs.Sample{}).Size())
	sampleLimit := maxInt / sampleSize
	lineLimit := (maxInt/64 + 1) / 9
	for _, n := range []int{0, 1, sampleLimit, sampleLimit + 1, maxInt} {
		for _, length := range []int{0, 1, lineLimit, lineLimit + 1, maxInt} {
			t.Run(fmt.Sprintf("n=%d/len=%d", n, length), func(t *testing.T) {
				bound := big.NewInt(int64(maxInt))
				records := new(big.Int).Mul(big.NewInt(int64(n)), big.NewInt(int64(sampleSize)))
				pixels := new(big.Int).Sub(new(big.Int).Mul(big.NewInt(int64(length)), big.NewInt(9)), big.NewInt(1))
				pixels.Mul(pixels, big.NewInt(8*8))
				wantError := n > 0 && (records.Cmp(bound) > 0 || (length > 0 && pixels.Cmp(bound) > 0))
				err := glyphs.ValidateGenerateDimensions(n, length, length)
				if (err != nil) != wantError {
					t.Fatalf("error=%v want refusal=%v, bytes records=%s/pixels=%s", err, wantError, records, pixels)
				}
			})
		}
	}
}
