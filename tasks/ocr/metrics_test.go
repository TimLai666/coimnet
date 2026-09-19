package ocr_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr"
)

// TestCERHandCases pins CER on the agreement of ticket 29 decision 4: rune
// level, rate = (S+D+I)/reference length, empty reference => undefined.
func TestCERHandCases(t *testing.T) {
	cases := []struct {
		name    string
		ref     string
		hyp     string
		subs    int
		dels    int
		ins     int
		refLen  int
		rate    float64
		defined bool
	}{
		{
			name: "kitten vs sitting", ref: "kitten", hyp: "sitting",
			subs: 2, dels: 0, ins: 1, refLen: 6, rate: 0.5, defined: true,
		},
		{
			name: "traditional vs simplified pair", ref: "台灣", hyp: "臺灣",
			subs: 1, dels: 0, ins: 0, refLen: 2, rate: 0.5, defined: true,
		},
		{
			name: "identical", ref: "abc", hyp: "abc",
			subs: 0, dels: 0, ins: 0, refLen: 3, rate: 0, defined: true,
		},
		{
			name: "empty hypothesis", ref: "abc", hyp: "",
			subs: 0, dels: 3, ins: 0, refLen: 3, rate: 1, defined: true,
		},
		{
			name: "empty reference", ref: "", hyp: "xy",
			subs: 0, dels: 0, ins: 2, refLen: 0, rate: 0, defined: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ocr.CER(tc.ref, tc.hyp)
			if got.Substitutions != tc.subs ||
				got.Deletions != tc.dels ||
				got.Insertions != tc.ins ||
				got.ReferenceLength != tc.refLen ||
				got.Rate != tc.rate ||
				got.Defined != tc.defined {
				t.Fatalf("CER(%q, %q) = %+v, want subs=%d dels=%d ins=%d refLen=%d rate=%v defined=%v",
					tc.ref, tc.hyp, got, tc.subs, tc.dels, tc.ins, tc.refLen, tc.rate, tc.defined)
			}
		})
	}
}

// TestWERHandCases pins WER on whitespace-separated tokens: extra spaces must
// not change the counts.
func TestWERHandCases(t *testing.T) {
	cases := []struct {
		name    string
		ref     string
		hyp     string
		subs    int
		dels    int
		ins     int
		refLen  int
		rate    float64
		defined bool
	}{
		{
			name: "one inserted word", ref: "the cat sat", hyp: "the cat sat down",
			subs: 0, dels: 0, ins: 1, refLen: 3, rate: 1.0 / 3.0, defined: true,
		},
		{
			name: "one deleted word", ref: "a b", hyp: "b",
			subs: 0, dels: 1, ins: 0, refLen: 2, rate: 0.5, defined: true,
		},
		{
			name: "leading trailing extra whitespace", ref: "  the \t cat \n sat ", hyp: "the  cat  sat",
			subs: 0, dels: 0, ins: 0, refLen: 3, rate: 0, defined: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ocr.WER(tc.ref, tc.hyp)
			if got.Substitutions != tc.subs ||
				got.Deletions != tc.dels ||
				got.Insertions != tc.ins ||
				got.ReferenceLength != tc.refLen ||
				got.Rate != tc.rate ||
				got.Defined != tc.defined {
				t.Fatalf("WER(%q, %q) = %+v, want subs=%d dels=%d ins=%d refLen=%d rate=%v defined=%v",
					tc.ref, tc.hyp, got, tc.subs, tc.dels, tc.ins, tc.refLen, tc.rate, tc.defined)
			}
		})
	}
}

// levenshteinDistance is an independent reference: it only computes the plain
// Levenshtein distance (no alternatives, no backtrace) so the test does not
// share the tie-break logic of the implementation under test.
func levenshteinDistance(ref, hyp []string) int {
	dp := make([][]int, len(ref)+1)
	for i := range dp {
		dp[i] = make([]int, len(hyp)+1)
		dp[i][0] = i
	}
	for j := range dp[0] {
		dp[0][j] = j
	}
	for i := 1; i <= len(ref); i++ {
		for j := 1; j <= len(hyp); j++ {
			cost := 1
			if ref[i-1] == hyp[j-1] {
				cost = 0
			}
			best := dp[i-1][j] + 1
			if v := dp[i][j-1] + 1; v < best {
				best = v
			}
			if v := dp[i-1][j-1] + cost; v < best {
				best = v
			}
			dp[i][j] = best
		}
	}
	return dp[len(ref)][len(hyp)]
}

// randomASCII draws a short string over {a,b,c} from the PCG stream.
func randomASCII(rng *rand.Rand) string {
	const letters = "abc"
	n := int(rng.Uint64() % 7)
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rng.Uint64()%uint64(len(letters))]
	}
	return string(b)
}

// TestEditReportRateIsConsistent checks on 20 PCG-seeded random ASCII pairs
// that S+D+I equals the plain Levenshtein distance of an independent reference
// implementation, for both CER (runes) and WER (fields), and that the rate is
// exactly S+D+I over the reference length when defined.
func TestEditReportRateIsConsistent(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 0))
	for i := 0; i < 20; i++ {
		ref := randomASCII(rng)
		hyp := randomASCII(rng)

		cer := ocr.CER(ref, hyp)
		refRunes := runeTokens(ref)
		hypRunes := runeTokens(hyp)
		want := levenshteinDistance(refRunes, hypRunes)
		if got := cer.Substitutions + cer.Deletions + cer.Insertions; got != want {
			t.Errorf("CER(%q, %q): S+D+I = %d, want distance %d", ref, hyp, got, want)
		}
		if cer.ReferenceLength != len(refRunes) {
			t.Errorf("CER(%q, %q): ReferenceLength = %d, want %d", ref, hyp, cer.ReferenceLength, len(refRunes))
		}
		if cer.Defined {
			if cer.Rate != float64(want)/float64(cer.ReferenceLength) {
				t.Errorf("CER(%q, %q): Rate = %v, want %v", ref, hyp, cer.Rate, float64(want)/float64(cer.ReferenceLength))
			}
		} else if cer.Rate != 0 {
			t.Errorf("CER(%q, %q): undefined report must have Rate 0, got %v", ref, hyp, cer.Rate)
		}

		wer := ocr.WER(ref, hyp)
		refTokens := strings.Fields(ref)
		hypTokens := strings.Fields(hyp)
		want = levenshteinDistance(refTokens, hypTokens)
		if got := wer.Substitutions + wer.Deletions + wer.Insertions; got != want {
			t.Errorf("WER(%q, %q): S+D+I = %d, want distance %d", ref, hyp, got, want)
		}
		if wer.ReferenceLength != len(refTokens) {
			t.Errorf("WER(%q, %q): ReferenceLength = %d, want %d", ref, hyp, wer.ReferenceLength, len(refTokens))
		}
		if wer.Defined {
			if wer.Rate != float64(want)/float64(wer.ReferenceLength) {
				t.Errorf("WER(%q, %q): Rate = %v, want %v", ref, hyp, wer.Rate, float64(want)/float64(wer.ReferenceLength))
			}
		} else if wer.Rate != 0 {
			t.Errorf("WER(%q, %q): undefined report must have Rate 0, got %v", ref, hyp, wer.Rate)
		}
	}
}

// runeTokens converts a string into one token per Unicode code point.
func runeTokens(s string) []string {
	rs := []rune(s)
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}
