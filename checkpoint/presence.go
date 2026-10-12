package checkpoint

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/internal/jsonkey"
)

// presenceObject owns a validation-only view with canonical lower-case
// simple-folded keys, matching encoding/json field aliases. Original payload
// bytes remain authoritative for checksum and strict decoding; data-map keys
// are never rewritten in the restored snapshot. Duplicate aliases are errors.
func presenceObject(object map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	view := make(map[string]json.RawMessage, len(object))
	original := make(map[string]string, len(object))
	for key, raw := range object {
		canonical := canonicalJSONKey(key)
		if previous, exists := original[canonical]; exists {
			return nil, fmt.Errorf("duplicate JSON field alias %q conflicts with %q", key, previous)
		}
		original[canonical] = key
		view[canonical] = raw
	}
	return view, nil
}

// jsonField resolves exported struct fields using the decoder's case-fold
// equivalence, including Unicode aliases. It does not rewrite scalar data.
func jsonField(object map[string]json.RawMessage, name string) (json.RawMessage, bool) {
	if raw, present := object[name]; present {
		return raw, true
	}
	canonical := canonicalJSONKey(name)
	for key, raw := range object {
		if canonicalJSONKey(key) == canonical {
			return raw, true
		}
	}
	return nil, false
}

func canonicalJSONKey(key string) string {
	return strings.ToLower(jsonkey.Fold(key))
}

// validateWireStrings rejects invalid UTF-8 in the typed producer before JSON
// marshaling can replace bytes. Only exported fields represented on the JSON
// wire are walked. Used with the finite, acyclic checkpoint schema values.
func validateWireStrings(value reflect.Value, path string) error {
	return validateWireStringsValue(value, path)
}

func validateWireStringsValue(value reflect.Value, path string) error {
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return validateWireStringsValue(value.Elem(), path)
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return fmt.Errorf("%s contains invalid UTF-8", path)
		}
		return nil
	case reflect.Struct:
		typeOfValue := value.Type()
		for index := 0; index < typeOfValue.NumField(); index++ {
			field := typeOfValue.Field(index)
			if field.PkgPath != "" {
				continue
			}
			name, _, skip := jsonFieldName(field)
			if skip {
				continue
			}
			if err := validateWireStringsValue(value.Field(index), path+"."+name); err != nil {
				return err
			}
		}
		return nil
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if err := validateWireStringsValue(value.Index(index), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		for _, key := range value.MapKeys() {
			keyPath := wireMapKeyPath(path, key)
			if err := validateWireStringsValue(key, keyPath+".key"); err != nil {
				return err
			}
			if err := validateWireStringsValue(value.MapIndex(key), keyPath); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func wireMapKeyPath(path string, key reflect.Value) string {
	switch key.Kind() {
	case reflect.String:
		return fmt.Sprintf("%s[%q]", path, key.String())
	case reflect.Bool:
		return fmt.Sprintf("%s[%t]", path, key.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%s[%d]", path, key.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return fmt.Sprintf("%s[%d]", path, key.Uint())
	default:
		return path + "[key]"
	}
}
