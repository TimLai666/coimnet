package glyphs_test

import (
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

func TestGlyphIsDeterministicAndBounded(t *testing.T) {
	fam := glyphs.Family{Seed: 42}
	seen := make(map[[8][8]uint8]bool)
	for i := 0; i < 50; i++ {
		r := '你' + rune(i)
		g1 := fam.Glyph(r)
		g2 := fam.Glyph(r)
		if g1 != g2 {
			t.Fatalf("glyph for rune %q is not deterministic", r)
		}
		ink := 0
		for _, row := range g1 {
			for _, p := range row {
				ink += int(p)
			}
		}
		if ink < 12 || ink > 40 {
			t.Fatalf("glyph for rune %q has %d ink pixels, want in [12, 40]", r, ink)
		}
		seen[g1] = true
	}
	if len(seen) < 45 {
		t.Fatalf("only %d distinct glyphs across 50 runes, want >= 45", len(seen))
	}
}

func TestFamiliesDiffer(t *testing.T) {
	f1, f2 := glyphs.Family{Seed: 1}, glyphs.Family{Seed: 2}
	diff := 0
	for _, r := range "台灣AZaz09" {
		if f1.Glyph(r) != f2.Glyph(r) {
			diff++
		}
	}
	if diff < 7 {
		t.Fatalf("families differ on only %d/8 runes, want >= 7", diff)
	}
}

func TestRenderGeometry(t *testing.T) {
	var o glyphs.Options
	img, boxes, err := glyphs.Render("台灣", glyphs.Family{Seed: 5}, o)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 17 || img.Height != 8 {
		t.Fatalf("size = %dx%d, want 17x8", img.Width, img.Height)
	}
	want := []glyphs.Box{{Rune: '台', X0: 0, X1: 8}, {Rune: '灣', X0: 9, X1: 17}}
	if !reflect.DeepEqual(boxes, want) {
		t.Fatalf("boxes = %+v, want %+v", boxes, want)
	}
	for y := 0; y < 8; y++ {
		v := img.Pixels[y*17+8]
		if v != o.Background {
			t.Fatalf("blank column pixel (%d,8) = %v, want background %v", y, v, o.Background)
		}
	}
}

func TestRenderOptionsRanges(t *testing.T) {
	fam := glyphs.Family{Seed: 7}
	if _, _, err := glyphs.Render("a", fam, glyphs.Options{Background: 0.6}); err == nil {
		t.Fatal("want error for Background 0.6")
	}
	if _, _, err := glyphs.Render("a", fam, glyphs.Options{Noise: 0.6}); err == nil {
		t.Fatal("want error for Noise 0.6")
	}
	if _, _, err := glyphs.Render("", fam, glyphs.Options{}); err == nil {
		t.Fatal("want error for empty text")
	}
	o := glyphs.Options{Background: 0.25, Invert: true}
	img, _, err := glyphs.Render("開", fam, o)
	if err != nil {
		t.Fatal(err)
	}
	g := fam.Glyph('開')
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			v := img.Pixels[y*8+x]
			if g[y][x] == 1 {
				if v != o.Background {
					t.Fatalf("inverted ink pixel (%d,%d) = %v, want %v", y, x, v, o.Background)
				}
			} else if v != 1 {
				t.Fatalf("inverted background pixel (%d,%d) = %v, want 1", y, x, v)
			}
		}
	}
}

func TestRenderNoiseIsDeterministicAndClamped(t *testing.T) {
	o := glyphs.Options{Noise: 0.4, Seed: 11}
	fam := glyphs.Family{Seed: 3}
	a, _, err := glyphs.Render("ab", fam, o)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := glyphs.Render("ab", fam, o)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Pixels, b.Pixels) {
		t.Fatal("noisy render not deterministic for the same seed")
	}
	for _, p := range a.Pixels {
		if p < 0 || p > 1 {
			t.Fatalf("pixel %v outside [0, 1]", p)
		}
	}
	c, _, err := glyphs.Render("ab", fam, glyphs.Options{Noise: 0.4, Seed: 12})
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(a.Pixels, c.Pixels) {
		t.Fatal("different noise seeds produced identical pixels")
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	alphabet := []rune("abcdefghij")
	fam := glyphs.Family{Seed: 3}
	o := glyphs.Options{Noise: 0.1, Seed: 0}
	a, err := glyphs.Generate(alphabet, 20, 1, 5, fam, o, 9)
	if err != nil {
		t.Fatal(err)
	}
	b, err := glyphs.Generate(alphabet, 20, 1, 5, fam, o, 9)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("generate is not deterministic for equal arguments")
	}
	if len(a) != 20 {
		t.Fatalf("got %d samples, want 20", len(a))
	}
	allowed := make(map[rune]bool)
	for _, r := range alphabet {
		allowed[r] = true
	}
	for _, s := range a {
		if l := len([]rune(s.Text)); l < 1 || l > 5 {
			t.Fatalf("sample length %d outside [1, 5]", l)
		}
		for _, r := range s.Text {
			if !allowed[r] {
				t.Fatalf("sample contains rune %q outside the alphabet", r)
			}
		}
	}
}

func TestUnseenCombinations(t *testing.T) {
	seen := []glyphs.Sample{{Text: "ab"}}
	candidates := []glyphs.Sample{{Text: "ab"}, {Text: "ba"}}
	got := glyphs.UnseenCombinations(seen, candidates)
	if len(got) != 1 || got[0].Text != "ba" {
		t.Fatalf("got %+v, want only the sample with text %q", got, "ba")
	}
}
