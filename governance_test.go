package coimnet_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func loadJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("cannot parse %s: %v", path, err)
	}
	return out
}

// extractIDsFromRequirements returns IDs from the top-level "requirements" array
// inside the given JSON map.
func extractIDsFromRequirements(t *testing.T, raw map[string]any, file string) []string {
	t.Helper()
	reqs, ok := raw["requirements"]
	if !ok {
		t.Fatalf("%s: top-level key \"requirements\" not found", file)
	}
	arr, ok := reqs.([]any)
	if !ok {
		t.Fatalf("%s: \"requirements\" is not an array", file)
	}
	var ids []string
	for i, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("%s: requirements[%d] is not an object", file, i)
		}
		idVal, ok := obj["id"]
		if !ok {
			t.Fatalf("%s: requirements[%d] missing \"id\"", file, i)
		}
		idStr, ok := idVal.(string)
		if !ok {
			t.Fatalf("%s: requirements[%d].id is not a string", file, i)
		}
		ids = append(ids, idStr)
	}
	return ids
}

func TestGovernanceRequirementIDsMatchHandoff(t *testing.T) {
	handoffPath := "docs/handoff/requirements.json"
	addendumPath := "docs/requirements-addendum.json"
	statusPath := "docs/requirements-status.json"

	handoffRaw := loadJSONFile(t, handoffPath)
	addendumRaw := loadJSONFile(t, addendumPath)
	statusRaw := loadJSONFile(t, statusPath)

	handoffIDs := extractIDsFromRequirements(t, handoffRaw, handoffPath)
	addendumIDs := extractIDsFromRequirements(t, addendumRaw, addendumPath)

	definedIDs := make(map[string]bool)
	for _, id := range handoffIDs {
		definedIDs[id] = true
	}
	for _, id := range addendumIDs {
		definedIDs[id] = true
	}

	// Extract IDs from status file
	statusReqs, ok := statusRaw["requirements"]
	if !ok {
		t.Fatalf("%s: top-level key \"requirements\" not found", statusPath)
	}
	statusArr, ok := statusReqs.([]any)
	if !ok {
		t.Fatalf("%s: \"requirements\" is not an array", statusPath)
	}
	statusIDs := make(map[string]bool)
	for i, item := range statusArr {
		obj, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("%s: requirements[%d] is not an object", statusPath, i)
		}
		idVal, ok := obj["id"]
		if !ok {
			t.Fatalf("%s: requirements[%d] missing \"id\"", statusPath, i)
		}
		idStr, ok := idVal.(string)
		if !ok {
			t.Fatalf("%s: requirements[%d].id is not a string", statusPath, i)
		}
		statusIDs[idStr] = true
	}

	// Check defined ⊆ status
	var missingInStatus []string
	for id := range definedIDs {
		if !statusIDs[id] {
			missingInStatus = append(missingInStatus, id)
		}
	}
	sort.Strings(missingInStatus)

	// Check status ⊆ defined
	var extraInStatus []string
	for id := range statusIDs {
		if !definedIDs[id] {
			extraInStatus = append(extraInStatus, id)
		}
	}
	sort.Strings(extraInStatus)

	if len(missingInStatus) > 0 {
		t.Errorf("IDs in handoff/addendum but missing from status: %s", strings.Join(missingInStatus, ", "))
	}
	if len(extraInStatus) > 0 {
		t.Errorf("IDs in status but not in handoff/addendum: %s", strings.Join(extraInStatus, ", "))
	}
	if len(missingInStatus) > 0 || len(extraInStatus) > 0 {
		t.Logf("handoff IDs count: %d, addendum IDs count: %d, status IDs count: %d",
			len(handoffIDs), len(addendumIDs), len(statusIDs))
	}
}

func TestGovernanceStatusValuesAreDeclared(t *testing.T) {
	statusPath := "docs/requirements-status.json"
	raw := loadJSONFile(t, statusPath)

	allowed := map[string]bool{
		"specified":              true,
		"implemented_unverified": true,
		"passed":                 true,
		"failed":                 true,
		"blocked_hardware":       true,
		"blocked_data":           true,
		"blocked_permission":     true,
	}

	reqs, ok := raw["requirements"]
	if !ok {
		t.Fatalf("%s: top-level key \"requirements\" not found", statusPath)
	}
	arr, ok := reqs.([]any)
	if !ok {
		t.Fatalf("%s: \"requirements\" is not an array", statusPath)
	}

	for i, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("%s: requirements[%d] is not an object", statusPath, i)
		}
		idVal, _ := obj["id"]
		idStr, _ := idVal.(string)
		statusVal, _ := obj["status"]
		statusStr, _ := statusVal.(string)
		if !allowed[statusStr] {
			t.Errorf("id=%s has invalid status %q", idStr, statusStr)
		}
	}
}

func TestGovernancePassedRequirementsHaveEvidence(t *testing.T) {
	statusPath := "docs/requirements-status.json"
	raw := loadJSONFile(t, statusPath)

	reqs, ok := raw["requirements"]
	if !ok {
		t.Fatalf("%s: top-level key \"requirements\" not found", statusPath)
	}
	arr, ok := reqs.([]any)
	if !ok {
		t.Fatalf("%s: \"requirements\" is not an array", statusPath)
	}

	for i, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("%s: requirements[%d] is not an object", statusPath, i)
		}
		idVal, _ := obj["id"]
		idStr, _ := idVal.(string)
		statusVal, _ := obj["status"]
		statusStr, _ := statusVal.(string)
		if statusStr != "passed" {
			continue
		}

		evidenceVal, ok := obj["evidence"]
		if !ok {
			t.Errorf("id=%s is passed but has no \"evidence\" key", idStr)
			continue
		}
		evidenceArr, ok := evidenceVal.([]any)
		if !ok {
			t.Errorf("id=%s: \"evidence\" is not an array", idStr)
			continue
		}
		if len(evidenceArr) == 0 {
			t.Errorf("id=%s: passed but evidence array is empty", idStr)
			continue
		}

		for _, ev := range evidenceArr {
			pathStr, ok := ev.(string)
			if !ok {
				t.Errorf("id=%s: evidence entry is not a string: %v", idStr, ev)
				continue
			}

			// Resolve path relative to docs/
			resolved := filepath.Join("docs", pathStr)
			if !filepath.IsAbs(resolved) {
				// filepath.Join cleans the path, which handles "../"
			}

			if _, err := os.Stat(resolved); os.IsNotExist(err) {
				t.Errorf("id=%s: evidence file does not exist: %s (resolved: %s)", idStr, pathStr, resolved)
				continue
			}

			if strings.HasSuffix(pathStr, ".json") {
				data, err := os.ReadFile(resolved)
				if err != nil {
					t.Errorf("id=%s: cannot read evidence file %s: %v", idStr, resolved, err)
					continue
				}
				var evObj map[string]any
				if err := json.Unmarshal(data, &evObj); err != nil {
					t.Errorf("id=%s: evidence file %s is not valid JSON: %v", idStr, resolved, err)
					continue
				}
				requiredKeys := []string{"requirement", "reproduction_command", "environment", "observed_result", "test_log"}
				var missing []string
				for _, key := range requiredKeys {
					if _, exists := evObj[key]; !exists {
						missing = append(missing, key)
					}
				}
				if len(missing) > 0 {
					t.Errorf("id=%s: evidence %s missing keys: %s", idStr, pathStr, strings.Join(missing, ", "))
				}
			}
		}
	}
}

func TestGovernanceTicketsHaveDatedRootDecisions(t *testing.T) {
	entries, err := os.ReadDir("docs/tickets")
	if err != nil {
		t.Fatalf("cannot read docs/tickets/: %v", err)
	}

	dateRe := regexp.MustCompile(`20\d\d-\d\d-\d\d`)

	var skippedFiles []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		// Must start with two digits
		if len(name) < 2 || name[0] < '0' || name[0] > '9' || name[1] < '0' || name[1] > '9' {
			continue
		}

		path := filepath.Join("docs/tickets", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cannot read %s: %v", path, err)
		}
		content := string(data)
		lines := strings.Split(content, "\n")

		for lineNum, line := range lines {
			if strings.Contains(line, "## Root 決策") {
				if !dateRe.MatchString(line) {
					t.Errorf("%s: line %d contains '## Root 決策' but no date matching 20\\d\\d-\\d\\d-\\d\\d", name, lineNum+1)
				}
			}
		}

		if !strings.Contains(content, "## Root 決策") {
			skippedFiles = append(skippedFiles, name)
		}
	}

	for _, name := range skippedFiles {
		t.Logf("no Root 決策 section: %s", name)
	}
}
