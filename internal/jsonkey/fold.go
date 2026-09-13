// Package jsonkey provides the Unicode case equivalence used when checking
// duplicate JSON struct fields before encoding/json decodes them.
package jsonkey

import (
	"strings"
	"unicode"
)

// Fold maps each rune to the smallest rune in its Unicode simple-fold cycle.
// Unlike strings.ToLower, this equates all aliases accepted by
// encoding/json's case-insensitive struct field matching.
func Fold(key string) string {
	return strings.Map(func(r rune) rune {
		minimum := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		return minimum
	}, key)
}
