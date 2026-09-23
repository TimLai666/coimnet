package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type bundleTestValue struct {
	Name   string         `json:"name"`
	Values []float64      `json:"values"`
	Rows   [][]float64    `json:"rows"`
	Counts map[string]int `json:"counts"`
}

func bundleTestManifest() BundleManifest {
	return BundleManifest{Kind: BundleIndividual, DocumentSchema: "test-document/v1", Topology: TopologyFingerprint{Nodes: 2, Edges: 1, SHA256: "topology"}}
}

func bundleTestValueFixture() bundleTestValue {
	return bundleTestValue{Name: "snapshot", Values: []float64{1.25, -2.5}, Rows: [][]float64{{1, 2}, {3}}, Counts: map[string]int{"a": 3, "b": 5}}
}

func writeTestBundle(t *testing.T, dir string) {
	t.Helper()
	if err := writeBundle(context.Background(), dir, bundleTestManifest(), bundleTestValueFixture()); err != nil {
		t.Fatal(err)
	}
}

func TestBundleRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshot")
	want := bundleTestValueFixture()
	if err := writeBundle(context.Background(), dir, bundleTestManifest(), want); err != nil {
		t.Fatal(err)
	}
	var got bundleTestValue
	manifest, err := readBundle(context.Background(), dir, BundleIndividual, &got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", got, want)
	}
	for name, file := range map[string]BundleFile{"document.json": manifest.Document, "arrays.bin": manifest.Arrays} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if file.Name != name || file.Bytes != int64(len(data)) || file.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("manifest file %q = %#v, actual bytes=%d sha256=%x", name, file, len(data), sum)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].Name() != "arrays.bin" || entries[1].Name() != "document.json" || entries[2].Name() != bundleManifestFile {
		t.Fatalf("bundle directory entries = %#v, want arrays.bin, document.json, manifest.json", entries)
	}
}

func TestBundleKeepsPointersToZero(t *testing.T) {
	zeroInt, zeroFloat, zeroBool := 0, float64(0), false
	want := struct {
		Int   *int     `json:"int"`
		Float *float64 `json:"float"`
		Bool  *bool    `json:"bool"`
		Nil   *int     `json:"nil"`
	}{&zeroInt, &zeroFloat, &zeroBool, nil}
	dir := filepath.Join(t.TempDir(), "snapshot")
	if err := writeBundle(context.Background(), dir, bundleTestManifest(), want); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Int   *int     `json:"int"`
		Float *float64 `json:"float"`
		Bool  *bool    `json:"bool"`
		Nil   *int     `json:"nil"`
	}
	if _, err := readBundle(context.Background(), dir, BundleIndividual, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Int == nil || *decoded.Int != 0 || decoded.Float == nil || *decoded.Float != 0 || decoded.Bool == nil || *decoded.Bool || decoded.Nil != nil {
		t.Fatalf("zero pointer round trip = %#v", decoded)
	}
}

type bundleNestedArrays struct {
	Samples []float32 `json:"samples"`
}

type bundleArrayFixture struct {
	Small  []float64            `json:"small"`
	Large  []float64            `json:"large"`
	Nested bundleNestedArrays   `json:"nested"`
	Ptr    *bundleNestedArrays  `json:"ptr"`
	Rows   [][]float64          `json:"rows"`
	Items  []bundleNestedArrays `json:"items"`
}

func TestBundleExtractsOnlyLargeArrays(t *testing.T) {
	value := bundleArrayFixture{
		Small: make([]float64, bundleArrayThreshold-1), Large: make([]float64, bundleArrayThreshold),
		Nested: bundleNestedArrays{Samples: make([]float32, bundleArrayThreshold)}, Ptr: &bundleNestedArrays{Samples: make([]float32, bundleArrayThreshold)},
		Rows: [][]float64{make([]float64, bundleArrayThreshold)}, Items: []bundleNestedArrays{{Samples: make([]float32, bundleArrayThreshold)}},
	}
	for i := range value.Large {
		value.Large[i] = float64(i) / 7
	}
	for i := range value.Nested.Samples {
		value.Nested.Samples[i] = float32(i) / 9
	}
	for i := range value.Ptr.Samples {
		value.Ptr.Samples[i] = float32(i) / 11
	}
	want := deepCopyBundleArrayFixture(value)
	dir := filepath.Join(t.TempDir(), "arrays")
	if err := writeBundle(context.Background(), dir, bundleTestManifest(), value); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(value, want) {
		t.Fatal("writeBundle mutated its input, including the pointed-to struct")
	}
	manifest, err := ReadBundleManifest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, entry := range manifest.Entries {
		paths[entry.Path] = true
	}
	for _, path := range []string{"large", "nested.samples", "ptr.samples"} {
		if !paths[path] {
			t.Errorf("manifest is missing array %q", path)
		}
	}
	if len(paths) != 3 {
		t.Fatalf("extracted paths = %#v", paths)
	}
	var got bundleArrayFixture
	if _, err := readBundle(context.Background(), dir, BundleIndividual, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, value) {
		t.Fatal("extracted arrays did not round trip")
	}
	document, err := os.ReadFile(filepath.Join(dir, "document.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(document), "rows") == false || strings.Contains(string(document), "items") == false {
		t.Fatalf("document omitted non-extracted composite fields: %s", document)
	}
}

type bundleFloat64s []float64
type bundleFloat32s []float32
type bundleInts []int
type bundleInt32s []int32
type bundleInt8s []int8
type bundleUint64s []uint64
type bundleUint32s []uint32
type bundleUint8s []uint8
type bundleBools []bool

func TestBundleArrayPrimitiveKinds(t *testing.T) {
	want := struct {
		Float64 bundleFloat64s `json:"float64"`
		Float32 bundleFloat32s `json:"float32"`
		Int     bundleInts     `json:"int"`
		Int32   bundleInt32s   `json:"int32"`
		Int8    bundleInt8s    `json:"int8"`
		Uint64  bundleUint64s  `json:"uint64"`
		Uint32  bundleUint32s  `json:"uint32"`
		Uint8   bundleUint8s   `json:"uint8"`
		Bool    bundleBools    `json:"bool"`
	}{
		Float64: make(bundleFloat64s, bundleArrayThreshold), Float32: make(bundleFloat32s, bundleArrayThreshold),
		Int: make(bundleInts, bundleArrayThreshold), Int32: make(bundleInt32s, bundleArrayThreshold),
		Int8: make(bundleInt8s, bundleArrayThreshold), Uint64: make(bundleUint64s, bundleArrayThreshold),
		Uint32: make(bundleUint32s, bundleArrayThreshold), Uint8: make(bundleUint8s, bundleArrayThreshold),
		Bool: make(bundleBools, bundleArrayThreshold),
	}
	want.Float64[0], want.Float32[0], want.Int[0], want.Int32[0], want.Int8[0] = -1.25, 1.5, -3, -4, -5
	want.Uint64[0], want.Uint32[0], want.Uint8[0], want.Bool[0] = 6, 7, 8, true
	want.Bool[1] = true
	dir := filepath.Join(t.TempDir(), "primitive-arrays")
	if err := writeBundle(context.Background(), dir, bundleTestManifest(), want); err != nil {
		t.Fatal(err)
	}
	manifest, err := ReadBundleManifest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) != 9 {
		t.Fatalf("array entry count = %d, want 9", len(manifest.Entries))
	}
	wantTypes := map[string]string{"float64": "float64", "float32": "float32", "int": "int64", "int32": "int32", "int8": "int8", "uint64": "uint64", "uint32": "uint32", "uint8": "uint8", "bool": "bool"}
	for _, entry := range manifest.Entries {
		if entry.Length != bundleArrayThreshold || wantTypes[entry.Path] != entry.DType {
			t.Fatalf("invalid extracted primitive array: %#v", entry)
		}
	}
	arrays, err := os.ReadFile(filepath.Join(dir, "arrays.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint64(arrays[:8]); got != 0xbff4000000000000 {
		t.Fatalf("first float64 bytes decode to %#x, want %#x", got, uint64(0xbff4000000000000))
	}
	var decoded struct {
		Float64 bundleFloat64s `json:"float64"`
		Float32 bundleFloat32s `json:"float32"`
		Int     bundleInts     `json:"int"`
		Int32   bundleInt32s   `json:"int32"`
		Int8    bundleInt8s    `json:"int8"`
		Uint64  bundleUint64s  `json:"uint64"`
		Uint32  bundleUint32s  `json:"uint32"`
		Uint8   bundleUint8s   `json:"uint8"`
		Bool    bundleBools    `json:"bool"`
	}
	if _, err := readBundle(context.Background(), dir, BundleIndividual, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatal("primitive array values changed across bundle round trip")
	}
}

func deepCopyBundleArrayFixture(value bundleArrayFixture) bundleArrayFixture {
	copy := value
	copy.Small = append([]float64(nil), value.Small...)
	copy.Large = append([]float64(nil), value.Large...)
	copy.Nested.Samples = append([]float32(nil), value.Nested.Samples...)
	ptr := *value.Ptr
	ptr.Samples = append([]float32(nil), value.Ptr.Samples...)
	copy.Ptr = &ptr
	copy.Rows = [][]float64{append([]float64(nil), value.Rows[0]...)}
	copy.Items = append([]bundleNestedArrays(nil), value.Items...)
	copy.Items[0].Samples = append([]float32(nil), value.Items[0].Samples...)
	return copy
}

func TestBundleRefusesExistingDir(t *testing.T) {
	for _, existingFile := range []bool{false, true} {
		name := "directory"
		if existingFile {
			name = "file"
		}
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "existing")
			if existingFile {
				if err := os.WriteFile(dir, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			err := writeBundle(context.Background(), dir, bundleTestManifest(), bundleTestValueFixture())
			if err == nil || !strings.Contains(err.Error(), dir) {
				t.Fatalf("writeBundle error = %v, want error naming %q", err, dir)
			}
			if existingFile {
				data, readErr := os.ReadFile(dir)
				if readErr != nil || string(data) != "keep" {
					t.Fatalf("existing file changed: %q, %v", data, readErr)
				}
			} else {
				entries, readErr := os.ReadDir(dir)
				if readErr != nil || len(entries) != 0 {
					t.Fatalf("existing directory changed: %#v, %v", entries, readErr)
				}
			}
		})
	}
	missingParent := filepath.Join(t.TempDir(), "missing", "bundle")
	if err := writeBundle(context.Background(), missingParent, bundleTestManifest(), bundleTestValueFixture()); err == nil {
		t.Fatal("writeBundle succeeded with a missing parent")
	}
}

func TestBundleIncompleteWithoutManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshot")
	writeTestBundle(t, dir)
	if err := os.Remove(filepath.Join(dir, bundleManifestFile)); err != nil {
		t.Fatal(err)
	}
	var got bundleTestValue
	if _, err := readBundle(context.Background(), dir, BundleIndividual, &got); err == nil || !strings.Contains(err.Error(), "incomplete bundle") {
		t.Fatalf("readBundle error = %v, want incomplete bundle", err)
	}
}

func TestBundleDetectsTampering(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		want   string
	}{
		{name: "arrays file byte", mutate: func(t *testing.T, dir string) {
			path := filepath.Join(dir, "arrays.bin")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data[0] ^= 1
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}, want: "arrays.bin checksum mismatch"},
		{name: "array entry byte", mutate: func(t *testing.T, dir string) {
			path := filepath.Join(dir, "arrays.bin")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data[0] ^= 1
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			mutateBundleManifest(t, dir, func(fields map[string]any) {
				arrays := fields["arrays"].(map[string]any)
				arrays["sha256"] = hex.EncodeToString(sum[:])
			})
		}, want: "entry \"large\" checksum mismatch"},
		{name: "document byte", mutate: func(t *testing.T, dir string) {
			path := filepath.Join(dir, "document.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data[0] ^= 1
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}, want: "document checksum mismatch"},
		{name: "short arrays file", mutate: func(t *testing.T, dir string) {
			path := filepath.Join(dir, "arrays.bin")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data[:len(data)-1], 0o600); err != nil {
				t.Fatal(err)
			}
		}, want: "size mismatch"},
		{name: "unknown manifest field", mutate: func(t *testing.T, dir string) {
			mutateBundleManifest(t, dir, func(fields map[string]any) { fields["unexpected"] = true })
		}, want: "unknown field"},
		{name: "kind mismatch", mutate: func(t *testing.T, dir string) {
			mutateBundleManifest(t, dir, func(fields map[string]any) { fields["kind"] = BundleTraining })
		}, want: "kind"},
		{name: "schema mismatch", mutate: func(t *testing.T, dir string) {
			mutateBundleManifest(t, dir, func(fields map[string]any) { fields["schema_version"] = "other/v1" })
		}, want: "schema"},
		{name: "document name traversal", mutate: func(t *testing.T, dir string) {
			mutateBundleManifest(t, dir, func(fields map[string]any) { fields["document"].(map[string]any)["name"] = "../x" })
		}, want: "file names"},
		{name: "discontinuous offset", mutate: func(t *testing.T, dir string) {
			mutateBundleManifest(t, dir, func(fields map[string]any) { fields["entries"].([]any)[0].(map[string]any)["offset"] = float64(1) })
		}, want: "offset"},
		{name: "unknown dtype", mutate: func(t *testing.T, dir string) {
			mutateBundleManifest(t, dir, func(fields map[string]any) { fields["entries"].([]any)[0].(map[string]any)["dtype"] = "wat" })
		}, want: "dtype"},
		{name: "duplicate path", mutate: func(t *testing.T, dir string) {
			mutateBundleManifest(t, dir, func(fields map[string]any) {
				entries := fields["entries"].([]any)
				duplicate := make(map[string]any)
				for key, value := range entries[0].(map[string]any) {
					duplicate[key] = value
				}
				fields["entries"] = append(entries, duplicate)
			})
		}, want: "duplicate"},
		{name: "missing path", mutate: func(t *testing.T, dir string) {
			mutateBundleManifest(t, dir, func(fields map[string]any) { fields["entries"].([]any)[0].(map[string]any)["path"] = "missing" })
		}, want: "path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "snapshot")
			value := struct {
				Large []float64 `json:"large"`
			}{Large: make([]float64, bundleArrayThreshold)}
			if err := writeBundle(context.Background(), dir, bundleTestManifest(), value); err != nil {
				t.Fatal(err)
			}
			tt.mutate(t, dir)
			var got struct {
				Large []float64 `json:"large"`
			}
			if _, err := readBundle(context.Background(), dir, BundleIndividual, &got); err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.want)) {
				t.Fatalf("readBundle error = %v, want text %q", err, tt.want)
			}
		})
	}
}

func mutateBundleManifest(t *testing.T, dir string, mutate func(map[string]any)) {
	t.Helper()
	path := filepath.Join(dir, bundleManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	mutate(fields)
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBundleWriteFailureLeavesNothing(t *testing.T) {
	badDir := filepath.Join(t.TempDir(), "bad-value")
	badValue := struct {
		Channel chan int `json:"channel"`
	}{Channel: make(chan int)}
	if err := writeBundle(context.Background(), badDir, bundleTestManifest(), badValue); err == nil {
		t.Fatal("writeBundle succeeded with an unencodable value")
	}
	if _, err := os.Stat(badDir); !os.IsNotExist(err) {
		t.Fatalf("failed bundle directory exists: %v", err)
	}
	canceledDir := filepath.Join(t.TempDir(), "canceled")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := writeBundle(ctx, canceledDir, bundleTestManifest(), bundleTestValueFixture()); err == nil {
		t.Fatal("writeBundle succeeded with a canceled context")
	}
	if _, err := os.Stat(canceledDir); !os.IsNotExist(err) {
		t.Fatalf("canceled bundle directory exists: %v", err)
	}
}

func TestBundleRejectsBadManifestInput(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*BundleManifest)
	}{{"unknown kind", func(m *BundleManifest) { m.Kind = "other" }}, {"empty document schema", func(m *BundleManifest) { m.DocumentSchema = "" }}} {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "invalid")
			manifest := bundleTestManifest()
			tt.change(&manifest)
			if err := writeBundle(context.Background(), dir, manifest, bundleTestValueFixture()); err == nil {
				t.Fatal("writeBundle accepted invalid manifest input")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("invalid manifest created bundle directory: %v", err)
			}
		})
	}
}
