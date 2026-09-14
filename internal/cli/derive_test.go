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
)

// deriveFixture builds a graph store with importFixture's two-node graph and
// writes the four derivation sources plus a rules file next to it. The graph
// selects bodies 1 and 2 and holds the edges 1->2 and 2->1 with raw weights
// 5 and 6.
func deriveFixture(t *testing.T) (dir, store, rules string) {
	t.Helper()
	dir = importFixture(t)
	store = filepath.Join(dir, "graph.coimgraph")
	var out, errout bytes.Buffer
	if err := Run(context.Background(), []string{"data", "import", "--manifest", filepath.Join(dir, "manifest.json"), "--temp-dir", t.TempDir(), "--out-store", store}, &out, &errout); err != nil {
		t.Fatalf("data import: %v; stderr=%s", err, errout.String())
	}

	transmitters := []string{"acetylcholine", "dopamine", "gaba", "glutamate", "histamine", "octopamine", "serotonin"}
	statsFields := []arrow.Field{
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "pre", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "post", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
	}
	writeImportFeather(t, filepath.Join(dir, "body-stats.feather"), arrow.NewSchema(statsFields, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{1, 2}, nil)
		b.Field(1).(*array.Int32Builder).AppendValues([]int32{4, 2}, nil)
		b.Field(2).(*array.Int32Builder).AppendValues([]int32{10, 5}, nil)
	})

	tbarFields := []arrow.Field{
		{Name: "x", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "y", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "z", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}
	for _, name := range transmitters {
		tbarFields = append(tbarFields, arrow.Field{Name: "nt_" + name + "_prob", Type: arrow.PrimitiveTypes.Float32, Nullable: true})
	}
	writeImportFeather(t, filepath.Join(dir, "tbar.feather"), arrow.NewSchema(tbarFields, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int32Builder).AppendValues([]int32{1, 2}, nil)
		b.Field(1).(*array.Int32Builder).AppendValues([]int32{1, 2}, nil)
		b.Field(2).(*array.Int32Builder).AppendValues([]int32{1, 2}, nil)
		b.Field(3).(*array.Int64Builder).AppendValues([]int64{1, 2}, nil)
		// Row 0 is acetylcholine 0.75, row 1 is gaba 0.75.
		values := [][]float32{{0.75, 0.0625}, {0.0625, 0.0625}, {0.0625, 0.75}, {0.0625, 0.0625}, {0.0625, 0.0625}, {0, 0}, {0, 0}}
		for i := range transmitters {
			b.Field(4+i).(*array.Float32Builder).AppendValues(values[i], nil)
		}
	})

	writeImportFeather(t, filepath.Join(dir, "syn-partners.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "x_pre", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "y_pre", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "z_pre", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "primary_post", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int32Builder).AppendValues([]int32{1, 2}, nil)
		b.Field(1).(*array.Int32Builder).AppendValues([]int32{1, 2}, nil)
		b.Field(2).(*array.Int32Builder).AppendValues([]int32{1, 2}, nil)
		b.Field(3).(*array.Int64Builder).AppendValues([]int64{1, 2}, nil)
		b.Field(4).(*array.Int64Builder).AppendValues([]int64{2, 1}, nil)
		b.Field(5).(*array.StringBuilder).AppendValues([]string{"AL", "MB"}, nil)
	})

	meta := "dataset:string,postHighAccuracyThreshold:float,preHPThreshold:float,postHPThreshold:float,roiHierarchy:string\n" +
		`fixture,0.5,0.0,0.7,"{""name"": ""CNS"", ""children"": [{""name"": ""AL""}, {""name"": ""MB""}]}"` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "meta.csv"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}

	sum := func(name string) string {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		return hex.EncodeToString(digest[:])
	}
	document := map[string]any{
		"schema_version": "coimnet-derivation-rules/v1",
		"sources": map[string]any{
			"body_stats":    map[string]any{"path": "body-stats.feather", "sha256": sum("body-stats.feather")},
			"tbar":          map[string]any{"path": "tbar.feather", "sha256": sum("tbar.feather")},
			"syn_partners":  map[string]any{"path": "syn-partners.feather", "sha256": sum("syn-partners.feather")},
			"neuprint_meta": map[string]any{"path": "meta.csv", "sha256": sum("meta.csv")},
		},
		"sign": map[string]any{
			"schema_version": "coimnet-sign-rule/v1",
			"mapping": map[string]any{
				"acetylcholine": "+1", "dopamine": "unknown", "gaba": "-1", "glutamate": "-1",
				"histamine": "-1", "octopamine": "unknown", "serotonin": "unknown",
			},
			"basis": map[string]any{
				"acetylcholine": "nAChR fast excitation",
				"dopamine":      "modulatory; kept unknown",
				"gaba":          "GABA-A chloride conductance",
				"glutamate":     "GluCl inhibition, an engineering assumption",
				"histamine":     "HisCl chloride conductance",
				"octopamine":    "modulatory; kept unknown",
				"serotonin":     "modulatory; kept unknown",
			},
			"min_probability":      0.5,
			"min_matched_fraction": 0.1,
		},
		"strength": map[string]any{"gain": 1.0, "normalizer": "post_total"},
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	rules = filepath.Join(dir, "rules.json")
	if err := os.WriteFile(rules, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, store, rules
}

type deriveOutput struct {
	SchemaVersion string `json:"schema_version"`
	Report        struct {
		SchemaVersion string `json:"schema_version"`
		RulesHash     string `json:"rules_hash"`
		Edges         struct {
			Edges     uint64 `json:"edges"`
			WithMatch uint64 `json:"edges_with_match"`
			Sign      struct {
				Positive uint64 `json:"positive"`
				Negative uint64 `json:"negative"`
				Unknown  uint64 `json:"unknown"`
			} `json:"sign"`
		} `json:"edges"`
		Sources []struct {
			Role   string `json:"role"`
			SHA256 string `json:"sha256"`
		} `json:"sources"`
	} `json:"report"`
	Set struct {
		Path      string `json:"path"`
		Bytes     int64  `json:"bytes"`
		SHA256    string `json:"sha256"`
		NodeCount uint64 `json:"node_count"`
		EdgeCount uint64 `json:"edge_count"`
	} `json:"set"`
}

func TestDataDeriveWritesAParameterSetAndReport(t *testing.T) {
	dir, store, rules := deriveFixture(t)
	out := filepath.Join(dir, "params.coimparams")
	var stdout, stderr bytes.Buffer
	args := []string{"data", "derive", "--store", store, "--rules", rules, "--out", out, "--temp-dir", t.TempDir()}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("data derive: %v; stderr=%s", err, stderr.String())
	}
	var derived deriveOutput
	if err := json.Unmarshal(stdout.Bytes(), &derived); err != nil {
		t.Fatalf("invalid derive JSON: %v\n%s", err, stdout.String())
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if derived.SchemaVersion != "coimnet-parameter-derive/v1" || derived.Set.Path != out || derived.Set.Bytes != info.Size() ||
		derived.Set.NodeCount != 2 || derived.Set.EdgeCount != 2 || len(derived.Set.SHA256) != 64 {
		t.Fatalf("derive output = %s", stdout.String())
	}
	// Edge 0 is 1->2 with an acetylcholine T-bar, edge 1 is 2->1 with a gaba one.
	if derived.Report.Edges.Edges != 2 || derived.Report.Edges.WithMatch != 2 ||
		derived.Report.Edges.Sign.Positive != 1 || derived.Report.Edges.Sign.Negative != 1 || derived.Report.Edges.Sign.Unknown != 0 {
		t.Fatalf("derive report edges = %+v", derived.Report.Edges)
	}
	if len(derived.Report.Sources) != 4 || len(derived.Report.RulesHash) != 64 {
		t.Fatalf("derive report identity = %+v", derived.Report)
	}

	// The output is never overwritten.
	stdout.Reset()
	if err := Run(context.Background(), args, &stdout, &stderr); err == nil {
		t.Fatal("a second derive overwrote the parameter set")
	}

	stdout.Reset()
	if err := Run(context.Background(), []string{"data", "validate", "--params", out, "--store", store}, &stdout, &stderr); err != nil {
		t.Fatalf("data validate --params: %v; stderr=%s", err, stderr.String())
	}
	var validated struct {
		SchemaVersion string `json:"schema_version"`
		Path          string `json:"path"`
		Bytes         int64  `json:"bytes"`
		SHA256        string `json:"sha256"`
		Nodes         uint64 `json:"nodes"`
		Edges         uint64 `json:"edges"`
		GraphChecked  bool   `json:"graph_checked"`
		Verified      bool   `json:"verified"`
		Report        struct {
			SchemaVersion string `json:"schema_version"`
			RulesHash     string `json:"rules_hash"`
		} `json:"report"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &validated); err != nil {
		t.Fatalf("invalid validate JSON: %v\n%s", err, stdout.String())
	}
	if validated.SchemaVersion != "coimnet-parameter-validate/v1" || validated.Path != out || validated.Bytes != info.Size() ||
		validated.SHA256 != derived.Set.SHA256 || validated.Nodes != 2 || validated.Edges != 2 ||
		!validated.GraphChecked || !validated.Verified || validated.Report.RulesHash != derived.Report.RulesHash {
		t.Fatalf("validate output = %s", stdout.String())
	}

	// Without --store the set is still verified, but no graph is checked.
	stdout.Reset()
	if err := Run(context.Background(), []string{"data", "validate", "--params", out}, &stdout, &stderr); err != nil {
		t.Fatalf("data validate --params without a store: %v", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &validated); err != nil {
		t.Fatal(err)
	}
	if validated.GraphChecked || !validated.Verified {
		t.Fatalf("validate without --store = %s", stdout.String())
	}
}

func TestDataDeriveAndValidateFailuresAndHelp(t *testing.T) {
	dir, store, rules := deriveFixture(t)
	out := filepath.Join(dir, "params.coimparams")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"data", "derive", "--store", store, "--rules", rules, "--out", out, "--temp-dir", t.TempDir()}, &stdout, &stderr); err != nil {
		t.Fatalf("data derive: %v; stderr=%s", err, stderr.String())
	}
	other := filepath.Join(dir, "other.coimgraph")
	if err := os.WriteFile(other, []byte("not a store"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"data", "derive"},
		{"data", "derive", "--store", store},
		{"data", "derive", "--store", store, "--rules", rules},
		{"data", "derive", "--store", store, "--rules", rules, "--out", out}, // exists
		{"data", "derive", "--store", store, "--rules", filepath.Join(dir, "absent.json"), "--out", out + "2"},
		{"data", "derive", "--store", filepath.Join(dir, "absent.coimgraph"), "--rules", rules, "--out", out + "3"},
		{"data", "derive", "--store", store, "--rules", rules, "--out", out + "4", "--max-memory-bytes", "0"},
		{"data", "derive", "--store", store, "--rules", rules, "--out", out + "5", "--max-rows", "1"},
		{"data", "derive", "--store", store, "--rules", rules, "--out", out + "6", "--temp-dir", filepath.Join(dir, "absent")},
		{"data", "derive", "--store", store, "--rules", rules, "--out", out + "7", "extra"},
		{"data", "derive", "--unknown"},
		{"data", "validate"},
		{"data", "validate", "--params", filepath.Join(dir, "absent.coimparams")},
		{"data", "validate", "--params", out, "--store", other},
		{"data", "validate", "--params", out, "--max-bytes", "10"},
		{"data", "validate", "--params", out, "--store", store, "extra"},
	} {
		var out, errout bytes.Buffer
		if err := Run(context.Background(), args, &out, &errout); err == nil {
			t.Errorf("accepted %v", args)
		}
	}

	stdout.Reset()
	if err := Run(context.Background(), []string{"data", "derive", "--help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"--store", "--rules", "--out", "temp-dir", "max-memory-bytes", "max-temp-bytes", "max-run-files", "max-arrow-bytes", "max-rows", "Example:", "Errors:", "Options:"} {
		if !strings.Contains(stdout.String(), word) {
			t.Errorf("derive help missing %s", word)
		}
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"data", "validate", "--help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "--params") {
		t.Error("validate help does not mention --params")
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "data derive") {
		t.Error("overview missing data derive")
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"data", "--help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "derive") {
		t.Error("data usage missing derive")
	}
}
