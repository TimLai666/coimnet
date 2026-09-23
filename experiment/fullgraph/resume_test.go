package fullgraph

import (
	"context"
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"
)

// TestResumeMatchesAndDetectsTampering builds a Run output directory on the
// fixture graph: the three artifacts shortTraining saves, and a report.json
// that holds only the fields Resume reads, filled in with the digests of the
// in-process continuation. Resume has to reproduce both digests from
// individual.coimbundle, stop matching once either digest in report.json differs by
// one character, and leave every file in the directory byte-identical.
func TestResumeMatchesAndDetectsTampering(t *testing.T) {
	ctx := context.Background()
	c, p, o := buildFixtureConfig(t)
	dir := t.TempDir()
	run := runOptions{Steps: 8, ContinueRows: 3, PlasticEdges: 2, Chemistry: true}
	ind, rep, err := shortTraining(ctx, c, p, o, run, dir)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := continueRows(ctx, ind, run.ContinueRows, rep.Chunk)
	if err != nil {
		t.Fatal(err)
	}
	neural, err := neuralDigest(ind.Snapshot().Neural)
	if err != nil {
		t.Fatal(err)
	}
	saved := Report{
		Options:            Options{ContinueRows: run.ContinueRows},
		Run:                runReport{Chunk: rep.Chunk},
		ContinuationDigest: continuationDigest(outputs),
		NeuralDigest:       neural,
	}
	writeRunReport(t, dir, saved)

	want := ResumeReport{
		SchemaVersion:      ResumeSchemaVersion,
		Dir:                dir,
		Rows:               run.ContinueRows,
		ContinuationDigest: saved.ContinuationDigest,
		NeuralDigest:       saved.NeuralDigest,
		Matches:            true,
	}
	if got := resumeReadOnly(t, dir); got != want {
		t.Fatalf("Resume = %+v, want %+v", got, want)
	}

	mismatch := want
	mismatch.Matches = false
	for _, tc := range []struct {
		name   string
		tamper func(*Report)
	}{
		{"continuation digest", func(r *Report) { r.ContinuationDigest = changeFirstCharacter(r.ContinuationDigest) }},
		{"neural digest", func(r *Report) { r.NeuralDigest = changeFirstCharacter(r.NeuralDigest) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tampered := saved
			tc.tamper(&tampered)
			writeRunReport(t, dir, tampered)
			if got := resumeReadOnly(t, dir); got != mismatch {
				t.Fatalf("Resume after changing the %s = %+v, want %+v", tc.name, got, mismatch)
			}
		})
	}
}

// writeRunReport writes r to dir/report.json as indented JSON the way Run
// does, replacing any earlier copy.
func writeRunReport(t *testing.T, dir string, r Report) {
	t.Helper()
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// resumeReadOnly runs Resume on dir and fails the test when the call added,
// removed or changed any file in dir.
func resumeReadOnly(t *testing.T, dir string) ResumeReport {
	t.Helper()
	before := hashTree(t, dir)
	got, err := Resume(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if after := hashTree(t, dir); !maps.Equal(before, after) {
		t.Fatalf("Resume changed the directory:\nbefore %v\nafter  %v", before, after)
	}
	return got
}

// hashTree maps every file under dir, by its path relative to dir, to the
// SHA-256 of its contents.
func hashTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	sums := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		sum, err := fileSHA256(path)
		if err != nil {
			return err
		}
		sums[rel] = sum
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sums
}

// changeFirstCharacter returns the hex digest with its first character
// replaced by a different hex character.
func changeFirstCharacter(digest string) string {
	if digest[0] == '0' {
		return "1" + digest[1:]
	}
	return "0" + digest[1:]
}
