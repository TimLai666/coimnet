package ocr_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr"
)

// fixtureConfig is the OCR fixture every test in this file runs: two seeds, a
// three-rune alphabet, one glyph family for training and a different one for
// testing, so the shapes never leak across the split.
func fixtureConfig() ocr.FixtureConfig {
	return ocr.FixtureConfig{
		Seeds:        []uint64{1, 2},
		Alphabet:     []rune{'a', 'b', 'c'},
		TrainSamples: 40,
		TestSamples:  15,
		MinLen:       1,
		MaxLen:       3,
		Epochs:       3,
		Hidden:       24,
		LearningRate: .05,
		TrainFamily:  1,
		TestFamily:   2,
		Background:   .1,
		Noise:        .05,
	}
}

// TestRunOCRFixtureReportsScopeAndScores is the pipeline gate: every seed runs
// to the end, the declared data scope names the fixture's actual limits, the
// report identifies itself and the whole report survives a JSON round trip.
func TestRunOCRFixtureReportsScopeAndScores(t *testing.T) {
	c := fixtureConfig()
	report, err := ocr.RunOCRFixture(context.Background(), c)
	if err != nil {
		t.Fatalf("RunOCRFixture: %v", err)
	}
	if len(report.Seeds) != len(c.Seeds) {
		t.Fatalf("report has %d seeds, want %d", len(report.Seeds), len(c.Seeds))
	}
	improved := 0
	for i, s := range report.Seeds {
		if s.Seed != c.Seeds[i] {
			t.Fatalf("seed %d of the report is %d, want %d", i, s.Seed, c.Seeds[i])
		}
		if s.Failed {
			t.Fatalf("seed %d failed: %s", s.Seed, s.Error)
		}
		if s.Before.Samples != c.TestSamples || s.After.Samples != c.TestSamples {
			t.Fatalf("seed %d evaluated %d before and %d after, want %d each", s.Seed, s.Before.Samples, s.After.Samples, c.TestSamples)
		}
		t.Logf("seed %d: held-out CER before %.4f, after %.4f; exact lines before %.4f, after %.4f",
			s.Seed, s.Before.CER.Rate, s.After.CER.Rate, s.Before.ExactLines, s.After.ExactLines)
		t.Logf("seed %d: unseen samples %d, unseen CER %.4f (defined %v); impossible targets %d",
			s.Seed, s.UnseenAfter.Samples, s.UnseenAfter.CER.Rate, s.UnseenAfter.CER.Defined, s.Impossible)
		if s.After.CER.Rate <= s.Before.CER.Rate {
			improved++
		}
	}
	if improved == 0 {
		t.Fatal("no seed lowered the held-out character error rate")
	}
	if report.SchemaVersion != ocr.OCRFixtureSchemaVersion || report.SchemaVersion == "" {
		t.Fatalf("schema version %q, want %q", report.SchemaVersion, ocr.OCRFixtureSchemaVersion)
	}
	if report.ConfigHash == "" {
		t.Fatal("report has no config hash")
	}
	if !strings.Contains(report.DataScope.Language, "3 runes") {
		t.Fatalf("data scope language %q does not name the 3-rune alphabet", report.DataScope.Language)
	}
	for name, field := range map[string]string{
		"data":            report.DataScope.Data,
		"resolution":      report.DataScope.Resolution,
		"sequence length": report.DataScope.SequenceLength,
		"periphery":       report.DataScope.Periphery,
	} {
		if field == "" {
			t.Fatalf("data scope %s is empty", name)
		}
	}
	if len(report.Assumptions) != 2 {
		t.Fatalf("report carries %d assumptions, want 2", len(report.Assumptions))
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded ocr.OCRReport
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(report, decoded) {
		t.Fatal("the report did not survive a JSON round trip")
	}
	if !strings.Contains(string(encoded), `"data_scope"`) {
		t.Fatalf("encoded report has no data_scope field: %s", encoded)
	}
}

// TestCoreDisconnectChangesOutput pins the ticket's native-core rule: with the
// core weights zeroed the same image must decode to different logits, so no
// output can bypass the core.
func TestCoreDisconnectChangesOutput(t *testing.T) {
	report, err := ocr.RunOCRFixture(context.Background(), fixtureConfig())
	if err != nil {
		t.Fatalf("RunOCRFixture: %v", err)
	}
	t.Logf("core disconnect: output changed %v, max absolute delta %g", report.CoreDisconnect.OutputChanged, report.CoreDisconnect.MaxAbsDelta)
	if !report.CoreDisconnect.OutputChanged {
		t.Fatal("zeroing the core weights left the output unchanged")
	}
	if !(report.CoreDisconnect.MaxAbsDelta > 0) {
		t.Fatalf("max absolute delta %g, want above 0", report.CoreDisconnect.MaxAbsDelta)
	}
}

// TestRunOCRFixtureValidateRejects covers the refusals the fixture owns, above
// all the split that would leak glyph shapes between training and testing.
func TestRunOCRFixtureValidateRejects(t *testing.T) {
	for name, mutate := range map[string]func(*ocr.FixtureConfig){
		"same family":        func(c *ocr.FixtureConfig) { c.TestFamily = c.TrainFamily },
		"alphabet duplicate": func(c *ocr.FixtureConfig) { c.Alphabet = []rune{'a', 'b', 'a'} },
		"epochs zero":        func(c *ocr.FixtureConfig) { c.Epochs = 0 },
		"length inverted":    func(c *ocr.FixtureConfig) { c.MinLen, c.MaxLen = 3, 2 },
	} {
		c := fixtureConfig()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Fatalf("Validate accepted %s", name)
		}
		if _, err := ocr.RunOCRFixture(context.Background(), c); err == nil {
			t.Fatalf("RunOCRFixture accepted %s", name)
		}
	}
	if err := fixtureConfig().Validate(); err != nil {
		t.Fatalf("Validate rejected the fixture configuration: %v", err)
	}
}

// TestRunOCRFixtureIsDeterministic pins that the evidence is reproducible: the
// same configuration produces the same per-seed scores.
func TestRunOCRFixtureIsDeterministic(t *testing.T) {
	ctx := context.Background()
	a, err := ocr.RunOCRFixture(ctx, fixtureConfig())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	b, err := ocr.RunOCRFixture(ctx, fixtureConfig())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !reflect.DeepEqual(a.Seeds, b.Seeds) {
		t.Fatalf("two identical runs differ:\nfirst  %+v\nsecond %+v", a.Seeds, b.Seeds)
	}
	if a.ConfigHash != b.ConfigHash {
		t.Fatalf("config hash %q then %q", a.ConfigHash, b.ConfigHash)
	}
}

// TestRunOCRFixtureHonoursCancellation pins that a cancelled context fails the
// whole run instead of reporting a half-finished fixture as evidence.
func TestRunOCRFixtureHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ocr.RunOCRFixture(ctx, fixtureConfig()); err == nil {
		t.Fatal("RunOCRFixture ignored the cancelled context")
	}
}
