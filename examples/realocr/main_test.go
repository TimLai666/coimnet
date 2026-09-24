package main

import (
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestReadDatasetRejectsTruncatedIDXImage(t *testing.T) {
	dir := t.TempDir()
	writeMetadata(t, dir)
	writeGzip(t, filepath.Join(dir, trainLabelsName), labelBytes([]byte{0}))
	writeGzip(t, filepath.Join(dir, trainImagesName), imageBytes(1, 28, 28)[:len(imageBytes(1, 28, 28))-1])
	writeGzip(t, filepath.Join(dir, testLabelsName), labelBytes([]byte{0}))
	writeGzip(t, filepath.Join(dir, testImagesName), imageBytes(1, 28, 28))

	_, err := LoadDataset(dir, DatasetLimits{TrainCount: 1, TestCount: 1})
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("LoadDataset error = %v, want truncated error", err)
	}
}

func TestReadDatasetRejectsWrongImageShape(t *testing.T) {
	dir := t.TempDir()
	writeMetadata(t, dir)
	writeGzip(t, filepath.Join(dir, trainLabelsName), labelBytes([]byte{0}))
	writeGzip(t, filepath.Join(dir, trainImagesName), imageBytes(1, 27, 28))
	writeGzip(t, filepath.Join(dir, testLabelsName), labelBytes([]byte{0}))
	writeGzip(t, filepath.Join(dir, testImagesName), imageBytes(1, 28, 28))

	_, err := LoadDataset(dir, DatasetLimits{TrainCount: 1, TestCount: 1})
	if err == nil || !strings.Contains(err.Error(), "28x28") {
		t.Fatalf("LoadDataset error = %v, want shape error", err)
	}
}

func TestReadDatasetRejectsOutOfRangeLabel(t *testing.T) {
	dir := t.TempDir()
	writeMetadata(t, dir)
	writeGzip(t, filepath.Join(dir, trainLabelsName), labelBytes([]byte{10}))
	writeGzip(t, filepath.Join(dir, trainImagesName), imageBytes(1, 28, 28))
	writeGzip(t, filepath.Join(dir, testLabelsName), labelBytes([]byte{0}))
	writeGzip(t, filepath.Join(dir, testImagesName), imageBytes(1, 28, 28))

	_, err := LoadDataset(dir, DatasetLimits{TrainCount: 1, TestCount: 1})
	if err == nil || !strings.Contains(err.Error(), "label 10") {
		t.Fatalf("LoadDataset error = %v, want label error", err)
	}
}

func TestReadDatasetRequiresCCBYSAAndOfficialURLs(t *testing.T) {
	dir := t.TempDir()
	meta := validMetadata()
	meta.License.Name = "unknown"
	writeJSON(t, filepath.Join(dir, metadataName), meta)
	writeOfficialFixture(t, dir)

	_, err := LoadDataset(dir, DatasetLimits{TrainCount: 1, TestCount: 1})
	if err == nil || !strings.Contains(err.Error(), "CC BY-SA 4.0") {
		t.Fatalf("LoadDataset error = %v, want license error", err)
	}
}

func TestLoadDatasetKeepsTrainAndTestIDXSourcesSeparate(t *testing.T) {
	dir := t.TempDir()
	writeMetadata(t, dir)
	writeOfficialFixture(t, dir)

	dataset, err := LoadDataset(dir, DatasetLimits{TrainCount: 1, TestCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if dataset.TrainTotal != 1 || dataset.TestTotal != 1 {
		t.Fatalf("full split counts = %d/%d, want 1/1", dataset.TrainTotal, dataset.TestTotal)
	}
	if len(dataset.Train) != 1 || len(dataset.Test) != 1 {
		t.Fatalf("selected split lengths = %d/%d, want 1/1", len(dataset.Train), len(dataset.Test))
	}
	if dataset.FileFingerprints[0].Name != trainImagesName || dataset.FileFingerprints[2].Name != testImagesName {
		t.Fatalf("source order does not preserve separate train/test image files: %#v", dataset.FileFingerprints)
	}
}

func TestBalancedIndicesAreStableAndLabelRoundRobin(t *testing.T) {
	labels := []int{0, 0, 0, 1, 1, 1, 2}
	got, err := balancedIndices(labels, 6, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{0, 3, 6, 1, 4, 2}
	if strings.Join(intStrings(got), ",") != strings.Join(intStrings(want), ",") {
		t.Fatalf("balanced indices = %v, want %v", got, want)
	}
}

func TestReportJSONIsStableAndExcludesRuntimeTiming(t *testing.T) {
	report := Report{
		SchemaVersion: "coimnet-realocr/v1",
		Config:        RunConfig{Seed: 7, TrainCount: 2, TestCount: 2, Epochs: 1},
		Before:        EvalResult{CER: 1, Exact: 0},
		After:         EvalResult{CER: 0.5, Exact: 0.5},
	}
	one, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	two, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(one) != string(two) {
		t.Fatal("report JSON changed between identical serializations")
	}
	if strings.Contains(string(one), "duration") || strings.Contains(string(one), "elapsed") {
		t.Fatalf("deterministic report unexpectedly contains runtime timing: %s", one)
	}
}

func validMetadata() Metadata {
	return Metadata{
		SchemaVersion: "coimnet-realocr-data/v1",
		Dataset:       "Kuzushiji-MNIST",
		RepositoryURL: "https://github.com/rois-codh/kmnist",
		DatasetURL:    "https://codh.rois.ac.jp/kmnist/",
		License: LicenseMetadata{
			Name:        "CC BY-SA 4.0",
			URL:         "https://creativecommons.org/licenses/by-sa/4.0/",
			Attribution: "KMNIST Dataset (created by CODH), adapted from Kuzushiji Dataset (created by NIJL and others), doi:10.20676/00000341",
		},
		Files: []FileMetadata{
			{Name: trainImagesName, URL: officialBaseURL + trainImagesName},
			{Name: trainLabelsName, URL: officialBaseURL + trainLabelsName},
			{Name: testImagesName, URL: officialBaseURL + testImagesName},
			{Name: testLabelsName, URL: officialBaseURL + testLabelsName},
			{Name: classMapName, URL: officialBaseURL + classMapName},
		},
		Mapping: []ClassMapping{
			{Index: 0, Codepoint: "U+304A", Char: "お"},
			{Index: 1, Codepoint: "U+304D", Char: "き"},
			{Index: 2, Codepoint: "U+3059", Char: "す"},
			{Index: 3, Codepoint: "U+3064", Char: "つ"},
			{Index: 4, Codepoint: "U+306A", Char: "な"},
			{Index: 5, Codepoint: "U+306F", Char: "は"},
			{Index: 6, Codepoint: "U+307E", Char: "ま"},
			{Index: 7, Codepoint: "U+3084", Char: "や"},
			{Index: 8, Codepoint: "U+308C", Char: "れ"},
			{Index: 9, Codepoint: "U+3092", Char: "を"},
		},
	}
}

func writeMetadata(t *testing.T, dir string) {
	t.Helper()
	meta := validMetadata()
	writeJSON(t, filepath.Join(dir, metadataName), meta)
	writeFile(t, filepath.Join(dir, classMapName), []byte("index,codepoint,char\n0,U+304A,お\n1,U+304D,き\n2,U+3059,す\n3,U+3064,つ\n4,U+306A,な\n5,U+306F,は\n6,U+307E,ま\n7,U+3084,や\n8,U+308C,れ\n9,U+3092,を\n"))
}

func writeOfficialFixture(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, classMapName), []byte("index,codepoint,char\n0,U+304A,お\n1,U+304D,き\n2,U+3059,す\n3,U+3064,つ\n4,U+306A,な\n5,U+306F,は\n6,U+307E,ま\n7,U+3084,や\n8,U+308C,れ\n9,U+3092,を\n"))
	writeGzip(t, filepath.Join(dir, trainLabelsName), labelBytes([]byte{0}))
	writeGzip(t, filepath.Join(dir, trainImagesName), imageBytes(1, 28, 28))
	writeGzip(t, filepath.Join(dir, testLabelsName), labelBytes([]byte{0}))
	writeGzip(t, filepath.Join(dir, testImagesName), imageBytes(1, 28, 28))
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, b)
}

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGzip(t *testing.T, path string, b []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	if _, err := gz.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func imageBytes(count, rows, cols int) []byte {
	b := make([]byte, 16+count*rows*cols)
	binary.BigEndian.PutUint32(b[0:4], 2051)
	binary.BigEndian.PutUint32(b[4:8], uint32(count))
	binary.BigEndian.PutUint32(b[8:12], uint32(rows))
	binary.BigEndian.PutUint32(b[12:16], uint32(cols))
	for i := 16; i < len(b); i++ {
		b[i] = byte(i)
	}
	return b
}

func labelBytes(labels []byte) []byte {
	b := make([]byte, 8+len(labels))
	binary.BigEndian.PutUint32(b[0:4], 2049)
	binary.BigEndian.PutUint32(b[4:8], uint32(len(labels)))
	copy(b[8:], labels)
	return b
}

func intStrings(values []int) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strconv.Itoa(value)
	}
	return out
}

var _ = io.EOF
