package checkpoint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// checkRequiredFields walks the JSON object in data against the struct type
// target and rejects any required value that encoding/json would silently
// turn into a zero value: a missing or null scalar, a missing or null nested
// struct, a missing array without omitempty, or a null element inside an
// array of scalars. A nil array (JSON null) stays legal because the episode
// format writes nil slices as null (for example core.weights of a graph with
// no edges), and fields tagged omitempty may be absent. An optional pointer to
// a struct (the LIF core of a spiking snapshot) is legal when absent or null,
// because a nil pointer is the declared "this core is not configured" value
// rather than a silently zeroed one; when present it is walked like a nested
// struct. A pointer to a scalar (the receptor indices of a plasticity rule) is
// optional for the same reason, and a present one is checked for its JSON token
// kind only. Unknown keys and remaining type mismatches are left to the strict
// decoder.
func checkRequiredFields(data []byte, target reflect.Type, path string) error {
	if target.Kind() != reflect.Struct {
		return fmt.Errorf("required-field check needs a struct, got %s", target.Kind())
	}
	if isJSONNull(data) {
		return fmt.Errorf("%s is null", path)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("%s is not a JSON object: %w", path, err)
	}
	for i := 0; i < target.NumField(); i++ {
		field := target.Field(i)
		if !field.IsExported() {
			continue
		}
		name, omitEmpty, skip := jsonFieldName(field)
		if skip {
			continue
		}
		fieldPath := path + "." + name
		raw, present := object[name]
		switch field.Type.Kind() {
		case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64, reflect.String:
			if !present {
				if omitEmpty {
					continue
				}
				return fmt.Errorf("%s is missing", fieldPath)
			}
			if isJSONNull(raw) {
				return fmt.Errorf("%s is null", fieldPath)
			}
		case reflect.Struct:
			if !present {
				return fmt.Errorf("%s is missing", fieldPath)
			}
			if err := checkRequiredFields(raw, field.Type, fieldPath); err != nil {
				return err
			}
		case reflect.Pointer:
			element := field.Type.Elem()
			// A pointer to a scalar is optional by declaration: absent or null
			// is the nil pointer the field is allowed to hold, not a silently
			// zeroed value. A present one still has to be the JSON token kind
			// that scalar decodes from, so a quoted number is reported here
			// with its path rather than as a decoder type error.
			if name, scalar := jsonScalarName(element.Kind()); scalar {
				if !present || isJSONNull(raw) {
					continue
				}
				if !isJSONScalarToken(raw, name) {
					return fmt.Errorf("%s is not a JSON %s", fieldPath, name)
				}
				continue
			}
			if !present {
				if omitEmpty {
					continue
				}
				return fmt.Errorf("%s is missing", fieldPath)
			}
			if isJSONNull(raw) {
				continue
			}
			if element.Kind() != reflect.Struct {
				return fmt.Errorf("%s has unsupported pointer element kind %s", fieldPath, element.Kind())
			}
			if err := checkRequiredFields(raw, element, fieldPath); err != nil {
				return err
			}
		case reflect.Slice:
			if !present {
				if omitEmpty {
					continue
				}
				return fmt.Errorf("%s is missing", fieldPath)
			}
			if isJSONNull(raw) {
				continue
			}
			if err := checkArrayElements(raw, field.Type.Elem(), fieldPath); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s has unsupported field kind %s", fieldPath, field.Type.Kind())
		}
	}
	return nil
}

func checkArrayElements(data []byte, element reflect.Type, path string) error {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("%s is not a JSON array: %w", path, err)
	}
	for i, item := range items {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		switch element.Kind() {
		case reflect.Struct:
			if err := checkRequiredFields(item, element, itemPath); err != nil {
				return err
			}
		case reflect.Slice:
			if isJSONNull(item) {
				return fmt.Errorf("%s is null", itemPath)
			}
			if err := checkArrayElements(item, element.Elem(), itemPath); err != nil {
				return err
			}
		default:
			if isJSONNull(item) {
				return fmt.Errorf("%s is null", itemPath)
			}
		}
	}
	return nil
}

// jsonScalarName names the JSON token kind a Go scalar kind decodes from, and
// reports false for every kind that is not a scalar.
func jsonScalarName(kind reflect.Kind) (string, bool) {
	switch kind {
	case reflect.Bool:
		return "boolean", true
	case reflect.String:
		return "string", true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number", true
	}
	return "", false
}

// isJSONScalarToken reports whether data is a JSON token of the named kind.
// Whether the value also fits the Go type is left to the strict decoder, like
// every other value this walk lets through.
func isJSONScalarToken(data []byte, name string) bool {
	value := bytes.TrimSpace(data)
	if len(value) == 0 {
		return false
	}
	switch name {
	case "boolean":
		return bytes.Equal(value, []byte("true")) || bytes.Equal(value, []byte("false"))
	case "string":
		return value[0] == '"'
	default:
		return value[0] == '-' || (value[0] >= '0' && value[0] <= '9')
	}
}

func jsonFieldName(field reflect.StructField) (name string, omitEmpty, skip bool) {
	tag, ok := field.Tag.Lookup("json")
	if !ok {
		return field.Name, false, false
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "-" {
		return "", false, true
	}
	name = parts[0]
	if name == "" {
		name = field.Name
	}
	for _, option := range parts[1:] {
		if option == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, false
}

func isJSONNull(data []byte) bool {
	return bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}
