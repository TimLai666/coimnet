package coimnet_test

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	formatCategory = "正式格式"
	rulesCategory  = "規則與雜湊識別"
	toolCategory   = "驗證工具格式"
	testCategory   = "測試資料與反例"
)

var (
	versionRE = regexp.MustCompile(`^coimnet-[A-Za-z0-9_-]+/v[0-9]+$`)
	findRE    = regexp.MustCompile(`coimnet-[A-Za-z0-9_-]+/v[0-9]+`)
	schemaRE  = regexp.MustCompile(`"schema_version"\s*:\s*"([^"]+)"`)
	linkRE    = regexp.MustCompile(`^\[([^\]]+)\]\(([^)]+)\)$`)
)

var governanceFiles = []string{
	"docs/requirements-status.json", "docs/requirements-addendum.json",
	"docs/handoff/requirements.json", "docs/handoff/sources.json",
}

// These lists are the small semantic inventory checked by the root agent. A
// test-only source by itself is deliberately not enough to infer test data.
var knownCategory = map[string]string{
	"connectome-builder/v1": rulesCategory, "policy-version/v2": rulesCategory,
	"derivation-rules-hash/v1": rulesCategory, "simulate-parameters/v1": rulesCategory,
	"distill-label/v1": rulesCategory, "logmel/v1": rulesCategory,
	"realnav-preprocessing/v1": rulesCategory, "realnav-memory-causal-features/v1": rulesCategory,
	"nav2d-causal-next-displacement/v1": rulesCategory, "textgen-fixture/v1": rulesCategory,
	"dataset-manifest/v9": testCategory, "graph-store/v9": testCategory,
	"individual-checkpoint/v9": testCategory, "model-package/v2": testCategory,
	"lif-state/v0": testCategory, "replay/v0": testCategory,
	"simulate-compare/v2": testCategory, "simulate-protocol/v2": testCategory,
	"simulate-state/v2": testCategory, "connectome-builder/v999": testCategory,
	"realnav-memory-bundle/v999": testCategory, "unknown/v9": testCategory,
	"real-task-evidence/v1": testCategory,
	"multitask-suite/v1":    toolCategory, "ppo-gradient-diagnostic/v1": toolCategory,
	"ppo-horizon-comparison/v1": toolCategory, "ppo-training-feedback/v1": toolCategory,
	"short-goal-cue-audit/v1": toolCategory, "synthetic-goal-cue-audit/v1": toolCategory,
}

type formatRef struct {
	Path string
	Line int
}
type formatInventory struct{ Refs map[string][]formatRef }
type formatRow struct {
	IndexLine, SourceLine                         int
	Version, Scope, Purpose, Category, SourcePath string
}
type parsedIndex struct {
	Rows     []formatRow
	Headings map[string]bool
	Errors   []string
}

func TestFormatIndex(t *testing.T) {
	inv, err := discoverFormatInventory(".")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("format inventory unique versions=%d", len(inv.Refs))
	counts := map[string]int{}
	for version, refs := range inv.Refs {
		counts[expectedCategory(version, refs)]++
	}
	t.Logf("format inventory categories: formal=%d rules=%d tools=%d test=%d unclassified=%d", counts[formatCategory], counts[rulesCategory], counts[toolCategory], counts[testCategory], counts[""])
	if err := validateFormatIndex("."); err != nil {
		t.Fatalf("inventory unique versions=%d: %v", len(inv.Refs), err)
	}
}

func TestFormatIndexControls(t *testing.T) {
	cases := []struct {
		name, want string
		mutate     func(*formatFixture)
	}{
		{"missing", "missing index row", func(f *formatFixture) { f.rows = removeRow(f.rows, f.formatVersion) }},
		{"duplicate", "duplicate index row", func(f *formatFixture) { f.rows = append(f.rows, f.rows[0]) }},
		{"stale", "stale index row", func(f *formatFixture) {
			f.rows = append(f.rows, formatRow{Version: fixtureVersion("stale", 99), Scope: "fixture", Purpose: "stale", Category: formatCategory, SourcePath: f.rows[0].SourcePath, SourceLine: f.rows[0].SourceLine})
		}},
		{"wrong source", "source for", func(f *formatFixture) {
			for i := range f.rows {
				if f.rows[i].Version == f.statusVersion {
					f.rows[i].SourcePath = "main.go"
					return
				}
			}
		}},
		{"wrong line", "source for", func(f *formatFixture) { f.rows[0].SourceLine += 100 }},
		{"known counterexample as formal", "category for", func(f *formatFixture) {
			for i := range f.rows {
				if f.rows[i].Version == fixtureVersion("dataset-manifest", 9) {
					f.rows[i].Category = formatCategory
				}
			}
		}},
		{"known report as test", "category for", func(f *formatFixture) {
			for i := range f.rows {
				if f.rows[i].Version == fixtureVersion("ppo-gradient-diagnostic", 1) {
					f.rows[i].Category = testCategory
				}
			}
		}},
		{"outside category", "no category table", func(f *formatFixture) {
			row := f.rows[0]
			f.extra = fmt.Sprintf("\n## 其他表\n\n| 版本 | 套件/用途範圍 | 用途說明 | 來源 |\n| --- | --- | --- | --- |\n| `%s` | %s | %s | [%s:%d](../%s#L%d) |\n", row.Version, row.Scope, row.Purpose, row.SourcePath, row.SourceLine, row.SourcePath, row.SourceLine)
		}},
		{"malformed extra row", "malformed version row", func(f *formatFixture) {
			f.extra = fmt.Sprintf("\n| `%s` | fixture | extra | missing final delimiter\n", f.formatVersion)
		}},
		{"empty", "contains no rows", func(f *formatFixture) { f.rows = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFormatFixture(t)
			tc.mutate(f)
			f.writeIndex(t)
			err := validateFormatIndex(f.root)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestFormatIndexGoLiteralsAndScopes(t *testing.T) {
	root := t.TempDir()
	escaped, raw, comment, tagged := fixtureVersion("escaped", 1), fixtureVersion("raw", 1), fixtureVersion("comment", 1), fixtureVersion("tagged", 1)
	encoded := strings.ReplaceAll(escaped, "/", `\u002f`)
	writeFixtureFile(t, root, "forms.go", fmt.Sprintf("package forms\nconst escaped = \"%s\"\nconst raw = `%s`\n// %s\n", encoded, raw, comment))
	writeFixtureFile(t, root, "tagged.go", fmt.Sprintf("//go:build fixture\n\npackage forms\nconst tagged = `%s`\n", tagged))
	for _, dir := range []string{".hidden", "vendor", "data", "runs", "bin"} {
		writeFixtureFile(t, root, filepath.Join(dir, "ignored.go"), fmt.Sprintf("package ignored\nconst _ = %q\n", fixtureVersion(strings.TrimPrefix(dir, "."), 1)))
	}
	for i, rel := range governanceFiles {
		writeFixtureFile(t, root, rel, fmt.Sprintf("{\"schema_version\":%q}\n", fixtureVersion(fmt.Sprintf("gov-%d", i), 1)))
	}
	inv := formatInventory{Refs: map[string][]formatRef{}}
	if err := scanGoRefs(root, "forms.go", &inv); err != nil {
		t.Fatal(err)
	}
	if len(inv.Refs[escaped]) != 1 || len(inv.Refs[raw]) != 1 || len(inv.Refs[comment]) != 0 {
		t.Fatalf("Go refs = %#v", inv.Refs)
	}
	full, err := discoverFormatInventory(root)
	if err != nil || len(full.Refs[tagged]) != 1 {
		t.Fatalf("scoped discovery refs=%#v err=%v", full.Refs, err)
	}
	for _, dir := range []string{"hidden", "vendor", "data", "runs", "bin"} {
		if len(full.Refs[fixtureVersion(dir, 1)]) != 0 {
			t.Fatalf("ignored directory %q was scanned", dir)
		}
	}
}

func TestFormatIndexNoGitAndReadErrors(t *testing.T) {
	f := newFormatFixture(t)
	if _, err := os.Stat(filepath.Join(f.root, ".git")); !os.IsNotExist(err) {
		t.Fatalf("fixture has .git: %v", err)
	}
	f.extra = "\n## Other documents\n\n| File | Purpose | Owner | Status |\n| --- | --- | --- | --- |\n| README | overview | maintainer | current |\n"
	f.writeIndex(t)
	if err := validateFormatIndex(f.root); err != nil {
		t.Fatalf("no-git fixture: %v", err)
	}

	t.Run("parse", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "broken.go", "package broken\nconst =\n")
		if _, err := discoverFormatInventory(root); err == nil || !strings.Contains(err.Error(), "broken.go") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("index read", func(t *testing.T) {
		if err := validateFormatIndex(t.TempDir()); err == nil || !strings.Contains(err.Error(), "docs/INDEX.md") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("JSON read", func(t *testing.T) {
		writeFixtureFile(t, f.root, governanceFiles[0], "{invalid")
		if _, err := discoverFormatInventory(f.root); err == nil || !strings.Contains(err.Error(), "requirements-status.json") {
			t.Fatalf("error = %v", err)
		}
	})
}

func validateFormatIndex(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "docs", "INDEX.md"))
	if err != nil {
		return fmt.Errorf("read docs/INDEX.md: %w", err)
	}
	inv, err := discoverFormatInventory(root)
	if err != nil {
		return err
	}
	doc := parseIndex(string(data))
	errs := append([]string(nil), doc.Errors...)
	if len(doc.Rows) == 0 {
		errs = append(errs, "index contains no rows")
	}
	for _, cat := range []string{formatCategory, rulesCategory, toolCategory, testCategory} {
		if !doc.Headings[cat] {
			errs = append(errs, fmt.Sprintf("missing category table %q", cat))
			continue
		}
		found := false
		for _, row := range doc.Rows {
			if row.Category == cat {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("empty category table %q", cat))
		}
	}
	rows := map[string][]formatRow{}
	for _, row := range doc.Rows {
		rows[row.Version] = append(rows[row.Version], row)
		refs := inv.Refs[row.Version]
		if len(refs) == 0 {
			errs = append(errs, fmt.Sprintf("stale index row %q at INDEX line %d", row.Version, row.IndexLine))
			continue
		}
		if !hasRef(refs, row.SourcePath, row.SourceLine) {
			errs = append(errs, fmt.Sprintf("source for %q at INDEX line %d was not discovered at %s:%d", row.Version, row.IndexLine, row.SourcePath, row.SourceLine))
		}
		if expected := expectedCategory(row.Version, refs); expected != "" && expected != row.Category {
			errs = append(errs, fmt.Sprintf("category for %q at INDEX line %d is %q, want %q", row.Version, row.IndexLine, row.Category, expected))
		}
		if row.Category == formatCategory || row.Category == rulesCategory {
			if !isFormalSource(row.SourcePath) {
				errs = append(errs, fmt.Sprintf("category source for %q at INDEX line %d needs non-test Go or governance JSON", row.Version, row.IndexLine))
			}
		}
		if row.Category == testCategory && !strings.HasSuffix(row.SourcePath, "_test.go") {
			errs = append(errs, fmt.Sprintf("test-data row %q at INDEX line %d must cite _test.go", row.Version, row.IndexLine))
		}
		if len(rows[row.Version]) == 2 {
			errs = append(errs, fmt.Sprintf("duplicate index row %q", row.Version))
		}
	}
	versions := make([]string, 0, len(inv.Refs))
	for version := range inv.Refs {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	for _, version := range versions {
		if len(rows[version]) == 0 {
			ref := inv.Refs[version][0]
			errs = append(errs, fmt.Sprintf("missing index row for %q at %s:%d", version, ref.Path, ref.Line))
		}
	}
	if len(inv.Refs) == 0 {
		errs = append(errs, "source inventory is empty")
	}
	if len(errs) == 0 {
		return nil
	}
	sort.Strings(errs)
	return fmt.Errorf("format index validation failed:\n%s", strings.Join(unique(errs), "\n"))
}

func discoverFormatInventory(root string) (formatInventory, error) {
	inv := formatInventory{Refs: map[string][]formatRef{}}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %s: %w", path, walkErr)
		}
		if entry.IsDir() {
			if path != root && skipFormatDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasSuffix(rel, ".go") {
			return scanGoRefs(root, rel, &inv)
		}
		if strings.HasPrefix(rel, "scripts/") && strings.HasSuffix(rel, ".sh") || strings.HasPrefix(rel, "evidence/") && strings.HasSuffix(rel, ".py") {
			return scanTextRefs(root, rel, &inv)
		}
		return nil
	})
	if err != nil {
		return formatInventory{}, err
	}
	for _, rel := range governanceFiles {
		if err := scanJSONRef(root, rel, &inv); err != nil {
			return formatInventory{}, err
		}
	}
	return inv, nil
}

func scanGoRefs(root, rel string, inv *formatInventory) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", rel, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return fmt.Errorf("parse %s: %w", rel, err)
	}
	var inspectErr error
	ast.Inspect(file, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if inspectErr != nil || !ok || lit.Kind != token.STRING {
			return inspectErr == nil
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			p := fset.Position(lit.Pos())
			inspectErr = fmt.Errorf("unquote %s:%d: %w", rel, p.Line, err)
			return false
		}
		line := fset.Position(lit.Pos()).Line
		for _, version := range findRE.FindAllString(value, -1) {
			inv.add(version, rel, line)
		}
		return true
	})
	return inspectErr
}

func scanTextRefs(root, rel string, inv *formatInventory) error {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return fmt.Errorf("read %s: %w", rel, err)
	}
	for line, text := range strings.Split(string(data), "\n") {
		for _, version := range findRE.FindAllString(text, -1) {
			inv.add(version, rel, line+1)
		}
	}
	return nil
}

func scanJSONRef(root, rel string, inv *formatInventory) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", rel, err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("parse %s: %w", rel, err)
	}
	version, ok := object["schema_version"].(string)
	if !ok || !versionRE.MatchString(version) {
		return fmt.Errorf("%s has invalid schema_version %q", rel, object["schema_version"])
	}
	text, match := string(data), schemaRE.FindStringSubmatchIndex(string(data))
	if match == nil {
		return fmt.Errorf("%s has no schema_version source", rel)
	}
	decoded, err := strconv.Unquote(`"` + text[match[2]:match[3]] + `"`)
	if err != nil || decoded != version {
		return fmt.Errorf("%s schema_version source disagrees", rel)
	}
	line := 1 + strings.Count(text[:match[0]], "\n")
	inv.add(version, rel, line)
	return nil
}

func parseIndex(text string) parsedIndex {
	doc := parsedIndex{Headings: map[string]bool{}}
	category := ""
	for lineNumber, raw := range strings.Split(text, "\n") {
		lineNumber++
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "#") {
			category = indexCategory(trimmed)
			if category != "" {
				doc.Headings[category] = true
			}
			continue
		}
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells, ok := splitRow(trimmed)
		if !ok {
			if findRE.MatchString(trimmed) {
				doc.Errors = append(doc.Errors, fmt.Sprintf("INDEX line %d: malformed version row", lineNumber))
			}
			continue
		}
		if separator(cells) || header(cells) {
			continue
		}
		candidate := len(cells) > 0 && findRE.MatchString(cells[0])
		if category == "" && !candidate {
			continue
		}
		if len(cells) != 4 {
			if candidate {
				doc.Errors = append(doc.Errors, fmt.Sprintf("INDEX line %d must have four columns", lineNumber))
			}
			continue
		}
		row, err := parseRow(cells, lineNumber)
		if err != nil {
			doc.Errors = append(doc.Errors, fmt.Sprintf("INDEX line %d: %v", lineNumber, err))
			continue
		}
		row.Category = category
		if category == "" {
			doc.Errors = append(doc.Errors, fmt.Sprintf("INDEX line %d has no category table", lineNumber))
		}
		doc.Rows = append(doc.Rows, row)
	}
	return doc
}

func parseRow(cells []string, indexLine int) (formatRow, error) {
	cell := strings.TrimSpace(cells[0])
	if len(cell) < 2 || cell[0] != '`' || cell[len(cell)-1] != '`' {
		return formatRow{}, fmt.Errorf("version must be backtick literal")
	}
	version := cell[1 : len(cell)-1]
	if !versionRE.MatchString(version) {
		return formatRow{}, fmt.Errorf("invalid version %q", version)
	}
	if strings.TrimSpace(cells[1]) == "" || strings.TrimSpace(cells[2]) == "" {
		return formatRow{}, fmt.Errorf("scope and purpose must be non-empty")
	}
	match := linkRE.FindStringSubmatch(strings.TrimSpace(cells[3]))
	if match == nil {
		return formatRow{}, fmt.Errorf("source must be [file:line](../file#Lline)")
	}
	label, target := match[1], match[2]
	colon := strings.LastIndex(label, ":")
	fragment := strings.LastIndex(target, "#L")
	if colon <= 0 || fragment <= 2 || !strings.HasPrefix(target, "../") {
		return formatRow{}, fmt.Errorf("source link is invalid")
	}
	path := filepath.ToSlash(filepath.Clean(label[:colon]))
	line, e1 := strconv.Atoi(label[colon+1:])
	targetPath := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(target[:fragment], "../")))
	targetLine, e2 := strconv.Atoi(target[fragment+2:])
	if e1 != nil || e2 != nil || line < 1 || line != targetLine || path != targetPath || path == "." || strings.Contains(path, "../") || filepath.IsAbs(path) {
		return formatRow{}, fmt.Errorf("source label and target disagree")
	}
	return formatRow{IndexLine: indexLine, Version: version, Scope: strings.TrimSpace(cells[1]), Purpose: strings.TrimSpace(cells[2]), SourcePath: path, SourceLine: line}, nil
}

func indexCategory(heading string) string {
	heading = strings.TrimSpace(strings.TrimLeft(heading, "#"))
	for _, cat := range []string{formatCategory, rulesCategory, toolCategory, testCategory} {
		if heading == cat {
			return cat
		}
	}
	return ""
}
func splitRow(line string) ([]string, bool) {
	parts := strings.Split(line, "|")
	if len(parts) < 3 || strings.TrimSpace(parts[0]) != "" || strings.TrimSpace(parts[len(parts)-1]) != "" {
		return nil, false
	}
	cells := make([]string, len(parts)-2)
	for i := range cells {
		cells[i] = strings.TrimSpace(parts[i+1])
	}
	return cells, true
}
func separator(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if strings.Trim(cell, "-:") != "" {
			return false
		}
	}
	return true
}
func header(cells []string) bool {
	return len(cells) > 0 && (cells[0] == "版本" || strings.EqualFold(cells[0], "schema version"))
}
func (inv *formatInventory) add(version, path string, line int) {
	inv.Refs[version] = append(inv.Refs[version], formatRef{path, line})
}
func hasRef(refs []formatRef, path string, line int) bool {
	for _, ref := range refs {
		if ref.Path == path && ref.Line == line {
			return true
		}
	}
	return false
}
func isFormalSource(path string) bool {
	return strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") || knownGovernanceFile(path)
}
func knownGovernanceFile(path string) bool {
	for _, rel := range governanceFiles {
		if path == rel {
			return true
		}
	}
	return false
}
func expectedCategory(version string, refs []formatRef) string {
	if cat := knownCategory[strings.TrimPrefix(version, "coimnet-")]; cat != "" {
		return cat
	}
	hasTool, hasFormal := false, false
	for _, ref := range refs {
		if strings.HasPrefix(ref.Path, "scripts/") || strings.HasPrefix(ref.Path, "evidence/") || strings.HasPrefix(ref.Path, "internal/cli/") || strings.HasSuffix(ref.Path, ".sh") || strings.HasSuffix(ref.Path, ".py") {
			hasTool = true
		}
		if isFormalSource(ref.Path) {
			hasFormal = true
		}
	}
	if hasFormal {
		return formatCategory
	}
	if hasTool {
		return toolCategory
	}
	return ""
}
func skipFormatDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "vendor" || name == "data" || name == "runs" || name == "bin"
}
func unique(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

type formatFixture struct {
	root                         string
	rows                         []formatRow
	formatVersion, statusVersion string
	extra                        string
}

func newFormatFixture(t *testing.T) *formatFixture {
	t.Helper()
	f := &formatFixture{root: t.TempDir(), formatVersion: fixtureVersion("fixture-format", 1), statusVersion: fixtureVersion("fixture-status", 1)}
	versions := []struct{ name, path, content string }{
		{"main", "main.go", "package fixture\nconst (\n Format = %q\n Rule = %q\n)\n"},
		{"test", "fixture_test.go", "package fixture\nconst (\n TestValue = %q\n Counterexample = %q\n ToolReport = %q\n)\n"},
	}
	rule, test, counterexample, toolReport := fixtureVersion("derivation-rules-hash", 1), fixtureVersion("fixture-test", 1), fixtureVersion("dataset-manifest", 9), fixtureVersion("ppo-gradient-diagnostic", 1)
	shell, python := fixtureVersion("fixture-shell", 1), fixtureVersion("fixture-python", 1)
	writeFixtureFile(t, f.root, versions[0].path, fmt.Sprintf(versions[0].content, f.formatVersion, rule))
	writeFixtureFile(t, f.root, versions[1].path, fmt.Sprintf(versions[1].content, test, counterexample, toolReport))
	writeFixtureFile(t, f.root, "scripts/check.sh", fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' %q\n", shell))
	writeFixtureFile(t, f.root, "evidence/report.py", fmt.Sprintf("REPORT = %q\n", python))
	jsonVersions := []string{f.statusVersion, fixtureVersion("fixture-addendum", 1), fixtureVersion("fixture-requirements", 1), fixtureVersion("fixture-sources", 1)}
	for i, rel := range governanceFiles {
		writeFixtureFile(t, f.root, rel, fmt.Sprintf("{\"schema_version\":%q}\n", jsonVersions[i]))
	}
	inv, err := discoverFormatInventory(f.root)
	if err != nil {
		t.Fatal(err)
	}
	items := []struct{ version, cat, scope, purpose string }{{f.formatVersion, formatCategory, "fixture", "formal"}, {rule, rulesCategory, "fixture", "rule"}, {shell, toolCategory, "scripts", "shell"}, {python, toolCategory, "evidence", "Python"}, {test, testCategory, "fixture", "test"}, {counterexample, testCategory, "fixture", "known counterexample"}, {toolReport, toolCategory, "fixture", "known report"}}
	for i, version := range jsonVersions {
		items = append(items, struct{ version, cat, scope, purpose string }{version, formatCategory, "governance", fmt.Sprintf("schema %d", i)})
	}
	for _, item := range items {
		refs := inv.Refs[item.version]
		if len(refs) == 0 {
			t.Fatalf("fixture version %q missing", item.version)
		}
		ref := refs[0]
		for _, candidate := range refs {
			if item.cat != testCategory && isFormalSource(candidate.Path) {
				ref = candidate
				break
			}
		}
		f.rows = append(f.rows, formatRow{Version: item.version, Scope: item.scope, Purpose: item.purpose, Category: item.cat, SourcePath: ref.Path, SourceLine: ref.Line})
	}
	f.writeIndex(t)
	return f
}

func (f *formatFixture) writeIndex(t *testing.T) {
	t.Helper()
	var b strings.Builder
	for _, cat := range []string{formatCategory, rulesCategory, toolCategory, testCategory} {
		fmt.Fprintf(&b, "## %s\n\n| 版本 | 套件/用途範圍 | 用途說明 | 來源 |\n| --- | --- | --- | --- |\n", cat)
		for _, row := range f.rows {
			if row.Category == cat {
				fmt.Fprintf(&b, "| `%s` | %s | %s | [%s:%d](../%s#L%d) |\n", row.Version, row.Scope, row.Purpose, row.SourcePath, row.SourceLine, row.SourcePath, row.SourceLine)
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString(f.extra)
	writeFixtureFile(t, f.root, "docs/INDEX.md", b.String())
}
func removeRow(rows []formatRow, version string) []formatRow {
	for i, row := range rows {
		if row.Version == version {
			return append(rows[:i], rows[i+1:]...)
		}
	}
	return rows
}
func fixtureVersion(name string, number int) string {
	return fmt.Sprintf("coimnet-%s/v%d", name, number)
}
func writeFixtureFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
