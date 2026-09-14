package params

import (
	"strings"
	"testing"
)

// validRulesJSON is the reference rules document every decode case mutates.
const validRulesJSON = `{
  "schema_version": "coimnet-derivation-rules/v1",
  "sources": {
    "body_stats": {"path": "body-stats.feather", "sha256": "1111111111111111111111111111111111111111111111111111111111111111"},
    "tbar": {"path": "tbar.feather", "sha256": "2222222222222222222222222222222222222222222222222222222222222222"},
    "syn_partners": {"path": "syn-partners.feather", "sha256": "3333333333333333333333333333333333333333333333333333333333333333"},
    "neuprint_meta": {"path": "Neuprint_Meta.csv", "sha256": "4444444444444444444444444444444444444444444444444444444444444444"}
  },
  "sign": {
    "schema_version": "coimnet-sign-rule/v1",
    "mapping": {"acetylcholine": "+1", "dopamine": "unknown", "gaba": "-1", "glutamate": "-1", "histamine": "-1", "octopamine": "unknown", "serotonin": "unknown"},
    "basis": {
      "acetylcholine": "nAChR fast excitation",
      "dopamine": "modulatory; kept unknown",
      "gaba": "GABA-A chloride conductance",
      "glutamate": "GluCl inhibition is an engineering assumption",
      "histamine": "HisCl chloride conductance",
      "octopamine": "modulatory; kept unknown",
      "serotonin": "modulatory; kept unknown"
    },
    "min_probability": 0.5,
    "min_matched_fraction": 0.25
  },
  "strength": {"gain": 1.0, "normalizer": "post_total"}
}`

func TestDecodeRulesAcceptsTheReferenceDocument(t *testing.T) {
	rules, err := DecodeRules(strings.NewReader(validRulesJSON))
	if err != nil {
		t.Fatalf("DecodeRules: %v", err)
	}
	if rules.SchemaVersion != RulesSchemaVersion || rules.Sign.SchemaVersion != SignRuleSchemaVersion {
		t.Fatalf("schema versions = %q %q", rules.SchemaVersion, rules.Sign.SchemaVersion)
	}
	if rules.Sources.Tbar.Path != "tbar.feather" || len(rules.Sources.NeuprintMeta.SHA256) != 64 {
		t.Fatalf("sources = %+v", rules.Sources)
	}
	if rules.Strength.Gain != 1 || rules.Strength.Normalizer != NormalizerPostTotal {
		t.Fatalf("strength = %+v", rules.Strength)
	}
	if len(rules.Sign.Mapping) != 7 || rules.Sign.Mapping["gaba"] != SignNegative {
		t.Fatalf("mapping = %+v", rules.Sign.Mapping)
	}
	if len(rules.Hash()) != 64 {
		t.Fatalf("rules hash = %q", rules.Hash())
	}
	// The hash covers the rule content, not the decoding order.
	again, err := DecodeRules(strings.NewReader(validRulesJSON))
	if err != nil {
		t.Fatal(err)
	}
	if again.Hash() != rules.Hash() {
		t.Fatalf("hash is not stable: %s vs %s", again.Hash(), rules.Hash())
	}
	changed := rules
	changed.Strength.Gain = 2
	if changed.Hash() == rules.Hash() {
		t.Fatal("hash ignores the gain")
	}
}

func TestDecodeRulesRejectsInvalidDocuments(t *testing.T) {
	const tbarSum = "2222222222222222222222222222222222222222222222222222222222222222"
	for name, mutate := range map[string]func(string) string{
		"wrong schema":        func(s string) string { return strings.Replace(s, "coimnet-derivation-rules/v1", "v2", 1) },
		"wrong sign schema":   func(s string) string { return strings.Replace(s, "coimnet-sign-rule/v1", "v9", 1) },
		"missing transmitter": func(s string) string { return strings.Replace(s, `, "serotonin": "unknown"}`, "}", 1) },
		"unknown transmitter": func(s string) string {
			return strings.Replace(s, `"serotonin": "unknown"`, `"nitric_oxide": "unknown"`, 1)
		},
		"bad sign value": func(s string) string { return strings.Replace(s, `"gaba": "-1"`, `"gaba": "inhibitory"`, 1) },
		"missing basis":  func(s string) string { return strings.Replace(s, `"histamine": "HisCl chloride conductance",`, "", 1) },
		"empty basis":    func(s string) string { return strings.Replace(s, "nAChR fast excitation", "", 1) },
		"glutamate basis lacks assumption": func(s string) string {
			return strings.Replace(s, "GluCl inhibition is an engineering assumption", "GluCl inhibition", 1)
		},
		"probability above one": func(s string) string {
			return strings.Replace(s, `"min_probability": 0.5`, `"min_probability": 1.5`, 1)
		},
		"negative fraction": func(s string) string {
			return strings.Replace(s, `"min_matched_fraction": 0.25`, `"min_matched_fraction": -0.1`, 1)
		},
		"zero gain":            func(s string) string { return strings.Replace(s, `"gain": 1.0`, `"gain": 0`, 1) },
		"unknown normalizer":   func(s string) string { return strings.Replace(s, `"post_total"`, `"sqrt_total"`, 1) },
		"absolute source path": func(s string) string { return strings.Replace(s, `"tbar.feather"`, `"/etc/tbar.feather"`, 1) },
		"escaping source path": func(s string) string { return strings.Replace(s, `"tbar.feather"`, `"../tbar.feather"`, 1) },
		"empty source path":    func(s string) string { return strings.Replace(s, `"tbar.feather"`, `""`, 1) },
		"short sha256":         func(s string) string { return strings.Replace(s, tbarSum, "22", 1) },
		"uppercase sha256": func(s string) string {
			return strings.Replace(s, tbarSum, strings.ToUpper(strings.ReplaceAll(tbarSum, "2", "a")), 1)
		},
		"unknown field": func(s string) string {
			return strings.Replace(s, `"strength":`, `"unknown_policy": "keep", "strength":`, 1)
		},
		"duplicate key": func(s string) string {
			return strings.Replace(s, `"strength":`, `"schema_version": "x", "strength":`, 1)
		},
		"trailing data": func(s string) string { return s + "{}" },
	} {
		document := mutate(validRulesJSON)
		if document == validRulesJSON {
			t.Fatalf("%s: mutation did not change the document", name)
		}
		if _, err := DecodeRules(strings.NewReader(document)); err == nil {
			t.Errorf("%s: DecodeRules accepted an invalid document", name)
		}
	}
}

func TestDecodeRulesRejectsOversizedInput(t *testing.T) {
	padding := strings.Repeat(" ", MaxRulesBytes)
	if _, err := DecodeRules(strings.NewReader(validRulesJSON + padding)); err == nil {
		t.Fatal("DecodeRules accepted input beyond the byte limit")
	}
	if _, err := DecodeRules(nil); err == nil {
		t.Fatal("DecodeRules accepted a nil reader")
	}
}

func TestSignRuleAcceptsAChineseAssumptionWord(t *testing.T) {
	document := strings.Replace(validRulesJSON, "GluCl inhibition is an engineering assumption", "果蠅多為 GluCl 抑制，屬工程假設", 1)
	if _, err := DecodeRules(strings.NewReader(document)); err != nil {
		t.Fatalf("DecodeRules rejected a Chinese assumption basis: %v", err)
	}
}
