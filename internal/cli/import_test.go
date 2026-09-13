package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func writeImportFeather(t *testing.T, path string, schema *arrow.Schema, fill func(*array.RecordBuilder)) {
	t.Helper()
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	fill(builder)
	record := builder.NewRecord()
	builder.Release()
	defer record.Release()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := ipc.NewFileWriter(file, ipc.WithSchema(schema))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(record); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func fingerprintJSON(t *testing.T, dir, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return map[string]any{"path": name, "bytes": len(data), "sha256": hex.EncodeToString(sum[:]), "hash_status": "locally_recorded"}
}

// importFixture writes three small official-like sources and a manifest
// with relative paths into one directory.
func importFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeImportFeather(t, filepath.Join(dir, "annotations.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "status", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		ids := b.Field(0).(*array.Int64Builder)
		status := b.Field(1).(*array.StringBuilder)
		ids.AppendValues([]int64{1, 2, 3}, nil)
		status.AppendValues([]string{"Traced", "Traced", "Orphan"}, nil)
	})
	writeImportFeather(t, filepath.Join(dir, "weights.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "weight", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{1, 2, 1}, nil)
		b.Field(1).(*array.Int64Builder).AppendValues([]int64{2, 1, 3}, nil)
		b.Field(2).(*array.Int64Builder).AppendValues([]int64{5, 6, 7}, nil)
	})
	writeImportFeather(t, filepath.Join(dir, "nt.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "consensus_nt", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{1}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"gaba"}, nil)
	})
	weights := fingerprintJSON(t, dir, "weights.feather")
	weights["role"] = "weights"
	annotations := fingerprintJSON(t, dir, "annotations.feather")
	annotations["role"] = "annotations"
	nt := fingerprintJSON(t, dir, "nt.feather")
	nt["role"] = "neurotransmitters"
	manifest := map[string]any{
		"schema_version": "coimnet-dataset-manifest/v1",
		"dataset":        "fixture",
		"namespace":      "fixture-v1",
		"source_version": "v1",
		"license":        map[string]any{"name": "CC-BY-4.0", "url": "https://creativecommons.org/licenses/by/4.0/"},
		"acquired_at":    "2026-09-13T00:00:00Z",
		"files":          []any{weights, annotations, nt},
		"field_mapping": map[string]any{
			"weights":           map[string]any{"source": "body_pre", "target": "body_post", "value": "weight"},
			"annotations":       map[string]any{"id": "bodyId", "status": "status"},
			"neurotransmitters": map[string]any{"id": "body", "consensus": "consensus_nt"},
		},
		"identity": map[string]any{
			"weights_endpoints_are_annotation_ids":    true,
			"neurotransmitter_ids_are_annotation_ids": true,
			"evidence": "fixture",
		},
		"selection":           map[string]any{"source": "annotations", "field": "status", "equals": "Traced", "label": "engineering selection"},
		"duplicate_semantics": "unknown",
		"coordinate_unit":     "unverified",
		"transform_history":   []any{map[string]any{"step": "generate", "description": "fixture", "version": "test"}},
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDataImportBuildsReportFromRelativeManifest(t *testing.T) {
	dir := importFixture(t)
	var out, errout bytes.Buffer
	err := Run(context.Background(), []string{"data", "import", "--manifest", filepath.Join(dir, "manifest.json"), "--temp-dir", t.TempDir(), "--max-memory-bytes", "1000000", "--max-temp-bytes", "1000000"}, &out, &errout)
	if err != nil {
		t.Fatalf("data import: %v; stderr=%s", err, errout.String())
	}
	var report struct {
		SchemaVersion string `json:"schema_version"`
		EdgeView      string `json:"edge_view"`
		Annotated     struct {
			Nodes uint64 `json:"nodes"`
			Edges uint64 `json:"edges"`
		} `json:"annotated_neurons"`
		Raw struct {
			Rows uint64 `json:"rows"`
		} `json:"raw_segments"`
		Sources []struct {
			Path              string `json:"path"`
			FingerprintStable bool   `json:"fingerprint_stable"`
		} `json:"sources"`
		Hashes struct {
			Report string `json:"report"`
		} `json:"hashes"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("invalid report JSON: %v\n%s", err, out.String())
	}
	if report.SchemaVersion != "coimnet-graph-report/v1" || report.EdgeView != "rows" || report.Annotated.Nodes != 2 || report.Annotated.Edges != 2 || report.Raw.Rows != 3 || len(report.Hashes.Report) != 64 {
		t.Fatalf("report = %s", out.String())
	}
	for _, source := range report.Sources {
		if !filepath.IsAbs(source.Path) || !strings.HasPrefix(source.Path, dir) || !source.FingerprintStable {
			t.Fatalf("source = %#v", source)
		}
	}
}

func TestDataImportFailuresAndHelp(t *testing.T) {
	dir := importFixture(t)
	manifest := filepath.Join(dir, "manifest.json")
	for _, args := range [][]string{
		{"data", "import"},
		{"data", "import", "--manifest", filepath.Join(dir, "missing.json")},
		{"data", "import", "--manifest", manifest, "--edge-view", "aggregated_pairs"},
		{"data", "import", "--manifest", manifest, "--max-memory-bytes", "0"},
		{"data", "import", "--manifest", manifest, "--max-rows", "1"},
		{"data", "import", "--manifest", manifest, "--temp-dir", filepath.Join(dir, "absent")},
		{"data", "import", "--manifest", manifest, "extra"},
		{"data", "import", "--unknown"},
	} {
		var out, errout bytes.Buffer
		if err := Run(context.Background(), args, &out, &errout); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte(`{"schema_version":"nope"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	if err := Run(context.Background(), []string{"data", "import", "--manifest", corrupt}, &out, &errout); err == nil {
		t.Fatal("accepted an invalid manifest")
	}
	out.Reset()
	if err := Run(context.Background(), []string{"data", "import", "--help"}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"--manifest", "max-memory-bytes", "max-temp-bytes", "max-run-files", "sort-buffer-bytes", "edge-view", "Example:", "Errors:"} {
		if !strings.Contains(out.String(), word) {
			t.Fatalf("help missing %s", word)
		}
	}
	out.Reset()
	if err := Run(context.Background(), []string{"--help"}, &out, &errout); err != nil || !strings.Contains(out.String(), "data import") {
		t.Fatalf("overview missing data import: %v\n%s", err, out.String())
	}
}
