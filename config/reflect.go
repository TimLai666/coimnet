package config

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

var secretRefType = reflect.TypeOf(SecretRef{})

// jsonField reports the JSON name of one struct field and whether it is
// omitted when empty. A field tagged "-" has no JSON name.
func jsonField(field reflect.StructField) (name string, omitEmpty, ok bool) {
	if field.PkgPath != "" {
		return "", false, false
	}
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, false
	}
	name = field.Name
	parts := strings.Split(tag, ",")
	if parts[0] != "" {
		name = parts[0]
	}
	for _, option := range parts[1:] {
		if option == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, true
}

// fieldByJSONName finds the field one JSON key addresses. The match is
// case-insensitive for the same reason encoding/json's is, so that a document
// and the pointers reported for it always agree about which field was set.
func fieldByJSONName(t reflect.Type, key string) (reflect.StructField, bool) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, _, ok := jsonField(field)
		if !ok {
			continue
		}
		if name == key {
			return field, true
		}
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, _, ok := jsonField(field)
		if ok && strings.EqualFold(name, key) {
			return field, true
		}
	}
	return reflect.StructField{}, false
}

// isEmptyValue follows encoding/json's own definition of empty, so that the
// leaves this package enumerates are exactly the keys Marshal writes.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}
	return false
}

// leafPointers lists the JSON pointer of every leaf of one value, in document
// order. A leaf is a scalar, a secret reference, an empty array or a nil
// optional section: exactly the places a single value can come from one layer.
func leafPointers(v reflect.Value, pointer string, out *[]string) {
	if v.Type() == secretRefType {
		*out = append(*out, pointer)
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			*out = append(*out, pointer)
			return
		}
		leafPointers(v.Elem(), pointer, out)
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			name, omitEmpty, ok := jsonField(t.Field(i))
			if !ok {
				continue
			}
			field := v.Field(i)
			if omitEmpty && isEmptyValue(field) {
				continue
			}
			leafPointers(field, pointer+"/"+name, out)
		}
	case reflect.Slice, reflect.Array:
		if v.Len() == 0 {
			*out = append(*out, pointer)
			return
		}
		for i := 0; i < v.Len(); i++ {
			leafPointers(v.Index(i), fmt.Sprintf("%s/%d", pointer, i), out)
		}
	default:
		*out = append(*out, pointer)
	}
}

// overridablePointers lists every pointer the environment and the command line
// may set: each leaf, plus each string array as a whole so that a list can be
// replaced with a comma separated value. A nil optional section is not
// overridable, because turning one on is a structural change that belongs in
// the file.
func overridablePointers(v reflect.Value, pointer string, out map[string]bool) {
	if v.Type() == secretRefType {
		out[pointer] = true
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		overridablePointers(v.Elem(), pointer, out)
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			name, _, ok := jsonField(t.Field(i))
			if !ok {
				continue
			}
			overridablePointers(v.Field(i), pointer+"/"+name, out)
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.String {
			out[pointer] = true
		}
		for i := 0; i < v.Len(); i++ {
			overridablePointers(v.Index(i), fmt.Sprintf("%s/%d", pointer, i), out)
		}
	default:
		out[pointer] = true
	}
}

// setByPointer parses raw as the type the pointer addresses and assigns it.
func setByPointer(root reflect.Value, pointer, raw string) error {
	v := root
	for _, segment := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return fmt.Errorf("config: %s is inside a section this configuration does not declare", pointer)
			}
			v = v.Elem()
		}
		switch v.Kind() {
		case reflect.Struct:
			field, ok := fieldByJSONName(v.Type(), segment)
			if !ok {
				return fmt.Errorf("config: %s addresses no field of the configuration", pointer)
			}
			v = v.FieldByIndex(field.Index)
		case reflect.Slice, reflect.Array:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= v.Len() {
				return fmt.Errorf("config: %s addresses no element of the configuration", pointer)
			}
			v = v.Index(index)
		default:
			return fmt.Errorf("config: %s addresses no field of the configuration", pointer)
		}
	}
	return assign(v, pointer, raw)
}

// assign writes one parsed value, refusing anything the target type cannot
// represent rather than falling back to a zero value.
func assign(v reflect.Value, pointer, raw string) error {
	if v.Type() == secretRefType {
		return fmt.Errorf("config: %s is a secret reference and can only be declared in the configuration file", pointer)
	}
	if !v.CanSet() {
		return fmt.Errorf("config: %s cannot be set", pointer)
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("config: %s expects true or false, got %q", pointer, raw)
		}
		v.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(raw, 10, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("config: %s expects a whole number, got %q", pointer, raw)
		}
		v.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		parsed, err := strconv.ParseUint(raw, 10, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("config: %s expects a non-negative whole number, got %q", pointer, raw)
		}
		v.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(raw, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("config: %s expects a number, got %q", pointer, raw)
		}
		v.SetFloat(parsed)
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.String {
			return fmt.Errorf("config: %s can only be set in the configuration file", pointer)
		}
		if raw == "" {
			v.Set(reflect.MakeSlice(v.Type(), 0, 0))
			return nil
		}
		parts := strings.Split(raw, ",")
		list := reflect.MakeSlice(v.Type(), len(parts), len(parts))
		for i, part := range parts {
			list.Index(i).SetString(strings.TrimSpace(part))
		}
		v.Set(list)
	default:
		return fmt.Errorf("config: %s can only be set in the configuration file", pointer)
	}
	return nil
}

// collectSecrets lists every secret reference the resolved configuration
// declares, addressed by the same JSON pointers provenance uses.
func collectSecrets(v reflect.Value, pointer string, out map[string]SecretRef) {
	if v.Type() == secretRefType {
		out[pointer] = v.Interface().(SecretRef)
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		collectSecrets(v.Elem(), pointer, out)
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			name, _, ok := jsonField(t.Field(i))
			if !ok {
				continue
			}
			collectSecrets(v.Field(i), pointer+"/"+name, out)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			collectSecrets(v.Index(i), fmt.Sprintf("%s/%d", pointer, i), out)
		}
	}
}
