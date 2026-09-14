package signal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/internal/jsonkey"
)

// MaxJSONBytes bounds JSON values accepted by the public decoding helpers.
const MaxJSONBytes int64 = 16 << 20

// maxJSONDepth bounds nesting during the duplicate-key walk; json.Decoder.Token
// itself has no nesting limit.
const maxJSONDepth = 64

type jsonPathPart struct {
	key   string
	index int
	array bool
}

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
	if err := validateJSONUnicode(data); err != nil {
		return err
	}
	if err := rejectDuplicateKeys(data); err != nil {
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
			return fmt.Errorf("trailing JSON data")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}

func decodeStrictBytes(data []byte, dst any) error {
	return decodeStrict(bytes.NewReader(data), dst)
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

// rejectDuplicateKeys walks the JSON syntax before decoding into structs. The
// standard decoder's field matching is case-insensitive, so keys are folded
// before duplicate detection as well.
func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var path []jsonPathPart
	if err := scanJSONValue(decoder, &path); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON data")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}

// scanJSONValue walks one JSON value. The path is kept as a stack and only
// formatted when an error is reported, so the walk allocates per key, not per
// key times depth.
func scanJSONValue(decoder *json.Decoder, path *[]jsonPathPart) error {
	if len(*path) > maxJSONDepth {
		return fmt.Errorf("JSON nesting exceeds %d levels at %s", maxJSONDepth, formatJSONPath(*path))
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object at %s has a non-string key", formatJSONPath(*path))
			}
			folded := jsonkey.Fold(key)
			if _, exists := seen[folded]; exists {
				return fmt.Errorf("duplicate JSON field %q at %s", key, formatJSONPath(*path))
			}
			seen[folded] = struct{}{}
			*path = append(*path, jsonPathPart{key: key})
			err = scanJSONValue(decoder, path)
			*path = (*path)[:len(*path)-1]
			if err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("JSON object at %s is not closed", formatJSONPath(*path))
		}
	case '[':
		index := 0
		for decoder.More() {
			*path = append(*path, jsonPathPart{index: index, array: true})
			err = scanJSONValue(decoder, path)
			*path = (*path)[:len(*path)-1]
			if err != nil {
				return err
			}
			index++
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("JSON array at %s is not closed", formatJSONPath(*path))
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q at %s", delim, formatJSONPath(*path))
	}
	return nil
}

func formatJSONPath(path []jsonPathPart) string {
	var builder strings.Builder
	builder.WriteByte('$')
	for _, part := range path {
		if part.array {
			builder.WriteByte('[')
			builder.WriteString(strconv.Itoa(part.index))
			builder.WriteByte(']')
			continue
		}
		builder.WriteByte('.')
		builder.WriteString(part.key)
	}
	return builder.String()
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
