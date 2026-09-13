package connectome

import (
	"fmt"
	"strings"

	"github.com/TimLai666/coimnet/feather"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
)

// stringValues reads nullable string-like columns: plain utf8 or a
// dictionary whose values are utf8. Dictionary values are decoded, never
// replaced by their index.
type stringValues interface {
	IsNull(i int) bool
	Value(i int) string
}

type dictionaryStrings struct{ dictionary *array.Dictionary }

func (d dictionaryStrings) IsNull(i int) bool  { return d.dictionary.IsNull(i) }
func (d dictionaryStrings) Value(i int) string { return d.dictionary.ValueStr(i) }

func columnIndex(record arrow.Record, name string) (int, error) {
	indices := record.Schema().FieldIndices(name)
	if len(indices) != 1 {
		return 0, fmt.Errorf("connectome: column %q has %d matches in the source schema", name, len(indices))
	}
	return indices[0], nil
}

func int64Column(record arrow.Record, name string) (*array.Int64, error) {
	index, err := columnIndex(record, name)
	if err != nil {
		return nil, err
	}
	values, ok := record.Column(index).(*array.Int64)
	if !ok {
		return nil, fmt.Errorf("connectome: column %q has Arrow type %v, want int64", name, record.Column(index).DataType())
	}
	return values, nil
}

func float64Column(record arrow.Record, name string) (*array.Float64, error) {
	index, err := columnIndex(record, name)
	if err != nil {
		return nil, err
	}
	values, ok := record.Column(index).(*array.Float64)
	if !ok {
		return nil, fmt.Errorf("connectome: column %q has Arrow type %v, want float64", name, record.Column(index).DataType())
	}
	return values, nil
}

func stringColumn(record arrow.Record, name string) (stringValues, error) {
	index, err := columnIndex(record, name)
	if err != nil {
		return nil, err
	}
	switch column := record.Column(index).(type) {
	case *array.String:
		return column, nil
	case *array.Dictionary:
		if _, ok := column.Dictionary().(*array.String); !ok {
			return nil, fmt.Errorf("connectome: column %q is a dictionary of %v, want utf8 values", name, column.Dictionary().DataType())
		}
		return dictionaryStrings{dictionary: column}, nil
	default:
		return nil, fmt.Errorf("connectome: column %q has Arrow type %v, want utf8 or dictionary<utf8>", name, record.Column(index).DataType())
	}
}

// optionalStringColumn returns nil for an unmapped (empty) column name.
func optionalStringColumn(record arrow.Record, name string) (stringValues, error) {
	if name == "" {
		return nil, nil
	}
	return stringColumn(record, name)
}

func optionalFloat64Column(record arrow.Record, name string) (*array.Float64, error) {
	if name == "" {
		return nil, nil
	}
	return float64Column(record, name)
}

// nullString copies the value: Arrow string values alias the record's
// value buffer, and retaining them would pin whole batches in memory.
func nullString(values stringValues, i int) NullString {
	if values == nil || values.IsNull(i) {
		return NullString{}
	}
	return NullString{Valid: true, Value: strings.Clone(values.Value(i))}
}

// checkReportedFields validates column presence and types from the scan
// report, which also covers sources with zero record batches.
func checkReportedFields(role FileRole, fields []feather.Field, want map[string]string) error {
	byName := make(map[string]feather.Field, len(fields))
	for _, field := range fields {
		byName[field.Name] = field
	}
	for name, kind := range want {
		if name == "" {
			continue
		}
		field, ok := byName[name]
		if !ok {
			return fmt.Errorf("connectome: %s source has no column %q", role, name)
		}
		switch kind {
		case "int64", "float64":
			if field.Type != kind {
				return fmt.Errorf("connectome: %s column %q has type %s, want %s", role, name, field.Type, kind)
			}
		case "string":
			if field.Type != "utf8" && !strings.HasPrefix(field.Type, "dictionary<values=utf8,") {
				return fmt.Errorf("connectome: %s column %q has type %s, want utf8 or dictionary<utf8>", role, name, field.Type)
			}
		default:
			return fmt.Errorf("connectome: unsupported column kind %q", kind)
		}
	}
	return nil
}
