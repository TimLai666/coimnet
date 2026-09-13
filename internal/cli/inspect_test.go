package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/ipc"
	"github.com/apache/arrow/go/v17/arrow/memory"
)

func inspectFixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "source.feather")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	s := arrow.NewSchema([]arrow.Field{{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64, Nullable: true}}, nil)
	b := array.NewInt64Builder(memory.DefaultAllocator)
	defer b.Release()
	b.Append(9007199254740993)
	b.AppendNull()
	a := b.NewArray()
	defer a.Release()
	r := array.NewRecord(s, []arrow.Array{a}, 2)
	defer r.Release()
	w, err := ipc.NewFileWriter(f, ipc.WithSchema(s))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Write(r); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDataInspectReadsFeather(t *testing.T) {
	p := inspectFixture(t)
	var out, errout bytes.Buffer
	err := Run(context.Background(), []string{"data", "inspect", "--input", p, "--max-bytes", "1000000"}, &out, &errout)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Rows     int64 `json:"rows"`
		Complete bool  `json:"complete"`
		Fields   []struct {
			Name string `json:"name"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Rows != 2 || !report.Complete || len(report.Fields) != 1 || report.Fields[0].Name != "bodyId" {
		t.Fatalf("report: %s", out.String())
	}
	out.Reset()
	err = Run(context.Background(), []string{"data", "inspect", "--input", p, "--max-bytes", "1000000", "--max-rows", "1"}, &out, &errout)
	if err == nil {
		t.Fatal("accepted scan over row limit")
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.Complete || report.Rows != 0 {
		t.Fatalf("partial report = %s, decode error = %v", out.String(), err)
	}
}

func TestDataInspectFailuresAndHelp(t *testing.T) {
	p := inspectFixture(t)
	for _, args := range [][]string{{"data", "inspect"}, {"data", "inspect", "--input", p, "--max-bytes", "1"}, {"data", "inspect", "--input", p, "--max-bytes", "1000000", "--max-rows", "1"}, {"data", "inspect", "--unknown"}} {
		var out, errout bytes.Buffer
		if err := Run(context.Background(), args, &out, &errout); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	var out, errout bytes.Buffer
	if err := Run(context.Background(), []string{"data", "inspect", "--help"}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"max-bytes", "max-footer-bytes", "max-arrow-bytes", "max-rows", "--input", "Example:"} {
		if !strings.Contains(out.String(), word) {
			t.Fatalf("help missing %s", word)
		}
	}
}
