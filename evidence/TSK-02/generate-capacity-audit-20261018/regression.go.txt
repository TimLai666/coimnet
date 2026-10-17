package asr_test

import (
	"math/big"
	"testing"
	"unsafe"

	"github.com/TimLai666/coimnet/tasks/asr"
)

func generateCapacityMaxInt() int {
	return int(^uint(0) >> 1)
}

func generateCapacityWithoutPanic(c asr.SynthConfig, n, minLen, maxLen int, seed uint64) (rows []asr.Utterance, err error, recovered any) {
	defer func() {
		recovered = recover()
	}()
	rows, err = asr.Generate(c, n, minLen, maxLen, seed)
	return
}

func assertGenerateCapacityExceedsUintptr(t *testing.T, elements int, elementSize uintptr) {
	t.Helper()
	product := new(big.Int).Mul(
		new(big.Int).SetInt64(int64(elements)),
		new(big.Int).SetUint64(uint64(elementSize)),
	)
	maxUintptr := new(big.Int).SetUint64(uint64(^uintptr(0)))
	if product.Cmp(maxUintptr) <= 0 {
		t.Fatalf("capacity product %s does not exceed uintptr maximum %s", product, maxUintptr)
	}
}

func generateCapacityConfig() asr.SynthConfig {
	return asr.SynthConfig{
		SampleRate: 1000,
		DurationMS: 1,
		GapMS:      0,
		Noise:      0,
		Alphabet:   []rune{'a'},
	}
}

func TestGenerateRejectsUnrepresentableCapacity(t *testing.T) {
	maxInt := generateCapacityMaxInt()
	runeSize := unsafe.Sizeof(rune(0))
	if runeSize != 4 {
		t.Fatalf("rune size = %d bytes, want 4", runeSize)
	}

	cases := []struct {
		name        string
		n           int
		minLen      int
		maxLen      int
		elementSize uintptr
	}{
		{
			name:        "utterance_container_max_int",
			n:           maxInt,
			minLen:      1,
			maxLen:      1,
			elementSize: unsafe.Sizeof(asr.Utterance{}),
		},
		{
			name:        "rune_buffer_max_int",
			n:           1,
			minLen:      maxInt,
			maxLen:      maxInt,
			elementSize: runeSize,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertGenerateCapacityExceedsUintptr(t, maxInt, tc.elementSize)
			rows, err, recovered := generateCapacityWithoutPanic(generateCapacityConfig(), tc.n, tc.minLen, tc.maxLen, 11)
			if recovered != nil {
				t.Fatalf("Generate panicked for unrepresentable %s capacity: %v", tc.name, recovered)
			}
			if err == nil {
				t.Fatalf("Generate(%s) returned nil error, want unrepresentable capacity rejection", tc.name)
			}
			if rows != nil {
				t.Fatalf("Generate(%s) returned %#v with error %v, want nil output", tc.name, rows, err)
			}
		})
	}
}

func TestGenerateRetainsNormalAndUnusedLength(t *testing.T) {
	maxInt := generateCapacityMaxInt()
	cases := []struct {
		name           string
		n              int
		minLen         int
		maxLen         int
		wantCount      int
		wantText       string
		wantSamples    int
		wantSpeaker    string
		wantSession    string
		wantZeroSample bool
	}{
		{
			name:           "normal_one_rune",
			n:              1,
			minLen:         1,
			maxLen:         1,
			wantCount:      1,
			wantText:       "a",
			wantSamples:    1,
			wantSpeaker:    "spk3",
			wantSession:    "ses11",
			wantZeroSample: true,
		},
		{
			name:      "zero_count_unused_max_length",
			n:         0,
			minLen:    maxInt,
			maxLen:    maxInt,
			wantCount: 0,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rows, err := asr.Generate(generateCapacityConfig(), tc.n, tc.minLen, tc.maxLen, 11)
			if err != nil {
				t.Fatalf("Generate(%s): %v", tc.name, err)
			}
			if rows == nil {
				t.Fatalf("Generate(%s) returned nil slice, want nonnil result", tc.name)
			}
			if len(rows) != tc.wantCount {
				t.Fatalf("Generate(%s) returned %d rows, want %d", tc.name, len(rows), tc.wantCount)
			}
			if tc.wantCount == 0 {
				return
			}
			row := rows[0]
			if row.Text != tc.wantText {
				t.Errorf("Generate(%s) text = %q, want %q", tc.name, row.Text, tc.wantText)
			}
			if len(row.Samples) != tc.wantSamples {
				t.Errorf("Generate(%s) samples = %d, want %d", tc.name, len(row.Samples), tc.wantSamples)
			}
			if tc.wantZeroSample && (len(row.Samples) != 1 || row.Samples[0] != 0) {
				t.Errorf("Generate(%s) samples = %#v, want one exact zero sample", tc.name, row.Samples)
			}
			if row.Speaker != tc.wantSpeaker {
				t.Errorf("Generate(%s) speaker = %q, want %q", tc.name, row.Speaker, tc.wantSpeaker)
			}
			if row.Session != tc.wantSession {
				t.Errorf("Generate(%s) session = %q, want %q", tc.name, row.Session, tc.wantSession)
			}
		})
	}
}
