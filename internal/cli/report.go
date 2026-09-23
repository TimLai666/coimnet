package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/TimLai666/coimnet/internal/fileio"
)

const (
	// reportSchemaVersion identifies the completion report this command
	// publishes. It reports the tracking document and the evidence records as
	// they stand; it never decides whether a requirement is met.
	reportSchemaVersion = "coimnet-report/v1"

	// defaultReportEvidenceDir is where the repository keeps its verification
	// records. It is only a fallback root: an evidence path is resolved next to
	// the status document first.
	defaultReportEvidenceDir = "evidence"

	// reportObservedResultRunes is how much of one observed result the report
	// quotes. The record itself stays the full account; the report is an index.
	reportObservedResultRunes = 300

	// maxReportInputBytes bounds every document the report reads.
	maxReportInputBytes = 16 << 20
)

// runReport summarises the requirements tracking document and the evidence
// records it points at into one JSON report and the same content as Markdown.
// It reads only; neither the status document nor any record is modified, and an
// existing output path is refused before anything is read.
func runReport(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var status, outJSON, outMD, settings string
	evidenceDir := defaultReportEvidenceDir
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&status, "status", "", "path to the requirements status document; never modified (required)")
	fs.StringVar(&evidenceDir, "evidence-dir", defaultReportEvidenceDir, "fallback root for an evidence record that is not found next to the status document (default "+defaultReportEvidenceDir+")")
	fs.StringVar(&outJSON, "out-json", "", "path of the new JSON report; an existing path is refused (required)")
	fs.StringVar(&outMD, "out-md", "", "path of the new Markdown report; an existing path is refused (required)")
	fs.StringVar(&settings, "settings", "", "path to a JSON settings document copied into the report as it stands (optional)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet report --status FILE --evidence-dir DIR --out-json FILE --out-md FILE [--settings FILE]")
		fmt.Fprintln(usageOutput, "Reads the requirements status document and every verification record it names, then writes one "+reportSchemaVersion+" document to --out-json and the same content as Markdown to --out-md. An evidence path is resolved relative to the directory holding --status, the way the tracking document writes it, and a path that is not found there is retried as <--evidence-dir>/<record directory>/<file name>.")
		fmt.Fprintln(usageOutput, "The report carries generated_at, the --settings document as it stands or null, completion counts by status, every requirement with its evidence paths and whether those files exist, the first "+fmt.Sprint(reportObservedResultRunes)+" characters of each passed requirement's observed result, the limitations each record declares and the input fingerprints each record lists. evidence_present reports only whether the files exist; it is not a judgement about the evidence, and the command never decides whether a requirement is met.")
		fmt.Fprintln(usageOutput, "Example: coimnet report --status docs/requirements-status.json --evidence-dir evidence --out-json report.json --out-md report.md")
		fmt.Fprintln(usageOutput, "Errors: missing --status, --out-json or --out-md, the same path for both outputs, an existing output, an unreadable or invalid status document, settings document or evidence record, cancellation or output failure. Usage errors exit with status 1 and name the flag.")
		fmt.Fprintln(usageOutput, "Options and their defaults:")
		fs.SetOutput(usageOutput)
		fs.PrintDefaults()
		fs.SetOutput(stderr)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	if fs.NArg() != 0 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("report takes no positional arguments; use report --help")}
	}
	if status == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--status is required; use report --help")}
	}
	if outJSON == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out-json is required; use report --help")}
	}
	if outMD == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out-md is required; use report --help")}
	}
	if outJSON == outMD {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out-json and --out-md must be different paths")}
	}
	if err := refuseExistingOutput("report", "--out-json", outJSON); err != nil {
		return err
	}
	if err := refuseExistingOutput("report", "--out-md", outMD); err != nil {
		return err
	}
	document, err := buildReport(ctx, status, evidenceDir, settings)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	if err := writeNewJSON(outJSON, document); err != nil {
		return fmt.Errorf("write report %s: %w", outJSON, err)
	}
	if err := writeNewTextFile(outMD, reportMarkdown(status, document)); err != nil {
		return fmt.Errorf("write report %s: %w", outMD, err)
	}
	_, err = fmt.Fprintf(stdout, "report: %d of %d requirements passed; wrote %s and %s\n", document.Completion.Passed, document.Completion.Total, outJSON, outMD)
	return err
}

// reportDocument is the whole published report. Settings, limitations and
// sources keep the JSON they were read as, so the report never reinterprets a
// record it only indexes.
type reportDocument struct {
	SchemaVersion string              `json:"schema_version"`
	GeneratedAt   string              `json:"generated_at"`
	Settings      json.RawMessage     `json:"settings"`
	Completion    reportCompletion    `json:"completion"`
	Requirements  []reportRequirement `json:"requirements"`
	Scores        []reportScore       `json:"scores"`
	Limitations   []reportLimitation  `json:"limitations"`
	Sources       []json.RawMessage   `json:"sources"`
}

// reportCompletion counts the statuses. Blocked includes the literal blocked
// status and every status with the blocked_ prefix; by_status carries every
// status actually seen, so a new one is reported instead of silently dropped.
type reportCompletion struct {
	Passed    int            `json:"passed"`
	Blocked   int            `json:"blocked"`
	Specified int            `json:"specified"`
	Total     int            `json:"total"`
	ByStatus  map[string]int `json:"by_status"`
}

// reportRequirement is one tracked requirement. EvidencePresent reports that
// every declared record exists as a file, nothing about what it says.
type reportRequirement struct {
	ID              string   `json:"id"`
	Status          string   `json:"status"`
	Evidence        []string `json:"evidence"`
	EvidencePresent bool     `json:"evidence_present"`
}

// reportScore quotes the opening of one passed requirement's observed result.
type reportScore struct {
	ID             string `json:"id"`
	ObservedResult string `json:"observed_result"`
}

// reportLimitation carries one record's declared limitations unchanged.
type reportLimitation struct {
	ID          string          `json:"id"`
	Limitations json.RawMessage `json:"limitations"`
}

// reportStatusDocument is the part of the tracking document this command reads.
// Requirements is a pointer so a file without the array is refused instead of
// reported as a project with no requirements at all.
type reportStatusDocument struct {
	SchemaVersion string                     `json:"schema_version"`
	Requirements  *[]reportStatusRequirement `json:"requirements"`
}

type reportStatusRequirement struct {
	ID       string   `json:"id"`
	Status   string   `json:"status"`
	Evidence []string `json:"evidence"`
}

// reportEvidenceRecord is the part of one verification record the report reads.
// Every field is kept raw, because the report quotes records and does not
// interpret them.
type reportEvidenceRecord struct {
	ObservedResult    json.RawMessage `json:"observed_result"`
	Limitations       json.RawMessage `json:"limitations"`
	InputFingerprints json.RawMessage `json:"input_fingerprints"`
}

// buildReport reads the status document, resolves every evidence path and
// assembles the report. A record that exists but cannot be decoded is an error
// naming the path, because reporting it as missing would hide a broken record.
func buildReport(ctx context.Context, status, evidenceDir, settings string) (reportDocument, error) {
	var empty reportDocument
	statusData, err := fileio.ReadRegular(ctx, status, maxReportInputBytes)
	if err != nil {
		return empty, fmt.Errorf("report --status %s: read: %w", status, err)
	}
	var tracking reportStatusDocument
	if err := json.Unmarshal(statusData, &tracking); err != nil {
		return empty, fmt.Errorf("report --status %s: decode: %w", status, err)
	}
	if tracking.Requirements == nil {
		return empty, fmt.Errorf(`report --status %s: the document has no "requirements" array`, status)
	}
	document := reportDocument{
		SchemaVersion: reportSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Completion:    reportCompletion{ByStatus: map[string]int{}},
		Requirements:  make([]reportRequirement, 0, len(*tracking.Requirements)),
		Scores:        make([]reportScore, 0, len(*tracking.Requirements)),
		Limitations:   make([]reportLimitation, 0, len(*tracking.Requirements)),
		Sources:       make([]json.RawMessage, 0, len(*tracking.Requirements)),
	}
	if settings != "" {
		settingsData, err := fileio.ReadRegular(ctx, settings, maxReportInputBytes)
		if err != nil {
			return empty, fmt.Errorf("report --settings %s: read: %w", settings, err)
		}
		if !json.Valid(settingsData) {
			return empty, fmt.Errorf("report --settings %s: the file is not valid JSON", settings)
		}
		document.Settings = json.RawMessage(settingsData)
	}
	statusDir := filepath.Dir(status)
	for _, requirement := range *tracking.Requirements {
		evidence := append([]string(nil), requirement.Evidence...)
		if evidence == nil {
			evidence = []string{}
		}
		entry := reportRequirement{ID: requirement.ID, Status: requirement.Status, Evidence: evidence, EvidencePresent: len(evidence) > 0}
		document.Completion.Total++
		document.Completion.ByStatus[requirement.Status]++
		switch requirement.Status {
		case "passed":
			document.Completion.Passed++
		case "specified":
			document.Completion.Specified++
		}
		if requirement.Status == "blocked" || strings.HasPrefix(requirement.Status, "blocked_") {
			document.Completion.Blocked++
		}
		scored := false
		for _, declared := range evidence {
			resolved, present := resolveEvidencePath(statusDir, evidenceDir, declared)
			if !present {
				entry.EvidencePresent = false
				continue
			}
			record, err := readEvidenceRecord(ctx, resolved)
			if err != nil {
				return empty, fmt.Errorf("report evidence %s of %s: %w", resolved, requirement.ID, err)
			}
			if requirement.Status == "passed" && !scored {
				if observed, ok := reportObservedText(record.ObservedResult); ok {
					document.Scores = append(document.Scores, reportScore{ID: requirement.ID, ObservedResult: observed})
					scored = true
				}
			}
			if limitations, ok := reportRawArray(record.Limitations); ok {
				document.Limitations = append(document.Limitations, reportLimitation{ID: requirement.ID, Limitations: limitations})
			}
			if fingerprints, ok := reportRawValue(record.InputFingerprints); ok {
				document.Sources = append(document.Sources, fingerprints)
			}
		}
		document.Requirements = append(document.Requirements, entry)
	}
	return document, nil
}

// resolveEvidencePath locates one declared evidence record. The tracking
// document writes its paths relative to its own directory, so that is the
// first place looked; --evidence-dir is the fallback root for a status
// document that was moved away from the records it names.
func resolveEvidencePath(statusDir, evidenceDir, declared string) (string, bool) {
	candidate := declared
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(statusDir, declared)
	}
	if isRegularFile(candidate) {
		return candidate, true
	}
	if evidenceDir != "" && !filepath.IsAbs(declared) {
		fallback := filepath.Join(evidenceDir, filepath.Base(filepath.Dir(declared)), filepath.Base(declared))
		if isRegularFile(fallback) {
			return fallback, true
		}
	}
	return candidate, false
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// readEvidenceRecord decodes the fields the report quotes from one record.
func readEvidenceRecord(ctx context.Context, path string) (reportEvidenceRecord, error) {
	var record reportEvidenceRecord
	data, err := fileio.ReadRegular(ctx, path, maxReportInputBytes)
	if err != nil {
		return record, fmt.Errorf("read: %w", err)
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, fmt.Errorf("decode: %w", err)
	}
	return record, nil
}

// reportObservedText quotes the opening of one observed result. A record that
// wrote its result as an array, or as anything else JSON allows, is compacted
// to one line first, so the quote is always readable in a single cell.
func reportObservedText(raw json.RawMessage) (string, bool) {
	text, ok := reportEntryText(raw)
	if !ok || text == "" {
		return "", false
	}
	return reportTruncate(text), true
}

// reportEntryText prints one raw value: a JSON string as its own text, anything
// else as the single-line JSON it was written as.
func reportEntryText(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, true
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", false
	}
	if compact.String() == "null" {
		return "", false
	}
	return compact.String(), true
}

// reportTruncate keeps the first reportObservedResultRunes characters, counting
// characters rather than bytes so a cut never splits one.
func reportTruncate(text string) string {
	runes := []rune(text)
	if len(runes) <= reportObservedResultRunes {
		return text
	}
	return string(runes[:reportObservedResultRunes])
}

// reportRawValue returns one declared value compacted to a single line, and
// reports whether the record declared it at all.
func reportRawValue(raw json.RawMessage) (json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, false
	}
	if compact.String() == "null" {
		return nil, false
	}
	return json.RawMessage(compact.Bytes()), true
}

// reportRawArray returns one declared array, and reports false for an absent,
// null or empty one, so the report lists only records that actually declared
// something.
func reportRawArray(raw json.RawMessage) (json.RawMessage, bool) {
	value, ok := reportRawValue(raw)
	if !ok {
		return nil, false
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(value, &entries); err != nil || len(entries) == 0 {
		return nil, false
	}
	return value, true
}

// reportMarkdown renders the same report for a reader: the completion counts,
// every requirement with whether its records exist, and the limitations the
// records declare.
func reportMarkdown(status string, document reportDocument) string {
	var b strings.Builder
	b.WriteString("# Requirements report\n\n")
	fmt.Fprintf(&b, "Generated at %s from %s. `evidence_present` reports only that the named record files exist.\n\n", document.GeneratedAt, status)
	b.WriteString("## Completion\n\n| Status | Count |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| passed | %d |\n", document.Completion.Passed)
	blockedKinds := reportBlockedStatuses(document.Completion.ByStatus)
	if len(blockedKinds) == 0 {
		fmt.Fprintf(&b, "| blocked | %d |\n", document.Completion.Blocked)
	} else {
		parts := make([]string, 0, len(blockedKinds))
		for _, name := range blockedKinds {
			parts = append(parts, fmt.Sprintf("%s %d", name, document.Completion.ByStatus[name]))
		}
		fmt.Fprintf(&b, "| blocked | %d (%s) |\n", document.Completion.Blocked, strings.Join(parts, ", "))
	}
	fmt.Fprintf(&b, "| specified | %d |\n", document.Completion.Specified)
	for _, name := range reportOtherStatuses(document.Completion.ByStatus) {
		fmt.Fprintf(&b, "| %s | %d |\n", exportCell(name), document.Completion.ByStatus[name])
	}
	fmt.Fprintf(&b, "| total | %d |\n", document.Completion.Total)
	b.WriteString("\n## Requirements\n\n| ID | Status | Evidence present |\n| --- | --- | --- |\n")
	for _, requirement := range document.Requirements {
		present := "no"
		if requirement.EvidencePresent {
			present = "yes"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", exportCell(requirement.ID), exportCell(requirement.Status), present)
	}
	b.WriteString("\n## Limitations\n\n")
	if len(document.Limitations) == 0 {
		b.WriteString("No record declared a limitation.\n")
	}
	for _, limitation := range document.Limitations {
		fmt.Fprintf(&b, "### %s\n\n", limitation.ID)
		var entries []json.RawMessage
		if err := json.Unmarshal(limitation.Limitations, &entries); err != nil {
			fmt.Fprintf(&b, "- %s\n\n", exportCell(string(limitation.Limitations)))
			continue
		}
		for _, entry := range entries {
			text, ok := reportEntryText(entry)
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", exportCell(text))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func reportBlockedStatuses(byStatus map[string]int) []string {
	blocked := make([]string, 0, len(byStatus))
	for name := range byStatus {
		if strings.HasPrefix(name, "blocked_") {
			blocked = append(blocked, name)
		}
	}
	sort.Strings(blocked)
	return blocked
}

// reportOtherStatuses names statuses beyond the named counters and blocked_*
// kinds, which are summarized in the blocked row. A status this build does not
// know still appears in the table.
func reportOtherStatuses(byStatus map[string]int) []string {
	others := make([]string, 0, len(byStatus))
	for name := range byStatus {
		if strings.HasPrefix(name, "blocked_") {
			continue
		}
		switch name {
		case "passed", "blocked", "specified":
			continue
		}
		others = append(others, name)
	}
	sort.Strings(others)
	return others
}
