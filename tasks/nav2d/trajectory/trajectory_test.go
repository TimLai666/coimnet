package trajectory

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const fixtureHeader = "fname,fly,condition,segment,t,x_cm,y_cm,reward,dist_fictive_reward_cm"

func fixtureSource(raw []byte) Source {
	sum := sha256.Sum256(raw)
	return Source{
		ID:           "titova-2023-fixture",
		URL:          "https://github.com/example/repo/blob/0eb07940a9ffdedd97ada81f75e9d5f7bface579/data.csv.gz",
		DOI:          "10.5061/dryad.vdncjsz0b",
		PinnedCommit: "0eb07940a9ffdedd97ada81f75e9d5f7bface579",
		SHA256:       hex.EncodeToString(sum[:]),
		License: License{
			Holder: "Titova et al.",
			Terms:  "CC0 1.0",
			Source: "https://doi.org/10.5061/dryad.vdncjsz0b",
		},
	}
}

func writeCSV(t *testing.T, name string, raw []byte, gzipFile bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if gzipFile {
		zw := gzip.NewWriter(f)
		if _, err := zw.Write(raw); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	} else if _, err := f.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func validCSV() []byte {
	return []byte(fixtureHeader + "\n" +
		"trial-a.csv,0,rewarded,baseline,0,0,0,99,12\n" +
		"trial-a.csv,0,rewarded,baseline,0.2,1,2,98,11\n" +
		"trial-a.csv,0,rewarded,baseline,0.5,3,3,97,10\n" +
		"trial-b.csv,1,non-rewarded,baseline,0,10,20,96,9\n" +
		"trial-b.csv,1,non-rewarded,baseline,0.1,11,20,95,8\n" +
		"trial-b.csv,1,non-rewarded,baseline,0.2,12,21,94,7\n")
}

func TestReadGzipVerifiesSourceAndPreservesOnlyTrajectoryColumns(t *testing.T) {
	raw := validCSV()
	path := writeCSV(t, "trajectory.csv.gz", raw, true)
	dataset, err := Read(context.Background(), path, fixtureSource(mustRawGzip(t, raw)), DefaultLimits())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if dataset.Source.PinnedCommit != "0eb07940a9ffdedd97ada81f75e9d5f7bface579" {
		t.Fatalf("source commit = %q", dataset.Source.PinnedCommit)
	}
	if len(dataset.Rows) != 6 {
		t.Fatalf("rows = %d, want 6", len(dataset.Rows))
	}
	if dataset.Rows[1].TrialID == "" || dataset.Rows[1].XCM != 1 || dataset.Rows[1].YCM != 2 {
		t.Fatalf("row = %+v", dataset.Rows[1])
	}
	if dataset.Rows[1].Condition != "rewarded" || dataset.Rows[1].Segment != "baseline" {
		t.Fatalf("provenance fields lost: %+v", dataset.Rows[1])
	}
}

func mustRawGzip(t *testing.T, raw []byte) []byte {
	t.Helper()
	path := writeCSV(t, "digest.csv.gz", raw, true)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestReadRejectsInvalidSourceAndRows(t *testing.T) {
	raw := validCSV()
	tests := []struct {
		name   string
		mutate func(*Source, []byte) []byte
		want   string
	}{
		{
			name: "sha mismatch",
			mutate: func(source *Source, data []byte) []byte {
				source.SHA256 = strings.Repeat("0", 64)
				return data
			},
			want: "SHA-256",
		},
		{
			name: "license missing",
			mutate: func(source *Source, data []byte) []byte {
				source.License.Terms = ""
				return data
			},
			want: "license.terms",
		},
		{
			name: "non finite",
			mutate: func(source *Source, data []byte) []byte {
				return []byte(strings.Replace(string(data), ",1,2,98,11", ",NaN,2,98,11", 1))
			},
			want: "finite",
		},
		{
			name: "duplicate time",
			mutate: func(source *Source, data []byte) []byte {
				return []byte(strings.Replace(string(data), "0.5,3,3", "0.2,3,3", 1))
			},
			want: "duplicate time",
		},
		{
			name: "malformed float",
			mutate: func(source *Source, data []byte) []byte {
				return []byte(strings.Replace(string(data), ",0.2,1,2,98,11", ",not-a-number,1,2,98,11", 1))
			},
			want: "parse t",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := append([]byte(nil), raw...)
			source := fixtureSource(mustRawGzip(t, raw))
			data = test.mutate(&source, data)
			if test.name != "sha mismatch" {
				sum := sha256.Sum256(data)
				source.SHA256 = hex.EncodeToString(sum[:])
			}
			path := writeCSV(t, "bad.csv", data, false)
			_, err := Read(context.Background(), path, source, DefaultLimits())
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("Read error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestReadRejectsSizeAndDuplicateHeaderLimits(t *testing.T) {
	raw := validCSV()
	path := writeCSV(t, "trajectory.csv", raw, false)
	limits := DefaultLimits()
	limits.MaxUncompressedBytes = int64(len(raw) - 1)
	if _, err := Read(context.Background(), path, fixtureSource(raw), limits); err == nil || !strings.Contains(err.Error(), "uncompressed") {
		t.Fatalf("size error = %v, want uncompressed limit error", err)
	}
	duplicate := []byte(strings.Replace(string(raw), "fname,fly", "fname,fname", 1))
	path = writeCSV(t, "duplicate.csv", duplicate, false)
	if _, err := Read(context.Background(), path, fixtureSource(duplicate), DefaultLimits()); err == nil || !strings.Contains(err.Error(), "duplicate header") {
		t.Fatalf("duplicate header error = %v", err)
	}
	malformedRow := []byte(strings.Replace(string(raw), ",99,12\n", ",99\n", 1))
	path = writeCSV(t, "malformed-row.csv", malformedRow, false)
	if _, err := Read(context.Background(), path, fixtureSource(malformedRow), DefaultLimits()); err == nil || !strings.Contains(err.Error(), "fields") {
		t.Fatalf("malformed row error = %v, want field-count error", err)
	}
}

func TestReadRejectsUnorderedOrInterleavedSourceRows(t *testing.T) {
	base := fixtureHeader + "\n" +
		"a.csv,0,rewarded,baseline,0,0,0,0,0\n" +
		"a.csv,0,rewarded,baseline,0.2,1,0,0,0\n" +
		"b.csv,1,rewarded,baseline,0,5,0,0,0\n" +
		"b.csv,1,rewarded,baseline,0.2,6,0,0,0\n"
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "time out of order",
			data: fixtureHeader + "\n" +
				"a.csv,0,rewarded,baseline,0,0,0,0,0\n" +
				"a.csv,0,rewarded,baseline,0.2,1,0,0,0\n" +
				"a.csv,0,rewarded,baseline,0.1,2,0,0,0\n",
			want: "time out of order",
		},
		{
			name: "trial interleaved",
			data: base + "a.csv,0,rewarded,baseline,0.3,2,0,0,0\n",
			want: "not contiguous",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := []byte(test.data)
			path := writeCSV(t, "unordered.csv", data, false)
			_, err := Read(context.Background(), path, fixtureSource(data), DefaultLimits())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Read error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestSplitByTrialIsDeterministicAndLeakFree(t *testing.T) {
	raw := validCSV()
	path := writeCSV(t, "trajectory.csv", raw, false)
	dataset, err := Read(context.Background(), path, fixtureSource(raw), DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	trainA, testA, err := SplitByTrial(dataset, 0.5, 7)
	if err != nil {
		t.Fatalf("SplitByTrial: %v", err)
	}
	trainB, testB, err := SplitByTrial(dataset, 0.5, 7)
	if err != nil {
		t.Fatalf("repeat SplitByTrial: %v", err)
	}
	if !reflect.DeepEqual(trainA.Rows, trainB.Rows) || !reflect.DeepEqual(testA.Rows, testB.Rows) {
		t.Fatal("same seed split is not deterministic")
	}
	trainIDs := trialSet(trainA.Rows)
	for id := range trialSet(testA.Rows) {
		if trainIDs[id] {
			t.Fatalf("trial %q appears in train and test", id)
		}
	}
	if len(trainA.Rows) == 0 || len(testA.Rows) == 0 {
		t.Fatalf("empty split: train=%d test=%d", len(trainA.Rows), len(testA.Rows))
	}
}

func trialSet(rows []Point) map[string]bool {
	set := make(map[string]bool)
	for _, row := range rows {
		set[row.TrialID] = true
	}
	return set
}

func TestBuildSamplesUsesOnlyCausalHistoryAndHandCalculatedTarget(t *testing.T) {
	dataset := Dataset{Rows: []Point{
		{TrialID: "a", T: 0, XCM: 0, YCM: 0},
		{TrialID: "a", T: 0.2, XCM: 1, YCM: 2},
		{TrialID: "a", T: 0.5, XCM: 3, YCM: 3},
		// A second trial must never provide the previous or next point.
		{TrialID: "b", T: 0, XCM: 100, YCM: 100},
		{TrialID: "b", T: 0.2, XCM: 101, YCM: 100},
		{TrialID: "b", T: 0.4, XCM: 101, YCM: 101},
	}}
	set, err := BuildSamples(dataset, SampleConfig{MaxDeltaT: 1, MaxStepDistanceCM: 10})
	if err != nil {
		t.Fatalf("BuildSamples: %v", err)
	}
	if set.RuleVersion != SampleRuleVersion {
		t.Fatalf("rule version = %q, want %q", set.RuleVersion, SampleRuleVersion)
	}
	if len(set.Samples) != 2 {
		t.Fatalf("samples = %d, want 2", len(set.Samples))
	}
	want := Sample{
		TrialID: "a",
		Input: Input{
			CurrentXCM: 1, CurrentYCM: 2, CurrentT: 0.2,
			PreviousDXCM: 1, PreviousDYCM: 2, PreviousDT: 0.2,
		},
		Target: Target{NextDXCM: 2, NextDYCM: 1, NextDT: 0.3},
	}
	if !reflect.DeepEqual(set.Samples[0], want) {
		t.Fatalf("sample = %+v, want %+v", set.Samples[0], want)
	}
	if set.Samples[0].Input.CurrentT >= 0.5 || set.Samples[0].Input.PreviousDT != 0.2 {
		t.Fatalf("future time leaked into input: %+v", set.Samples[0].Input)
	}
}

func TestBuildSamplesSkipsGapsRelocationsAndCrossTrialWindows(t *testing.T) {
	dataset := Dataset{Rows: []Point{
		{TrialID: "a", Segment: "baseline", T: 0, XCM: 0, YCM: 0},
		{TrialID: "a", Segment: "baseline", T: 0.1, XCM: 1, YCM: 0},
		{TrialID: "a", Segment: "baseline", T: 2, XCM: 2, YCM: 0},    // large gap
		{TrialID: "a", Segment: "baseline", T: 2.1, XCM: 20, YCM: 0}, // relocation jump
		{TrialID: "a", Segment: "baseline", T: 2.2, XCM: 21, YCM: 0},
		{TrialID: "a", Segment: "after_relocation", T: 2.3, XCM: 22, YCM: 0}, // segment boundary
		{TrialID: "a", Segment: "after_relocation", T: 2.4, XCM: 23, YCM: 0},
		{TrialID: "b", Segment: "baseline", T: 2.5, XCM: 24, YCM: 0},
	}}
	set, err := BuildSamples(dataset, SampleConfig{MaxDeltaT: 0.5, MaxStepDistanceCM: 5})
	if err != nil {
		t.Fatalf("BuildSamples: %v", err)
	}
	if len(set.Samples) != 0 {
		t.Fatalf("samples = %d, want no windows crossing rejected transitions", len(set.Samples))
	}
	if set.Stats.SkippedLargeGap == 0 || set.Stats.SkippedLargeStep != 1 || set.Stats.SkippedManualRelocation != 0 || set.Stats.SkippedRelocation != 1 || set.Stats.SkippedBoundary == 0 {
		t.Fatalf("skip stats = %+v, want gap, relocation and boundary counts", set.Stats)
	}
}

func TestBuildSamplesSkipsExplicitRelocationSegment(t *testing.T) {
	dataset := Dataset{Rows: []Point{
		{TrialID: "a", Segment: "relocation", T: 0, XCM: 0, YCM: 0},
		{TrialID: "a", Segment: "relocation", T: 0.1, XCM: 1, YCM: 0},
		{TrialID: "a", Segment: "relocation", T: 0.2, XCM: 2, YCM: 0},
	}}
	set, err := BuildSamples(dataset, SampleConfig{MaxDeltaT: 1, MaxStepDistanceCM: 10})
	if err != nil {
		t.Fatalf("BuildSamples: %v", err)
	}
	if len(set.Samples) != 0 || set.Stats.SkippedRelocation != 1 || set.Stats.SkippedManualRelocation != 1 || set.Stats.SkippedLargeStep != 0 {
		t.Fatalf("set = %+v, want one explicit relocation skip", set)
	}
}

func TestBuildSamplesRejectsInvalidConfigAndDataset(t *testing.T) {
	dataset := Dataset{Rows: []Point{
		{TrialID: "a", T: 0, XCM: 0, YCM: 0},
		{TrialID: "a", T: 0.1, XCM: 1, YCM: 0},
		{TrialID: "a", T: 0.2, XCM: 2, YCM: 0},
	}}
	for _, cfg := range []SampleConfig{
		{MaxDeltaT: 0},
		{MaxDeltaT: -1},
		{MaxDeltaT: 1, MaxStepDistanceCM: 0},
		{MaxDeltaT: 1, MaxStepDistanceCM: -1},
	} {
		if _, err := BuildSamples(dataset, cfg); err == nil {
			t.Fatalf("BuildSamples(%+v) succeeded, want config error", cfg)
		}
	}
	bad := dataset
	bad.Rows = append([]Point(nil), dataset.Rows...)
	bad.Rows[1].T = 0
	if _, err := BuildSamples(bad, SampleConfig{MaxDeltaT: 1, MaxStepDistanceCM: 10}); err == nil || !strings.Contains(err.Error(), "duplicate time") {
		t.Fatalf("duplicate time error = %v", err)
	}
}

func TestSampleInputHasNoForbiddenMetadataFields(t *testing.T) {
	typ := reflect.TypeOf(Input{})
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		for _, forbidden := range []string{"reward", "fictive", "condition", "segment", "trial", "fly"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("Input field %q exposes forbidden metadata %q", typ.Field(i).Name, forbidden)
			}
		}
	}
}

func TestTrialIDsAreStableAndSortedForInspection(t *testing.T) {
	rows := []Point{
		{FName: "f", Fly: "2", Condition: "rewarded", Segment: "baseline"},
		{FName: "f", Fly: "1", Condition: "rewarded", Segment: "after_relocation"},
	}
	for i := range rows {
		rows[i].TrialID = makeTrialID(rows[i].FName, rows[i].Fly)
	}
	ids := []string{rows[0].TrialID, rows[1].TrialID}
	sort.Strings(ids)
	if ids[0] == ids[1] || !strings.HasSuffix(ids[0], string(trialSeparator)+"1") {
		t.Fatalf("trial IDs = %v", ids)
	}
}

func TestSplitByTrialKeepsAllSegmentsOfOneFlyTogether(t *testing.T) {
	rows := []Point{
		{TrialID: makeTrialID("fly.csv", "7"), FName: "fly.csv", Fly: "7", Condition: "rewarded", Segment: "baseline", T: 0, XCM: 0, YCM: 0},
		{TrialID: makeTrialID("fly.csv", "7"), FName: "fly.csv", Fly: "7", Condition: "rewarded", Segment: "after_relocation", T: 0.1, XCM: 1, YCM: 0},
		{TrialID: makeTrialID("other.csv", "8"), FName: "other.csv", Fly: "8", Condition: "rewarded", Segment: "baseline", T: 0, XCM: 0, YCM: 0},
		{TrialID: makeTrialID("other.csv", "8"), FName: "other.csv", Fly: "8", Condition: "rewarded", Segment: "after_relocation", T: 0.1, XCM: 1, YCM: 0},
	}
	train, test, err := SplitByTrial(Dataset{Rows: rows}, 0.5, 1)
	if err != nil {
		t.Fatalf("SplitByTrial: %v", err)
	}
	if len(trialSet(train.Rows)) != 1 || len(trialSet(test.Rows)) != 1 {
		t.Fatalf("trial split = train %v test %v, want one complete trial each", trialSet(train.Rows), trialSet(test.Rows))
	}
	for _, side := range [][]Point{train.Rows, test.Rows} {
		segments := map[string]bool{}
		for _, row := range side {
			segments[row.Segment] = true
		}
		if !segments["baseline"] || !segments["after_relocation"] {
			t.Fatalf("segments split apart: %v", segments)
		}
	}
}

func TestReadTitovaSourceWhenConfigured(t *testing.T) {
	path := os.Getenv("COIMNET_TSK11_TRAJECTORY")
	if path == "" {
		t.Skip("set COIMNET_TSK11_TRAJECTORY to run the Git-out real-data check")
	}
	source := Source{
		ID:           "titova-2023-all-ds-t01-d2-cm-no2",
		URL:          "https://github.com/strawlab/titova_et_al_displacement_supplemental/blob/0eb07940a9ffdedd97ada81f75e9d5f7bface579/all_ds_t01_d2_cm_no2.csv.gz",
		DOI:          "10.5061/dryad.vdncjsz0b",
		PinnedCommit: "0eb07940a9ffdedd97ada81f75e9d5f7bface579",
		SHA256:       "83e13b7057957cf41a45dc91996a4519be7066519242b6912c65b2418cb91670",
		License: License{
			Holder: "Titova et al.",
			Terms:  "CC0 1.0",
			Source: "https://doi.org/10.5061/dryad.vdncjsz0b",
		},
	}
	dataset, err := Read(context.Background(), path, source, DefaultLimits())
	if err != nil {
		t.Fatalf("Read real source: %v", err)
	}
	if len(dataset.Rows) != 231130 {
		t.Fatalf("real rows = %d, want 231130", len(dataset.Rows))
	}
	if got := len(trialSet(dataset.Rows)); got != 39 {
		t.Fatalf("real trials = %d, want 39 fname/fly trials", got)
	}
	set, err := BuildSamples(dataset, DefaultSampleConfig())
	if err != nil {
		t.Fatalf("BuildSamples real source: %v", err)
	}
	if len(set.Samples) == 0 || set.Stats.SkippedLargeGap == 0 {
		t.Fatalf("real sample stats = %+v, want samples and skipped gaps", set.Stats)
	}
	t.Logf("real source rows=%d trials=%d samples=%d stats=%+v", len(dataset.Rows), len(trialSet(dataset.Rows)), len(set.Samples), set.Stats)
}
