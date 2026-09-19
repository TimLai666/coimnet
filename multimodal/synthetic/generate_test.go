package synthetic

import (
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/multimodal"
)

func audioPresenceOffset(t *testing.T, l multimodal.Layout) int {
	t.Helper()
	offset := 0
	for _, name := range l.Order {
		if name == "audio" {
			return offset + l.Widths[name]
		}
		offset += l.Widths[name] + 1
	}
	t.Fatalf("layout has no audio modality")
	return 0
}

func sumValues(v []float64) float64 {
	total := 0.0
	for _, x := range v {
		total += x
	}
	return total
}

func TestGenerateIsDeterministicAndValid(t *testing.T) {
	a, labelsA, err := Generate(5, Config{})
	if err != nil {
		t.Fatalf("Generate(5, zero Config): %v", err)
	}
	b, labelsB, err := Generate(5, Config{})
	if err != nil {
		t.Fatalf("Generate(5, zero Config) again: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("two Generate(5) runs differ")
	}
	if !reflect.DeepEqual(labelsA, labelsB) {
		t.Fatalf("two Generate(5) label runs differ")
	}
	if len(a) != 60 || len(labelsA) != 60 {
		t.Fatalf("Generate(5) lengths = %d samples / %d labels, want 60/60", len(a), len(labelsA))
	}
	layout := DefaultLayout(Config{})
	if err := layout.Validate(); err != nil {
		t.Fatalf("DefaultLayout validate: %v", err)
	}
	for i, s := range a {
		if err := s.Validate(); err != nil {
			t.Fatalf("sample %d Validate: %v", i, err)
		}
		v, err := multimodal.Vector(s, layout)
		if err != nil {
			t.Fatalf("sample %d Vector: %v", i, err)
		}
		if len(v) != 92 {
			t.Fatalf("sample %d vector length = %d, want 92", i, len(v))
		}
	}
}

func TestImageEncodesShapeAndColour(t *testing.T) {
	samples, labels, err := Generate(5, Config{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	square, circle := -1, -1
	for i, l := range labels {
		if square == -1 && l.Shape == 0 {
			square = i
		}
		if circle == -1 && l.Shape == 2 {
			circle = i
		}
		if square != -1 && circle != -1 {
			break
		}
	}
	if square == -1 || circle == -1 {
		t.Fatalf("seed 5 must generate at least one square (shape 0) and one circle (shape 2), got square=%d circle=%d", square, circle)
	}
	n := 8
	centre := n / 2
	centreIdx := centre*n + centre
	image := func(i int) []float64 { return samples[i].Modalities["image"].Values }
	intensity := func(l Label) float64 { return float64(l.Colour+1) / Colours }

	sq := image(square)
	if got := sq[centreIdx]; got != intensity(labels[square]) {
		t.Fatalf("square centre %v, want %v", got, intensity(labels[square]))
	}
	if sq[0] != 0 {
		t.Fatalf("square corner %v, want 0", sq[0])
	}
	ci := image(circle)
	if got := ci[centreIdx]; got != intensity(labels[circle]) {
		t.Fatalf("circle centre %v, want %v", got, intensity(labels[circle]))
	}
	if ci[0] != 0 {
		t.Fatalf("circle corner %v, want 0", ci[0])
	}
	if sumValues(sq) == sumValues(ci) {
		t.Fatalf("square and circle pixel sums must differ, both %v", sumValues(sq))
	}
}

func TestTextIsOneHotOfShapeAndColour(t *testing.T) {
	samples, labels, err := Generate(5, Config{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for i := range samples {
		text := samples[i].Modalities["text"].Values
		if len(text) != Shapes+Colours {
			t.Fatalf("sample %d text length = %d, want %d", i, len(text), Shapes+Colours)
		}
		for j := 0; j < Shapes; j++ {
			want := 0.0
			if j == labels[i].Shape {
				want = 1
			}
			if text[j] != want {
				t.Fatalf("sample %d shape token text[%d] = %v, want %v", i, j, text[j], want)
			}
		}
		for j := 0; j < Colours; j++ {
			want := 0.0
			if j == labels[i].Colour {
				want = 1
			}
			if text[Shapes+j] != want {
				t.Fatalf("sample %d colour token text[%d] = %v, want %v", i, Shapes+j, text[j], want)
			}
		}
	}
}

func TestAudioDropRateProducesMissingNotZero(t *testing.T) {
	cfg := Config{Samples: 200, AudioDropRate: 0.5, ImageSize: 8, AudioSamples: 16}
	samples, _, err := Generate(5, cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	layout := DefaultLayout(cfg)
	presenceIdx := audioPresenceOffset(t, layout)
	missing := 0
	for i, s := range samples {
		audio := s.Modalities["audio"]
		v, err := multimodal.Vector(s, layout)
		if err != nil {
			t.Fatalf("sample %d Vector: %v", i, err)
		}
		if !audio.Present {
			missing++
			if audio.Values != nil {
				t.Fatalf("sample %d: missing audio must have nil values", i)
			}
			if v[presenceIdx] != 0 {
				t.Fatalf("sample %d: missing audio presence = %v, want 0", i, v[presenceIdx])
			}
			continue
		}
		if len(audio.Values) != cfg.AudioSamples {
			t.Fatalf("sample %d: present audio values length = %d, want %d", i, len(audio.Values), cfg.AudioSamples)
		}
		if v[presenceIdx] != 1 {
			t.Fatalf("sample %d: present audio presence = %v, want 1", i, v[presenceIdx])
		}
		allZero := true
		for _, val := range audio.Values {
			if val != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			t.Fatalf("sample %d: present audio must not be all zeros", i)
		}
	}
	if missing < 60 || missing > 140 {
		t.Fatalf("missing audio count = %d, want in [60,140]", missing)
	}
}

func TestSplitUnseenCombinationsIsDisjointAndComplete(t *testing.T) {
	_, labels, err := Generate(5, Config{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	holdout := []Label{{Shape: 0, Colour: 0}, {Shape: 2, Colour: 1}}
	train, eval, err := SplitUnseenCombinations(labels, holdout)
	if err != nil {
		t.Fatalf("SplitUnseenCombinations: %v", err)
	}
	inHoldout := func(l Label) bool {
		for _, h := range holdout {
			if l.Shape == h.Shape && l.Colour == h.Colour {
				return true
			}
		}
		return false
	}
	if len(train)+len(eval) != len(labels) {
		t.Fatalf("train %d + eval %d != total %d", len(train), len(eval), len(labels))
	}
	seen := make(map[int]bool, len(labels))
	for _, i := range train {
		if seen[i] {
			t.Fatalf("index %d used twice in train", i)
		}
		seen[i] = true
		if inHoldout(labels[i]) {
			t.Fatalf("train index %d has holdout label %+v", i, labels[i])
		}
	}
	for _, i := range eval {
		if seen[i] {
			t.Fatalf("index %d in both train and eval", i)
		}
		seen[i] = true
		if !inHoldout(labels[i]) {
			t.Fatalf("eval index %d has non-holdout label %+v", i, labels[i])
		}
	}
	if len(seen) != len(labels) {
		t.Fatalf("train ∪ eval covers %d of %d indices", len(seen), len(labels))
	}
	if len(train) == 0 || len(eval) == 0 {
		t.Fatalf("split left an empty side: train %d eval %d", len(train), len(eval))
	}
	for i := 1; i < len(train); i++ {
		if train[i-1] >= train[i] {
			t.Fatalf("train not sorted: %v", train)
		}
	}
	for i := 1; i < len(eval); i++ {
		if eval[i-1] >= eval[i] {
			t.Fatalf("eval not sorted: %v", eval)
		}
	}

	if _, _, err := SplitUnseenCombinations(labels, nil); err == nil {
		t.Fatal("empty holdout: want error")
	}
	if _, _, err := SplitUnseenCombinations(labels, []Label{{0, 0}, {0, 0}}); err == nil {
		t.Fatal("duplicate holdout (0,0): want error")
	}
	if _, _, err := SplitUnseenCombinations(labels, []Label{{Shape: -1, Colour: 0}}); err == nil {
		t.Fatal("holdout shape -1: want error")
	}
	if _, _, err := SplitUnseenCombinations(labels, []Label{{Shape: 3, Colour: 0}}); err == nil {
		t.Fatal("holdout shape 3: want error")
	}
	if _, _, err := SplitUnseenCombinations(labels, []Label{{Shape: 0, Colour: 3}}); err == nil {
		t.Fatal("holdout colour 3: want error")
	}
	if _, _, err := SplitUnseenCombinations([]Label{{0, 0}, {0, 0}, {0, 0}}, []Label{{0, 0}}); err == nil {
		t.Fatal("all samples in holdout: want error")
	}
	if _, _, err := SplitUnseenCombinations([]Label{{1, 1}, {1, 2}}, []Label{{0, 0}}); err == nil {
		t.Fatal("no sample in holdout: want error")
	}
}
