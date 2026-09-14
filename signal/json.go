package signal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/internal/strictjson"
)

// MaxJSONBytes bounds JSON values accepted by the public decoding helpers.
const MaxJSONBytes int64 = 16 << 20

// decodeStrict decodes exactly one JSON value and rejects unknown fields and
// any non-whitespace data after it.
func decodeStrict(r io.Reader, dst any) error {
	if isNilReader(r) {
		return errors.New("JSON reader must not be nil")
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxJSONBytes+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > MaxJSONBytes {
		return fmt.Errorf("JSON input exceeds %d bytes", MaxJSONBytes)
	}
	return decodeStrictBytes(data, dst)
}

// decodeStrictBytes applies the shared strict rules of internal/strictjson (the
// byte limit, the 64 level nesting bound, duplicate keys under the case folding
// encoding/json accepts for struct fields, unknown fields and trailing data)
// after the Unicode check that encoding/json would otherwise paper over.
func decodeStrictBytes(data []byte, dst any) error {
	if err := validateJSONUnicode(data); err != nil {
		return err
	}
	return strictjson.Decode(bytes.NewReader(data), MaxJSONBytes, dst)
}

func isNilReader(r io.Reader) bool {
	if r == nil {
		return true
	}
	value := reflect.ValueOf(r)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// jsonNumber preserves the distinction between a numeric zero and JSON null.
// Omitted scalar fields retain their existing zero-value defaults.
type jsonNumber[T int | int64 | uint64 | float64] struct{ value T }

func (n jsonNumber[T]) MarshalJSON() ([]byte, error) { return json.Marshal(n.value) }

func (n *jsonNumber[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("numeric JSON value must not be null")
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	n.value = value
	return nil
}

// jsonValues uses the ordinary numeric array representation when marshaling.
type jsonValues []float64

func (v *jsonValues) UnmarshalJSON(data []byte) error {
	// A valid numeric array cannot contain this literal. Other invalid types
	// (including strings containing "null") are rejected by the slice decoder.
	// Check the bytes once so large arrays do not need per-number decoders.
	if bytes.Contains(data, []byte("null")) {
		return fmt.Errorf("numeric JSON array must not contain null")
	}
	var values []float64
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	*v = values
	return nil
}

// validateJSONUnicode runs before encoding/json can replace invalid UTF-8 or
// unpaired UTF-16 escapes with U+FFFD. Other syntax remains the decoder's job.
func validateJSONUnicode(data []byte) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("JSON text must be valid UTF-8")
	}
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return fmt.Errorf("incomplete JSON escape")
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return fmt.Errorf("incomplete JSON Unicode escape")
		}
		code, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if err != nil {
			return fmt.Errorf("invalid JSON Unicode escape")
		}
		i += 4
		if code >= 0xdc00 && code <= 0xdfff {
			return fmt.Errorf("unpaired low surrogate in JSON text")
		}
		if code < 0xd800 || code > 0xdbff {
			continue
		}
		if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
			return fmt.Errorf("unpaired high surrogate in JSON text")
		}
		low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return fmt.Errorf("invalid low surrogate in JSON text")
		}
		i += 6
	}
	return nil
}
