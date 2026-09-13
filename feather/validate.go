package feather

import (
	"context"
	"fmt"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
)

// validateRecord checks the array invariants that Arrow's IPC decoder does not
// necessarily check while constructing an array. In particular, it checks
// variable-length offsets, child bounds, and non-null dictionary indices before
// the borrowed record is handed to the visitor.
func validateRecord(ctx context.Context, record arrow.Record) (retErr error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			retErr = fmt.Errorf("feather: Arrow record validation panic: %v", recovered)
		}
	}()
	if err := checkContext(ctx); err != nil {
		return err
	}
	if record == nil {
		return fmt.Errorf("feather: Arrow record is nil")
	}
	schema := record.Schema()
	if schema == nil {
		return fmt.Errorf("feather: Arrow record schema is nil")
	}
	fields := schema.Fields()
	columns := record.Columns()
	if len(columns) != len(fields) {
		return fmt.Errorf("feather: Arrow record has %d columns, want %d", len(columns), len(fields))
	}
	rows := record.NumRows()
	if rows < 0 {
		return fmt.Errorf("feather: Arrow record has negative row count %d", rows)
	}
	for i, column := range columns {
		if column == nil {
			return fmt.Errorf("feather: Arrow record column %d is nil", i)
		}
		if int64(column.Len()) != rows {
			return fmt.Errorf("feather: Arrow record column %d has %d rows, want %d", i, column.Len(), rows)
		}
		if !arrow.TypeEqual(fields[i].Type, column.DataType()) {
			return fmt.Errorf("feather: Arrow record column %d type %v does not match schema type %v", i, column.DataType(), fields[i].Type)
		}
		if err := validateArray(ctx, column, 0); err != nil {
			return fmt.Errorf("feather: Arrow record column %q: %w", fields[i].Name, err)
		}
	}
	return checkContext(ctx)
}

func validateArray(ctx context.Context, value arrow.Array, depth int) (retErr error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			retErr = fmt.Errorf("Arrow array validation panic: %v", recovered)
		}
	}()
	if err := checkContext(ctx); err != nil {
		return err
	}
	if depth > maxTypeDepth {
		return fmt.Errorf("array nesting exceeds %d levels", maxTypeDepth)
	}
	if value == nil || value.Data() == nil {
		return fmt.Errorf("array data is missing")
	}
	data := value.Data()
	length := data.Len()
	offset := data.Offset()
	if length < 0 || offset < 0 {
		return fmt.Errorf("array has invalid length=%d offset=%d", length, offset)
	}
	total, ok := checkedAdd(offset, length)
	if !ok {
		return fmt.Errorf("array length and offset overflow")
	}
	nulls := data.NullN()
	if nulls < -1 || nulls > length {
		return fmt.Errorf("array has invalid null count %d", nulls)
	}
	if buffers := data.Buffers(); len(buffers) > 0 && buffers[0] != nil {
		validityBytes, ok := ceilBytes(total)
		if !ok || buffers[0].Len() < validityBytes {
			return fmt.Errorf("validity buffer has %d bytes, want at least %d", buffers[0].Len(), validityBytes)
		}
	}

	switch data.DataType().ID() {
	case arrow.INT8, arrow.INT16, arrow.INT32, arrow.INT64,
		arrow.UINT8, arrow.UINT16, arrow.UINT32, arrow.UINT64,
		arrow.FLOAT64:
		return validateFixedWidth(data, total)
	case arrow.STRING:
		return validateString(data, offset, total)
	case arrow.LIST:
		return validateList(ctx, value, data, offset, total, depth)
	case arrow.DICTIONARY:
		return validateDictionary(ctx, value, depth)
	default:
		return fmt.Errorf("unsupported decoded Arrow type %v", data.DataType())
	}
}

func validateFixedWidth(data arrow.ArrayData, elements int) error {
	if len(data.Buffers()) < 2 {
		return fmt.Errorf("fixed-width array has %d buffers, want at least 2", len(data.Buffers()))
	}
	fixed, ok := data.DataType().(arrow.FixedWidthDataType)
	if !ok || fixed.Bytes() <= 0 {
		return fmt.Errorf("fixed-width Arrow type %v has no positive byte width", data.DataType())
	}
	want, ok := checkedMul(elements, fixed.Bytes())
	if !ok {
		return fmt.Errorf("fixed-width buffer size overflows")
	}
	values := data.Buffers()[1]
	if values == nil || values.Len() < want {
		got := 0
		if values != nil {
			got = values.Len()
		}
		return fmt.Errorf("fixed-width value buffer has %d bytes, want at least %d", got, want)
	}
	return nil
}

func validateString(data arrow.ArrayData, offset, total int) error {
	if len(data.Buffers()) < 3 {
		return fmt.Errorf("string array has %d buffers, want at least 3", len(data.Buffers()))
	}
	if total == 0 {
		return nil
	}
	offsetCount, ok := checkedAdd(total, 1)
	if !ok {
		return fmt.Errorf("string offset count overflows")
	}
	offsetBuffer := data.Buffers()[1]
	offsetBytes, ok := checkedMul(offsetCount, 4)
	if !ok || offsetBuffer == nil || offsetBuffer.Len() < offsetBytes {
		got := 0
		if offsetBuffer != nil {
			got = offsetBuffer.Len()
		}
		return fmt.Errorf("string offset buffer has %d bytes, want at least %d", got, offsetBytes)
	}
	valueBytes := 0
	if data.Buffers()[2] != nil {
		valueBytes = data.Buffers()[2].Len()
	}
	offsets := arrow.Int32Traits.CastFromBytes(offsetBuffer.Bytes())
	previous := int64(-1)
	for i := offset; i < offsetCount; i++ {
		current := int64(offsets[i])
		if current < 0 || current < previous || current > int64(valueBytes) {
			return fmt.Errorf("string offsets are not monotone or exceed value buffer at index %d: %d", i, current)
		}
		previous = current
	}
	return nil
}

func validateList(ctx context.Context, value arrow.Array, data arrow.ArrayData, offset, total, depth int) error {
	if len(data.Buffers()) < 2 {
		return fmt.Errorf("list array has %d buffers, want at least 2", len(data.Buffers()))
	}
	list, ok := value.(*array.List)
	if !ok {
		return fmt.Errorf("unsupported decoded list implementation %T", value)
	}
	child := list.ListValues()
	if child == nil {
		return fmt.Errorf("list child array is missing")
	}
	if err := validateArray(ctx, child, depth+1); err != nil {
		return fmt.Errorf("list child: %w", err)
	}
	if total == 0 {
		return nil
	}
	offsetCount, ok := checkedAdd(total, 1)
	if !ok {
		return fmt.Errorf("list offset count overflows")
	}
	offsetBuffer := data.Buffers()[1]
	offsetBytes, ok := checkedMul(offsetCount, 4)
	if !ok || offsetBuffer == nil || offsetBuffer.Len() < offsetBytes {
		got := 0
		if offsetBuffer != nil {
			got = offsetBuffer.Len()
		}
		return fmt.Errorf("list offset buffer has %d bytes, want at least %d", got, offsetBytes)
	}
	offsets := list.Offsets()
	if len(offsets) < offsetCount {
		return fmt.Errorf("list has %d offsets, want at least %d", len(offsets), offsetCount)
	}
	previous := int64(-1)
	for i := offset; i < offsetCount; i++ {
		current := int64(offsets[i])
		if current < 0 || current < previous || current > int64(child.Len()) {
			return fmt.Errorf("list offsets are not monotone or exceed child length at index %d: %d", i, current)
		}
		previous = current
	}
	return nil
}

func validateDictionary(ctx context.Context, value arrow.Array, depth int) error {
	dictionary, ok := value.(*array.Dictionary)
	if !ok {
		return fmt.Errorf("unsupported decoded dictionary implementation %T", value)
	}
	indices := dictionary.Indices()
	if indices == nil {
		return fmt.Errorf("dictionary indices are missing")
	}
	if err := validateArray(ctx, indices, depth+1); err != nil {
		return fmt.Errorf("dictionary indices: %w", err)
	}
	values := dictionary.Dictionary()
	if values == nil {
		if value.Len() == 0 {
			return nil
		}
		return fmt.Errorf("dictionary values are missing")
	}
	if err := validateArray(ctx, values, depth+1); err != nil {
		return fmt.Errorf("dictionary values: %w", err)
	}
	if indices.Len() != value.Len() {
		return fmt.Errorf("dictionary indices length %d does not match array length %d", indices.Len(), value.Len())
	}
	for i := 0; i < value.Len(); i++ {
		if i%4096 == 0 {
			if err := checkContext(ctx); err != nil {
				return err
			}
		}
		if value.IsNull(i) {
			continue
		}
		index := dictionary.GetValueIndex(i)
		if index < 0 || index >= values.Len() {
			return fmt.Errorf("dictionary index %d at row %d is outside dictionary length %d", index, i, values.Len())
		}
	}
	return nil
}

func checkedAdd(left, right int) (int, bool) {
	if right < 0 || left > int(^uint(0)>>1)-right {
		return 0, false
	}
	return left + right, true
}

func checkedMul(left, right int) (int, bool) {
	if left < 0 || right < 0 || (right != 0 && left > int(^uint(0)>>1)/right) {
		return 0, false
	}
	return left * right, true
}

func ceilBytes(elements int) (int, bool) {
	withPadding, ok := checkedAdd(elements, 7)
	if !ok {
		return 0, false
	}
	return withPadding / 8, true
}
