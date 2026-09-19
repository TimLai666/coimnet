// Feather writing for the flywire adapter: the three intermediate tables are
// Arrow IPC Feather V2 files, each written in fixed-size batches so the
// connectome builder scans them with bounded memory. Nullable columns carry
// source blank cells as null; the schema is fixed per table.
package flywire

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/ipc"
	"github.com/apache/arrow/go/v17/arrow/memory"
)

// featherBatchRows is the batch size every Feather file is written with.
const featherBatchRows = 250000

var (
	weightsSchema = arrow.NewSchema([]arrow.Field{
		{Name: "pre_root_id", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "post_root_id", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "syn_count", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "neuropil", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil)
	annotationsSchema = arrow.NewSchema([]arrow.Field{
		{Name: "root_id", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "included", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "flow", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "super_class", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "class", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "sub_class", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "cell_type", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "side", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil)
	neurotransmittersSchema = arrow.NewSchema([]arrow.Field{
		{Name: "root_id", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "nt_type", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "nt_type_score", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
	}, nil)
)

func writeAnnotations(ctx context.Context, table *csvTable, path string) (int64, error) {
	rows := int64(0)
	err := writeBatched(ctx, path, annotationsSchema, table.name, func(builder *array.RecordBuilder) (bool, error) {
		record, ok, err := table.next()
		if err != nil || !ok {
			return false, err
		}
		values, err := cells(table, record, "root_id", "flow", "super_class", "class", "sub_class", "cell_type", "side")
		if err != nil {
			return false, err
		}
		if err := appendRootID(builder.Field(0).(*array.Int64Builder), values[0]); err != nil {
			return false, err
		}
		builder.Field(1).(*array.StringBuilder).Append(releasedLabel)
		flow := builder.Field(2).(*array.StringBuilder)
		superClass := builder.Field(3).(*array.StringBuilder)
		class := builder.Field(4).(*array.StringBuilder)
		subClass := builder.Field(5).(*array.StringBuilder)
		cellType := builder.Field(6).(*array.StringBuilder)
		side := builder.Field(7).(*array.StringBuilder)
		appendString(flow, values[1])
		appendString(superClass, values[2])
		appendString(class, values[3])
		appendString(subClass, values[4])
		appendString(cellType, values[5])
		appendString(side, values[6])
		rows++
		return true, nil
	})
	return rows, err
}

func writeWeights(ctx context.Context, table *csvTable, path string) (int64, error) {
	rows := int64(0)
	err := writeBatched(ctx, path, weightsSchema, table.name, func(builder *array.RecordBuilder) (bool, error) {
		record, ok, err := table.next()
		if err != nil || !ok {
			return false, err
		}
		values, err := cells(table, record, "pre_root_id", "post_root_id", "neuropil", "syn_count")
		if err != nil {
			return false, err
		}
		if err := appendRootID(builder.Field(0).(*array.Int64Builder), values[0]); err != nil {
			return false, err
		}
		if err := appendRootID(builder.Field(1).(*array.Int64Builder), values[1]); err != nil {
			return false, err
		}
		if err := appendInt64(builder.Field(2).(*array.Int64Builder), values[3], "syn_count"); err != nil {
			return false, err
		}
		appendString(builder.Field(3).(*array.StringBuilder), values[2])
		rows++
		return true, nil
	})
	return rows, err
}

func writeNeurotransmitters(ctx context.Context, table *csvTable, path string) (int64, error) {
	rows := int64(0)
	err := writeBatched(ctx, path, neurotransmittersSchema, table.name, func(builder *array.RecordBuilder) (bool, error) {
		record, ok, err := table.next()
		if err != nil || !ok {
			return false, err
		}
		values, err := cells(table, record, "root_id", "nt_type", "nt_type_score")
		if err != nil {
			return false, err
		}
		if err := appendRootID(builder.Field(0).(*array.Int64Builder), values[0]); err != nil {
			return false, err
		}
		appendString(builder.Field(1).(*array.StringBuilder), values[1])
		if err := appendFloat64(builder.Field(2).(*array.Float64Builder), values[2], "nt_type_score"); err != nil {
			return false, err
		}
		rows++
		return true, nil
	})
	return rows, err
}

// writeBatched drains appender into path in batches of featherBatchRows rows,
// mirroring the writeWeightsBatched pattern of the cancellation fixture. The
// appender reports false once when the table is exhausted.
func writeBatched(ctx context.Context, path string, schema *arrow.Schema, table string, appender func(*array.RecordBuilder) (bool, error)) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("flywire: create %s feather: %w", table, err)
	}
	writer, err := ipc.NewFileWriter(file, ipc.WithSchema(schema))
	if err != nil {
		file.Close()
		return fmt.Errorf("flywire: open %s feather writer: %w", table, err)
	}
	closeFiles := func() error {
		writerErr := writer.Close()
		fileErr := file.Close()
		return errors.Join(writerErr, fileErr)
	}
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	rows := int64(0)
	for {
		if err := ctx.Err(); err != nil {
			builder.Release()
			_ = closeFiles()
			return fmt.Errorf("flywire: convert %s: %w", table, err)
		}
		if rows > 0 && rows%featherBatchRows == 0 {
			if err := flush(writer, builder); err != nil {
				_ = closeFiles()
				return fmt.Errorf("flywire: write %s feather: %w", table, err)
			}
			builder = array.NewRecordBuilder(memory.DefaultAllocator, schema)
		}
		more, err := appender(builder)
		if err != nil {
			builder.Release()
			_ = closeFiles()
			return err
		}
		if !more {
			break
		}
		rows++
	}
	if rows%featherBatchRows != 0 {
		if err := flush(writer, builder); err != nil {
			_ = closeFiles()
			return fmt.Errorf("flywire: write %s feather: %w", table, err)
		}
	} else {
		builder.Release()
	}
	if err := closeFiles(); err != nil {
		return fmt.Errorf("flywire: close %s feather: %w", table, err)
	}
	return nil
}

func flush(writer *ipc.FileWriter, builder *array.RecordBuilder) error {
	record := builder.NewRecord()
	builder.Release()
	defer record.Release()
	return writer.Write(record)
}

// appendRootID parses a root id as uint64 and stores it in an int64 column.
// Blank cells become null. A value above math.MaxInt64 has the fixed contract
// error; it is never truncated silently.
func appendRootID(builder *array.Int64Builder, text string) error {
	if text == "" {
		builder.AppendNull()
		return nil
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return fmt.Errorf("root id %q is not a decimal integer", text)
	}
	if value > math.MaxInt64 {
		return fmt.Errorf("root id %s overflows int64", text)
	}
	builder.Append(int64(value))
	return nil
}

func appendInt64(builder *array.Int64Builder, text, column string) error {
	if text == "" {
		builder.AppendNull()
		return nil
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("%s %q is not a decimal integer", column, text)
	}
	builder.Append(value)
	return nil
}

func appendFloat64(builder *array.Float64Builder, text, column string) error {
	if text == "" {
		builder.AppendNull()
		return nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fmt.Errorf("%s %q is not a number", column, text)
	}
	builder.Append(value)
	return nil
}

// appendString writes blank source cells as null and keeps non-blank cells as
// their exact text.
func appendString(builder *array.StringBuilder, text string) {
	if text == "" {
		builder.AppendNull()
		return
	}
	builder.Append(text)
}
