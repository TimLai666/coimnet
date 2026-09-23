package fullgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/TimLai666/coimnet/internal/fileio"
)

// ResumeSchemaVersion is the schema version written into ResumeReport documents.
const ResumeSchemaVersion = "coimnet-full-graph-resume/v1"

// maxReportBytes bounds the report.json Resume reads; the one Run writes is a
// few kilobytes.
const maxReportBytes = 1 << 20

// ResumeReport is what a fresh process reproduced from a Run output directory.
type ResumeReport struct {
	SchemaVersion      string `json:"schema_version"`
	Dir                string `json:"dir"`
	Rows               int    `json:"rows"`
	ContinuationDigest string `json:"continuation_digest"`
	NeuralDigest       string `json:"neural_digest"`
	Matches            bool   `json:"matches"` // both digests equal the ones in Dir/report.json
}

// Resume reads Dir/report.json and runs resumeAndContinue on dir with
// report.Options.ContinueRows rows in the chunk Run used (report.Run.Chunk).
// It digests the continued rows with continuationDigest and the final
// snapshot's Neural with neuralDigest, the helpers Run uses, and sets Matches
// when both equal the digests in the report. A mismatch is a report with
// Matches false, not an error; an unreadable report or individual is an
// error. It never writes into Dir.
func Resume(ctx context.Context, dir string) (ResumeReport, error) {
	data, err := fileio.ReadRegular(ctx, filepath.Join(dir, "report.json"), maxReportBytes)
	if err != nil {
		return ResumeReport{}, fmt.Errorf("fullgraph: read report: %w", err)
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return ResumeReport{}, fmt.Errorf("fullgraph: decode report: %w", err)
	}
	outputs, snapshot, err := resumeAndContinue(ctx, dir, report.Options.ContinueRows, report.Run.Chunk)
	if err != nil {
		return ResumeReport{}, fmt.Errorf("fullgraph: resume individual: %w", err)
	}
	neural, err := neuralDigest(snapshot.Neural)
	if err != nil {
		return ResumeReport{}, fmt.Errorf("fullgraph: neural digest: %w", err)
	}
	continuation := continuationDigest(outputs)
	return ResumeReport{
		SchemaVersion:      ResumeSchemaVersion,
		Dir:                dir,
		Rows:               len(outputs),
		ContinuationDigest: continuation,
		NeuralDigest:       neural,
		Matches:            continuation == report.ContinuationDigest && neural == report.NeuralDigest,
	}, nil
}
