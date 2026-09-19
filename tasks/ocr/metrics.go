// Package ocr computes text evaluation metrics for OCR and ASR output:
// character error rate over Unicode code points and word error rate over
// whitespace-separated tokens, both derived from Levenshtein edit counts
// with a fixed tie-break so the counts are reproducible.
package ocr

import "strings"

// EditReport is one edit-distance measurement.
// Rate = (Substitutions+Deletions+Insertions)/ReferenceLength; with an empty
// reference the rate is undefined: Defined is false and Rate is 0 (never a
// division by zero).
type EditReport struct {
	Substitutions   int     `json:"substitutions"`
	Deletions       int     `json:"deletions"`
	Insertions      int     `json:"insertions"`
	ReferenceLength int     `json:"reference_length"`
	Rate            float64 `json:"rate"`
	Defined         bool    `json:"defined"`
}

// CER compares two strings as sequences of Unicode code points (runes), so
// every multi-byte character counts once.
func CER(reference, hypothesis string) EditReport {
	ref := runeTokens(reference)
	hyp := runeTokens(hypothesis)
	subs, dels, ins := editOps(ref, hyp)
	return newReport(len(ref), subs, dels, ins)
}

// WER compares two strings as sequences of whitespace-separated tokens
// (strings.Fields).
func WER(reference, hypothesis string) EditReport {
	ref := strings.Fields(reference)
	hyp := strings.Fields(hypothesis)
	subs, dels, ins := editOps(ref, hyp)
	return newReport(len(ref), subs, dels, ins)
}

// runeTokens splits s into one token per Unicode code point.
func runeTokens(s string) []string {
	rs := []rune(s)
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}

// editOps returns the Levenshtein edit counts between ref and hyp. Every
// operation costs 1; when several operations reach the same distance during
// the backtrace the tie is resolved in the fixed order substitution > deletion
// > insertion, so the split is reproducible.
func editOps(ref, hyp []string) (subs, dels, ins int) {
	n, m := len(ref), len(hyp)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
		dp[i][0] = i
	}
	for j := range dp[0] {
		dp[0][j] = j
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
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
	i, j := n, m
	for i > 0 || j > 0 {
		if i > 0 && j > 0 {
			cost := 1
			if ref[i-1] == hyp[j-1] {
				cost = 0
			}
			if dp[i][j] == dp[i-1][j-1]+cost {
				if cost == 1 {
					subs++
				}
				i--
				j--
				continue
			}
		}
		if i > 0 && dp[i][j] == dp[i-1][j]+1 {
			dels++
			i--
			continue
		}
		ins++
		j--
	}
	return subs, dels, ins
}

// newReport packages edit counts into an EditReport with the rate undefined
// for an empty reference.
func newReport(refLen, subs, dels, ins int) EditReport {
	r := EditReport{
		Substitutions:   subs,
		Deletions:       dels,
		Insertions:      ins,
		ReferenceLength: refLen,
	}
	if refLen > 0 {
		r.Rate = float64(subs+dels+ins) / float64(refLen)
		r.Defined = true
	}
	return r
}
