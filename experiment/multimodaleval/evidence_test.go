package multimodaleval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"text/tabwriter"
)

// TestMultimodalEvidence is the TSK-10 evidence run. It is skipped unless
// COIMNET_TSK10_EVIDENCE names an absolute output directory (go test runs in
// the package directory). It runs DefaultConfig, fails if any seed run failed,
// logs the summary table and writes two new files there, failing if either
// already exists: multimodal.json, the indented Report, and summary.txt, the
// summary table. Reproduce from the repository root with
//
//	COIMNET_TSK10_EVIDENCE=$PWD/evidence/TSK-10 go test -count=1 -v -run '^TestMultimodalEvidence$' ./experiment/multimodaleval/
func TestMultimodalEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_TSK10_EVIDENCE")
	if dir == "" {
		t.Skip("set COIMNET_TSK10_EVIDENCE to an absolute directory to write the TSK-10 evidence")
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("COIMNET_TSK10_EVIDENCE = %q, want an absolute directory", dir)
	}
	report, err := Run(context.Background(), DefaultConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, run := range report.Runs {
		if run.Failed {
			t.Fatalf("seed %d failed: %s", run.Seed, run.Error)
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	summary := evidenceSummary(t, report)
	t.Logf("TSK-10 summary:\n%s", summary)
	writeNewEvidenceFile(t, filepath.Join(dir, "multimodal.json"), append(data, '\n'))
	writeNewEvidenceFile(t, filepath.Join(dir, "summary.txt"), []byte(summary))
}

// evidenceSummary renders one row per seed and a last row with the mean over
// the seeds. Column names follow the Report JSON: i2t, t2i and a2i are
// image_to_text, text_to_image and audio_to_image_shape; unseen_retrieval
// queries the held-out inputs against the whole evaluation set; the unseen
// accuracies are the held-out group's per-modality shape and colour accuracies.
func evidenceSummary(t *testing.T, report Report) string {
	t.Helper()
	accuracy := func(seed uint64, kind string, byModality map[string]float64, modality string) float64 {
		v, ok := byModality[modality]
		if !ok {
			t.Fatalf("seed %d: unseen %s accuracy has no %q entry", seed, kind, modality)
		}
		return v
	}
	labels := make([]string, 0, len(report.Runs)+1)
	rows := make([][]float64, 0, len(report.Runs)+1)
	for _, r := range report.Runs {
		labels = append(labels, fmt.Sprint(r.Seed))
		rows = append(rows, []float64{
			float64(r.Updates),
			r.SeenBefore.ImageToText, r.Seen.ImageToText, r.Seen.TextToImage,
			r.UnseenRetrievalBefore.ImageToText, r.UnseenRetrievalBefore.TextToImage, r.UnseenRetrievalBefore.AudioToImageShape,
			r.UnseenRetrieval.ImageToText, r.UnseenRetrieval.TextToImage, r.UnseenRetrieval.AudioToImageShape,
			accuracy(r.Seed, "shape", r.Unseen.ShapeAccuracy, "image"), accuracy(r.Seed, "colour", r.Unseen.ColourAccuracy, "image"),
			accuracy(r.Seed, "shape", r.Unseen.ShapeAccuracy, "text"), accuracy(r.Seed, "colour", r.Unseen.ColourAccuracy, "text"),
		})
	}
	mean := make([]float64, len(rows[0]))
	for _, row := range rows {
		for i, v := range row {
			mean[i] += v
		}
	}
	for i := range mean {
		mean[i] /= float64(len(rows))
	}
	labels = append(labels, "mean")
	rows = append(rows, mean)

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "seed\tupdates\tseen_before.i2t\tseen.i2t\tseen.t2i\t"+
		"unseen_retrieval_before.i2t\tunseen_retrieval_before.t2i\tunseen_retrieval_before.a2i\t"+
		"unseen_retrieval.i2t\tunseen_retrieval.t2i\tunseen_retrieval.a2i\t"+
		"unseen.image_shape\tunseen.image_colour\tunseen.text_shape\tunseen.text_colour")
	for i, row := range rows {
		fmt.Fprintf(w, "%s\t%g", labels[i], row[0])
		for _, v := range row[1:] {
			fmt.Fprintf(w, "\t%.4f", v)
		}
		fmt.Fprintln(w)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("render summary: %v", err)
	}
	return buf.String()
}

// writeNewEvidenceFile creates path, failing if it already exists, and writes
// data to it.
func writeNewEvidenceFile(t *testing.T, path string, data []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
