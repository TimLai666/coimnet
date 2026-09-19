package flywire_test

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/connectome/flywire"
)

const (
	fixtureVersion   = "783"
	fixtureAcquired  = "2026-09-19T00:00:00Z"
	fixtureLicense   = "https://codex.flywire.ai/app/graph/flywire_public_release"
	rootIDLargeShift = "9007199254740993"
	rootIDMaxInt64   = "9223372036854775807"
	rootIDThird      = "720575940621039145"
	rootIDOverflow   = "9223372036854775808"
)

const fixtureConnections = `pre_root_id,post_root_id,neuropil,syn_count,nt_type
9007199254740993,9223372036854775807,AL,3,acetylcholine
9007199254740993,9223372036854775807,PB,4,acetylcholine
9223372036854775807,720575940621039145,AL,2,gaba
`

const fixtureClassification = `root_id,flow,super_class,class,sub_class,cell_type,hemibrain_type,hemilineage,side,nerve
9007199254740993,efferent,cb_intrinsic,ALIN_a,ALIN_a.1,combining_cell,,,L,
9223372036854775807,interneuron,cb_intrinsic,PBON1_b,PBON1_b.1,combining_cell,,,R,
720575940621039145,efferent,optic_lobe,Lamina,Lamina.1,lamina_monopolar_cell,,,L,
`

const fixtureNeurons = `root_id,group,nt_type,nt_type_score
9007199254740993,groupA,acetylcholine,0.88
9223372036854775807,groupB,gaba,0.70
720575940621039145,groupC,glutamate,0.92
`

func fixtureInputs(t *testing.T, write func(t *testing.T, name, content string) string) flywire.Inputs {
	t.Helper()
	return flywire.Inputs{
		Connections:    write(t, "connections.csv", fixtureConnections),
		Classification: write(t, "classification.csv", fixtureClassification),
		Neurons:        write(t, "neurons.csv", fixtureNeurons),
		Version:        fixtureVersion,
		AcquiredAt:     fixtureAcquired,
		LicenseURL:     fixtureLicense,
	}
}

func writeText(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeGzipped(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name+".gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	if _, err := io.WriteString(gz, content); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func fixtureLimits() connectome.ResourceLimits {
	return connectome.ResourceLimits{
		MaxMemoryBytes: 256 << 20,
		MaxTempBytes:   1 << 30,
		MaxRunFiles:    1024,
		MaxArrowBytes:  64 << 20,
		MaxFooterBytes: 1 << 20,
		MaxRows:        1 << 23,
	}
}

func TestConvertFixtureRoundTripsThroughBuild(t *testing.T) {
	inputs := fixtureInputs(t, writeText)
	outDir := filepath.Join(t.TempDir(), "out")
	result, err := flywire.Convert(context.Background(), inputs, outDir)
	if err != nil {
		t.Fatalf("flywire.Convert: %v", err)
	}
	if result.Rows.Connections != 3 || result.Rows.Classification != 3 || result.Rows.Neurons != 3 {
		t.Fatalf("Rows = %+v, want Connections/Classification/Neurons = 3", result.Rows)
	}
	if result.ManifestPath != filepath.Join(outDir, "manifest.json") {
		t.Fatalf("ManifestPath %q, want %q", result.ManifestPath, filepath.Join(outDir, "manifest.json"))
	}
	if _, err := os.Stat(result.ManifestPath); err != nil {
		t.Fatalf("manifest.json was not written: %v", err)
	}
	if result.Manifest.Namespace != "flywire-"+fixtureVersion {
		t.Fatalf("Namespace = %q, want flywire-783", result.Manifest.Namespace)
	}
	if len(result.Manifest.Files) != 3 {
		t.Fatalf("Files = %d entries, want 3", len(result.Manifest.Files))
	}

	built, err := connectome.Build(context.Background(), connectome.BuildRequest{
		Manifest: result.Manifest,
		Limits:   fixtureLimits(),
		TempDir:  t.TempDir(),
		EdgeView: connectome.EdgeViewRows,
	})
	if err != nil {
		t.Fatalf("connectome.Build on converted fixture: %v", err)
	}
	if got := built.Graph.NodeCount(); got != 3 {
		t.Fatalf("NodeCount = %d, want 3", got)
	}
	seen := make(map[string]bool)
	for _, id := range built.Graph.NeuronIDs() {
		if id.Namespace != "flywire-"+fixtureVersion {
			t.Errorf("neuron namespace = %q, want flywire-783", id.Namespace)
		}
		seen[id.ExternalID] = true
	}
	for _, want := range []string{rootIDLargeShift, rootIDMaxInt64, rootIDThird} {
		if !seen[want] {
			t.Errorf("NeuronIDs do not contain the exact external id %q: %v", want, seen)
		}
	}
	// EdgeViewRows keeps every weight row as its own edge: two rows for the
	// (pre, post) pair split across neuropils plus one row for the third pair.
	if got := built.Graph.EdgeCount(); got != 3 {
		t.Fatalf("EdgeCount = %d, want 3 (rows mode keeps each source row)", got)
	}
}

func TestConvertRefusesOverflow(t *testing.T) {
	inputs := flywire.Inputs{
		Connections: writeText(t, "connections.csv", fixtureConnections),
		Classification: writeText(t, "classification.csv",
			"root_id,flow,super_class,class,sub_class,cell_type,side\n"+rootIDOverflow+",efferent,cb_intrinsic,ALIN_a,ALIN_a.1,combining_cell,L\n"),
		Neurons:    writeText(t, "neurons.csv", fixtureNeurons),
		Version:    fixtureVersion,
		AcquiredAt: fixtureAcquired,
		LicenseURL: fixtureLicense,
	}
	outDir := filepath.Join(t.TempDir(), "out")
	if _, err := flywire.Convert(context.Background(), inputs, outDir); err == nil {
		t.Fatal("flywire.Convert should refuse a root id above int64")
	} else if !strings.Contains(err.Error(), "overflows") {
		t.Fatalf("error = %q, want it to mention overflows", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("manifest.json must not exist after an overflow error, stat err = %v", err)
	}
}

func TestConvertRefusesMissingColumn(t *testing.T) {
	inputs := flywire.Inputs{
		Connections: writeText(t, "connections.csv",
			"pre_root_id,post_root_id,neuropil,nt_type\n"+rootIDLargeShift+","+rootIDMaxInt64+",AL,acetylcholine\n"),
		Classification: writeText(t, "classification.csv", fixtureClassification),
		Neurons:        writeText(t, "neurons.csv", fixtureNeurons),
		Version:        fixtureVersion,
		AcquiredAt:     fixtureAcquired,
		LicenseURL:     fixtureLicense,
	}
	if _, err := flywire.Convert(context.Background(), inputs, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("flywire.Convert should refuse a connections table without syn_count")
	} else if !strings.Contains(err.Error(), "syn_count") || !strings.Contains(err.Error(), "connections") {
		t.Fatalf("error = %q, want it to name both the column and the table", err)
	}
}

func TestConvertReadsGzip(t *testing.T) {
	inputs := fixtureInputs(t, writeGzipped)
	result, err := flywire.Convert(context.Background(), inputs, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatalf("flywire.Convert on gzipped tables: %v", err)
	}
	if result.Rows.Connections != 3 || result.Rows.Classification != 3 || result.Rows.Neurons != 3 {
		t.Fatalf("Rows = %+v, want the same row counts as the plain fixture", result.Rows)
	}
	if result.Manifest.Namespace != "flywire-"+fixtureVersion {
		t.Fatalf("Namespace = %q, want flywire-783", result.Manifest.Namespace)
	}
}

func TestManifestIdentity(t *testing.T) {
	inputs := fixtureInputs(t, writeText)
	result, err := flywire.Convert(context.Background(), inputs, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatalf("flywire.Convert: %v", err)
	}
	if result.Manifest.Dataset != "flywire" {
		t.Errorf("Dataset = %q, want flywire", result.Manifest.Dataset)
	}
	if result.Manifest.Namespace != "flywire-783" {
		t.Errorf("Namespace = %q, want flywire-783", result.Manifest.Namespace)
	}
	if got := result.Manifest.Selection.Field; got != "included" {
		t.Errorf("Selection.Field = %q, want included", got)
	}
	if got := result.Manifest.DuplicateSemantics; got != connectome.DuplicateSemanticsAdditivePartitions {
		t.Errorf("DuplicateSemantics = %q, want additive_partitions", got)
	}
	if got := result.Manifest.TransformHistory; len(got) != 1 || got[0].Step != "flywire-csv-to-feather" {
		t.Errorf("TransformHistory = %+v, want one flywire-csv-to-feather step", got)
	}

	bad := inputs
	bad.Version = "783/1"
	if _, err := flywire.Convert(context.Background(), bad, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("flywire.Convert should refuse a release version containing '/'")
	}
}
