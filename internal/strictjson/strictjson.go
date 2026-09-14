// Package strictjson decodes exactly one JSON value under the rules every
// CoImNet file entry shares: a byte limit checked before allocation, duplicate
// keys rejected under Unicode simple folding (the aliases encoding/json accepts
// for struct fields), a bounded nesting depth, unknown struct fields rejected
// and no trailing data. It never repairs input and never retains the reader.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/TimLai666/coimnet/internal/jsonkey"
)

// MaxDepth bounds nesting during the duplicate-key walk; json.Decoder.Token
// itself has no nesting limit.
const MaxDepth = 64

// Decode reads at most maxBytes from r and decodes one JSON value into dst.
// Input longer than the limit, duplicate keys, nesting beyond MaxDepth,
// unknown struct fields and trailing data are all errors; dst is then left in
// whatever state the failing decode produced, so callers must discard it.
func Decode(r io.Reader, maxBytes int64, dst any) error {
	if r == nil || (reflect.ValueOf(r).Kind() == reflect.Pointer && reflect.ValueOf(r).IsNil()) {
		return errors.New("JSON reader must not be nil")
	}
	if maxBytes <= 0 {
		return fmt.Errorf("JSON byte limit must be positive, got %d", maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maxBytes {
		return fmt.Errorf("JSON input exceeds %d bytes", maxBytes)
	}
	if err := RejectDuplicateKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON data")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}

// RejectDuplicateKeys walks the JSON syntax of data before any struct decoding
// and reports the first duplicate key or over-deep nesting it finds.
func RejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var path []string
	return scanValue(decoder, &path)
}

// scanValue walks one JSON value. The path is kept as a stack and only
// formatted when an error is reported, so the walk allocates per key, not
// per key times depth.
func scanValue(decoder *json.Decoder, path *[]string) error {
	if len(*path) > MaxDepth {
		return fmt.Errorf("JSON nesting exceeds %d levels at %s", MaxDepth, formatPath(*path))
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key at %s is not a string", formatPath(*path))
			}
			folded := jsonkey.Fold(key)
			if _, exists := seen[folded]; exists {
				return fmt.Errorf("duplicate JSON key %q at %s", key, formatPath(*path))
			}
			seen[folded] = struct{}{}
			*path = append(*path, key)
			if err := scanValue(decoder, path); err != nil {
				return err
			}
			*path = (*path)[:len(*path)-1]
		}
		_, err = decoder.Token()
		return err
	case '[':
		for i := 0; decoder.More(); i++ {
			*path = append(*path, strconv.Itoa(i))
			if err := scanValue(decoder, path); err != nil {
				return err
			}
			*path = (*path)[:len(*path)-1]
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %v at %s", delim, formatPath(*path))
	}
}

func formatPath(path []string) string {
	if len(path) == 0 {
		return "$"
	}
	return "$." + strings.Join(path, ".")
}
